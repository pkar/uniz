package uniz_test

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pkar/uniz"
)

func TestExpr(t *testing.T) {
	runCases(t, []commandCase{
		{"add", []string{"expr", "2", "+", "3"}, "", "5\n", 0},
		{"precedence", []string{"expr", "2", "+", "3", "*", "4"}, "", "14\n", 0},
		{"parens", []string{"expr", "(", "2", "+", "3", ")", "*", "4"}, "", "20\n", 0},
		{"negative", []string{"expr", "-5", "/", "2"}, "", "-2\n", 0},
		{"mod", []string{"expr", "7", "%", "3"}, "", "1\n", 0},
		{"zero is false", []string{"expr", "3", "-", "3"}, "", "0\n", 1},
		{"empty is false", []string{"expr", ""}, "", "\n", 1},
		{"numeric compare", []string{"expr", "10", ">", "9"}, "", "1\n", 0},
		{"string compare", []string{"expr", "10", ">", "9a"}, "", "0\n", 1},
		{"equal", []string{"expr", "a", "=", "a"}, "", "1\n", 0},
		{"or", []string{"expr", "", "|", "b"}, "", "b\n", 0},
		{"or both null", []string{"expr", "0", "|", ""}, "", "0\n", 1},
		{"and", []string{"expr", "a", "&", "b"}, "", "a\n", 0},
		{"and null", []string{"expr", "a", "&", "0"}, "", "0\n", 1},
		{"match length", []string{"expr", "abcd", ":", "ab*c"}, "", "3\n", 0},
		{"match anchored", []string{"expr", "xabc", ":", "abc"}, "", "0\n", 1},
		{"match group", []string{"expr", "file.txt", ":", `\(.*\)\.txt`}, "", "file\n", 0},
		{"match group miss", []string{"expr", "file", ":", `\(.*\)\.txt`}, "", "\n", 1},
		{"match keyword", []string{"expr", "match", "abc", "a."}, "", "2\n", 0},
		{"length", []string{"expr", "length", "héllo"}, "", "5\n", 0},
		{"substr", []string{"expr", "substr", "hello", "2", "3"}, "", "ell\n", 0},
		{"substr past end", []string{"expr", "substr", "hi", "5", "1"}, "", "\n", 1},
		{"index", []string{"expr", "index", "hello", "lo"}, "", "3\n", 0},
		{"plus token", []string{"expr", "+", "length"}, "", "length\n", 0},
		{"leading --", []string{"expr", "--", "-1", "+", "1"}, "", "0\n", 1},
		{"non-integer", []string{"expr", "a", "+", "1"}, "", "", 2},
		{"divide by zero", []string{"expr", "1", "/", "0"}, "", "", 2},
		{"overflow", []string{"expr", "9223372036854775807", "+", "1"}, "", "", 2},
		{"missing", []string{"expr", "1", "+"}, "", "", 2},
		{"extra", []string{"expr", "1", "2"}, "", "", 2},
		{"unclosed", []string{"expr", "(", "1"}, "", "", 2},
		{"none", []string{"expr"}, "", "", 2},
		{"bad regexp", []string{"expr", "a", ":", `\(`}, "", "", 2},
	})
	if _, err := uniz.Expr([]string{")"}); !errors.Is(err, uniz.ErrExprSyntax) {
		t.Fatal(err)
	}
	if !uniz.ExprNull("0") || !uniz.ExprNull("-0") || uniz.ExprNull("00a") {
		t.Fatal("ExprNull")
	}
}

