package uniz_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/pkar/uniz"
)

func TestTransformCommands(t *testing.T) {
	t.Chdir(t.TempDir())
	write(t, "f1", "1\n2\n")
	write(t, "f2", "a\nb\nc\n")
	runCases(t, []commandCase{
		{"cut fields", []string{"cut", "-f2", "-d,"}, "a,b,c\nnodelim\nx,y", "b\nnodelim\ny", 0},
		{"cut -s", []string{"cut", "-s", "-f1,3", "-d,"}, "a,b,c\nnodelim\n", "a,c\n", 0},
		{"cut chars", []string{"cut", "-c2-3"}, "héllo\n", "él\n", 0},
		{"cut bytes", []string{"cut", "-b1-2,4-"}, "abcdef\n", "abdef\n", 0},
		{"cut tab fields", []string{"cut", "-f", "-2"}, "a\tb\tc\n", "a\tb\n", 0},
		{"cut overlapping ranges keep order", []string{"cut", "-c3,1-2,2"}, "abcd\n", "abc\n", 0},
		{"cut no mode", []string{"cut"}, "", "", 2},
		{"cut two modes", []string{"cut", "-b1", "-c1"}, "", "", 2},
		{"cut decreasing", []string{"cut", "-c", "3-1"}, "", "", 2},
		{"cut -d without -f", []string{"cut", "-d,", "-c1"}, "", "", 2},
		{"cut long delimiter", []string{"cut", "-f1", "-d", "ab"}, "", "", 2},
		{"cut zero", []string{"cut", "-f", "0"}, "", "", 2},
		{"tr upper", []string{"tr", "a-z", "A-Z"}, "hello\n", "HELLO\n", 0},
		{"tr delete class", []string{"tr", "-d", "[:digit:]"}, "a1b2\n", "ab\n", 0},
		{"tr squeeze", []string{"tr", "-s", " "}, "a   b  c\n", "a b c\n", 0},
		{"tr complement delete", []string{"tr", "-cd", `[:alpha:]\n`}, "a1-b\n", "ab\n", 0},
		{"tr pads set2", []string{"tr", "abc", "x"}, "aabbcc", "xxxxxx", 0},
		{"tr translate squeeze", []string{"tr", "-s", "a-z", "A-Z"}, "aabbc", "ABC", 0},
		{"tr delete squeeze", []string{"tr", "-ds", "a", "b"}, "aabbb", "b", 0},
		{"tr classes are ASCII", []string{"tr", "[:lower:]", "[:upper:]"}, "héllo", "HéLLO", 0},
		{"tr unicode", []string{"tr", "é", "e"}, "café", "cafe", 0},
		{"tr complement translate", []string{"tr", "-c", "a", "x"}, "abc\n", "axxx", 0},
		{"tr invalid utf8 kept", []string{"tr", "a", "b"}, "\xffa", "\xffb", 0},
		{"tr octal escape", []string{"tr", `\101`, "b"}, "AA", "bb", 0},
		{"tr one set", []string{"tr", "a"}, "", "", 2},
		{"tr -d two sets", []string{"tr", "-d", "a", "b"}, "", "", 2},
		{"tr three sets", []string{"tr", "a", "b", "c"}, "", "", 2},
		{"tr reverse range", []string{"tr", "z-a", "b"}, "", "", 2},
		{"tr bad class", []string{"tr", "[:foo:]", "x"}, "", "", 2},
		{"paste", []string{"paste", "f1", "f2"}, "", "1\ta\n2\tb\n\tc\n", 0},
		{"paste -d", []string{"paste", "-d,", "f1", "f2"}, "", "1,a\n2,b\n,c\n", 0},
		{"paste cycle", []string{"paste", "-d", ":,", "f1", "f2", "f1"}, "", "1:a,1\n2:b,2\n:c,\n", 0},
		{"paste -s", []string{"paste", "-s", "f1", "f2"}, "", "1\t2\na\tb\tc\n", 0},
		{"paste stdin twice", []string{"paste", "-", "-"}, "a\nb\nc\n", "a\tb\nc\t\n", 0},
		{"paste \\0", []string{"paste", "-d", `\0`, "f1", "f1"}, "", "11\n22\n", 0},
		{"paste newline delim", []string{"paste", "-s", "-d", `\n`, "f1"}, "", "1\n2\n", 0},
		{"paste missing", []string{"paste", "f1", "missing"}, "", "", 1},
		{"nl", []string{"nl"}, "a\n\nb\n", "     1\ta\n       \n     2\tb\n", 0},
		{"nl options", []string{"nl", "-ba", "-w2", "-s:", "-v10", "-i5"}, "a\n\nb", "10:a\n15:\n20:b", 0},
		{"nl none", []string{"nl", "-bn"}, "a\n", "       a\n", 0},
		{"nl continues", []string{"nl", "-w1", "-s", " ", "f1", "f2"}, "", "1 1\n2 2\n3 a\n4 b\n5 c\n", 0},
		{"nl bad style", []string{"nl", "-b", "x"}, "", "", 2},
		{"tac", []string{"tac"}, "a\nb\nc", "c\nb\na\n", 0},
		{"tac files", []string{"tac", "f1", "f2"}, "", "2\n1\nc\nb\na\n", 0},
		{"rev", []string{"rev"}, "abc\nhé\nab", "cba\néh\nba", 0},
		{"fold", []string{"fold", "-w3"}, "abcdefg\n", "abc\ndef\ng\n", 0},
		{"fold exact width", []string{"fold", "-w3"}, "abc\n", "abc\n", 0},
		{"fold spaces", []string{"fold", "-w", "5", "-s"}, "aa bb cc\n", "aa \nbb cc\n", 0},
		{"fold -N", []string{"fold", "-2"}, "abc", "ab\nc", 0},
		{"fold chars", []string{"fold", "-w2"}, "héllo", "hé\nll\no", 0},
		{"fold bytes", []string{"fold", "-b", "-w2"}, "abcd", "ab\ncd", 0},
		{"fold zero", []string{"fold", "-w", "0"}, "", "", 2},
	})
}

