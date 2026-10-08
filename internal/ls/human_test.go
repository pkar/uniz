package ls

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHumanSize(t *testing.T) {
	for n, want := range map[int64]string{
		0: "0", 1023: "1023", 1024: "1.0K", 1025: "1.1K", 1536: "1.5K",
		10239: "10K", 10240: "10K", 10241: "11K", 1048575: "1.0M", 1 << 20: "1.0M",
		5 << 30: "5.0G", 1<<62 + 1: "4.1E", 1<<63 - 1: "8.0E", 1023 << 20: "1023M", 1023<<20 + 1: "1.0G",
	} {
		if got := HumanSize(n); got != want {
			t.Errorf("HumanSize(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestLongHuman(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f"), make([]byte, 2048), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Run(&out, []string{dir}, Options{Long: true, Human: true}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "     2.0K ") {
		t.Fatalf("%q", out.String())
	}
	out.Reset()
	if err := Run(&out, []string{dir}, Options{Long: true}); err != nil || !strings.Contains(out.String(), "     2048 ") {
		t.Fatalf("%q %v", out.String(), err)
	}
}
