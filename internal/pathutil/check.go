package pathutil

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// CheckOptions controls CheckPath.
type CheckOptions struct {
	Portable bool // POSIX limits (256-byte paths, 14-byte names) and the portable character set.
	Extra    bool // Also reject components starting with '-'.
}

// CheckPath reports whether path is valid and portable like pathchk. Without
// Portable it checks common limits (4095-byte paths, 255-byte names) and that
// existing leading components are directories.
func CheckPath(path string, opts CheckOptions) error {
	if path == "" {
		return errors.New("empty file name")
	}
	maxPath, maxName := 4095, 255
	if opts.Portable {
		maxPath, maxName = 255, 14
	}
	if len(path) > maxPath {
		return fmt.Errorf("%q: path length %d exceeds the limit of %d", path, len(path), maxPath)
	}
	if opts.Portable {
		for _, c := range path {
			if !(c == '/' || c == '.' || c == '_' || c == '-' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
				return fmt.Errorf("%q: nonportable character %q", path, c)
			}
		}
	}
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if len(part) > maxName {
			return fmt.Errorf("%q: name %q is longer than %d bytes", path, part, maxName)
		}
		if opts.Extra && strings.HasPrefix(part, "-") {
			return fmt.Errorf("%q: name %q starts with '-'", path, part)
		}
		if opts.Portable || part == "" || i == len(parts)-1 {
			continue
		}
		prefix := strings.Join(parts[:i+1], "/")
		if info, err := os.Stat(prefix); err == nil && !info.IsDir() {
			return fmt.Errorf("%q: %s is not a directory", path, prefix)
		}
	}
	return nil
}
