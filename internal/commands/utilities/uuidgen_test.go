package utilities

import (
	"bytes"
	"context"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/rumpl/gash/internal/command"
)

func TestUUIDGenRandomVersion4(t *testing.T) {
	for _, args := range [][]string{nil, {"-r"}, {"--random"}} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			exitCode := commandUUIDGen(context.Background(), args, &command.Context{Stdout: &stdout, Stderr: &stderr})
			if exitCode != 0 || stderr.Len() != 0 {
				t.Fatalf("exit=%d stderr=%q", exitCode, stderr.String())
			}
			value := strings.TrimSuffix(stdout.String(), "\n")
			if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
				t.Fatalf("uuidgen output is not canonical: %q", stdout.String())
			}
			decoded, err := hex.DecodeString(strings.ReplaceAll(value, "-", ""))
			if err != nil {
				t.Fatalf("decode UUID %q: %v", value, err)
			}
			if version := decoded[6] >> 4; version != 4 {
				t.Fatalf("UUID version = %d, want 4 (%q)", version, value)
			}
			if variant := decoded[8] >> 6; variant != 2 {
				t.Fatalf("UUID variant bits = %02b, want 10 (%q)", variant, value)
			}
			if value != strings.ToLower(value) {
				t.Fatalf("UUID is not lowercase: %q", value)
			}
		})
	}
}

func TestUUIDGenRejectsUnsupportedOptionsAndOperands(t *testing.T) {
	for _, args := range [][]string{{"-t"}, {"--time"}, {"--bogus"}, {"operand"}, {"--", "operand"}} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			exitCode := commandUUIDGen(context.Background(), args, &command.Context{Stdout: &stdout, Stderr: &stderr})
			if exitCode == 0 || stdout.Len() != 0 || stderr.Len() == 0 {
				t.Fatalf("exit=%d stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
			}
		})
	}
}