func TestPathchkLognameTty(t *testing.T) {
	t.Chdir(t.TempDir())
	write(t, "file", "")
	long := strings.Repeat("a", 15)
	runCases(t, []commandCase{
		{"ok", []string{"pathchk", "a/b", long}, "", "", 0},
		{"file as dir", []string{"pathchk", "file/x"}, "", "", 1},
		{"portable name length", []string{"pathchk", "-p", long}, "", "", 1},
		{"portable chars", []string{"pathchk", "-p", "a b"}, "", "", 1},
		{"portable ok", []string{"pathchk", "-p", "a/b_c.d-e"}, "", "", 0},
		{"dash", []string{"pathchk", "-P", "--", "a/-b"}, "", "", 1},
		{"empty", []string{"pathchk", ""}, "", "", 1},
		{"too long", []string{"pathchk", strings.Repeat("a/", 2048)}, "", "", 1},
		{"long component", []string{"pathchk", strings.Repeat("a", 256)}, "", "", 1},
		{"missing", []string{"pathchk"}, "", "", 2},
		{"tty pipe", []string{"tty"}, "", "not a tty\n", 1},
		{"tty -s", []string{"tty", "-s"}, "", "", 1},
		{"tty extra", []string{"tty", "x"}, "", "", 2},
	})
	t.Setenv("LOGNAME", "someone")
	if out, err := run(t, "", "logname"); out != "someone\n" || err != nil {
		t.Fatalf("%q %v", out, err)
	}
	t.Setenv("LOGNAME", "")
	if _, err := run(t, "", "logname"); err == nil {
		t.Fatal("logname without LOGNAME should fail")
	}
	if err := uniz.CheckPath("x", uniz.PathCheckOptions{Portable: true}); err != nil {
		t.Fatal(err)
	}
}

func TestStat(t *testing.T) {
	t.Chdir(t.TempDir())
	write(t, "f", "hello")
	if err := os.Chmod("f", 0o640); err != nil {
		t.Fatal(err)
	}
	mtime := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := os.Chtimes("f", mtime, mtime); err != nil {
		t.Fatal(err)
	}
	os.Mkdir("d", 0o755)
	out, err := run(t, "", "stat", "-c", "%n|%s|%F|%Y|%5s|%-3s|%05s|%%|%q", "f", "d")
	if err != nil || !strings.HasPrefix(out, "f|5|regular file|1577934245|    5|5  |00005|%|%q\nd|") || !strings.Contains(out, "|directory|") {
		t.Fatalf("%q %v", out, err)
	}
	if runtime.GOOS != "windows" {
		out, err = run(t, "", "stat", "-c", "%a %A %f", "f")
		if err != nil || out != "640 -rw-r----- 81a0\n" {
			t.Fatalf("%q %v", out, err)
		}
		if err := os.Symlink("f", "l"); err != nil {
			t.Fatal(err)
		}
		out, _ = run(t, "", "stat", "-c", "%N %F", "l")
		if out != "\"l\" -> \"f\" symbolic link\n" {
			t.Fatalf("%q", out)
		}
		out, _ = run(t, "", "stat", "-L", "-c", "%s", "l")
		if out != "5\n" {
			t.Fatalf("%q", out)
		}
		out, _ = run(t, "", "stat", "-c", "%h %u", "f")
		if out != "1 "+strconv.Itoa(os.Getuid())+"\n" {
			t.Fatalf("%q", out)
		}
	}
	out, err = run(t, "", "stat", "f")
	if err != nil || !strings.Contains(out, `File: "f"`) || !strings.Contains(out, "Size: 5") || !strings.Contains(out, "Modify: 2020-01-0") {
		t.Fatalf("%q %v", out, err)
	}
	if out, err := run(t, "", "stat", "missing", "f"); err == nil || !strings.Contains(out, "File: \"f\"") {
		t.Fatalf("missing operand should fail but still print f: %v", err)
	}
	if _, err := run(t, "", "stat"); uniz.ExitCode(err) != 2 {
		t.Fatal(err)
	}
	for m, want := range map[fs.FileMode]string{0o755 | fs.ModeDir: "drwxr-xr-x", 0o4755: "-rwsr-xr-x", 0o1644 | fs.ModeDir | fs.ModeSticky: "drw-r--r-T", fs.ModeNamedPipe | 0o600: "prw-------"} {
		if m&0o1000 != 0 {
			m = m&^0o1000 | fs.ModeSticky
		}
		if m&0o4000 != 0 {
			m = m&^0o4000 | fs.ModeSetuid
		}
		if got := uniz.FileModeString(m); got != want {
			t.Errorf("%v: %q want %q", m, got, want)
		}
	}
}

