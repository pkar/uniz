package uniz_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pkar/uniz"
)

func TestCount(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  uniz.Counts
	}{
		{"", uniz.Counts{}},
		{"one two\nthree", uniz.Counts{Lines: 1, Words: 3, Bytes: 13, Chars: 13}},
		{"é\u2003猫\n", uniz.Counts{Lines: 1, Words: 2, Bytes: 9, Chars: 4}},
		{"a\xffb", uniz.Counts{Words: 1, Bytes: 3, Chars: 3}},
		{" \t\r\n", uniz.Counts{Lines: 1, Bytes: 4, Chars: 4}},
		{strings.Repeat("x", 100000), uniz.Counts{Words: 1, Bytes: 100000, Chars: 100000}},
	} {
		got, err := uniz.Count(context.Background(), strings.NewReader(tc.input))
		if err != nil || got != tc.want {
			t.Fatalf("len=%d: got %+v, %v; want %+v", len(tc.input), got, err, tc.want)
		}
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
func TestStreamErrors(t *testing.T) {
	ctx := context.Background()
	counts, err := uniz.Count(ctx, io.MultiReader(strings.NewReader("hi\n"), failingReader{}))
	if !errors.Is(err, io.ErrUnexpectedEOF) || counts != (uniz.Counts{Lines: 1, Words: 1, Bytes: 3, Chars: 3}) {
		t.Fatalf("%+v %v", counts, err)
	}
	var out bytes.Buffer
	n, err := uniz.Cat(ctx, &out, io.MultiReader(strings.NewReader("hi"), failingReader{}))
	if n != 2 || out.String() != "hi" || !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("%d %q %v", n, out.String(), err)
	}
	if _, err := uniz.Cat(ctx, failWriter{}, strings.NewReader("hi")); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal(err)
	}
}

func TestCatBinaryAndLarge(t *testing.T) {
	input := bytes.Repeat([]byte{0, 255, '\n', 'a'}, 50000)
	var out bytes.Buffer
	n, err := uniz.Cat(context.Background(), &out, bytes.NewReader(input))
	if err != nil || n != int64(len(input)) || !bytes.Equal(out.Bytes(), input) {
		t.Fatalf("n=%d err=%v", n, err)
	}
}

func TestOperationCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := uniz.Cat(ctx, io.Discard, strings.NewReader("x")); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := uniz.Count(ctx, strings.NewReader("x")); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := uniz.WorkingDirectory(ctx, uniz.DirectoryOptions{}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	w := &cancelWriter{cancel: cancel}
	_, err := uniz.Cat(ctx, w, strings.NewReader(strings.Repeat("x", 100000)))
	if !errors.Is(err, context.Canceled) || w.writes != 1 {
		t.Fatalf("%v writes=%d", err, w.writes)
	}
}

func TestWorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	want, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	got, err := uniz.WorkingDirectory(context.Background(), uniz.DirectoryOptions{})
	if err != nil || got != want {
		t.Fatalf("%q %v want %q", got, err, want)
	}
	physical, err := filepath.EvalSymlinks(want)
	if err != nil {
		t.Fatal(err)
	}
	got, err = uniz.WorkingDirectory(context.Background(), uniz.DirectoryOptions{Physical: true})
	if err != nil || got != physical {
		t.Fatalf("%q %v want %q", got, err, physical)
	}
	link := filepath.Join(t.TempDir(), "logical")
	if err := os.Symlink(dir, link); err != nil {
		t.Skip(err)
	}
	t.Setenv("PWD", link)
	got, err = uniz.WorkingDirectory(context.Background(), uniz.DirectoryOptions{})
	if err != nil || got != link {
		t.Fatalf("logical: %q %v", got, err)
	}
	got, err = uniz.WorkingDirectory(context.Background(), uniz.DirectoryOptions{Physical: true})
	if err != nil || got != physical {
		t.Fatalf("physical: %q %v", got, err)
	}
}

