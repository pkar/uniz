// Package testexpr evaluates test(1) expressions.
package testexpr

import (
	"fmt"
	"io/fs"
	"os"
	"strconv"
	"strings"
)

// Evaluate returns the value of a test(1) expression, using POSIX rules for
// one to four arguments and precedence parsing (! > -a > -o, parentheses)
// beyond that. Errors indicate bad syntax or invalid integers.
func Evaluate(args []string) (bool, error) {
	switch len(args) {
	case 0:
		return false, nil
	case 1:
		return args[0] != "", nil
	case 2:
		if args[0] == "!" {
			return args[1] == "", nil
		}
		if unaryOps[args[0]] {
			return unary(args[0], args[1])
		}
		return false, fmt.Errorf("%s: unary operator expected", args[0])
	case 3:
		if binaryOps[args[1]] {
			return binary(args[0], args[1], args[2])
		}
		if args[1] == "-a" || args[1] == "-o" {
			l, r := args[0] != "", args[2] != ""
			if args[1] == "-a" {
				return l && r, nil
			}
			return l || r, nil
		}
		if args[0] == "!" {
			v, err := Evaluate(args[1:])
			return !v, err
		}
		if args[0] == "(" && args[2] == ")" {
			return args[1] != "", nil
		}
		return false, fmt.Errorf("%s: binary operator expected", args[1])
	case 4:
		if args[0] == "!" {
			v, err := Evaluate(args[1:])
			return !v, err
		}
		if args[0] == "(" && args[3] == ")" {
			return Evaluate(args[1:3])
		}
	}
	p := &parser{args: args}
	v, err := p.or()
	if err == nil && p.i < len(args) {
		err = fmt.Errorf("unexpected argument %q", args[p.i])
	}
	return v, err
}

var unaryOps = map[string]bool{}
var binaryOps = map[string]bool{}

func init() {
	for _, op := range strings.Fields("-b -c -d -e -f -g -h -k -L -n -p -r -s -S -u -w -x -z") {
		unaryOps[op] = true
	}
	for _, op := range strings.Fields("= == != < > -eq -ne -lt -le -gt -ge -nt -ot -ef") {
		binaryOps[op] = true
	}
}

type parser struct {
	args []string
	i    int
}

func (p *parser) peek(s string) bool { return p.i < len(p.args) && p.args[p.i] == s }

func (p *parser) or() (bool, error) {
	v, err := p.and()
	for err == nil && p.peek("-o") {
		p.i++
		var r bool
		r, err = p.and()
		v = v || r
	}
	return v, err
}

func (p *parser) and() (bool, error) {
	v, err := p.not()
	for err == nil && p.peek("-a") {
		p.i++
		var r bool
		r, err = p.not()
		v = v && r
	}
	return v, err
}

func (p *parser) not() (bool, error) {
	if p.peek("!") {
		p.i++
		v, err := p.not()
		return !v, err
	}
	return p.primary()
}

func (p *parser) primary() (bool, error) {
	a := p.args
	if p.i >= len(a) {
		return false, fmt.Errorf("argument expected")
	}
	if p.peek("(") {
		p.i++
		v, err := p.or()
		if err != nil {
			return false, err
		}
		if !p.peek(")") {
			return false, fmt.Errorf("missing ')'")
		}
		p.i++
		return v, nil
	}
	if p.i+2 < len(a) && binaryOps[a[p.i+1]] {
		p.i += 3
		return binary(a[p.i-3], a[p.i-2], a[p.i-1])
	}
	if unaryOps[a[p.i]] && p.i+1 < len(a) {
		p.i += 2
		return unary(a[p.i-2], a[p.i-1])
	}
	p.i++
	return a[p.i-1] != "", nil
}

func unary(op, arg string) (bool, error) {
	switch op {
	case "-n":
		return arg != "", nil
	case "-z":
		return arg == "", nil
	case "-h", "-L":
		info, err := os.Lstat(arg)
		return err == nil && info.Mode()&fs.ModeSymlink != 0, nil
	case "-r":
		return access(arg, 4), nil
	case "-w":
		return access(arg, 2), nil
	case "-x":
		return access(arg, 1), nil
	}
	info, err := os.Stat(arg)
	if err != nil {
		return false, nil
	}
	m := info.Mode()
	switch op {
	case "-b":
		return m&fs.ModeDevice != 0 && m&fs.ModeCharDevice == 0, nil
	case "-c":
		return m&fs.ModeCharDevice != 0, nil
	case "-d":
		return m.IsDir(), nil
	case "-e":
		return true, nil
	case "-f":
		return m.IsRegular(), nil
	case "-g":
		return m&fs.ModeSetgid != 0, nil
	case "-k":
		return m&fs.ModeSticky != 0, nil
	case "-p":
		return m&fs.ModeNamedPipe != 0, nil
	case "-s":
		return info.Size() > 0, nil
	case "-S":
		return m&fs.ModeSocket != 0, nil
	case "-u":
		return m&fs.ModeSetuid != 0, nil
	}
	return false, fmt.Errorf("%s: unknown unary operator", op)
}

func integer(s string) (int64, error) {
	v, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%q: integer expression expected", s)
	}
	return v, nil
}

func binary(l, op, r string) (bool, error) {
	switch op {
	case "=", "==":
		return l == r, nil
	case "!=":
		return l != r, nil
	case "<":
		return l < r, nil
	case ">":
		return l > r, nil
	case "-nt", "-ot", "-ef":
		li, lerr := os.Stat(l)
		ri, rerr := os.Stat(r)
		switch op {
		case "-nt":
			return lerr == nil && (rerr != nil || li.ModTime().After(ri.ModTime())), nil
		case "-ot":
			return rerr == nil && (lerr != nil || li.ModTime().Before(ri.ModTime())), nil
		}
		return lerr == nil && rerr == nil && os.SameFile(li, ri), nil
	}
	a, err := integer(l)
	if err != nil {
		return false, err
	}
	b, err := integer(r)
	if err != nil {
		return false, err
	}
	switch op {
	case "-eq":
		return a == b, nil
	case "-ne":
		return a != b, nil
	case "-lt":
		return a < b, nil
	case "-le":
		return a <= b, nil
	case "-gt":
		return a > b, nil
	}
	return a >= b, nil
}