func TestDigestCommands(t *testing.T) {
	t.Chdir(t.TempDir())
	write(t, "h", "hello\n")
	write(t, "other", "different")
	write(t, "empty", "")
	sha := "5891b5b522d5df086d0ff0b110fbd9d21bb4fc7163af34d08286a2e846f6be03"
	write(t, "sums", sha+"  h\n"+strings.Repeat("0", 64)+"  other\n"+sha+" *missing\nnot a checksum line\n")
	write(t, "good", sha+"  h\n")
	write(t, "junk", "nothing here\n")
	runCases(t, []commandCase{
		{"md5 stdin", []string{"md5sum"}, "hello\n", "b1946ac92492d2347c6235b4d2611184  -\n", 0},
		{"sha256 file", []string{"sha256sum", "h"}, "", sha + "  h\n", 0},
		{"sha1", []string{"sha1sum", "h"}, "", "f572d396fae9206628714fb2ce00f72e94f2258f  h\n", 0},
		{"sha256 missing continues", []string{"sha256sum", "missing", "h"}, "", sha + "  h\n", 1},
		{"check ok", []string{"sha256sum", "-c", "good"}, "", "h: OK\n", 0},
		{"check failures", []string{"sha256sum", "-c", "sums"}, "", "h: OK\nother: FAILED\nmissing: FAILED open or read\n", 1},
		{"check junk", []string{"sha256sum", "-c", "junk"}, "", "", 1},
		{"check wrong length", []string{"md5sum", "-c", "good"}, "", "", 1},
		{"cksum stdin", []string{"cksum"}, "hello\n", "3015617425 6\n", 0},
		{"cksum file", []string{"cksum", "empty", "h"}, "", "4294967295 0 empty\n3015617425 6 h\n", 0},
		{"base64", []string{"base64"}, "hello\n", "aGVsbG8K\n", 0},
		{"base64 wrap", []string{"base64", "-w4"}, "hello\n", "aGVs\nbG8K\n", 0},
		{"base64 nowrap", []string{"base64", "-w0"}, "hello\n", "aGVsbG8K", 0},
		{"base64 default wrap", []string{"base64"}, strings.Repeat("a", 60), strings.Repeat("YWFh", 19) + "\nYWFh\n", 0},
		{"base64 empty", []string{"base64"}, "", "", 0},
		{"base64 decode", []string{"base64", "-d"}, "aGVs\nbG8K\n", "hello\n", 0},
		{"base64 decode error", []string{"base64", "-d"}, "!!!!", "", 1},
		{"base64 extra", []string{"base64", "a", "b"}, "", "", 2},
	})
	ctx := context.Background()
	for alg, want := range map[uniz.HashAlgorithm]string{
		uniz.SHA224: "d14a028c2a3a2bc9476102bb288234c415a2b01f828ea62ac5b3e42f",
		uniz.SHA384: "38b060a751ac96384cd9327eb1b1e36a21fdb71114be07434c0cc7bf63f6e1da274edebfe76f65fbd51ad2f14898b95b",
		uniz.SHA512: "cf83e1357eefb8bdf1542850d66d8007d620e4050b5715dc83f4a921d36ce9ce47d0d13c5d85f2b0ff8318d2877eec2f63b931bd47417a81a538327af927da3e",
	} {
		if got, err := uniz.Checksum(ctx, alg, strings.NewReader("")); err != nil || got != want {
			t.Fatalf("%s: %s %v", alg, got, err)
		}
	}
	if _, err := uniz.Checksum(ctx, "crc64", strings.NewReader("")); err == nil {
		t.Fatal("unknown algorithm accepted")
	}
	if sum, name, ok := uniz.ParseChecksumLine("ABCD *file name"); !ok || sum != "abcd" || name != "file name" {
		t.Fatal(sum, name, ok)
	}
	var round bytes.Buffer
	data := strings.Repeat("\x00\xff binary ", 50)
	var enc bytes.Buffer
	if err := uniz.Base64Encode(ctx, &enc, strings.NewReader(data), 10); err != nil {
		t.Fatal(err)
	}
	if err := uniz.Base64Decode(ctx, &round, &enc); err != nil || round.String() != data {
		t.Fatal(err)
	}
}

