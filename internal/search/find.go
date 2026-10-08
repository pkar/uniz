package search

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Entry is a file visited by Find.
type Entry struct {
	Path  string
	Info  fs.FileInfo // lstat information; symlinks are not followed
	Depth int         // 0 for a starting path
}

type node func(*visit) (bool, error)

type visit struct {
	Entry
	prune bool
	quit  bool
	emit  func(path string, terminator byte) error
}

// Expression is a compiled find expression.
type Expression struct {
	root               node
	minDepth, maxDepth int
}

type parser struct {
	args      []string
	i         int
	hasAction bool
	now       time.Time
	expr      *Expression
}

// ParseExpression compiles find(1) expression arguments, for example
// []string{"-name", "*.go", "-type", "f"}. Supported: ( ) ! -not -a -and -o
// -or, -name -iname -path -ipath -type -size -mtime -mmin -newer -empty
// -true -false, -print -print0 -prune -quit, and -maxdepth -mindepth.
// Without an action, matching entries are printed. now is the reference for
// -mtime and -mmin.
func ParseExpression(args []string, now time.Time) (*Expression, error) {
	e := &Expression{maxDepth: -1}
	p := &parser{args: args, now: now, expr: e}
	if len(args) == 0 {
		e.root = func(*visit) (bool, error) { return true, nil }
	} else {
		n, err := p.or()
		if err != nil {
			return nil, err
		}
		if p.i < len(args) {
			return nil, fmt.Errorf("unexpected %q", args[p.i])
		}
		e.root = n
	}
	if !p.hasAction {
		inner := e.root
		e.root = func(v *visit) (bool, error) {
			ok, err := inner(v)
			if ok && err == nil && !v.quit { // -quit exits before the implicit -print
				err = v.emit(v.Path, '\n')
			}
			return ok, err
		}
	}
	return e, nil
}

func (p *parser) peek() string {
	if p.i < len(p.args) {
		return p.args[p.i]
	}
	return ""
}

func (p *parser) or() (node, error) {
	left, err := p.and()
	for err == nil && (p.peek() == "-o" || p.peek() == "-or") {
		p.i++
		var right node
		if right, err = p.and(); err != nil {
			break
		}
		l := left
		left = func(v *visit) (bool, error) {
			if ok, err := l(v); ok || err != nil {
				return ok, err
			}
			return right(v)
		}
	}
	return left, err
}

func (p *parser) and() (node, error) {
	left, err := p.not()
	for err == nil && p.i < len(p.args) {
		switch p.peek() {
		case "-o", "-or", ")":
			return left, nil
		case "-a", "-and":
			p.i++
		}
		var right node
		if right, err = p.not(); err != nil {
			break
		}
		l := left
		left = func(v *visit) (bool, error) {
			if ok, err := l(v); !ok || err != nil {
				return ok, err
			}
			return right(v)
		}
	}
	return left, err
}

func (p *parser) not() (node, error) {
	if p.peek() == "!" || p.peek() == "-not" {
		p.i++
		n, err := p.not()
		if err != nil {
			return nil, err
		}
		return func(v *visit) (bool, error) {
			ok, err := n(v)
			return !ok, err
		}, nil
	}
	return p.primary()
}

func (p *parser) value(name string) (string, error) {
	if p.i >= len(p.args) {
		return "", fmt.Errorf("missing argument to %s", name)
	}
	p.i++
	return p.args[p.i-1], nil
}

func constant(b bool) node { return func(*visit) (bool, error) { return b, nil } }

