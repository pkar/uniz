package uniz_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/pkar/uniz"
)

// run executes a command with the given stdin and returns stdout and the error.
func run(t *testing.T, input string, args ...string) (string, error) {
	t.Helper()
	var out, stderr bytes.Buffer
	err := uniz.New(uniz.Config{Stdin: strings.NewReader(input), Stdout: &out, Stderr: &stderr}).Run(context.Background(), args)
	if stderr.Len() != 0 {
		t.Fatalf("library wrote to stderr: %q", stderr.String())
	}
	return out.String(), err
}

func write(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

type commandCase struct {
	name  string
	args  []string
	input string
	want  string
	code  int
}

func runCases(t *testing.T, cases []commandCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := run(t, tc.input, tc.args...)
			if out != tc.want || uniz.ExitCode(err) != tc.code {
				t.Fatalf("out=%q err=%v code=%d; want out=%q code=%d", out, err, uniz.ExitCode(err), tc.want, tc.code)
			}
		})
	}
}

func TestTextCommands(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	write(t, "f1", "1\n2\n")
	write(t, "f2", "3\n")
	nums := "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11\n12\n"
	runCases(t, []commandCase{
		{"echo", []string{"echo", "a", "b"}, "", "a b\n", 0},
		{"echo -n", []string{"echo", "-n", "x"}, "", "x", 0},
		{"echo escapes", []string{"echo", "-e", `a\tb\x41\0101\\`}, "", "a\tbAA\\\n", 0},
		{"echo \\c", []string{"echo", "-e", `a\cb`, "z"}, "", "a", 0},
		{"echo -E wins", []string{"echo", "-eE", `a\tb`}, "", "a\\tb\n", 0},
		{"echo literal", []string{"echo", "--", "-x", "-nq"}, "", "-- -x -nq\n", 0},
		{"echo --help in text", []string{"echo", "a", "--help"}, "", "a --help\n", 0},
		{"true", []string{"true", "--anything"}, "", "", 0},
		{"false", []string{"false"}, "", "", 1},
		{"head default", []string{"head"}, nums, "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n", 0},
		{"head -n", []string{"head", "-n", "2"}, nums, "1\n2\n", 0},
		{"head -N", []string{"head", "-3"}, nums, "1\n2\n3\n", 0},
		{"head -c", []string{"head", "-c3"}, nums, "1\n2", 0},
		{"head 0", []string{"head", "-n0"}, nums, "", 0},
		{"head no newline", []string{"head", "-n5"}, "a\nb", "a\nb", 0},
		{"head files", []string{"head", "-n1", "f1", "f2"}, "", "==> f1 <==\n1\n\n==> f2 <==\n3\n", 0},
		{"head stdin label", []string{"head", "-n1", "-", "f2"}, "x\n", "==> standard input <==\nx\n\n==> f2 <==\n3\n", 0},
		{"head missing", []string{"head", "missing", "f2"}, "", "==> f2 <==\n3\n", 1},
		{"head negative", []string{"head", "-n", "-1"}, nums, "", 2},
		{"head both", []string{"head", "-n1", "-c1"}, nums, "", 2},
		{"head bad", []string{"head", "-n", "x"}, nums, "", 2},
		{"tail default", []string{"tail"}, nums, "3\n4\n5\n6\n7\n8\n9\n10\n11\n12\n", 0},
		{"tail -n", []string{"tail", "-n", "2"}, nums, "11\n12\n", 0},
		{"tail -n -2", []string{"tail", "-n", "-2"}, nums, "11\n12\n", 0},
		{"tail +N", []string{"tail", "-n", "+11"}, nums, "11\n12\n", 0},
		{"tail +0", []string{"tail", "-n", "+0"}, "a\nb\n", "a\nb\n", 0},
		{"tail -c", []string{"tail", "-c", "3"}, nums, "12\n", 0},
		{"tail -c +N", []string{"tail", "-c", "+3"}, "abcdef", "cdef", 0},
		{"tail no newline", []string{"tail", "-n1"}, "a\nb", "b", 0},
		{"tail more than input", []string{"tail", "-n50"}, "a\nb\n", "a\nb\n", 0},
		{"tail files", []string{"tail", "-n1", "f1", "f2"}, "", "==> f1 <==\n2\n\n==> f2 <==\n3\n", 0},
		{"sort", []string{"sort"}, "b\na\nB\n10\n9\n", "10\n9\nB\na\nb\n", 0},
		{"sort -n", []string{"sort", "-n"}, "b\n10\n-1.5\n9\na\n", "-1.5\na\nb\n9\n10\n", 0},
		{"sort -r", []string{"sort", "-r"}, "b\na\nc\n", "c\nb\na\n", 0},
		{"sort -u", []string{"sort", "-u"}, "b\na\nb\n", "a\nb\n", 0},
		{"sort -f", []string{"sort", "-f"}, "b\nB\na\n", "a\nB\nb\n", 0},
		{"sort -fu keeps first", []string{"sort", "-fu"}, "b\na\nB\n", "a\nb\n", 0},
		{"sort -nu", []string{"sort", "-nu"}, "2\n02\n1\n", "1\n2\n", 0},
		{"sort files", []string{"sort", "f2", "f1"}, "", "1\n2\n3\n", 0},
		{"sort missing writes nothing", []string{"sort", "f1", "missing"}, "", "", 1},
		{"sort no final newline", []string{"sort"}, "b\na", "a\nb\n", 0},
		{"uniq", []string{"uniq"}, "a\na\nb\na\n", "a\nb\na\n", 0},
		{"uniq -c", []string{"uniq", "-c"}, "a\na\nb\n", "      2 a\n      1 b\n", 0},
		{"uniq -d", []string{"uniq", "-d"}, "a\nA\nb\nb\nc\n", "b\n", 0},
		{"uniq -u", []string{"uniq", "-u"}, "a\nA\nb\nb\nc\n", "a\nA\nc\n", 0},
		{"uniq -ic", []string{"uniq", "-ic"}, "a\nA\nb\n", "      2 a\n      1 b\n", 0},
		{"uniq final newline", []string{"uniq"}, "a\na", "a\n", 0},
		{"uniq file", []string{"uniq", "f1"}, "", "1\n2\n", 0},
		{"uniq output operand", []string{"uniq", "f1", "out"}, "", "", 2},
		{"seq", []string{"seq", "3"}, "", "1\n2\n3\n", 0},
		{"seq range", []string{"seq", "-2", "1"}, "", "-2\n-1\n0\n1\n", 0},
		{"seq step", []string{"seq", "-s", ",", "2", "2", "7"}, "", "2,4,6\n", 0},
		{"seq attached sep", []string{"seq", "-s:", "3"}, "", "1:2:3\n", 0},
		{"seq down", []string{"seq", "5", "-2", "1"}, "", "5\n3\n1\n", 0},
		{"seq empty", []string{"seq", "3", "1"}, "", "", 0},
		{"seq zero step", []string{"seq", "1", "0", "3"}, "", "", 2},
		{"seq invalid", []string{"seq", "1.5"}, "", "", 2},
		{"seq too many", []string{"seq", "1", "2", "3", "4"}, "", "", 2},
		{"seq bad option", []string{"seq", "-w", "3"}, "", "", 2},
		{"seq near max", []string{"seq", "9223372036854775806", "9223372036854775807"}, "", "9223372036854775806\n9223372036854775807\n", 0},
		{"sleep", []string{"sleep", "0", "0.001s"}, "", "", 0},
		{"sleep negative", []string{"sleep", "-1"}, "", "", 2},
		{"sleep invalid", []string{"sleep", "1x"}, "", "", 2},
		{"sleep missing", []string{"sleep"}, "", "", 2},
		{"sleep huge", []string{"sleep", "999999999d"}, "", "", 2},
		{"tee stdout", []string{"tee"}, "data", "data", 0},
		{"whoami operand", []string{"whoami", "x"}, "", "", 2},
		{"hostname operand", []string{"hostname", "x"}, "", "", 2},
	})
}