func TestDu(t *testing.T) {
	t.Chdir(t.TempDir())
	write(t, "d/a", strings.Repeat("x", 3000))
	write(t, "d/sub/b", strings.Repeat("y", 100))
	out, err := run(t, "", "du", "-b", "d")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 || !strings.HasSuffix(lines[0], "\t"+filepath.Join("d", "sub")) || !strings.HasSuffix(lines[1], "\td") {
		t.Fatalf("%q", out)
	}
	dirTotal, _ := strconv.Atoi(strings.Fields(lines[1])[0])
	if dirTotal < 3100 {
		t.Fatalf("apparent total %d too small", dirTotal)
	}
	out, _ = run(t, "", "du", "-a", "-b", "d")
	if !strings.Contains(out, "3000\t"+filepath.Join("d", "a")+"\n") || !strings.Contains(out, "100\t"+filepath.Join("d", "sub", "b")) {
		t.Fatalf("%q", out)
	}
	out, _ = run(t, "", "du", "-s", "-c", "d", "d/sub")
	if lines := strings.Split(strings.TrimSpace(out), "\n"); len(lines) != 3 || !strings.HasSuffix(lines[2], "\ttotal") {
		t.Fatalf("%q", out)
	}
	out, _ = run(t, "", "du", "-d", "0", "-h", "d")
	if !strings.HasSuffix(out, "\td\n") || strings.Count(out, "\n") != 1 {
		t.Fatalf("%q", out)
	}
	if runtime.GOOS != "windows" {
		os.Symlink("/", "d/root")
		if err := os.Link("d/a", "d/hard"); err != nil {
			t.Fatal(err)
		}
		out, _ = run(t, "", "du", "-s", "-b", "d")
		if n, _ := strconv.Atoi(strings.Fields(out)[0]); n > dirTotal+200 {
			t.Fatalf("hard link or symlink counted: %d vs %d", n, dirTotal)
		}
	}
	for _, bad := range [][]string{{"du", "-s", "-a"}, {"du", "-d", "x"}} {
		if _, err := run(t, "", bad...); uniz.ExitCode(err) != 2 {
			t.Fatalf("%v: %v", bad, err)
		}
	}
	if _, err := run(t, "", "du", "missing"); uniz.ExitCode(err) != 1 {
		t.Fatal(err)
	}
	var seen []string
	total, err := uniz.DiskUsage(context.Background(), "d/sub", uniz.DuOptions{All: true, MaxDepth: -1, Apparent: true}, func(e uniz.DuEntry) error {
		seen = append(seen, e.Path)
		return nil
	})
	if err != nil || total < 100 || len(seen) != 2 {
		t.Fatalf("%d %v %v", total, seen, err)
	}
}

func TestDf(t *testing.T) {
	if runtime.GOOS == "windows" {
		if _, err := run(t, "", "df"); err == nil {
			t.Fatal("df should be unsupported")
		}
		return
	}
	out, err := run(t, "", "df", "-P", ".")
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if err != nil || len(lines) != 2 || !strings.HasPrefix(lines[0], "Filesystem") || !strings.HasSuffix(lines[0], "Capacity Mounted on") {
		t.Fatalf("%q %v", out, err)
	}
	if out, err := run(t, "", "df", "-hT"); err != nil || !strings.Contains(out, "Type") || strings.Count(out, "\n") < 2 {
		t.Fatalf("%q %v", out, err)
	}
	u, err := uniz.DiskFree(".")
	if err != nil || u.Total == 0 || u.Avail > u.Total || u.Used() > u.Total {
		t.Fatalf("%+v %v", u, err)
	}
	if _, err := run(t, "", "df", "no/such/path"); err == nil {
		t.Fatal("missing path should fail")
	}
}