func TestPrintf(t *testing.T) {
	runCases(t, []commandCase{
		{"reuse", []string{"printf", `%s-%s\n`, "a", "b", "c"}, "", "a-b\nc-\n", 0},
		{"bases", []string{"printf", `%d %i\n`, "0x10", "010"}, "", "16 8\n", 0},
		{"char code", []string{"printf", `%d\n`, "'A"}, "", "65\n", 0},
		{"invalid number", []string{"printf", `%d\n`, "abc"}, "", "0\n", 1},
		{"width precision", []string{"printf", `%5s|%-5s|%.2s\n`, "a", "b", "xyz"}, "", "    a|b    |xy\n", 0},
		{"star", []string{"printf", `%*d|%-*d|\n`, "4", "7", "3", "1"}, "", "   7|1  |\n", 0},
		{"negative star", []string{"printf", `%*d|\n`, "-3", "1"}, "", "1  |\n", 0},
		{"unsigned", []string{"printf", `%u %x %o %X\n`, "-1", "-1", "8", "255"}, "", "18446744073709551615 ffffffffffffffff 10 FF\n", 0},
		{"%b stops", []string{"printf", "%b", `a\nb\c`, "x"}, "", "a\nb", 0},
		{"no newline", []string{"printf", "no newline"}, "", "no newline", 0},
		{"format escapes", []string{"printf", `\101\x42\t\"\n`}, "", "AB\t\"\n", 0},
		{"percent", []string{"printf", `%%\n`}, "", "%\n", 0},
		{"bad conversion", []string{"printf", "%q"}, "", "", 1},
		{"missing conversion", []string{"printf", "a%5"}, "", "a", 1},
		{"no format", []string{"printf"}, "", "", 2},
		{"missing args", []string{"printf", `%s|%d\n`}, "", "|0\n", 0},
		{"dash dash", []string{"printf", "--", "%s", "x"}, "", "x", 0},
		{"char", []string{"printf", "%c", "hello"}, "", "h", 0},
		{"floats", []string{"printf", `%.3f %e %G %g\n`, "2.5", "0", "1e-10", "100000000"}, "", "2.500 0.000000e+00 1E-10 1e+08\n", 0},
		{"flags", []string{"printf", `%05d|%+d|% d|%#x|%#o\n`, "42", "5", "5", "255", "8"}, "", "00042|+5| 5|0xff|010\n", 0},
		{"inf", []string{"printf", `%f %F\n`, "inf", "-inf"}, "", "inf -INF\n", 0},
		{"help only alone", []string{"printf", "%s", "--help"}, "", "--help", 0},
	})
	out, err := uniz.Printf("%s=%d;", []string{"a", "1", "b", "x"})
	if out != "a=1;b=0;" || err == nil {
		t.Fatalf("%q %v", out, err)
	}
	if got, stop := uniz.Escapes(`a\tb\cc`); got != "a\tb" || !stop {
		t.Fatalf("%q %v", got, stop)
	}
}