func TestHelpForEveryCommand(t *testing.T) {
	out, err := run(t, "", "--help")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "  ") {
			names = append(names, strings.Fields(line)[0])
		}
	}
	for _, want := range []string{"cp", "echo", "head", "ln", "mv", "rm", "sort", "tail", "tee", "touch", "uniq", "yes"} {
		if !slices.Contains(names, want) {
			t.Fatalf("help lacks %s: %s", want, out)
		}
	}
	for _, name := range names {
		help, err := run(t, "", name, "--help")
		want := "Usage: uniz " + name
		if name == "[" {
			want = "Usage: uniz test"
		}
		if err != nil || !strings.HasPrefix(help, want) {
			t.Fatalf("%s --help: %q %v", name, help, err)
		}
	}
}

func TestExitErrorAndEnvironment(t *testing.T) {
	_, err := run(t, "", "false")
	var exit *uniz.ExitError
	if !errors.As(err, &exit) || exit.Code != 1 {
		t.Fatalf("%v", err)
	}
	t.Setenv("UNIZ_TEST_VAR", "value")
	out, err := run(t, "", "printenv", "UNIZ_TEST_VAR")
	if err != nil || out != "value\n" {
		t.Fatalf("%q %v", out, err)
	}
	out, err = run(t, "", "printenv", "UNIZ_TEST_VAR", "UNIZ_DEFINITELY_UNSET", "A=B")
	if out != "value\n" || uniz.ExitCode(err) != 1 || !errors.As(err, &exit) {
		t.Fatalf("%q %v", out, err)
	}
	out, err = run(t, "", "printenv")
	if err != nil || !strings.Contains(out, "UNIZ_TEST_VAR=value\n") {
		t.Fatalf("%v", err)
	}
	for _, command := range []string{"whoami", "hostname"} {
		out, err := run(t, "", command)
		if err != nil || len(strings.TrimSpace(out)) == 0 || !strings.HasSuffix(out, "\n") {
			t.Fatalf("%s: %q %v", command, out, err)
		}
	}
}

