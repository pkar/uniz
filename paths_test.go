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

func TestPathOperations(t *testing.T) {
	for _, tc := range []struct{ path, base, dir string }{
		{"", "", "."}, {"/", "/", "/"}, {"//", "/", "/"}, {"////", "/", "/"},
		{"file", "file", "."}, {"file///", "file", "."}, {"/a/b/", "b", "/a"},
		{"a//b///", "b", "a"}, {"a/./b", "b", "a/."}, {"a/../b", "b", "a/.."},
		{".", ".", "."}, {"..", "..", "."}, {"/a", "a", "/"},
		{"//a", "a", "/"}, {"a\\b", "a\\b", "."}, {"猫/é.txt", "é.txt", "猫"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			if got := uniz.BaseName(tc.path, ""); got != tc.base {
				t.Fatalf("base=%q want=%q", got, tc.base)
			}
			if got := uniz.DirName(tc.path); got != tc.dir {
				t.Fatalf("dir=%q want=%q", got, tc.dir)
			}
		})
	}
	for _, tc := range []struct{ path, suffix, want string }{
		{"/a/name.txt/", ".txt", "name"}, {"name", "name", "name"},
		{"name.txt", ".TXT", "name.txt"}, {"x.tar.gz", ".gz", "x.tar"}, {"/", "/", "/"},
	} {
		if got := uniz.BaseName(tc.path, tc.suffix); got != tc.want {
			t.Fatalf("%q %q: got %q want %q", tc.path, tc.suffix, got, tc.want)
		}
	}
}

func TestMakeDirectory(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	dir := filepath.Join(root, "new")
	if err := uniz.MakeDirectory(ctx, dir, uniz.MakeDirectoryOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := uniz.MakeDirectory(ctx, dir, uniz.MakeDirectoryOptions{}); !errors.Is(err, os.ErrExist) {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := uniz.MakeDirectory(ctx, dir, uniz.MakeDirectoryOptions{Parents: true}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() || info.Mode().Perm() != 0700 {
		t.Fatalf("%v", info.Mode())
	}
	nested := filepath.Join(root, "parent", "child")
	if err := uniz.MakeDirectory(ctx, nested, uniz.MakeDirectoryOptions{}); !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	if err := uniz.MakeDirectory(ctx, nested, uniz.MakeDirectoryOptions{Parents: true}); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := uniz.MakeDirectory(ctx, file, uniz.MakeDirectoryOptions{Parents: true}); err == nil {
		t.Fatal("accepted file")
	}
	if err := uniz.MakeDirectory(ctx, filepath.Join(file, "child"), uniz.MakeDirectoryOptions{Parents: true}); err == nil {
		t.Fatal("accepted file parent")
	}
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	canceled := filepath.Join(root, "canceled")
	if err := uniz.MakeDirectory(ctx, canceled, uniz.MakeDirectoryOptions{Parents: true}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := os.Stat(canceled); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("canceled call created directory: %v", err)
	}
}

func TestPathCommands(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
		code int
	}{
		{[]string{"basename", "/a/b.txt/", ".txt"}, "b\n", 0},
		{[]string{"basename", "--", "-file"}, "-file\n", 0},
		{[]string{"basename", ""}, "\n", 0},
		{[]string{"basename"}, "", 2},
		{[]string{"basename", "a", "b", "c"}, "", 2},
		{[]string{"basename", "-a", "x"}, "", 2},
		{[]string{"dirname", "a/b", "name", "/"}, "a\n.\n/\n", 0},
		{[]string{"dirname", "--", "-a/b"}, "-a\n", 0},
		{[]string{"dirname"}, "", 2},
		{[]string{"dirname", "-z", "x"}, "", 2},
		{[]string{"mkdir"}, "", 2},
		{[]string{"mkdir", "-m", "700", "x"}, "", 2},
	} {
		var out, stderr bytes.Buffer
		err := uniz.New(uniz.Config{Stdout: &out, Stderr: &stderr}).Run(context.Background(), tc.args)
		if uniz.ExitCode(err) != tc.code || out.String() != tc.want || stderr.Len() != 0 {
			t.Fatalf("%v: out=%q err=%v stderr=%q", tc.args, out.String(), err, stderr.String())
		}
	}
	for _, command := range []string{"basename", "dirname", "mkdir"} {
		var out bytes.Buffer
		r := uniz.New(uniz.Config{Stdout: &out})
		if err := r.Run(context.Background(), []string{command, "--help"}); err != nil || !strings.Contains(out.String(), "Usage: uniz "+command) {
			t.Fatalf("%q %v", out.String(), err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if err := r.Run(ctx, []string{command, "x"}); !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	}
	for _, command := range []string{"basename", "dirname"} {
		if err := uniz.New(uniz.Config{Stdout: failWriter{}}).Run(context.Background(), []string{command, "a"}); !errors.Is(err, io.ErrClosedPipe) {
			t.Fatal(err)
		}
	}
}

func TestMkdirCommand(t *testing.T) {
	t.Chdir(t.TempDir())
	r := uniz.New(uniz.Config{})
	ctx := context.Background()
	if err := r.Run(ctx, []string{"mkdir", "-p", "a/b", "c/d"}); err != nil {
		t.Fatal(err)
	}
	if err := r.Run(ctx, []string{"mkdir", "a", "later"}); !errors.Is(err, os.ErrExist) {
		t.Fatal(err)
	}
	for _, path := range []string{"a/b", "c/d", "later"} {
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			t.Fatalf("%s: %v", path, err)
		}
	}
	if err := r.Run(ctx, []string{"mkdir", "--", "-literal"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat("-literal"); err != nil {
		t.Fatal(err)
	}
	// Parse every flag before doing work, so invalid usage has no side effects.
	if err := r.Run(ctx, []string{"mkdir", "not-created", "-z"}); uniz.ExitCode(err) != 2 {
		t.Fatal(err)
	}
	if _, err := os.Stat("not-created"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
}
