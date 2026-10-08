package uniz_test

import (
	"bytes"
	"context"
	"errors"
	"math/rand/v2"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/pkar/uniz"
)

func TestMoreTextCommands(t *testing.T) {
	t.Chdir(t.TempDir())
	write(t, "a", "a\nb\nd\n")
	write(t, "j1", "1 a\n2 b\n")
	write(t, "j2", "1 x\n1 z\n3 y\n")
	runCases(t, []commandCase{
		{"expand", []string{"expand"}, "a\tb\n", "a       b\n", 0},
		{"expand -t", []string{"expand", "-t4"}, "\tx\ty", "    x   y", 0},
		{"expand list", []string{"expand", "-t", "2,5"}, "\ta\tb\tc\n", "  a  b c\n", 0},
		{"expand -i", []string{"expand", "-i", "-t2"}, "\ta\tb\n", "  a\tb\n", 0},
		{"expand bad", []string{"expand", "-t", "4,2"}, "", "", 2},
		{"unexpand leading", []string{"unexpand"}, "        a        b\n", "\ta        b\n", 0},
		{"unexpand -a", []string{"unexpand", "-a"}, "        a       b c\n", "\ta\tb c\n", 0},
		{"unexpand single space", []string{"unexpand", "-t", "2"}, "a b\n", "a b\n", 0},
		{"fmt", []string{"fmt", "-w", "10"}, "one two three\nfour\n\nfive\n", "one two\nthree four\n\nfive\n", 0},
		{"fmt -N", []string{"fmt", "-7"}, "aa bb cc\n", "aa bb\ncc\n", 0},
		{"fmt indent", []string{"fmt"}, "  a\n    b\n    c\n", "  a b c\n", 0},
		{"fmt -s", []string{"fmt", "-s"}, "a\nb\n", "a\nb\n", 0},
		{"comm", []string{"comm", "a", "-"}, "b\nc\nd\n", "a\n\t\tb\n\tc\n\t\td\n", 0},
		{"comm -12", []string{"comm", "-12", "a", "-"}, "b\nc\nd\n", "b\nd\n", 0},
		{"comm two stdin", []string{"comm", "-", "-"}, "", "", 2},
		{"join", []string{"join", "j1", "j2"}, "", "1 a x\n1 a z\n", 0},
		{"join -a", []string{"join", "-a", "1", "-a2", "j1", "j2"}, "", "1 a x\n1 a z\n2 b\n3 y\n", 0},
		{"join -v -o -e", []string{"join", "-v", "2", "-o", "0,1.2,2.2", "-e", "-", "j1", "j2"}, "", "3 - y\n", 0},
		{"join -t", []string{"join", "-t", ",", "-2", "2", "-", "j1"}, "k,x\n", "", 0},
		{"join -t fields", []string{"join", "-t", ",", "-1", "2", "-", "-"}, "", "", 2},
		{"join -i", []string{"join", "-i", "-", "j2"}, "1 Q\n", "1 Q x\n1 Q z\n", 0},
		{"join bad -o", []string{"join", "-o", "3.1", "j1", "j2"}, "", "", 2},
		{"base32", []string{"base32"}, "hello\n", "NBSWY3DPBI======\n", 0},
		{"base32 -d", []string{"base32", "-d"}, "NBSWY3DP\nBI======\n", "hello\n", 0},
		{"base32 -w", []string{"base32", "-w", "4"}, "hi", "NBUQ\n====\n", 0},
		{"base32 bad", []string{"base32", "-d"}, "!!", "", 1},
		{"tsort", []string{"tsort"}, "a b b c\nd d\n", "a\nd\nb\nc\n", 0},
		{"tsort loop", []string{"tsort"}, "a b\nb a\n", "a\nb\n", 1},
		{"tsort odd", []string{"tsort"}, "a\n", "", 1},
		{"shuf -n 0", []string{"shuf", "-n", "0", "-e", "a"}, "", "", 0},
		{"shuf single", []string{"shuf"}, "only\n", "only\n", 0},
		{"shuf -r -n", []string{"shuf", "-r", "-n", "3", "-e", "z"}, "", "z\nz\nz\n", 0},
		{"shuf bad range", []string{"shuf", "-i", "5"}, "", "", 2},
		{"od default", []string{"od"}, "ab", "0000000 061141\n0000002\n", 0},
		{"od -c", []string{"od", "-c", "-A", "n"}, "a\n\x01", "   a  \\n 001\n", 0},
		{"od -t x1 -A x", []string{"od", "-A", "x", "-t", "x1"}, "\x00\xff", "000000 00 ff\n000002\n", 0},
		{"od -t a", []string{"od", "-An", "-ta"}, "\x00 ~\x7f", " nul  sp   ~ del\n", 0},
		{"od -t d1", []string{"od", "-An", "-t", "d1"}, "\xff\x01", "   -1    1\n", 0},
		{"od -j -N", []string{"od", "-j1", "-N2", "-tc"}, "abcd", "0000001   b   c\n0000003\n", 0},
		{"od star", []string{"od", "-w2", "-tx1", "-Ad"}, "aaaaab", "0000000 61 61\n*\n0000004 61 62\n0000006\n", 0},
		{"od -v", []string{"od", "-v", "-w1", "-tx1", "-An"}, "aa", " 61\n 61\n", 0},
		{"od skip past end", []string{"od", "-j", "9"}, "ab", "", 1},
		{"od bad type", []string{"od", "-t", "q"}, "", "", 2},
		{"od bad size", []string{"od", "-t", "f2"}, "", "", 2},
	})
}