func TestYesAndSleepCancellation(t *testing.T) {
	var out bytes.Buffer
	w := &limitWriter{limit: 10, buf: &out}
	err := uniz.New(uniz.Config{Stdout: w}).Run(context.Background(), []string{"yes", "a", "b"})
	if !errors.Is(err, io.ErrShortBuffer) || !strings.HasPrefix(out.String(), "a b\na b\n") {
		t.Fatalf("%q %v", out.String(), err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	start := time.Now()
	if err := uniz.New(uniz.Config{}).Run(ctx, []string{"sleep", "10"}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatal("sleep ignored cancellation")
	}
	ctx, cancel = context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := uniz.Yes(ctx, io.Discard, "y"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
}

// limitWriter accepts limit writes, then fails.
type limitWriter struct {
	limit int
	buf   *bytes.Buffer
}

func (w *limitWriter) Write(p []byte) (int, error) {
	if w.limit == 0 {
		return 0, io.ErrShortBuffer
	}
	w.limit--
	return w.buf.Write(p)
}

func TestTee(t *testing.T) {
	t.Chdir(t.TempDir())
	out, err := run(t, "one\n", "tee", "a", "b")
	if err != nil || out != "one\n" || read(t, "a") != "one\n" || read(t, "b") != "one\n" {
		t.Fatalf("%q %v", out, err)
	}
	if _, err := run(t, "two\n", "tee", "-a", "a"); err != nil || read(t, "a") != "one\ntwo\n" {
		t.Fatal(err)
	}
	out, err = run(t, "x", "tee", "missing/file", "c")
	if uniz.ExitCode(err) != 1 || !errors.Is(err, os.ErrNotExist) || out != "x" || read(t, "c") != "x" {
		t.Fatalf("%q %v", out, err)
	}
	err = uniz.New(uniz.Config{Stdin: strings.NewReader("x"), Stdout: failWriter{}}).Run(context.Background(), []string{"tee", "d"})
	if !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal(err)
	}
}

func TestTouchFlagAppliesToAllOperands(t *testing.T) {
	t.Chdir(t.TempDir())
	if _, err := run(t, "", "touch", "new", "-c", "not-created"); err != nil {
		t.Fatal(err)
	}
	if exists("new") || exists("not-created") {
		t.Fatal("options are parsed before work, so -c applies to every operand")
	}
}

func TestTouch(t *testing.T) {
	t.Chdir(t.TempDir())
	if _, err := run(t, "", "touch", "new"); err != nil || read(t, "new") != "" {
		t.Fatal(err)
	}
	write(t, "kept", "content")
	old := time.Unix(1000, 0)
	if err := os.Chtimes("kept", old, old); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, "", "touch", "kept"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat("kept")
	if err != nil || !info.ModTime().After(old) || read(t, "kept") != "content" {
		t.Fatalf("%v %v", info.ModTime(), err)
	}
	if _, err := run(t, "", "touch", "-c", "absent"); err != nil || exists("absent") {
		t.Fatal(err)
	}
	if _, err := run(t, "", "touch"); uniz.ExitCode(err) != 2 {
		t.Fatal(err)
	}
	when := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := uniz.Touch(context.Background(), "typed", uniz.TouchOptions{Time: when}); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat("typed"); err != nil || !info.ModTime().Equal(when) {
		t.Fatalf("%v", err)
	}
	if _, err := run(t, "", "touch", "missing-dir/file", "after"); uniz.ExitCode(err) != 1 || !exists("after") {
		t.Fatal(err)
	}
}

func TestRm(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	write(t, "file", "x")
	write(t, "tree/a/b", "x")
	write(t, "outside/keep", "x")
	if err := os.Mkdir("empty", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("outside", "link"); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, "", "rm", "file"); err != nil || exists("file") {
		t.Fatal(err)
	}
	if _, err := run(t, "", "rm", "tree"); !errors.Is(err, uniz.ErrIsDirectory) || uniz.ExitCode(err) != 1 {
		t.Fatal(err)
	}
	if _, err := run(t, "", "rm", "-d", "empty"); err != nil || exists("empty") {
		t.Fatal(err)
	}
	if _, err := run(t, "", "rm", "-r", "tree"); err != nil || exists("tree") {
		t.Fatal(err)
	}
	// A trailing slash must remove the link itself, never the target's contents.
	if _, err := run(t, "", "rm", "-r", "link/"); err != nil || exists("link") || read(t, "outside/keep") != "x" {
		t.Fatal(err)
	}
	if _, err := run(t, "", "rm", "missing"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	if _, err := run(t, "", "rm", "-f", "missing"); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, "", "rm", "-f"); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, "", "rm"); uniz.ExitCode(err) != 2 {
		t.Fatal(err)
	}
	for _, path := range []string{".", "..", "./", "outside/..", "/", "//"} {
		if _, err := run(t, "", "rm", "-rf", path); !errors.Is(err, uniz.ErrRefused) {
			t.Fatalf("%s: %v", path, err)
		}
	}
	if read(t, "outside/keep") != "x" {
		t.Fatal("refused removal deleted data")
	}
	write(t, "later", "x")
	if _, err := run(t, "", "rm", "missing", "later"); uniz.ExitCode(err) != 1 || exists("later") {
		t.Fatal("rm must continue after a failed operand")
	}
}

func TestRmdir(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, dir := range []string{"a/b/c", "full", "solo"} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write(t, "full/file", "x")
	write(t, "file", "x")
	if err := os.Symlink("solo", "dirlink"); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, "", "rmdir", "solo/"); err != nil || exists("solo") {
		t.Fatal(err)
	}
	if _, err := run(t, "", "rmdir", "full"); err == nil || !exists("full/file") {
		t.Fatal("removed non-empty directory")
	}
	for _, p := range []string{"file", "dirlink"} {
		if _, err := run(t, "", "rmdir", p); !errors.Is(err, uniz.ErrNotDirectory) {
			t.Fatalf("%s: %v", p, err)
		}
	}
	if _, err := run(t, "", "rmdir", "-p", "a/b/c"); err != nil || exists("a") {
		t.Fatal(err)
	}
}