func TestTestCommand(t *testing.T) {
	t.Chdir(t.TempDir())
	write(t, "file", "x")
	write(t, "empty", "")
	write(t, "script", "#!/bin/sh\n")
	if err := os.Chmod("script", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("file", "link"); err != nil {
		t.Fatal(err)
	}
	if err := os.Link("file", "hard"); err != nil {
		t.Fatal(err)
	}
	old := time.Unix(1000, 0)
	if err := os.Chtimes("empty", old, old); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		args []string
		code int
	}{
		{nil, 1}, {[]string{""}, 1}, {[]string{"x"}, 0}, {[]string{"-f"}, 0},
		{[]string{"-n", ""}, 1}, {[]string{"-z", ""}, 0}, {[]string{"!", ""}, 0},
		{[]string{"a", "=", "a"}, 0}, {[]string{"a", "==", "a"}, 0}, {[]string{"a", "!=", "a"}, 1},
		{[]string{"a", "<", "b"}, 0}, {[]string{"a", ">", "b"}, 1},
		{[]string{"2", "-lt", "10"}, 0}, {[]string{"10", "-lt", "2"}, 1}, {[]string{" 3 ", "-eq", "3"}, 0},
		{[]string{"-1", "-le", "-1"}, 0}, {[]string{"x", "-eq", "1"}, 2},
		{[]string{"!", "a", "=", "b"}, 0}, {[]string{"(", "a", ")"}, 0},
		{[]string{"a", "-a", ""}, 1}, {[]string{"a", "-o", ""}, 0},
		{[]string{"a", "=", "a", "-a", "b", "=", "c"}, 1},
		{[]string{"a", "=", "a", "-o", "b", "=", "c"}, 0},
		{[]string{"!", "a", "=", "a", "-o", "b", "=", "b"}, 0},
		{[]string{"(", "a", "=", "b", ")", "-o", "x"}, 0},
		{[]string{"(", "a", "=", "b", "-a"}, 2},
		{[]string{"a", "b"}, 2}, {[]string{"a", "b", "c", "d", "e"}, 2},
		{[]string{"-f", "file"}, 0}, {[]string{"-d", "file"}, 1}, {[]string{"-d", "."}, 0},
		{[]string{"-e", "missing"}, 1}, {[]string{"-s", "empty"}, 1}, {[]string{"-s", "file"}, 0},
		{[]string{"-h", "link"}, 0}, {[]string{"-L", "file"}, 1}, {[]string{"-f", "link"}, 0},
		{[]string{"-x", "script"}, 0}, {[]string{"-x", "file"}, 1}, {[]string{"-r", "file"}, 0},
		{[]string{"-w", "missing"}, 1},
		{[]string{"file", "-nt", "empty"}, 0}, {[]string{"empty", "-ot", "file"}, 0},
		{[]string{"file", "-nt", "missing"}, 0}, {[]string{"file", "-ef", "hard"}, 0}, {[]string{"file", "-ef", "empty"}, 1},
	}
	for _, tc := range cases {
		_, err := run(t, "", append([]string{"test"}, tc.args...)...)
		if uniz.ExitCode(err) != tc.code {
			t.Fatalf("test %q: %v (code %d, want %d)", tc.args, err, uniz.ExitCode(err), tc.code)
		}
		_, err = run(t, "", append(append([]string{"["}, tc.args...), "]")...)
		if uniz.ExitCode(err) != tc.code {
			t.Fatalf("[ %q ]: %v", tc.args, err)
		}
	}
	if _, err := run(t, "", "[", "a", "=", "a"); uniz.ExitCode(err) != 2 {
		t.Fatal(err)
	}
	if ok, err := uniz.Test([]string{"-d", "."}); !ok || err != nil {
		t.Fatal(ok, err)
	}
}