func TestOdFloatAndMixed(t *testing.T) {
	var out bytes.Buffer
	types, err := uniz.ParseOdTypes("f4")
	if err != nil {
		t.Fatal(err)
	}
	// 1.5 as float32 in native (little-endian on supported hosts) order.
	if err := uniz.Od(context.Background(), &out, strings.NewReader("\x00\x00\xc0\x3f"), uniz.OdOptions{Types: types, Radix: 'n', Limit: -1}); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "1.5" {
		t.Fatalf("%q", out.String())
	}
	out.Reset()
	if err := uniz.Od(context.Background(), &out, strings.NewReader("ab"), uniz.OdOptions{Types: []uniz.OdType{{Kind: 'u', Size: 2}, {Kind: 'c', Size: 1}}, Radix: 'n', Limit: -1}); err != nil {
		t.Fatal(err)
	}
	if out.String() != "   25185\n   a   b\n" {
		t.Fatalf("%q", out.String())
	}
}

func TestShuffleAPI(t *testing.T) {
	items := []string{"a", "b", "c", "d"}
	var out bytes.Buffer
	if err := uniz.Shuffle(context.Background(), &out, items, uniz.ShuffleOptions{Count: -1, Rand: rand.New(rand.NewPCG(1, 2))}); err != nil {
		t.Fatal(err)
	}
	got := strings.Fields(out.String())
	slices.Sort(got)
	if !slices.Equal(got, items) {
		t.Fatal(out.String())
	}
	out.Reset()
	if err := uniz.Shuffle(context.Background(), &out, nil, uniz.ShuffleOptions{Count: 2, Repeat: true}); err == nil {
		t.Fatal("repeat from nothing should fail")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := uniz.Shuffle(ctx, &out, items, uniz.ShuffleOptions{Count: -1, Repeat: true}); !errors.Is(err, context.Canceled) {
		t.Fatalf("unbounded repeat should stop on cancel: %v", err)
	}
	sout, err := run(t, "", "shuf", "-i", "3-7")
	lines := strings.Fields(sout)
	slices.Sort(lines)
	if err != nil || strings.Join(lines, ",") != "3,4,5,6,7" {
		t.Fatalf("%q %v", sout, err)
	}
	if err := uniz.Tsort(context.Background(), &bytes.Buffer{}, strings.NewReader("x y y x")); !errors.Is(err, uniz.ErrLoop) {
		t.Fatal(err)
	}
}

func TestSplitCommands(t *testing.T) {
	t.Chdir(t.TempDir())
	nums := "1\n2\n3\n4\n5\n"
	if _, err := run(t, nums, "split", "-l", "2"); err != nil {
		t.Fatal(err)
	}
	if read(t, "xaa") != "1\n2\n" || read(t, "xac") != "5\n" || exists("xad") {
		t.Fatal("split -l")
	}
	write(t, "in", "abcdefg")
	if _, err := run(t, "", "split", "-b", "3", "-d", "-a", "3", "in", "p"); err != nil {
		t.Fatal(err)
	}
	if read(t, "p000") != "abc" || read(t, "p002") != "g" {
		t.Fatal("split -b")
	}
	if _, err := run(t, "", "split", "-n", "3", "in", "n"); err != nil {
		t.Fatal(err)
	}
	if read(t, "naa") != "ab" || read(t, "nab") != "cd" || read(t, "nac") != "efg" {
		t.Fatal("split -n")
	}
	if _, err := run(t, "abcd", "split", "-n", "2", "-", "s"); err != nil || read(t, "sab") != "cd" {
		t.Fatalf("split -n stdin: %v", err)
	}
	if _, err := run(t, "abc", "split", "-b", "1", "-a", "1", "-d", "-", "e"); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, "0123456789ab", "split", "-b", "1", "-a", "1", "-d", "-", "f"); err == nil {
		t.Fatal("suffixes should run out")
	}
	if err := os.Symlink("in", "lnkaa"); err == nil {
		if _, err := run(t, "x\n", "split", "-", "lnk"); err == nil || read(t, "in") != "abcdefg" {
			t.Fatal("split wrote through a symlink")
		}
	}
	for _, bad := range [][]string{{"split", "-l", "0"}, {"split", "-l1", "-b1"}, {"split", "-b", "1Q"}, {"split", "a", "b", "c"}} {
		if _, err := run(t, "", bad...); uniz.ExitCode(err) != 2 {
			t.Fatalf("%v: %v", bad, err)
		}
	}
	if _, err := run(t, "", "split", "-b", "2K", "in", "k"); err != nil || read(t, "kaa") != "abcdefg" {
		t.Fatalf("split -b 2K: %v", err)
	}

	out, err := run(t, "a\nx1\nb\nx2\nc\n", "csplit", "-", "/x/", "{*}")
	if err != nil || out != "2\n5\n5\n" || read(t, "xx01") != "x1\nb\n" || read(t, "xx02") != "x2\nc\n" {
		t.Fatalf("csplit regexp: %q %v", out, err)
	}
	out, err = run(t, "1\n2\n3\n4\n5\n6\n", "csplit", "-s", "-f", "n", "-n", "1", "-", "2", "{1}")
	if err != nil || out != "" || read(t, "n0") != "1\n" || read(t, "n1") != "2\n3\n" || read(t, "n2") != "4\n5\n6\n" {
		t.Fatalf("csplit lines: %q %v", out, err)
	}
	out, err = run(t, "a\nb\nc\nd\n", "csplit", "-z", "-f", "s", "-", "%b%", "/d/-1")
	if err != nil || out != "2\n4\n" {
		t.Fatalf("csplit skip: %q %v", out, err)
	}
	if read(t, "s00") != "b\n" || read(t, "s01") != "c\nd\n" {
		t.Fatalf("csplit skip files: %q %q", read(t, "s00"), read(t, "s01"))
	}
	if _, err := run(t, "a\n", "csplit", "-f", "m", "-", "/zz/"); err == nil || exists("m00") {
		t.Fatalf("csplit no match should fail and clean up: %v", err)
	}
	if _, err := run(t, "a\nb\n", "csplit", "-k", "-f", "k", "-", "2", "5"); err == nil || !exists("k00") {
		t.Fatalf("csplit -k: %v", err)
	}
	for _, bad := range [][]string{{"csplit", "-"}, {"csplit", "-", "{2}"}, {"csplit", "-", "/a"}, {"csplit", "-", "x"}, {"csplit", "-", "/\\(/"}} {
		if _, err := run(t, "a\n", bad...); err == nil {
			t.Fatalf("%v should fail", bad)
		}
	}
	names, sizes, err := uniz.Csplit(context.Background(), strings.NewReader("q\nr\n"), []string{"2"}, uniz.CsplitOptions{Prefix: "api"})
	if err != nil || len(names) != 2 || sizes[1] != 2 {
		t.Fatalf("%v %v %v", names, sizes, err)
	}
	names, err = uniz.Split(context.Background(), strings.NewReader("z"), uniz.SplitOptions{Prefix: "api-"})
	if err != nil || !slices.Equal(names, []string{"api-aa"}) {
		t.Fatalf("%v %v", names, err)
	}
}