func TestCp(t *testing.T) {
	t.Chdir(t.TempDir())
	write(t, "src", "data")
	if err := os.Chmod("src", 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, "", "cp", "src", "dst"); err != nil || read(t, "dst") != "data" {
		t.Fatal(err)
	}
	if info, _ := os.Stat("dst"); info.Mode().Perm() != 0o640 {
		t.Fatalf("mode %v", info.Mode())
	}
	write(t, "dst", "longer old content")
	if _, err := run(t, "", "cp", "src", "dst"); err != nil || read(t, "dst") != "data" {
		t.Fatal("overwrite must truncate")
	}
	if err := os.Mkdir("dir", 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, "other", "o")
	if _, err := run(t, "", "cp", "src", "other", "dir"); err != nil || read(t, "dir/src") != "data" || read(t, "dir/other") != "o" {
		t.Fatal(err)
	}
	if _, err := run(t, "", "cp", "src", "other", "nodir"); uniz.ExitCode(err) != 2 {
		t.Fatal(err)
	}
	if _, err := run(t, "", "cp", "src"); uniz.ExitCode(err) != 2 {
		t.Fatal(err)
	}
	// Same file, including through a hard link, must not truncate the source.
	if err := os.Link("src", "hard"); err != nil {
		t.Fatal(err)
	}
	for _, dst := range []string{"src", "hard", "./src"} {
		if _, err := run(t, "", "cp", "src", dst); !errors.Is(err, uniz.ErrSameFile) || read(t, "src") != "data" {
			t.Fatalf("%s: %v", dst, err)
		}
	}
	write(t, "tree/sub/file", "deep")
	if err := os.Symlink("sub/file", "tree/link"); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, "", "cp", "tree", "copy"); !errors.Is(err, uniz.ErrIsDirectory) || exists("copy") {
		t.Fatal(err)
	}
	if _, err := run(t, "", "cp", "-r", "tree", "copy"); err != nil || read(t, "copy/sub/file") != "deep" {
		t.Fatal(err)
	}
	if target, err := os.Readlink("copy/link"); err != nil || target != "sub/file" {
		t.Fatalf("symlink not preserved: %q %v", target, err)
	}
	// Existing destination directory: copy goes inside it.
	if _, err := run(t, "", "cp", "-R", "tree", "dir"); err != nil || read(t, "dir/tree/sub/file") != "deep" {
		t.Fatal(err)
	}
	for _, dst := range []string{"tree/inner", "tree/sub/x", "tree/new/deep", "./tree/../tree/x"} {
		if _, err := run(t, "", "cp", "-r", "tree", dst); !errors.Is(err, uniz.ErrIntoItself) || exists(dst) {
			t.Fatalf("%s: %v", dst, err)
		}
	}
	old := time.Unix(5000, 0)
	if err := os.Chtimes("src", old, old); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, "", "cp", "-p", "src", "preserved"); err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Stat("preserved"); !info.ModTime().Equal(old) || info.Mode().Perm() != 0o640 {
		t.Fatalf("not preserved: %v %v", info.ModTime(), info.Mode())
	}
	if _, err := run(t, "", "cp", "missing", "src", "dir"); uniz.ExitCode(err) != 1 || !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	// Typed Copy takes an exact destination even when it is a directory.
	if err := uniz.Copy(context.Background(), "src", "dir", uniz.CopyOptions{}); err == nil {
		t.Fatal("typed copy onto a directory should fail")
	}
}