func TestInstallShredSyncMkfifo(t *testing.T) {
	t.Chdir(t.TempDir())
	write(t, "src", "payload")
	if _, err := run(t, "", "install", "-m", "640", "src", "dst"); err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Stat("dst"); read(t, "dst") != "payload" || runtime.GOOS != "windows" && info.Mode().Perm() != 0o640 {
		t.Fatalf("install mode %v", info.Mode())
	}
	if _, err := run(t, "", "install", "-D", "src", "a/b/c"); err != nil || read(t, "a/b/c") != "payload" {
		t.Fatal(err)
	}
	os.Mkdir("dir", 0o755)
	write(t, "src2", "two")
	if _, err := run(t, "", "install", "src", "src2", "dir"); err != nil || read(t, "dir/src2") != "two" {
		t.Fatal(err)
	}
	if _, err := run(t, "", "install", "-d", "-m", "u=rwx", "x/y"); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat("x/y"); err != nil || !info.IsDir() || runtime.GOOS != "windows" && info.Mode().Perm() != 0o700 {
		t.Fatalf("install -d: %v %v", info, err)
	}
	for _, bad := range [][]string{{"install", "src"}, {"install", "src", "src2", "dst"}, {"install", "-m", "q", "src", "z"}} {
		if _, err := run(t, "", bad...); uniz.ExitCode(err) != 2 {
			t.Fatalf("%v: %v", bad, err)
		}
	}
	for _, bad := range [][]string{{"install", "src", "src"}, {"install", "dir", "q"}, {"install", "missing", "q"}} {
		if _, err := run(t, "", bad...); uniz.ExitCode(err) != 1 {
			t.Fatalf("%v: %v", bad, err)
		}
	}
	if read(t, "src") != "payload" {
		t.Fatal("install onto itself damaged the source")
	}
	for _, dst := range []string{"..", ".", "dir/.."} {
		if err := uniz.Install(context.Background(), "src", dst, uniz.InstallOptions{}); err == nil {
			t.Fatalf("Install to %q should be refused", dst)
		}
	}
	if runtime.GOOS != "windows" {
		write(t, "target", "keep")
		os.Symlink("target", "link")
		if _, err := run(t, "", "install", "src", "link"); err != nil {
			t.Fatal(err)
		}
		if info, _ := os.Lstat("link"); info.Mode()&fs.ModeSymlink != 0 || read(t, "target") != "keep" {
			t.Fatal("install followed a symlink")
		}
		os.Symlink("target", "link2")
		if _, err := run(t, "", "shred", "link2"); err == nil || read(t, "target") != "keep" {
			t.Fatalf("shred followed a symlink: %v", err)
		}
	}
	write(t, "secret", "top secret data")
	if _, err := run(t, "", "shred", "-n", "1", "-z", "secret"); err != nil {
		t.Fatal(err)
	}
	if got := read(t, "secret"); got != strings.Repeat("\x00", 15) {
		t.Fatalf("%q", got)
	}
	if _, err := run(t, "", "shred", "-u", "secret"); err != nil || exists("secret") {
		t.Fatalf("shred -u: %v", err)
	}
	for _, bad := range []string{"dir", ".", "..", "/", "missing"} {
		if _, err := run(t, "", "shred", bad); err == nil {
			t.Fatalf("shred %s should fail", bad)
		}
	}
	write(t, "r", "abcd")
	if err := uniz.Shred(context.Background(), "r", uniz.ShredOptions{Passes: 2, Random: bytes.NewReader([]byte("12345678"))}); err != nil || read(t, "r") != "5678" {
		t.Fatalf("%q %v", read(t, "r"), err)
	}
	if _, err := run(t, "", "sync", "src"); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, "", "sync", "missing"); err == nil {
		t.Fatal("sync missing should fail")
	}
	if runtime.GOOS == "windows" {
		if _, err := run(t, "", "mkfifo", "p"); err == nil {
			t.Fatal("mkfifo should be unsupported")
		}
		return
	}
	if _, err := run(t, "", "sync"); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, "", "mkfifo", "-m", "600", "p"); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat("p"); err != nil || info.Mode()&fs.ModeNamedPipe == 0 || info.Mode().Perm() != 0o600 {
		t.Fatalf("%v %v", info, err)
	}
	if _, err := run(t, "", "mkfifo", "p"); err == nil {
		t.Fatal("mkfifo over an existing file should fail")
	}
	if _, err := run(t, "", "shred", "p"); err == nil {
		t.Fatal("shred of a fifo should fail")
	}
}