func TestTextAPIs(t *testing.T) {
	ctx := context.Background()
	var out bytes.Buffer
	stops, err := uniz.ParseTabStops("4")
	if err != nil {
		t.Fatal(err)
	}
	if err := uniz.Expand(ctx, &out, strings.NewReader("\tx"), uniz.ExpandOptions{Tabs: stops}); err != nil || out.String() != "    x" {
		t.Fatalf("%q %v", out.String(), err)
	}
	out.Reset()
	if err := uniz.Unexpand(ctx, &out, strings.NewReader("    x"), uniz.UnexpandOptions{Tabs: stops}); err != nil || out.String() != "\tx" {
		t.Fatalf("%q %v", out.String(), err)
	}
	out.Reset()
	if err := uniz.Fmt(ctx, &out, strings.NewReader("a\nb\n"), uniz.FmtOptions{}); err != nil || out.String() != "a b\n" {
		t.Fatalf("%q %v", out.String(), err)
	}
	out.Reset()
	if err := uniz.Comm(ctx, &out, strings.NewReader("a\n"), strings.NewReader("a\n"), uniz.CommOptions{Delimiter: "|"}); err != nil || out.String() != "||a\n" {
		t.Fatalf("%q %v", out.String(), err)
	}
	out.Reset()
	fields, err := uniz.ParseJoinFields("2.1 0")
	if err != nil {
		t.Fatal(err)
	}
	if err := uniz.Join(ctx, &out, strings.NewReader("k a\n"), strings.NewReader("k b\n"), uniz.JoinOptions{Output: fields}); err != nil || out.String() != "k k\n" {
		t.Fatalf("%q %v", out.String(), err)
	}
	out.Reset()
	if err := uniz.Base32Encode(ctx, &out, strings.NewReader("f"), 0); err != nil || out.String() != "MY======" {
		t.Fatalf("%q %v", out.String(), err)
	}
	var dec bytes.Buffer
	if err := uniz.Base32Decode(ctx, &dec, &out); err != nil || dec.String() != "f" {
		t.Fatalf("%q %v", dec.String(), err)
	}
}