func TestMv(t *testing.T) {
	t.Chdir(t.TempDir())
	write(t, "a", "A")
	if _, err := run(t, "", "mv", "a", "b"); err != nil || exists("a") || read(t, "b") != "A" {
		t.Fatal(err)
	}
	if err := os.Mkdir("dir", 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, "c", "C")
	if _, err := run(t, "", "mv", "b", "c", "dir"); err != nil || read(t, "dir/b") != "A" || read(t, "dir/c") != "C" {
		t.Fatal(err)
	}
	if _, err := run(t, "", "mv", "dir", "dir/inside"); !errors.Is(err, uniz.ErrIntoItself) || !exists("dir/b") {
		t.Fatal(err)
	}
	if err := os.Link("dir/b", "hard"); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, "", "mv", "dir/b", "hard"); !errors.Is(err, uniz.ErrSameFile) || !exists("dir/b") {
		t.Fatal(err)
	}
	if _, err := run(t, "", "mv", "dir", "renamed"); err != nil || read(t, "renamed/c") != "C" {
		t.Fatal(err)
	}
	if err := os.Symlink("renamed", "link"); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, "", "mv", "link/", "link2"); err != nil || !exists("link2") || !exists("renamed/c") {
		t.Fatal("mv with trailing slash must move the link itself")
	}
	if _, err := run(t, "", "mv", "missing", "x"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
}

