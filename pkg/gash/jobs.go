package gash

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

const (
	internalStartJobCommand = "__gash_start_job"
	internalWaitJobCommand  = "__gash_wait_job"
	internalJobsCommand     = "__gash_jobs"
	lastBackgroundVariable  = "GASH_LAST_BACKGROUND_PID"
)

type virtualJob struct {
	id           int
	pid          string
	command      string
	done         chan struct{}
	status       int
	forcedStatus int
	cancel       context.CancelFunc
}

type jobState struct {
	mu      sync.Mutex
	nextID  int
	nextPID int
	last    string
	jobs    map[string]*virtualJob
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

func (s *executionScope) initializeJobs(ctx context.Context) context.Context {
	jobCtx, cancel := context.WithCancel(ctx)
	s.commands = &atomic.Int64{}
	s.input = &atomic.Int64{}
	s.jobs = &jobState{nextPID: 2001, jobs: map[string]*virtualJob{}, cancel: cancel}
	return jobCtx
}

func (s *executionScope) forkForJob() *executionScope {
	child := &executionScope{limits: s.limits, commands: s.commands, input: s.input, jobs: s.jobs}
	s.trapsMu.RLock()
	if len(s.traps) > 0 {
		child.traps = make(map[string]string, len(s.traps))
		for signal, callback := range s.traps {
			child.traps[signal] = callback
		}
	}
	s.trapsMu.RUnlock()
	return child
}

func (s *executionScope) allocateJob(command string) *virtualJob {
	s.jobs.mu.Lock()
	defer s.jobs.mu.Unlock()
	s.jobs.nextID++
	job := &virtualJob{id: s.jobs.nextID, pid: strconv.Itoa(s.jobs.nextPID), command: command, done: make(chan struct{})}
	s.jobs.nextPID++
	s.jobs.jobs[job.pid] = job
	s.jobs.last = job.pid
	return job
}

func (s *jobState) lastPID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.last
}

func (s *executionScope) startJob(job *virtualJob, cancel context.CancelFunc, run func() int) {
	s.jobs.mu.Lock()
	job.cancel = cancel
	s.jobs.mu.Unlock()
	s.jobs.wg.Add(1)
	go func() {
		defer s.jobs.wg.Done()
		status := run()
		s.jobs.mu.Lock()
		if job.forcedStatus != 0 {
			status = job.forcedStatus
		}
		job.status = status
		close(job.done)
		s.jobs.mu.Unlock()
	}()
}

func (s *executionScope) stopJobs() {
	if s.jobs == nil {
		return
	}
	s.jobs.cancel()
	s.jobs.wg.Wait()
}

func rewriteBackgroundJobs(program syntax.Node) error {
	var rewriteErr error
	syntax.Walk(program, func(node syntax.Node) bool {
		if rewriteErr != nil {
			return false
		}
		statement, ok := node.(*syntax.Stmt)
		if !ok || !statement.Background {
			return true
		}
		statement.Background = false
		var source strings.Builder
		if err := syntax.NewPrinter().Print(&source, statement); err != nil {
			rewriteErr = err
			return false
		}
		command := strings.TrimSpace(source.String())
		statement.Comments = nil
		statement.Redirs = nil
		statement.Negated = false
		statement.Cmd = &syntax.CallExpr{Args: []*syntax.Word{
			literalWordValue(internalStartJobCommand),
			quotedWordValue(command),
		}}
		return false
	})
	return rewriteErr
}

func rewriteJobBuiltins(program syntax.Node) {
	syntax.Walk(program, func(node syntax.Node) bool {
		call, ok := node.(*syntax.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}
		name := call.Args[0].Lit()
		if (name == "command" || name == "builtin") && len(call.Args) > 1 {
			switch call.Args[1].Lit() {
			case "wait":
				call.Args = append([]*syntax.Word{literalWordValue(internalWaitJobCommand)}, call.Args[2:]...)
			case "jobs":
				call.Args = append([]*syntax.Word{literalWordValue(internalJobsCommand)}, call.Args[2:]...)
			}
			return true
		}
		switch name {
		case "wait":
			call.Args[0] = literalWordValue(internalWaitJobCommand)
		case "jobs":
			call.Args[0] = literalWordValue(internalJobsCommand)
		}
		return true
	})
}

func literalWordValue(value string) *syntax.Word {
	return &syntax.Word{Parts: []syntax.WordPart{&syntax.Lit{Value: value}}}
}

func quotedWordValue(value string) *syntax.Word {
	return &syntax.Word{Parts: []syntax.WordPart{&syntax.SglQuoted{Value: value}}}
}

func (b *Bash) prepareJob(ctx context.Context, argv []string, scope *executionScope) ([]string, bool) {
	if len(argv) != 2 || argv[0] != internalStartJobCommand {
		return argv, false
	}
	job := scope.allocateJob(compactJobCommand(argv[1]))
	return []string{internalStartJobCommand, job.pid, argv[1]}, true
}

func compactJobCommand(command string) string {
	return strings.Join(strings.Fields(command), " ")
}

