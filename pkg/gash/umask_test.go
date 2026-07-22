package gash

import (
	"context"
	"testing"
)

func TestUmaskControlsCreatedFileAndDirectoryModes(t *testing.T) {
	shell, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}

	result := shell.Exec(context.Background(), `
umask
umask 0077
printf data > redirected
mkdir private
touch touched
stat -c '%a' redirected private touched
umask -S
umask -p
`, ExecOptions{})
	if result.ExitCode != 0 || result.Stderr != "" {
		t.Fatalf("result = %#v", result)
	}
	want := "0022\n600\n700\n600\nu=rwx,g=,o=\numask 0077\n"
	if result.Stdout != want {
		t.Fatalf("stdout = %q, want %q", result.Stdout, want)
	}
}

func TestUmaskDoesNotChangeExistingFileMode(t *testing.T) {
	shell, err := New(Options{Cwd: "/", Files: map[string]string{"existing": "old"}})
	if err != nil {
		t.Fatal(err)
	}

	result := shell.Exec(context.Background(), `umask 077; printf new > existing; stat -c %a existing`, ExecOptions{})
	if result.ExitCode != 0 || result.Stdout != "644\n" || result.Stderr != "" {
		t.Fatalf("result = %#v", result)
	}
}

func TestUmaskIsScopedInSubshellsAndChildShells(t *testing.T) {
	shell, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}

	result := shell.Exec(context.Background(), `umask 022; (umask 077); bash -c 'umask 007'; umask`, ExecOptions{})
	if result.ExitCode != 0 || result.Stdout != "0022\n" || result.Stderr != "" {
		t.Fatalf("result = %#v", result)
	}
}

func TestUmaskMutationInPipelineIsRejectedWithoutRacing(t *testing.T) {
	shell, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}

	result := shell.Exec(context.Background(), `printf data | { umask 077; cat; }`, ExecOptions{})
	if result.ExitCode != 2 || result.Stdout != "" || result.Stderr != "bash: changing umask inside a pipeline is not supported by the isolated interpreter\n" {
		t.Fatalf("result = %#v", result)
	}
}

func TestUmaskSymbolicMode(t *testing.T) {
	shell, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}

	result := shell.Exec(context.Background(), `umask u=rwx,g=rx,o=; umask`, ExecOptions{})
	if result.ExitCode != 0 || result.Stdout != "0027\n" || result.Stderr != "" {
		t.Fatalf("result = %#v", result)
	}
}