func TestLn(t *testing.T) {
	t.Chdir(t.TempDir())
	write(t, "target", "T")
	write(t, "other", "O")
	if _, err := run(t, "", "ln", "target", "hard"); err != nil {
		t.Fatal(err)
	}
	a, _ := os.Stat("target")
	b, _ := os.Stat("hard")
	if !os.SameFile(a, b) {
		t.Fatal("not a hard link")
	}
	if _, err := run(t, "", "ln", "-s", "target", "soft"); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.Readlink("soft"); got != "target" {
		t.Fatal(got)
	}
	if _, err := run(t, "", "ln", "-s", "other", "soft"); !errors.Is(err, os.ErrExist) {
		t.Fatal(err)
	}
	if _, err := run(t, "", "ln", "-sf", "other", "soft"); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.Readlink("soft"); got != "other" {
		t.Fatal(got)
	}
	if _, err := run(t, "", "ln", "-f", "target", "hard"); !errors.Is(err, uniz.ErrSameFile) {
		t.Fatal(err)
	}
	if err := os.Mkdir("dir", 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, "", "ln", "-s", "../target", "dir"); err != nil || read(t, "dir/target") != "T" {
		t.Fatal(err)
	}
	if err := os.Mkdir("one", 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir("one")
	if _, err := run(t, "", "ln", "-s", "../other"); err != nil || read(t, "other") != "O" {
		t.Fatal(err)
	}
	t.Chdir("..")
	// -n replaces a symlink to a directory instead of linking inside it.
	if err := os.Symlink("dir", "dirlink"); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, "", "ln", "-sfn", "one", "dirlink"); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.Readlink("dirlink"); got != "one" {
		t.Fatal(got)
	}
	if _, err := run(t, "", "ln", "-sf", "x", "dir"); err != nil || !exists("dir/x") {
		t.Fatal(err)
	}
	if err := uniz.Link(context.Background(), "x", "dir", uniz.LinkOptions{Symbolic: true, Force: true}); !errors.Is(err, uniz.ErrIsDirectory) {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(".")
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".uniz-link-") {
			t.Fatalf("temporary link left behind: %s", e.Name())
		}
	}
}

