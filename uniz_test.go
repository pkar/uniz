package uniz_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/pkar/uniz"
)

func TestSingleImport(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hello"), []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	entries, err := uniz.List(ctx, dir, uniz.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name != "hello" || entries[0].Info.Size() != 5 {
		t.Fatalf("%+v", entries)
	}
	var out, diagnostics bytes.Buffer
	runner := uniz.New(uniz.Config{Stdout: &out, Stderr: &diagnostics})
	if err := runner.Run(ctx, []string{"ls", dir}); err != nil {
		t.Fatal(err)
	}
	if out.String() != "hello\n" {
		t.Fatal(out.String())
	}
	err = runner.Run(ctx, []string{"ls", filepath.Join(dir, "missing")})
	if !errors.Is(err, os.ErrNotExist) || uniz.ExitCode(err) != 1 {
		t.Fatalf("%v", err)
	}
	if diagnostics.Len() != 0 {
		t.Fatalf("library printed error: %s", diagnostics.String())
	}
	err = runner.Run(ctx, []string{"unknown"})
	var usage *uniz.UsageError
	if !errors.As(err, &usage) || uniz.ExitCode(err) != 2 {
		t.Fatalf("%v", err)
	}
	if uniz.ExitCode(nil) != 0 {
		t.Fatal("nil error must map to success")
	}
}

func TestDefaultsAndVersion(t *testing.T) {
	var runner uniz.Runner
	if err := runner.Run(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	r := uniz.New(uniz.Config{Stdout: &out})
	if err := r.Run(context.Background(), []string{"--version"}); err != nil {
		t.Fatal(err)
	}
	if out.String() != "uniz dev\n" {
		t.Fatal(out.String())
	}
	out.Reset()
	r = uniz.New(uniz.Config{Stdout: &out, Version: "test"})
	if err := r.Run(context.Background(), []string{"--version"}); err != nil {
		t.Fatal(err)
	}
	if out.String() != "uniz test\n" {
		t.Fatal(out.String())
	}
}

func TestCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out bytes.Buffer
	if err := uniz.New(uniz.Config{Stdout: &out}).Run(ctx, []string{"ls"}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Fatal("canceled command wrote output")
	}
	entries, err := uniz.List(ctx, ".", uniz.ListOptions{})
	if !errors.Is(err, context.Canceled) || len(entries) != 0 {
		t.Fatalf("%v %v", entries, err)
	}
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
func TestWriterError(t *testing.T) {
	runner := uniz.New(uniz.Config{Stdout: failWriter{}})
	for _, args := range [][]string{nil, {"ls", "--help"}, {"ls", "."}} {
		if err := runner.Run(context.Background(), args); !errors.Is(err, io.ErrClosedPipe) {
			t.Fatal(err)
		}
	}
}

type cancelWriter struct {
	cancel context.CancelFunc
	writes int
}

func (w *cancelWriter) Write(p []byte) (int, error) { w.writes++; w.cancel(); return len(p), nil }
func TestCancelBetweenWrites(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a", "b"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := &cancelWriter{cancel: cancel}
	err := uniz.New(uniz.Config{Stdout: w}).Run(ctx, []string{"ls", dir})
	if !errors.Is(err, context.Canceled) || w.writes != 1 {
		t.Fatalf("err=%v writes=%d", err, w.writes)
	}
}