func TestUtilityCommands(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	for name, data := range map[string]string{"a": "one two\n", "b": "é\n", "-file": "dash"} {
		if err := os.WriteFile(name, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name        string
		args        []string
		input, want string
		code        int
	}{
		{"cat stdin", []string{"cat"}, "input", "input", 0},
		{"cat sequence", []string{"cat", "a", "-", "b"}, "stdin", "one two\nstdiné\n", 0},
		{"cat repeated stdin", []string{"cat", "-", "-"}, "once", "once", 0},
		{"cat literal", []string{"cat", "--", "-file"}, "", "dash", 0},
		{"cat missing", []string{"cat", "missing", "a"}, "", "one two\n", 1},
		{"cat directory", []string{"cat", ".", "a"}, "", "one two\n", 1},
		{"cat bad flag", []string{"cat", "-n"}, "", "", 2},
		{"wc default", []string{"wc"}, "one two\n", "1 2 8\n", 0},
		{"wc all", []string{"wc", "-lwcm", "b"}, "", "1 1 3 2 b\n", 0},
		{"wc order", []string{"wc", "-mc", "b"}, "", "3 2 b\n", 0},
		{"wc totals", []string{"wc", "a", "b"}, "", "1 2 8 a\n1 1 3 b\n2 3 11 total\n", 0},
		{"wc stdin label", []string{"wc", "-c", "-"}, "hello", "5 -\n", 0},
		{"wc repeated stdin", []string{"wc", "-c", "-", "-"}, "hello", "5 -\n0 -\n5 total\n", 0},
		{"wc missing", []string{"wc", "-c", "missing", "a"}, "", "8 a\n8 total\n", 1},
		{"wc bad flag", []string{"wc", "-z"}, "", "", 2},
		{"pwd", []string{"pwd"}, "", cwd + "\n", 0},
		{"pwd argument", []string{"pwd", "a"}, "", "", 2},
		{"pwd bad flag", []string{"pwd", "-z"}, "", "", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out, stderr bytes.Buffer
			runner := uniz.New(uniz.Config{Stdin: strings.NewReader(tc.input), Stdout: &out, Stderr: &stderr})
			err := runner.Run(context.Background(), tc.args)
			if uniz.ExitCode(err) != tc.code || out.String() != tc.want {
				t.Fatalf("out=%q err=%v code=%d; want=%q code=%d", out.String(), err, uniz.ExitCode(err), tc.want, tc.code)
			}
			if stderr.Len() != 0 {
				t.Fatal("library printed errors")
			}
		})
	}
}

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) { return len(p) - 1, nil }

func TestUtilityReadAndOutputFailures(t *testing.T) {
	for _, command := range []string{"cat", "wc"} {
		t.Run(command, func(t *testing.T) {
			var out bytes.Buffer
			input := io.MultiReader(strings.NewReader("hi\n"), failingReader{})
			r := uniz.New(uniz.Config{Stdin: input, Stdout: &out})
			err := r.Run(context.Background(), []string{command})
			if !errors.Is(err, io.ErrUnexpectedEOF) {
				t.Fatal(err)
			}
			want := "hi\n"
			if command == "wc" {
				want = "1 1 3\n"
			}
			if out.String() != want {
				t.Fatalf("%q", out.String())
			}
			// An output failure must stop before opening the later missing operand.
			r = uniz.New(uniz.Config{Stdin: strings.NewReader("hi"), Stdout: failWriter{}})
			err = r.Run(context.Background(), []string{command, "-", "does-not-exist"})
			if !errors.Is(err, io.ErrClosedPipe) || errors.Is(err, os.ErrNotExist) {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if err := r.Run(ctx, []string{command}); !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
		})
	}
	r := uniz.New(uniz.Config{Stdin: strings.NewReader("hi"), Stdout: shortWriter{}})
	if err := r.Run(context.Background(), []string{"cat"}); !errors.Is(err, io.ErrShortWrite) {
		t.Fatal(err)
	}
}

func TestUtilityHelpAndDefaults(t *testing.T) {
	for _, command := range []string{"cat", "wc", "pwd"} {
		var out bytes.Buffer
		r := uniz.New(uniz.Config{Stdout: &out})
		if err := r.Run(context.Background(), []string{command, "--help"}); err != nil || !strings.Contains(out.String(), "Usage: uniz "+command) {
			t.Fatalf("%q %v", out.String(), err)
		}
		if err := new(uniz.Runner).Run(context.Background(), []string{command}); err != nil {
			t.Fatal(err)
		}
		r = uniz.New(uniz.Config{Stdin: strings.NewReader("x"), Stdout: failWriter{}})
		if err := r.Run(context.Background(), []string{command}); !errors.Is(err, io.ErrClosedPipe) {
			t.Fatal(err)
		}
	}
}