func TestProcessControl(t *testing.T) {
	needShell(t)
	out, err := run(t, "", "kill", "-l", "9", "TERM", "sigint", "143")
	if err != nil || out != "KILL\n15\n2\nTERM\n" {
		t.Fatalf("%q %v", out, err)
	}
	if out, _ := run(t, "", "kill", "-l"); !strings.Contains(out, "HUP\n") || !strings.Contains(out, "KILL\n") {
		t.Fatalf("%q", out)
	}
	for _, bad := range [][]string{{"kill"}, {"kill", "-NOPE", "1"}, {"kill", "-s"}} {
		if _, err := run(t, "", bad...); uniz.ExitCode(err) != 2 {
			t.Fatalf("%v: %v", bad, err)
		}
	}
	if _, err := run(t, "", "kill", "abc"); uniz.ExitCode(err) != 1 {
		t.Fatal(err)
	}
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pid := strconv.Itoa(cmd.Process.Pid)
	if _, err := run(t, "", "kill", "-0", pid); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, "", "kill", "-s", "KILL", "--", pid); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err == nil || !strings.Contains(err.Error(), "killed") {
		t.Fatalf("child not killed: %v", err)
	}
	if _, err := run(t, "", "kill", "-0", pid); err == nil {
		t.Fatal("signal 0 to a dead process should fail")
	}
	if n, err := uniz.SignalNumber("SIGHUP"); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	if name, ok := uniz.SignalName(15); !ok || name != "TERM" {
		t.Fatal(name)
	}

	niceOut, err := run(t, "", "nice")
	base, convErr := strconv.Atoi(strings.TrimSpace(niceOut))
	if err != nil || convErr != nil {
		t.Fatalf("%q %v", niceOut, err)
	}
	out, err = run(t, "", "nice", "-n", "3", "sh", "-c", "echo ok")
	if err != nil || out != "ok\n" {
		t.Fatalf("%q %v", out, err)
	}
	out, err = run(t, "", "nice", "-5", "sh", "-c", "exit 3")
	if uniz.ExitCode(err) != 3 {
		t.Fatalf("nice exit status: %v", err)
	}
	if _, err := run(t, "", "nice", "-n", "2"); uniz.ExitCode(err) != 2 {
		t.Fatal(err)
	}
	if _, err := run(t, "", "nice", "-n", "x", "true"); uniz.ExitCode(err) != 2 {
		t.Fatal(err)
	}
	if _, err := run(t, "", "nice", "no-such-program-uniz"); uniz.ExitCode(err) != 127 {
		t.Fatal(err)
	}
	_ = base

	out, err = run(t, "in\n", "nohup", "sh", "-c", "cat; exit 4")
	if out != "in\n" || uniz.ExitCode(err) != 4 {
		t.Fatalf("%q %v", out, err)
	}
	if _, err := run(t, "", "nohup"); uniz.ExitCode(err) != 2 {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	err = uniz.New(uniz.Config{Stdout: &stdout, Stderr: &stderr}).Run(context.Background(), []string{"time", "-p", "sh", "-c", "echo hi; exit 5"})
	if uniz.ExitCode(err) != 5 || stdout.String() != "hi\n" {
		t.Fatalf("%q %v", stdout.String(), err)
	}
	if lines := strings.Split(stderr.String(), "\n"); len(lines) != 4 || !strings.HasPrefix(lines[0], "real ") || !strings.HasPrefix(lines[1], "user ") || !strings.HasPrefix(lines[2], "sys ") {
		t.Fatalf("%q", stderr.String())
	}
	if _, err := run(t, "", "time", "no-such-program-uniz"); uniz.ExitCode(err) != 127 {
		t.Fatal(err)
	}
	if _, err := run(t, "", "time"); uniz.ExitCode(err) != 2 {
		t.Fatal(err)
	}
}