func TestDate(t *testing.T) {
	t.Chdir(t.TempDir())
	write(t, "file", "")
	when := time.Unix(1000, 0)
	if err := os.Chtimes("file", when, when); err != nil {
		t.Fatal(err)
	}
	runCases(t, []commandCase{
		{"epoch", []string{"date", "-u", "-d", "@86400", "+%F %T"}, "", "1970-01-02 00:00:00\n", 0},
		{"leap day", []string{"date", "-u", "-d", "2024-02-29", "+%j"}, "", "060\n", 0},
		{"default", []string{"date", "-u", "-d", "@0"}, "", "Thu Jan  1 00:00:00 UTC 1970\n", 0},
		{"iso", []string{"date", "-u", "-I", "-d", "@0"}, "", "1970-01-01\n", 0},
		{"rfc", []string{"date", "-u", "-R", "-d", "@0"}, "", "Thu, 01 Jan 1970 00:00:00 +0000\n", 0},
		{"pad flags", []string{"date", "-u", "-d", "2024-03-05 07:08:09", "+%-d %_m %0e %-H %I%p %P"}, "", "5  3 05 7 07AM am\n", 0},
		{"zone conversion", []string{"date", "-u", "-d", "2024-01-02T03:04:05+02:00", "+%H %z %:z"}, "", "01 +0000 +00:00\n", 0},
		{"fraction", []string{"date", "-u", "-d", "@1.5", "+%s %N"}, "", "1 500000000\n", 0},
		{"weeks", []string{"date", "-u", "-d", "2024-12-30 05:06:07", "+%U %V %W %G %g %u %w"}, "", "52 01 53 2025 25 1 1\n", 0},
		{"literal and unknown", []string{"date", "-u", "-d", "@0", "+100%% %Q"}, "", "100% %Q\n", 0},
		{"file time", []string{"date", "-u", "-r", "file", "+%s"}, "", "1000\n", 0},
		{"bad date", []string{"date", "-d", "bogus"}, "", "", 2},
		{"set date", []string{"date", "0101"}, "", "", 2},
		{"both sources", []string{"date", "-d", "now", "-r", "file"}, "", "", 2},
		{"missing file", []string{"date", "-r", "missing"}, "", "", 1},
	})
	out, err := run(t, "", "date", "+%Y")
	if err != nil || out != time.Now().Format("2006")+"\n" {
		t.Fatalf("%q %v", out, err)
	}
	loc := time.FixedZone("X", 3600)
	got, err := uniz.ParseDate("2024-05-06 07:08", time.Now(), loc)
	if err != nil || uniz.Strftime(got, "%F %R %z") != "2024-05-06 07:08 +0100" {
		t.Fatal(got, err)
	}
}

func TestTypedTransforms(t *testing.T) {
	ctx := context.Background()
	if set, err := uniz.ExpandSet(`a-c\n[:digit:]`); err != nil || string(set) != "abc\n0123456789" {
		t.Fatalf("%q %v", string(set), err)
	}
	if r, err := uniz.ParseRanges("-2,4,6-"); err != nil || len(r) != 3 || r[0] != (uniz.Range{Start: 1, End: 2}) || r[2] != (uniz.Range{Start: 6}) {
		t.Fatal(r, err)
	}
	for _, bad := range []string{"", "-", "a", "0", "1,,2", "5-2"} {
		if _, err := uniz.ParseRanges(bad); err == nil {
			t.Fatalf("%q accepted", bad)
		}
	}
	var out bytes.Buffer
	if err := uniz.Cut(ctx, &out, strings.NewReader("a"), uniz.CutOptions{Unit: uniz.CutFields}); err == nil {
		t.Fatal("cut without ranges accepted")
	}
	if err := uniz.Translate(ctx, &out, strings.NewReader("a"), "a", "", uniz.TranslateOptions{}); err == nil {
		t.Fatal("translate without set2 accepted")
	}
	a, b := strings.NewReader("1\n2\n"), strings.NewReader("x\n")
	out.Reset()
	if err := uniz.Paste(ctx, &out, []io.Reader{a, b}, uniz.PasteOptions{Delimiters: []string{" | "}}); err != nil || out.String() != "1 | x\n2 | \n" {
		t.Fatalf("%q %v", out.String(), err)
	}
	out.Reset()
	opts := uniz.DefaultNumberOptions()
	opts.Start = 7
	next, err := uniz.NumberLines(ctx, &out, strings.NewReader("a\nb\n"), opts)
	if err != nil || next != 9 || out.String() != "     7\ta\n     8\tb\n" {
		t.Fatalf("%d %q %v", next, out.String(), err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	for name, err := range map[string]error{
		"cut":  uniz.Cut(canceled, &out, strings.NewReader("a"), uniz.CutOptions{Ranges: []uniz.Range{{Start: 1}}}),
		"tr":   uniz.Translate(canceled, &out, strings.NewReader("a"), "a", "b", uniz.TranslateOptions{}),
		"tac":  uniz.ReverseLines(canceled, &out, strings.NewReader("a")),
		"fold": uniz.Fold(canceled, &out, strings.NewReader("a"), uniz.FoldOptions{}),
	} {
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("%s: %v", name, err)
		}
	}
}
