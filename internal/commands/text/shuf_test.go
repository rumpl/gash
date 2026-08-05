package text

import (
	"context"
	"strings"
	"testing"
)

type sequenceShufRandom struct {
	values []int
	index  int
}

func (r *sequenceShufRandom) Intn(limit int) (int, error) {
	value := r.values[r.index%len(r.values)] % limit
	r.index++
	return value, nil
}

func TestShufHeadCountFromStdin(t *testing.T) {
	random := &sequenceShufRandom{values: []int{2, 0}}
	code, stdout, stderr, _ := runTextCommandBytes(t, func(ctx context.Context, args []string, c *CommandContext) int {
		return commandShufWithRandom(ctx, args, c, random)
	}, []string{"-n", "2"}, []byte("a\nb\nc\n"), nil)
	if code != 0 || string(stdout) != "c\nb\n" || stderr != "" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestShufRangeAndFile(t *testing.T) {
	t.Run("range", func(t *testing.T) {
		random := &sequenceShufRandom{values: []int{1, 1, 0}}
		code, stdout, stderr, _ := runTextCommandBytes(t, func(ctx context.Context, args []string, c *CommandContext) int {
			return commandShufWithRandom(ctx, args, c, random)
		}, []string{"--input-range=1-3"}, nil, map[string][]byte{"ignored": []byte("host-like data\n")})
		if code != 0 || string(stdout) != "2\n3\n1\n" || stderr != "" {
			t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout, stderr)
		}
	})

	t.Run("file", func(t *testing.T) {
		random := &sequenceShufRandom{values: []int{0, 0}}
		code, stdout, stderr, _ := runTextCommandBytes(t, func(ctx context.Context, args []string, c *CommandContext) int {
			return commandShufWithRandom(ctx, args, c, random)
		}, []string{"-n", "2", "items"}, nil, map[string][]byte{"items": []byte("red\nblue\n")})
		if code != 0 || string(stdout) != "red\nblue\n" || stderr != "" {
			t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout, stderr)
		}
	})
}

func TestShufRepeatAllowsRepeatedRangeValues(t *testing.T) {
	random := &sequenceShufRandom{values: []int{1}}
	code, stdout, stderr, _ := runTextCommandBytes(t, func(ctx context.Context, args []string, c *CommandContext) int {
		return commandShufWithRandom(ctx, args, c, random)
	}, []string{"-r", "-n", "5", "-i", "1-2"}, nil, nil)
	if code != 0 || string(stdout) != "2\n2\n2\n2\n2\n" || stderr != "" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestShufRejectsInvalidArguments(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "negative count", args: []string{"-n", "-1"}, want: "invalid line count"},
		{name: "bad count", args: []string{"--head-count=no"}, want: "invalid line count"},
		{name: "descending range", args: []string{"-i", "3-1"}, want: "invalid input range"},
		{name: "malformed range", args: []string{"--input-range=1"}, want: "invalid input range"},
		{name: "unsupported option", args: []string{"--random-source=x"}, want: "unrecognized option"},
		{name: "unbounded repeat", args: []string{"-r", "-i", "1-2"}, want: "requires --head-count"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			code, stdout, stderr, _ := runTextCommandBytes(t, commandShuf, test.args, nil, nil)
			if code == 0 || len(stdout) != 0 || !strings.Contains(stderr, test.want) {
				t.Fatalf("exit=%d stdout=%q stderr=%q, want diagnostic containing %q", code, stdout, stderr, test.want)
			}
		})
	}
}