// numeric parses find's [+-]N and returns a comparison against N.
func numeric(name, s string) (func(int64) bool, error) {
	cmp := func(a, n int64) bool { return a == n }
	switch {
	case strings.HasPrefix(s, "+"):
		s, cmp = s[1:], func(a, n int64) bool { return a > n }
	case strings.HasPrefix(s, "-"):
		s, cmp = s[1:], func(a, n int64) bool { return a < n }
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n < 0 {
		return nil, fmt.Errorf("invalid argument %q to %s", s, name)
	}
	return func(a int64) bool { return cmp(a, n) }, nil
}

func (p *parser) primary() (node, error) {
	if p.i >= len(p.args) {
		return nil, errors.New("expected an expression")
	}
	tok := p.args[p.i]
	p.i++
	switch tok {
	case "(":
		n, err := p.or()
		if err != nil {
			return nil, err
		}
		if p.peek() != ")" {
			return nil, errors.New("missing ')'")
		}
		p.i++
		return n, nil
	case "-true":
		return constant(true), nil
	case "-false":
		return constant(false), nil
	case "-print", "-print0":
		p.hasAction = true
		term := byte('\n')
		if tok == "-print0" {
			term = 0
		}
		return func(v *visit) (bool, error) { return true, v.emit(v.Path, term) }, nil
	case "-prune":
		return func(v *visit) (bool, error) { v.prune = true; return true, nil }, nil
	case "-quit":
		return func(v *visit) (bool, error) { v.quit = true; return true, nil }, nil
	case "-empty":
		return func(v *visit) (bool, error) {
			switch {
			case v.Info.Mode().IsRegular():
				return v.Info.Size() == 0, nil
			case v.Info.IsDir():
				f, err := os.Open(v.Path)
				if err != nil {
					return false, err
				}
				defer f.Close()
				names, err := f.Readdirnames(1)
				if len(names) == 0 && (err == nil || errors.Is(err, io.EOF)) {
					return true, nil
				}
				return false, nil
			}
			return false, nil
		}, nil
	case "-maxdepth", "-mindepth":
		s, err := p.value(tok)
		if err != nil {
			return nil, err
		}
		n, err := strconv.Atoi(s)
		if err != nil || n < 0 {
			return nil, fmt.Errorf("invalid argument %q to %s", s, tok)
		}
		if tok == "-maxdepth" {
			p.expr.maxDepth = n
		} else {
			p.expr.minDepth = n
		}
		return constant(true), nil
	}
	arg, err := p.value(tok)
	if err != nil {
		return nil, err
	}
	switch tok {
	case "-name", "-iname", "-path", "-ipath":
		re, err := glob(arg, strings.HasPrefix(tok, "-i"))
		if err != nil {
			return nil, err
		}
		whole := strings.HasSuffix(tok, "path")
		return func(v *visit) (bool, error) {
			s := v.Path
			if !whole {
				s = baseName(s)
			}
			return re.MatchString(s), nil
		}, nil
	case "-type":
		var modes []func(fs.FileMode) bool
		for _, t := range strings.Split(arg, ",") {
			check, ok := map[string]func(fs.FileMode) bool{
				"f": fs.FileMode.IsRegular,
				"d": fs.FileMode.IsDir,
				"l": func(m fs.FileMode) bool { return m&fs.ModeSymlink != 0 },
				"p": func(m fs.FileMode) bool { return m&fs.ModeNamedPipe != 0 },
				"s": func(m fs.FileMode) bool { return m&fs.ModeSocket != 0 },
				"c": func(m fs.FileMode) bool { return m&fs.ModeCharDevice != 0 },
				"b": func(m fs.FileMode) bool { return m&fs.ModeDevice != 0 && m&fs.ModeCharDevice == 0 },
			}[t]
			if !ok {
				return nil, fmt.Errorf("unknown argument %q to -type", t)
			}
			modes = append(modes, check)
		}
		return func(v *visit) (bool, error) {
			for _, m := range modes {
				if m(v.Info.Mode()) {
					return true, nil
				}
			}
			return false, nil
		}, nil
	case "-size":
		unit := int64(512)
		num := arg
		if n := len(arg); n > 0 {
			if u, ok := map[byte]int64{'c': 1, 'k': 1 << 10, 'M': 1 << 20, 'G': 1 << 30, 'b': 512}[arg[n-1]]; ok {
				num, unit = arg[:n-1], u
			}
		}
		cmp, err := numeric(tok, num)
		if err != nil {
			return nil, err
		}
		return func(v *visit) (bool, error) {
			size := v.Info.Size()
			return cmp((size + unit - 1) / unit), nil
		}, nil
	case "-mtime", "-mmin":
		cmp, err := numeric(tok, arg)
		if err != nil {
			return nil, err
		}
		unit := 24 * time.Hour
		if tok == "-mmin" {
			unit = time.Minute
		}
		now := p.now
		return func(v *visit) (bool, error) {
			age := now.Sub(v.Info.ModTime())
			return cmp(int64(math.Floor(float64(age) / float64(unit)))), nil
		}, nil
	case "-newer":
		info, err := os.Stat(arg)
		if err != nil {
			return nil, err
		}
		ref := info.ModTime()
		return func(v *visit) (bool, error) { return v.Info.ModTime().After(ref), nil }, nil
	}
	return nil, fmt.Errorf("unknown predicate %q", tok)
}

func baseName(p string) string {
	trimmed := strings.TrimRight(p, `/`+string(filepath.Separator))
	if trimmed == "" {
		return p[:1]
	}
	return filepath.Base(trimmed)
}

// glob converts an fnmatch pattern (without FNM_PATHNAME, so * matches '/')
// to an anchored regular expression.
func glob(pattern string, fold bool) (*regexp.Regexp, error) {
	var b strings.Builder
	b.WriteString("(?s)^")
	if fold {
		b.WriteString("(?i)")
	}
	for i := 0; i < len(pattern); i++ {
		switch c := pattern[i]; c {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteString(".")
		case '\\':
			if i+1 < len(pattern) {
				i++
			}
			b.WriteString(regexp.QuoteMeta(pattern[i : i+1]))
		case '[':
			j := bracketEnd(pattern, i)
			if j < 0 {
				b.WriteString(`\[`)
				continue
			}
			body := pattern[i+1 : j]
			if strings.HasPrefix(body, "!") {
				body = "^" + body[1:]
			}
			b.WriteString("[" + strings.ReplaceAll(body, `\`, `\\`) + "]")
			i = j
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	b.WriteString("$")
	return regexp.Compile(b.String())
}

// Walk visits each root and, below directories, their entries in name order
// without following symlinks. emit receives the paths selected by -print
// (terminator '\n') and -print0 (terminator 0). Errors for individual paths
// are collected and walking continues; errors from emit and cancellation stop
// it.
func (e *Expression) Walk(ctx context.Context, roots []string, emit func(path string, terminator byte) error) error {
	var errs []error
	stop := errors.New("stop")
	var emitErr error
	output := func(path string, terminator byte) error {
		if err := emit(path, terminator); err != nil {
			emitErr = err
			return err
		}
		return nil
	}
	var walk func(path string, info fs.FileInfo, depth int) error
	walk = func(path string, info fs.FileInfo, depth int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		v := &visit{Entry: Entry{Path: path, Info: info, Depth: depth}, emit: output}
		if depth >= e.minDepth {
			if _, err := e.root(v); err != nil {
				if emitErr != nil {
					return emitErr
				}
				errs = append(errs, fmt.Errorf("%s: %w", path, err))
			}
			if v.quit {
				return stop
			}
		}
		if !info.IsDir() || v.prune || e.maxDepth >= 0 && depth >= e.maxDepth {
			return nil
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			errs = append(errs, err)
		}
		for _, d := range entries {
			child := path + string(filepath.Separator) + d.Name()
			if strings.HasSuffix(path, "/") || strings.HasSuffix(path, string(filepath.Separator)) {
				child = path + d.Name()
			}
			info, err := d.Info()
			if err != nil {
				errs = append(errs, err)
				continue
			}
			if err := walk(child, info, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	for _, root := range roots {
		info, err := os.Lstat(root)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if err := walk(root, info, 0); err != nil {
			if err == stop {
				break
			}
			return errors.Join(append(errs, err)...)
		}
	}
	return errors.Join(errs...)
}
