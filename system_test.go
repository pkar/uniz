package uniz_test

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pkar/uniz"
)

func needShell(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("needs /bin/sh")
	}
}

func TestGrep(t *testing.T) {
	t.Chdir(t.TempDir())
	write(t, "f1", "hit one\nmiss\n")
	write(t, "f2", "nothing\n")
	write(t, "dir/a", "hit\n")
	write(t, "dir/sub/b", "hit\nmiss\n")
	write(t, "pats", "one\nnothing\n")
	write(t, "nopats", "")
	runCases(t, []commandCase{
		{"basic", []string{"grep", "b"}, "abc\nxyz\n", "abc\n", 0},
		{"no match", []string{"grep", "q"}, "abc\n", "", 1},
		{"empty pattern", []string{"grep", ""}, "a\n\nb", "a\n\nb\n", 0},
		{"ere alternation", []string{"grep", "-E", "a|z"}, "abc\nxyz\nm\n", "abc\nxyz\n", 0},
		{"bre alternation", []string{"grep", `a\|z`}, "abc\nxyz\nm\n", "abc\nxyz\n", 0},
		{"bre plus literal", []string{"grep", "a+"}, "a+\naa\n", "a+\n", 0},
		{"bre group interval", []string{"grep", `\(ab\)\{2\}`}, "abab\nab\n", "abab\n", 0},
		{"bre star at start", []string{"grep", "*a"}, "*a\nba\n", "*a\n", 0},
		{"bre dollar inside", []string{"grep", "a$b"}, "a$b\nab\n", "a$b\n", 0},
		{"anchors", []string{"grep", "^a.*c$"}, "abc\nabcd\nxabc\n", "abc\n", 0},
		{"ere interval", []string{"grep", "-E", "^a{2}$"}, "a\naa\naaa\n", "aa\n", 0},
		{"ere literal brace", []string{"grep", "-E", "a{"}, "a{\n", "a{\n", 0},
		{"bracket", []string{"grep", "[]x]"}, "]\ny\n", "]\n", 0},
		{"bracket backslash", []string{"grep", `[\]`}, "a\\b\nab\n", "a\\b\n", 0},
		{"class", []string{"grep", "-c", "[[:digit:]]"}, "a1\nb\n2\n", "2\n", 0},
		{"word boundary", []string{"grep", `\<fo`}, "foo\nafoo\n", "foo\n", 0},
		{"fixed", []string{"grep", "-F", "a.c"}, "abc\na.c\n", "a.c\n", 0},
		{"ignore case", []string{"grep", "-i", "ABC"}, "abc\nx\n", "abc\n", 0},
		{"invert", []string{"grep", "-v", "a"}, "a\nb\n", "b\n", 0},
		{"count", []string{"grep", "-c", "a"}, "a\nb\na\n", "2\n", 0},
		{"count zero", []string{"grep", "-c", "z"}, "a\n", "0\n", 1},
		{"line numbers", []string{"grep", "-n", "b"}, "a\nb\n", "2:b\n", 0},
		{"only matching", []string{"grep", "-o", "[0-9]+", "-E"}, "a12b3\nx\n", "12\n3\n", 0},
		{"longest match", []string{"grep", "-oE", "a|ab"}, "ab\n", "ab\n", 0},
		{"word", []string{"grep", "-w", "foo"}, "foo bar\nfoobar\nbar_foo\n", "foo bar\n", 0},
		{"word later occurrence", []string{"grep", "-w", "foo"}, "foobar foo\n", "foobar foo\n", 0},
		{"line", []string{"grep", "-x", "-e", "ab", "-e", "c"}, "ab\nabc\nc\n", "ab\nc\n", 0},
		{"multiple -e", []string{"grep", "-e", "a", "-e", "b"}, "a\nb\nc\n", "a\nb\n", 0},
		{"pattern newline", []string{"grep", "a\nb"}, "a\nb\nc\n", "a\nb\n", 0},
		{"pattern file", []string{"grep", "-f", "pats", "f1", "f2"}, "", "f1:hit one\nf2:nothing\n", 0},
		{"empty pattern file", []string{"grep", "-f", "nopats"}, "a\n", "", 1},
		{"missing pattern file", []string{"grep", "-f", "missing"}, "a\n", "", 2},
		{"max count", []string{"grep", "-m", "1", "a"}, "a1\na2\n", "a1\n", 0},
		{"max count zero", []string{"grep", "-m0", "a"}, "a\n", "", 1},
		{"quiet", []string{"grep", "-q", "a"}, "a\n", "", 0},
		{"quiet with missing file", []string{"grep", "-q", "hit", "missing", "f1"}, "", "", 0},
		{"files names", []string{"grep", "hit", "f1", "f2"}, "", "f1:hit one\n", 0},
		{"-h", []string{"grep", "-h", "hit", "f1", "f2"}, "", "hit one\n", 0},
		{"-H", []string{"grep", "-H", "hit", "f1"}, "", "f1:hit one\n", 0},
		{"-c files", []string{"grep", "-c", "hit", "f1", "f2"}, "", "f1:1\nf2:0\n", 0},
		{"-l", []string{"grep", "-l", "hit", "f1", "f2"}, "", "f1\n", 0},
		{"-L", []string{"grep", "-L", "hit", "f1", "f2"}, "", "f2\n", 0},
		{"stdin dash", []string{"grep", "a", "-", "f2"}, "a\n", "(standard input):a\n", 0},
		{"recursive", []string{"grep", "-r", "hit", "dir"}, "", "dir/a:hit\ndir/sub/b:hit\n", 0},
		{"recursive -n", []string{"grep", "-rn", "miss", "dir"}, "", "dir/sub/b:2:miss\n", 0},
		{"directory without -r", []string{"grep", "hit", "dir"}, "", "", 2},
		{"missing file", []string{"grep", "hit", "missing", "f1"}, "", "f1:hit one\n", 2},
		{"missing file -s", []string{"grep", "-s", "hit", "missing"}, "", "", 2},
		{"binary", []string{"grep", "a"}, "a\x00b\n", "Binary file (standard input) matches\n", 0},
		{"binary as text", []string{"grep", "-a", "a"}, "a\x00b\n", "a\x00b\n", 0},
		{"back-reference", []string{"grep", `\(a\)\1`}, "aa\n", "", 2},
		{"unmatched bracket", []string{"grep", "[a"}, "", "", 2},
		{"no pattern", []string{"grep"}, "", "", 2},
		{"bad max", []string{"grep", "-m", "x", "a"}, "", "", 2},
	})

	t.Run("implicit recursive root", func(t *testing.T) {
		t.Chdir("dir")
		out, err := run(t, "", "grep", "-r", "hit")
		if err != nil || out != "a:hit\nsub/b:hit\n" {
			t.Fatalf("%q %v", out, err)
		}
	})

	m, err := uniz.CompilePattern([]string{"x+y"}, uniz.PatternOptions{Syntax: uniz.ExtendedRegexp, IgnoreCase: true})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	n, err := uniz.Grep(context.Background(), &out, strings.NewReader("XXY\nz\nxy"), m, uniz.GrepOptions{LineNumbers: true})
	if err != nil || n != 2 || out.String() != "1:XXY\n3:xy\n" {
		t.Fatalf("%d %q %v", n, out.String(), err)
	}
	if !m.Match([]byte("axyb")) || m.Match([]byte("y")) {
		t.Fatal("Match")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := uniz.Grep(ctx, &out, strings.NewReader("x"), m, uniz.GrepOptions{}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestFind(t *testing.T) {
	t.Chdir(t.TempDir())
	write(t, "a.go", "package a\n")
	write(t, "b.txt", "")
	write(t, "sub/c.go", "package c\n")
	write(t, "sub/deep/d.go", strings.Repeat("x", 2000))
	if err := os.Mkdir("empty", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("a.go", "link"); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-72 * time.Hour)
	if err := os.Chtimes("a.go", old, old); err != nil {
		t.Fatal(err)
	}
	all := ".\n./a.go\n./b.txt\n./empty\n./link\n./sub\n./sub/c.go\n./sub/deep\n./sub/deep/d.go\n"
	runCases(t, []commandCase{
		{"all", []string{"find"}, "", all, 0},
		{"dot", []string{"find", "."}, "", all, 0},
		{"name", []string{"find", ".", "-name", "*.go"}, "", "./a.go\n./sub/c.go\n./sub/deep/d.go\n", 0},
		{"iname", []string{"find", ".", "-iname", "A.GO"}, "", "./a.go\n", 0},
		{"path", []string{"find", ".", "-path", "./sub/*", "-type", "f"}, "", "./sub/c.go\n./sub/deep/d.go\n", 0},
		{"type d", []string{"find", ".", "-type", "d"}, "", ".\n./empty\n./sub\n./sub/deep\n", 0},
		{"type l", []string{"find", ".", "-type", "l"}, "", "./link\n", 0},
		{"type list", []string{"find", ".", "-maxdepth", "1", "-type", "l,d"}, "", ".\n./empty\n./link\n./sub\n", 0},
		{"maxdepth", []string{"find", ".", "-maxdepth", "1", "-type", "f"}, "", "./a.go\n./b.txt\n", 0},
		{"mindepth", []string{"find", ".", "-mindepth", "2", "-type", "f"}, "", "./sub/c.go\n./sub/deep/d.go\n", 0},
		{"prune", []string{"find", ".", "-name", "sub", "-prune", "-o", "-type", "f", "-print"}, "", "./a.go\n./b.txt\n", 0},
		{"empty", []string{"find", ".", "-empty"}, "", "./b.txt\n./empty\n", 0},
		{"size", []string{"find", ".", "-type", "f", "-size", "+1k"}, "", "./sub/deep/d.go\n", 0},
		{"size bytes", []string{"find", ".", "-type", "f", "-size", "-1c"}, "", "./b.txt\n", 0},
		{"mtime", []string{"find", ".", "-type", "f", "-mtime", "+1"}, "", "./a.go\n", 0},
		{"mmin", []string{"find", ".", "-type", "f", "-mmin", "-60", "-name", "*.go"}, "", "./sub/c.go\n./sub/deep/d.go\n", 0},
		{"newer", []string{"find", ".", "-maxdepth", "1", "-type", "f", "-newer", "a.go"}, "", "./b.txt\n", 0},
		{"not", []string{"find", ".", "-maxdepth", "1", "!", "-type", "d", "-not", "-type", "l"}, "", "./a.go\n./b.txt\n", 0},
		{"or precedence", []string{"find", ".", "-type", "f", "-name", "*.go", "-o", "-name", "b.txt"}, "", "./a.go\n./b.txt\n./sub/c.go\n./sub/deep/d.go\n", 0},
		{"parens", []string{"find", ".", "-type", "f", "(", "-name", "a.go", "-o", "-name", "c.go", ")"}, "", "./a.go\n./sub/c.go\n", 0},
		{"quit", []string{"find", ".", "-name", "*.go", "-print", "-quit"}, "", "./a.go\n", 0},
		{"action suppresses default", []string{"find", ".", "-name", "*.go", "-quit"}, "", "", 0},
		{"print0", []string{"find", "sub", "-print0"}, "", "sub\x00sub/c.go\x00sub/deep\x00sub/deep/d.go\x00", 0},
		{"trailing slash", []string{"find", "sub/", "-maxdepth", "1"}, "", "sub/\nsub/c.go\nsub/deep\n", 0},
		{"several roots", []string{"find", "b.txt", "empty"}, "", "b.txt\nempty\n", 0},
		{"symlink root not followed", []string{"find", "link", "-type", "l"}, "", "link\n", 0},
		{"missing root", []string{"find", "missing", "b.txt"}, "", "b.txt\n", 1},
		{"unknown predicate", []string{"find", ".", "-bogus"}, "", "", 2},
		{"missing argument", []string{"find", ".", "-name"}, "", "", 2},
		{"bad type", []string{"find", ".", "-type", "q"}, "", "", 2},
		{"unbalanced", []string{"find", ".", "(", "-true"}, "", "", 2},
		{"newer missing", []string{"find", ".", "-newer", "missing"}, "", "", 2},
	})
	paths, err := uniz.FindPaths(context.Background(), []string{"sub"}, []string{"-type", "f"})
	if err != nil || strings.Join(paths, ",") != filepath.Join("sub", "c.go")+","+filepath.Join("sub", "deep", "d.go") {
		t.Fatal(paths, err)
	}
	if _, err := uniz.FindPaths(context.Background(), []string{"."}, []string{"("}); err == nil {
		t.Fatal("bad expression accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := uniz.FindPaths(ctx, []string{"."}, nil); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func perm(t *testing.T, path string) fs.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode().Perm()
}

func TestChmod(t *testing.T) {
	needShell(t) // permission bits are not meaningful on Windows
	t.Chdir(t.TempDir())
	write(t, "f", "")
	steps := []struct {
		args []string
		want fs.FileMode
	}{
		{[]string{"600", "f"}, 0o600},
		{[]string{"644", "f"}, 0o644},
		{[]string{"u+x,g-r", "f"}, 0o704},
		{[]string{"a=r", "f"}, 0o444},
		{[]string{"u+w", "f"}, 0o644},
		{[]string{"-w", "f"}, 0o444},
		{[]string{"u=rw,g=u", "f"}, 0o664},
		{[]string{"o=", "f"}, 0o660},
		{[]string{"+X", "f"}, 0o660},
		{[]string{"--", "a+rx", "f"}, 0o775},
		{[]string{"go-w+r", "f"}, 0o755},
	}
	for _, s := range steps {
		if _, err := run(t, "", append([]string{"chmod"}, s.args...)...); err != nil {
			t.Fatalf("%v: %v", s.args, err)
		}
		if got := perm(t, "f"); got != s.want {
			t.Fatalf("%v: %o, want %o", s.args, got, s.want)
		}
	}

	write(t, "outside/secret", "")
	if err := os.Chmod("outside/secret", 0o600); err != nil {
		t.Fatal(err)
	}
	write(t, "tree/file", "")
	if err := os.Mkdir("tree/dir", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../outside/secret", "tree/link"); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, "", "chmod", "-R", "a+rX", "tree"); err != nil {
		t.Fatal(err)
	}
	if perm(t, "tree/file") != 0o644 || perm(t, "tree/dir") != 0o755 || perm(t, "outside/secret") != 0o600 {
		t.Fatalf("recursive: %o %o %o", perm(t, "tree/file"), perm(t, "tree/dir"), perm(t, "outside/secret"))
	}
	if _, err := run(t, "", "chmod", "-R", "go-rx", "tree"); err != nil {
		t.Fatal(err)
	}
	if perm(t, "tree") != 0o700 || perm(t, "tree/dir") != 0o700 {
		t.Fatal("recursive removal")
	}

	runCases(t, []commandCase{
		{"bad mode", []string{"chmod", "u+q", "f"}, "", "", 2},
		{"bad octal", []string{"chmod", "999", "f"}, "", "", 2},
		{"too large", []string{"chmod", "17777", "f"}, "", "", 2},
		{"missing operand", []string{"chmod", "644"}, "", "", 2},
		{"unknown option", []string{"chmod", "-Z", "f"}, "", "", 2},
		{"missing file continues", []string{"chmod", "600", "missing", "f"}, "", "", 1},
	})
	if perm(t, "f") != 0o600 {
		t.Fatal("chmod did not continue past a missing file")
	}

	for spec, want := range map[string][2]fs.FileMode{
		"4755":   {0o644, fs.ModeSetuid | 0o755},
		"u+s":    {0o755, fs.ModeSetuid | 0o755},
		"g+s,+t": {0o755, fs.ModeSetgid | fs.ModeSticky | 0o755},
		"0755":   {fs.ModeSetuid | 0o644, 0o755},
		"a-x":    {0o755, 0o644},
		"u=g":    {0o750, 0o550},
	} {
		change, err := uniz.ParseMode(spec)
		if err != nil {
			t.Fatal(spec, err)
		}
		if got := change(want[0], false); got != want[1] {
			t.Fatalf("%s: %v, want %v", spec, got, want[1])
		}
	}
	x, _ := uniz.ParseMode("+X")
	if x(0o644, true) != 0o755 || x(0o644, false) != 0o644 || x(0o744, false) != 0o755 {
		t.Fatal("+X")
	}
}

func TestMktemp(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.Mkdir("d", 0o755); err != nil {
		t.Fatal(err)
	}
	out, err := run(t, "", "mktemp", "-p", "d")
	path := strings.TrimSuffix(out, "\n")
	if err != nil || filepath.Dir(path) != "d" || !strings.HasPrefix(filepath.Base(path), "tmp.") {
		t.Fatalf("%q %v", out, err)
	}
	if info, err := os.Lstat(path); err != nil || !info.Mode().IsRegular() || runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatal(info, err)
	}
	out, err = run(t, "", "mktemp", "-d", "-p", "d", "work.XXXX")
	path = strings.TrimSuffix(out, "\n")
	if info, serr := os.Lstat(path); err != nil || serr != nil || !info.IsDir() || !strings.HasPrefix(filepath.Base(path), "work.") || len(filepath.Base(path)) != 9 {
		t.Fatalf("%q %v %v", out, err, serr)
	}
	out, err = run(t, "", "mktemp", "local.XXXXX")
	if err != nil || !exists(strings.TrimSuffix(out, "\n")) || strings.ContainsRune(out, filepath.Separator) {
		t.Fatalf("%q %v", out, err)
	}
	runCases(t, []commandCase{
		{"short template", []string{"mktemp", "fooXX"}, "", "", 1},
		{"directory in template with -p", []string{"mktemp", "-p", "d", "a/XXX"}, "", "", 2},
		{"two templates", []string{"mktemp", "aXXX", "bXXX"}, "", "", 2},
		{"missing dir", []string{"mktemp", "-p", "missing"}, "", "", 1},
	})
	seen := map[string]bool{}
	for range 50 {
		p, err := uniz.MakeTemp("d", "u.XXX", false)
		if err != nil || seen[p] {
			t.Fatal(p, err)
		}
		seen[p] = true
	}
}

func size(t *testing.T, path string) int64 {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Size()
}

func TestTruncate(t *testing.T) {
	t.Chdir(t.TempDir())
	steps := []struct {
		size string
		want int64
	}{
		{"10", 10}, {"+5", 15}, {"-20", 0}, {"1K", 1024}, {"%1000", 2000}, {"/3", 1998},
		{"<100", 100}, {">200", 200}, {"<500", 200}, {"1KB", 1000}, {"1KiB", 1024}, {"0", 0},
	}
	for _, s := range steps {
		if _, err := run(t, "", "truncate", "-s", s.size, "f"); err != nil {
			t.Fatalf("%s: %v", s.size, err)
		}
		if got := size(t, "f"); got != s.want {
			t.Fatalf("%s: %d, want %d", s.size, got, s.want)
		}
	}
	write(t, "data", "hello world")
	if _, err := run(t, "", "truncate", "-s", "5", "--", "data"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile("data"); string(b) != "hello" {
		t.Fatalf("%q", b)
	}
	if _, err := run(t, "", "truncate", "-s", "+3", "data"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile("data"); string(b) != "hello\x00\x00\x00" {
		t.Fatalf("%q", b)
	}
	runCases(t, []commandCase{
		{"no create", []string{"truncate", "-c", "-s", "1", "nope"}, "", "", 0},
		{"no size", []string{"truncate", "f"}, "", "", 2},
		{"bad size", []string{"truncate", "-s", "1X", "f"}, "", "", 2},
		{"divide by zero", []string{"truncate", "-s", "%0", "f"}, "", "", 2},
		{"overflow", []string{"truncate", "-s", "9E", "f"}, "", "", 2},
		{"no file", []string{"truncate", "-s", "1"}, "", "", 2},
		{"directory", []string{"truncate", "-s", "1", "."}, "", "", 1},
	})
	if exists("nope") {
		t.Fatal("-c created a file")
	}
	if c, err := uniz.ParseSize("+1M"); err != nil || c.Apply(1) != 1+1<<20 {
		t.Fatal(c, err)
	}
}

func TestLinkUnlink(t *testing.T) {
	t.Chdir(t.TempDir())
	write(t, "f", "x")
	if err := os.Mkdir("d", 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, "", "link", "f", "g"); err != nil {
		t.Fatal(err)
	}
	a, _ := os.Stat("f")
	b, _ := os.Stat("g")
	if !os.SameFile(a, b) {
		t.Fatal("link did not create a hard link")
	}
	if err := os.Symlink("f", "sym"); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, "", "unlink", "sym"); err != nil || exists("sym") || !exists("f") {
		t.Fatal(err)
	}
	if _, err := run(t, "", "unlink", "g"); err != nil || exists("g") {
		t.Fatal(err)
	}
	runCases(t, []commandCase{
		{"link one operand", []string{"link", "f"}, "", "", 2},
		{"link existing", []string{"link", "f", "f"}, "", "", 1},
		{"link missing", []string{"link", "missing", "x"}, "", "", 1},
		{"unlink directory", []string{"unlink", "d"}, "", "", 1},
		{"unlink two", []string{"unlink", "f", "d"}, "", "", 2},
		{"unlink missing", []string{"unlink", "missing"}, "", "", 1},
	})
	if !exists("d") || !exists("f") {
		t.Fatal("refused operands were removed")
	}
	if err := uniz.Unlink("d"); !errors.Is(err, uniz.ErrIsDirectory) {
		t.Fatal(err)
	}
}

func TestSystemInfo(t *testing.T) {
	out, err := run(t, "", "nproc")
	if err != nil || out != strconv.Itoa(runtime.NumCPU())+"\n" {
		t.Fatalf("%q %v", out, err)
	}
	info, err := uniz.Uname()
	if err != nil || info.Sysname == "" || info.Machine == "" {
		t.Fatal(info, err)
	}
	for args, want := range map[string]string{
		"":    info.Sysname,
		"-s":  info.Sysname,
		"-m":  info.Machine,
		"-sm": info.Sysname + " " + info.Machine,
		"-a":  strings.Join([]string{info.Sysname, info.Nodename, info.Release, info.Version, info.Machine}, " "),
	} {
		argv := []string{"uname"}
		if args != "" {
			argv = append(argv, args)
		}
		if out, err := run(t, "", argv...); err != nil || out != want+"\n" {
			t.Fatalf("uname %s: %q %v", args, out, err)
		}
	}
	runCases(t, []commandCase{
		{"nproc operand", []string{"nproc", "x"}, "", "", 2},
		{"uname operand", []string{"uname", "x"}, "", "", 2},
		{"id -n alone", []string{"id", "-n"}, "", "", 2},
		{"id two modes", []string{"id", "-u", "-g"}, "", "", 2},
		{"id unknown user", []string{"id", "no-such-user-uniz"}, "", "", 1},
		{"groups extra", []string{"groups", "a", "b"}, "", "", 2},
	})
	if runtime.GOOS == "windows" {
		return
	}
	if out, err := run(t, "", "id", "-u"); err != nil || out != strconv.Itoa(os.Getuid())+"\n" {
		t.Fatalf("%q %v", out, err)
	}
	u, err := user.Current()
	if err != nil {
		t.Skip(err)
	}
	if out, err := run(t, "", "id", "-un"); err != nil || out != u.Username+"\n" {
		t.Fatalf("%q %v", out, err)
	}
	if out, err := run(t, "", "id", "-g", u.Username); err != nil || out != u.Gid+"\n" {
		t.Fatalf("%q %v", out, err)
	}
	if _, err := u.GroupIds(); err != nil {
		t.Skip("group database unavailable:", err)
	}
	out, err = run(t, "", "id")
	if err != nil || !strings.HasPrefix(out, "uid="+u.Uid+"("+u.Username+") gid="+u.Gid+"(") || !strings.Contains(out, " groups="+u.Gid+"(") {
		t.Fatalf("%q %v", out, err)
	}
	groups, err := run(t, "", "groups")
	gids, _ := run(t, "", "id", "-Gn")
	if err != nil || groups != gids || groups == "\n" {
		t.Fatalf("%q %q %v", groups, gids, err)
	}
}

// runStreams runs a command and returns stdout and stderr separately.
func runStreams(ctx context.Context, input string, args ...string) (string, string, error) {
	var out, stderr bytes.Buffer
	err := uniz.New(uniz.Config{Stdin: strings.NewReader(input), Stdout: &out, Stderr: &stderr}).Run(ctx, args)
	return out.String(), stderr.String(), err
}

func TestEnv(t *testing.T) {
	t.Setenv("UNIZ_TEST_VAR", "present")
	out, err := run(t, "", "env")
	if err != nil || !strings.Contains(out, "\nUNIZ_TEST_VAR=present\n") && !strings.HasPrefix(out, "UNIZ_TEST_VAR=present\n") {
		t.Fatalf("env: %v", err)
	}
	for _, args := range [][]string{{"env", "-u", "UNIZ_TEST_VAR"}, {"env", "-uUNIZ_TEST_VAR"}} {
		if out, err := run(t, "", args...); err != nil || strings.Contains(out, "UNIZ_TEST_VAR=") || !strings.Contains(out, "=") {
			t.Fatal(args, err)
		}
	}
	runCases(t, []commandCase{
		{"empty", []string{"env", "-i"}, "", "", 0},
		{"set", []string{"env", "-i", "A=1", "B=2"}, "", "A=1\nB=2\n", 0},
		{"override", []string{"env", "-", "A=1", "A=2"}, "", "A=2\n", 0},
		{"unset joined", []string{"env", "-i", "-uA", "B=1"}, "", "B=1\n", 0},
		{"options end at first assignment", []string{"env", "-i", "A=1", "-uA"}, "", "", 127},
		{"unset missing value", []string{"env", "-u"}, "", "", 2},
		{"unknown option", []string{"env", "-x"}, "", "", 2},
		{"not found", []string{"env", "no-such-program-uniz"}, "", "", 127},
	})
	needShell(t)
	runCases(t, []commandCase{
		{"run with env", []string{"env", "-i", "A=hi", "/bin/sh", "-c", `echo "$A"`}, "", "hi\n", 0},
		{"stdin passes through", []string{"env", "/bin/sh", "-c", `read x; echo "got $x"`}, "line\n", "got line\n", 0},
		{"exit status", []string{"env", "/bin/sh", "-c", "exit 3"}, "", "", 3},
		{"signal", []string{"env", "/bin/sh", "-c", "kill -TERM $$"}, "", "", 143},
		{"path search uses new PATH", []string{"env", "-i", "PATH=/nonexistent", "sh", "-c", "true"}, "", "", 127},
		{"dash dash", []string{"env", "-i", "--", "/bin/sh", "-c", "echo ok"}, "", "ok\n", 0},
	})
	_, stderr, err := runStreams(context.Background(), "", "env", "/bin/sh", "-c", "echo oops >&2")
	if err != nil || stderr != "oops\n" {
		t.Fatalf("%q %v", stderr, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, _, err := runStreams(ctx, "", "env", "/bin/sleep", "10"); !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > 5*time.Second {
		t.Fatalf("env ignored cancellation: %v after %v", err, time.Since(start))
	}
}

func TestXargs(t *testing.T) {
	runCases(t, []commandCase{
		{"default echo", []string{"xargs"}, "a b\nc", "a b c\n", 0},
		{"-n", []string{"xargs", "-n", "2"}, "a b c", "a b\nc\n", 0},
		{"quotes", []string{"xargs", "-n1"}, `"a b" 'c d' e\ f`, "a b\nc d\ne f\n", 0},
		{"nul", []string{"xargs", "-0", "-n1"}, "a b\x00c\x00", "a b\nc\n", 0},
		{"empty runs once", []string{"xargs"}, "", "\n", 0},
		{"empty -r", []string{"xargs", "-r"}, "  \n", "", 0},
		{"unmatched quote", []string{"xargs"}, `"abc`, "", 1},
		{"bad -n", []string{"xargs", "-n", "0"}, "", "", 2},
		{"unknown option", []string{"xargs", "-z"}, "", "", 2},
		{"not found", []string{"xargs", "no-such-program-uniz"}, "a", "", 127},
	})
	out, err := run(t, strings.Repeat("x ", 100000), "xargs")
	if err != nil || strings.Count(out, "x") != 100000 || strings.Count(out, "\n") < 2 {
		t.Fatalf("batching: %d lines, %v", strings.Count(out, "\n"), err)
	}
	_, stderr, err := runStreams(context.Background(), "a b", "xargs", "-t")
	if err != nil || stderr != "echo a b\n" {
		t.Fatalf("%q %v", stderr, err)
	}
	needShell(t)
	runCases(t, []commandCase{
		{"program", []string{"xargs", "/bin/sh", "-c", `echo "$#:$*"`, "sh"}, "a b", "2:a b\n", 0},
		{"replace", []string{"xargs", "-I", "{}", "/bin/sh", "-c", "echo '<{}>'"}, "a b\n  c\n\n", "<a b>\n<c>\n", 0},
		{"status 1", []string{"xargs", "/bin/sh", "-c", "exit 1"}, "x", "", 123},
		{"status 255 stops", []string{"xargs", "-n1", "/bin/sh", "-c", `echo "$0"; exit 255`}, "a b", "a\n", 124},
		{"signal", []string{"xargs", "/bin/sh", "-c", "kill -TERM $$"}, "x", "", 125},
	})
}

func TestTimeout(t *testing.T) {
	runCases(t, []commandCase{
		{"bad duration", []string{"timeout", "x", "true"}, "", "", 2},
		{"missing program", []string{"timeout", "5"}, "", "", 2},
		{"bad -k", []string{"timeout", "-k", "-1", "5", "true"}, "", "", 2},
		{"not found", []string{"timeout", "5", "no-such-program-uniz"}, "", "", 127},
	})
	needShell(t)
	runCases(t, []commandCase{
		{"status", []string{"timeout", "5", "/bin/sh", "-c", "echo hi; exit 3"}, "", "hi\n", 3},
		{"zero disables", []string{"timeout", "0", "/bin/sh", "-c", "exit 0"}, "", "", 0},
		{"stdin", []string{"timeout", "5", "/bin/sh", "-c", "read x; echo $x"}, "in\n", "in\n", 0},
	})
	start := time.Now()
	if _, err := run(t, "", "timeout", "0.2", "/bin/sleep", "10"); uniz.ExitCode(err) != 124 || time.Since(start) > 5*time.Second {
		t.Fatalf("%v after %v", err, time.Since(start))
	}
	start = time.Now()
	_, err := run(t, "", "timeout", "0.2", "/bin/sh", "-c", `trap "exit 0" TERM; /bin/sleep 10 >/dev/null 2>&1 & wait`)
	if uniz.ExitCode(err) != 124 || time.Since(start) > 5*time.Second {
		t.Fatalf("handled TERM: %v after %v", err, time.Since(start))
	}
	start = time.Now()
	_, err = run(t, "", "timeout", "-k", "0.2", "0.2", "/bin/sh", "-c", `trap "" TERM; /bin/sleep 10 >/dev/null 2>&1`)
	if uniz.ExitCode(err) != 124 || time.Since(start) > 5*time.Second {
		t.Fatalf("-k: %v after %v", err, time.Since(start))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start = time.Now()
	if _, _, err := runStreams(ctx, "", "timeout", "10", "/bin/sleep", "10"); !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > 5*time.Second {
		t.Fatalf("caller cancellation: %v after %v", err, time.Since(start))
	}
}