func TestRealpathAndReadlink(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	write(t, "sub/file", "x")
	if err := os.Symlink("sub/file", "link"); err != nil {
		t.Fatal(err)
	}
	out, err := run(t, "", "realpath", "link", "sub/../sub")
	if err != nil || out != filepath.Join(real, "sub/file")+"\n"+filepath.Join(real, "sub")+"\n" {
		t.Fatalf("%q %v", out, err)
	}
	if out, err := run(t, "", "readlink", "link"); err != nil || out != "sub/file\n" {
		t.Fatalf("%q %v", out, err)
	}
	if out, err := run(t, "", "readlink", "-f", "link"); err != nil || out != filepath.Join(real, "sub/file")+"\n" {
		t.Fatalf("%q %v", out, err)
	}
	if out, err := run(t, "", "readlink", "sub/file", "link"); uniz.ExitCode(err) != 1 || out != "sub/file\n" {
		t.Fatalf("%q %v", out, err)
	}
	if _, err := run(t, "", "realpath", "missing"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	if _, err := run(t, "", "realpath"); uniz.ExitCode(err) != 2 {
		t.Fatal(err)
	}
	if got, err := uniz.ReadLink("link"); err != nil || got != "sub/file" {
		t.Fatal(got, err)
	}
}

func TestTypedFilters(t *testing.T) {
	ctx := context.Background()
	in := []string{"b", "a", "c"}
	if got := uniz.SortLines(in, uniz.SortOptions{}); !slices.Equal(got, []string{"a", "b", "c"}) || in[0] != "b" {
		t.Fatalf("%v %v", got, in)
	}
	var out bytes.Buffer
	if err := uniz.Head(ctx, &out, strings.NewReader("a\nb\n"), uniz.SliceOptions{Count: -1}); !errors.Is(err, uniz.ErrNegativeCount) {
		t.Fatal(err)
	}
	if err := uniz.Tail(ctx, &out, strings.NewReader("abc"), uniz.SliceOptions{Count: 0, Bytes: true}); err != nil || out.Len() != 0 {
		t.Fatal(err)
	}
	long := strings.Repeat("x", 100000) + "\n" + strings.Repeat("y", 70000)
	if err := uniz.Tail(ctx, &out, strings.NewReader(long), uniz.SliceOptions{Count: 1}); err != nil || out.String() != strings.Repeat("y", 70000) {
		t.Fatal(err)
	}
	out.Reset()
	if err := uniz.Tail(ctx, &out, io.MultiReader(strings.NewReader("a\n"), failingReader{}), uniz.SliceOptions{Count: 1}); !errors.Is(err, io.ErrUnexpectedEOF) || out.Len() != 0 {
		t.Fatal(err)
	}
	if err := uniz.Uniq(ctx, &out, io.MultiReader(strings.NewReader("a\na\n"), failingReader{}), uniz.UniqOptions{}); !errors.Is(err, io.ErrUnexpectedEOF) || out.String() != "a\n" {
		t.Fatalf("%q %v", out.String(), err)
	}
	out.Reset()
	if err := uniz.Seq(ctx, &out, uniz.SeqOptions{First: 1, Step: 1, Last: 3}); err != nil || out.String() != "123\n" {
		t.Fatalf("%q %v", out.String(), err)
	}
	if err := uniz.Seq(ctx, &out, uniz.SeqOptions{Step: 0}); !errors.Is(err, uniz.ErrZeroStep) {
		t.Fatal(err)
	}
	if got := slices.Collect(uniz.Sequence(math.MaxInt64-1, 1, math.MaxInt64)); len(got) != 2 {
		t.Fatal(got)
	}
	if got := slices.Collect(uniz.Sequence(math.MinInt64+1, -1, math.MinInt64)); len(got) != 2 {
		t.Fatal(got)
	}
	if got := slices.Collect(uniz.Sequence(1, 0, 5)); len(got) != 0 {
		t.Fatal(got)
	}
	for v := range uniz.Sequence(1, 1, math.MaxInt64) {
		if v == 3 {
			break // early termination must be honored
		}
	}
	lines, err := uniz.ReadLines(ctx, strings.NewReader("a\n\nb"))
	if err != nil || !slices.Equal(lines, []string{"a", "", "b"}) {
		t.Fatalf("%q %v", lines, err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	for name, err := range map[string]error{
		"head": uniz.Head(canceled, io.Discard, strings.NewReader("x"), uniz.SliceOptions{Count: 1}),
		"tail": uniz.Tail(canceled, io.Discard, strings.NewReader("x"), uniz.SliceOptions{Count: 1}),
		"sort": uniz.Sort(canceled, io.Discard, strings.NewReader("x"), uniz.SortOptions{}),
		"uniq": uniz.Uniq(canceled, io.Discard, strings.NewReader("x"), uniz.UniqOptions{}),
		"seq":  uniz.Seq(canceled, io.Discard, uniz.SeqOptions{Step: 1, Last: 1}),
		"cp":   uniz.Copy(canceled, "a", "b", uniz.CopyOptions{}),
		"rm":   uniz.Remove(canceled, "a", uniz.RemoveOptions{}),
	} {
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

func TestOutputFailureStopsMultiInputCommands(t *testing.T) {
	t.Chdir(t.TempDir())
	write(t, "a", "a\n")
	for _, command := range []string{"head", "tail", "uniq"} {
		args := []string{command, "a", "missing"}
		if command == "uniq" {
			args = []string{command, "a"}
		}
		err := uniz.New(uniz.Config{Stdout: failWriter{}}).Run(context.Background(), args)
		if !errors.Is(err, io.ErrClosedPipe) || errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s: %v", command, err)
		}
	}
	for _, args := range [][]string{{"sort", "a"}, {"seq", "3"}, {"echo", "x"}, {"realpath", "a", "a"}, {"printenv"}} {
		if err := uniz.New(uniz.Config{Stdout: failWriter{}}).Run(context.Background(), args); !errors.Is(err, io.ErrClosedPipe) {
			t.Fatalf("%v: %v", args, err)
		}
	}
}
