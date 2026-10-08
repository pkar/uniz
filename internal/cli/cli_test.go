package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	for _, name := range []string{"a", ".hidden", "-file"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name     string
		args     []string
		code     int
		out, err string
	}{
		{"help", nil, 0, "Usage: uniz", ""},
		{"version", []string{"--version"}, 0, "uniz test\n", ""},
		{"command", []string{"wat"}, 2, "", "unknown command"},
		{"ls help", []string{"ls", "--help"}, 0, "Usage: uniz ls", ""},
		{"default", []string{"ls"}, 0, "-file\na\n", ""},
		{"combined", []string{"ls", "-Ar1"}, 0, "a\n.hidden\n-file\n", ""},
		{"after path", []string{"ls", ".", "-A"}, 0, "-file\n.hidden\na\n", ""},
		{"literal", []string{"ls", "--", "-file"}, 0, "-file\n", ""},
		{"bad flag", []string{"ls", "-z"}, 2, "", "unknown option"},
		{"bad long", []string{"ls", "--all"}, 2, "", "unknown option"},
		{"missing", []string{"ls", "missing", "a"}, 1, "a\n", "missing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out, err bytes.Buffer
			runErr := Run(context.Background(), tc.args, strings.NewReader(""), &out, io.Discard, "test")
			code := 0
			if runErr != nil {
				code = 1
				var usage *UsageError
				if errors.As(runErr, &usage) {
					code = 2
				}
				fmt.Fprint(&err, runErr)
			}
			if code != tc.code || !strings.Contains(out.String(), tc.out) || !strings.Contains(err.String(), tc.err) {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, out.String(), err.String())
			}
			if tc.err == "" && err.Len() != 0 {
				t.Fatal(err.String())
			}
		})
	}
}