func (b *Bash) runStartJob(ctx context.Context, args []string, depth int, scope *executionScope) int {
	if len(args) != 2 {
		return 2
	}
	pid, script := args[0], args[1]
	scope.jobs.mu.Lock()
	job := scope.jobs.jobs[pid]
	scope.jobs.mu.Unlock()
	if job == nil {
		return 1
	}
	handler := interp.HandlerCtx(ctx)
	env, declarations := snapshotJobEnvironment(handler.Env)
	cwd := handler.Dir
	jobCtx, cancel := context.WithCancel(ctx)
	jobScope := scope.forkForJob()
	scope.startJob(job, cancel, func() int {
		code, _ := b.execute(jobCtx, declarations+script, "", cwd, env, nil, "background job", handler.Stdout, handler.Stderr, depth+1, jobScope, true)
		return code
	})
	return 0
}

func snapshotJobEnvironment(environment expand.Environ) (map[string]string, string) {
	exported := map[string]string{}
	var declarations strings.Builder
	environment.Each(func(name string, variable expand.Variable) bool {
		if !variable.IsSet() || isHiddenInternalEnv(name) {
			return true
		}
		if variable.Exported && variable.Kind == expand.String {
			exported[name] = variable.String()
		} else if syntax.ValidName(name) {
			writeJobVariable(&declarations, name, variable)
		}
		return true
	})
	enforcePublicInternalEnv(exported)
	return exported, declarations.String()
}

func writeJobVariable(output *strings.Builder, name string, variable expand.Variable) {
	switch variable.Kind {
	case expand.Indexed:
		fmt.Fprintf(output, "%s=(", name)
		for _, value := range variable.List {
			fmt.Fprintf(output, "%s ", shellQuote(value))
		}
		fmt.Fprintln(output, ")")
	case expand.Associative:
		keys := make([]string, 0, len(variable.Map))
		for key := range variable.Map {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		fmt.Fprintf(output, "declare -A %s=(", name)
		for _, key := range keys {
			fmt.Fprintf(output, "[%s]=%s ", shellQuote(key), shellQuote(variable.Map[key]))
		}
		fmt.Fprintln(output, ")")
	default:
		fmt.Fprintf(output, "%s=%s\n", name, shellQuote(variable.String()))
	}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func (s *executionScope) signalJob(pid string, status int) bool {
	s.jobs.mu.Lock()
	job := s.jobs.jobs[pid]
	if job == nil {
		s.jobs.mu.Unlock()
		return false
	}
	select {
	case <-job.done:
		s.jobs.mu.Unlock()
		return false
	default:
	}
	job.forcedStatus = status
	cancel := job.cancel
	s.jobs.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return true
}

func (b *Bash) runWaitJob(args []string, output io.Writer, scope *executionScope) int {
	jobs, missing := scope.selectedJobs(args)
	if missing != "" {
		fmt.Fprintf(output, "wait: %s: no such job\n", missing)
		return 127
	}
	status := 0
	for _, job := range jobs {
		<-job.done
		status = job.status
	}
	if len(args) == 0 {
		return 0
	}
	return status
}

func (s *executionScope) selectedJobs(args []string) ([]*virtualJob, string) {
	s.jobs.mu.Lock()
	defer s.jobs.mu.Unlock()
	if len(args) == 0 {
		jobs := make([]*virtualJob, 0, len(s.jobs.jobs))
		for _, job := range s.jobs.jobs {
			jobs = append(jobs, job)
		}
		sort.Slice(jobs, func(i, j int) bool {
			return jobs[i].id < jobs[j].id
		})
		return jobs, ""
	}
	jobs := make([]*virtualJob, 0, len(args))
	for _, requested := range args {
		requested = strings.TrimPrefix(requested, "%")
		var found *virtualJob
		for _, job := range s.jobs.jobs {
			if job.pid == requested || strconv.Itoa(job.id) == requested {
				found = job
				break
			}
		}
		if found == nil {
			return nil, requested
		}
		jobs = append(jobs, found)
	}
	return jobs, ""
}

func (b *Bash) runJobs(args []string, output, errorOutput io.Writer, scope *executionScope) int {
	pidsOnly := false
	for _, argument := range args {
		if argument == "-p" {
			pidsOnly = true
			continue
		}
		fmt.Fprintf(errorOutput, "jobs: invalid option: %s\n", argument)
		return 2
	}
	scope.jobs.mu.Lock()
	defer scope.jobs.mu.Unlock()
	jobs := make([]*virtualJob, 0, len(scope.jobs.jobs))
	for _, job := range scope.jobs.jobs {
		jobs = append(jobs, job)
	}
	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].id < jobs[j].id
	})
	for _, job := range jobs {
		if pidsOnly {
			fmt.Fprintln(output, job.pid)
			continue
		}
		state := "Running"
		select {
		case <-job.done:
			state = "Done"
		default:
		}
		fmt.Fprintf(output, "[%d]  %s\t%s &\n", job.id, state, job.command)
	}
	return 0
}
