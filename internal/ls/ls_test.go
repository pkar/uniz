package ls

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func fixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"b", "a", ".hidden"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("hello"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}
func TestList(t *testing.T) {
	dir := fixture(t)
	for _, tc := range []struct {
		name string
		opts Options
		want []string
	}{
		{"default", Options{}, []string{"a", "b"}},
		{"all", Options{All: true}, []string{".", "..", ".hidden", "a", "b"}},
		{"almost", Options{AlmostAll: true}, []string{".hidden", "a", "b"}},
		{"reverse", Options{Reverse: true}, []string{"b", "a"}},
		{"directory", Options{Directory: true}, []string{dir}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entries, err := List(dir, tc.opts)
			if err != nil {
				t.Fatal(err)
			}
			var names []string
			for _, e := range entries {
				names = append(names, e.Name)
				if e.Info == nil {
					t.Fatal("missing metadata")
				}
			}
			if !reflect.DeepEqual(names, tc.want) {
				t.Fatalf("got %v, want %v", names, tc.want)
			}
		})
	}
	entries, err := List(filepath.Join(dir, ".hidden"), Options{})
	if err != nil || len(entries) != 1 {
		t.Fatalf("explicit hidden file: %v %v", entries, err)
	}
}
func TestTimeSort(t *testing.T) {
	dir := fixture(t)
	old := time.Unix(100, 0)
	newer := time.Unix(200, 0)
	for name, stamp := range map[string]time.Time{"a": old, "b": newer} {
		if err := os.Chtimes(filepath.Join(dir, name), stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}
	for _, reverse := range []bool{false, true} {
		entries, err := List(dir, Options{SortTime: true, Reverse: reverse})
		if err != nil {
			t.Fatal(err)
		}
		want := "b"
		if reverse {
			want = "a"
		}
		if entries[0].Name != want {
			t.Fatalf("got %s, want %s", entries[0].Name, want)
		}
	}
	if err := os.Chtimes(filepath.Join(dir, "a"), newer, newer); err != nil {
		t.Fatal(err)
	}
	entries, err := List(dir, Options{SortTime: true})
	if err != nil || entries[0].Name != "a" {
		t.Fatalf("tie: %v %v", entries, err)
	}
}
func TestSymlinksAndLong(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "link")
	if err := os.Symlink("missing", link); err != nil {
		t.Skip(err)
	}
	entries, err := List(link, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].LinkTarget != "missing" || entries[0].Info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("%+v", entries)
	}
	var out bytes.Buffer
	if err := Run(&out, []string{link}, Options{Long: true}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), " -> missing\n") {
		t.Fatal(out.String())
	}
	if err := os.Symlink(dir, filepath.Join(dir, "dirlink")); err != nil {
		t.Fatal(err)
	}
	entries, err = List(filepath.Join(dir, "dirlink"), Options{})
	if err != nil || len(entries) != 1 || entries[0].Info.IsDir() {
		t.Fatalf("directory symlink: %v %v", entries, err)
	}
}
func TestRunPartialAndHeaders(t *testing.T) {
	dir := fixture(t)
	empty := t.TempDir()
	var out bytes.Buffer
	err := Run(&out, []string{filepath.Join(dir, "missing"), dir, empty}, Options{})
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("got %v", err)
	}
	if !strings.Contains(out.String(), dir+":\na\nb\n") || !strings.Contains(out.String(), empty+":\n") {
		t.Fatal(out.String())
	}
}

type brokenWriter struct{}

func (brokenWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
func TestOutputError(t *testing.T) {
	dir := fixture(t)
	for _, opts := range []Options{{}, {Long: true}} {
		if err := Run(brokenWriter{}, []string{dir}, opts); !errors.Is(err, io.ErrClosedPipe) {
			t.Fatalf("got %v", err)
		}
	}
	if err := Run(brokenWriter{}, []string{dir, dir}, Options{}); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal(err)
	}
}
func TestDefaultAndSafeNames(t *testing.T) {
	dir := fixture(t)
	t.Chdir(dir)
	if err := os.WriteFile("line\nbreak", nil, 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Run(&out, nil, Options{}); err != nil {
		t.Fatal(err)
	}
	if out.String() != "a\nb\n\"line\\nbreak\"\n" {
		t.Fatalf("%q", out.String())
	}
	entries, err := List("", Options{})
	if err != nil || len(entries) != 3 {
		t.Fatalf("%v %v", entries, err)
	}
}
