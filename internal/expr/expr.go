// Package expr evaluates POSIX expr(1) expressions.
package expr

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/pkar/uniz/internal/search"
)

// ErrSyntax reports a malformed expression.
var ErrSyntax = errors.New("syntax error")

// IsNull reports whether an expr result counts as false: empty or an integer zero.
func IsNull(v string) bool {
	if v == "" {
		return true
	}
	n, ok := toInt(v)
	return ok && n == 0
}

func toInt(s string) (int64, bool) {
	digits := strings.TrimPrefix(s, "-")
	if digits == "" || strings.Trim(digits, "0123456789") != "" {
		return 0, false
	}
	n, err := strconv.ParseInt(s, 10, 64)
	return n, err == nil
}

type parser struct {
	args []string
	pos  int
}

func (p *parser) peek() (string, bool) {
	if p.pos < len(p.args) {
		return p.args[p.pos], true
	}
	return "", false
}

func (p *parser) accept(ops ...string) (string, bool) {
	if t, ok := p.peek(); ok {
		for _, op := range ops {
			if t == op {
				p.pos++
				return op, true
			}
		}
	}
	return "", false
}

// Evaluate evaluates expr(1) arguments, lowest precedence first:
//
//	A | B   A if not null, else B if not null, else 0
//	A & B   A if neither is null, else 0
//	= == != < <= > >=   numeric if both are integers, else bytewise; 1 or 0
//	+ - * / %           64-bit integer arithmetic
//	S : RE  anchored basic regexp; the first \(group\) or the match length
//	match S RE, substr S POS LEN, index S CHARS, length S, + TOKEN, ( A )
//
// Positions and lengths count characters.
func Evaluate(args []string) (string, error) {
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	if len(args) == 0 {
		return "", fmt.Errorf("%w: missing operand", ErrSyntax)
	}
	p := &parser{args: args}
	v, err := p.or()
	if err != nil {
		return "", err
	}
	if t, ok := p.peek(); ok {
		return "", fmt.Errorf("%w: unexpected argument %q", ErrSyntax, t)
	}
	return v, nil
}

func (p *parser) or() (string, error) {
	l, err := p.and()
	for err == nil {
		if _, ok := p.accept("|"); !ok {
			break
		}
		var r string
		if r, err = p.and(); err != nil {
			break
		}
		switch {
		case !IsNull(l):
		case !IsNull(r):
			l = r
		default:
			l = "0"
		}
	}
	return l, err
}

func (p *parser) and() (string, error) {
	l, err := p.compare()
	for err == nil {
		if _, ok := p.accept("&"); !ok {
			break
		}
		var r string
		if r, err = p.compare(); err != nil {
			break
		}
		if IsNull(l) || IsNull(r) {
			l = "0"
		}
	}
	return l, err
}

func (p *parser) compare() (string, error) {
	l, err := p.additive()
	for err == nil {
		op, ok := p.accept("=", "==", "!=", "<", "<=", ">", ">=")
		if !ok {
			break
		}
		var r string
		if r, err = p.additive(); err != nil {
			break
		}
		var c int
		ln, lok := toInt(l)
		rn, rok := toInt(r)
		if lok && rok {
			c = map[bool]int{true: -1, false: 0}[ln < rn] + map[bool]int{true: 1, false: 0}[ln > rn]
		} else {
			c = strings.Compare(l, r)
		}
		result := map[string]bool{"=": c == 0, "==": c == 0, "!=": c != 0, "<": c < 0, "<=": c <= 0, ">": c > 0, ">=": c >= 0}[op]
		l = map[bool]string{true: "1", false: "0"}[result]
	}
	return l, err
}

func arith(op, l, r string) (string, error) {
	a, aok := toInt(l)
	b, bok := toInt(r)
	if !aok || !bok {
		return "", errors.New("non-integer argument")
	}
	var v int64
	switch op {
	case "+":
		v = a + b
		if (b > 0 && v < a) || (b < 0 && v > a) {
			return "", errors.New("integer overflow")
		}
	case "-":
		v = a - b
		if (b < 0 && v < a) || (b > 0 && v > a) {
			return "", errors.New("integer overflow")
		}
	case "*":
		v = a * b
		if a != 0 && (v/a != b || (a == -1 && b == math.MinInt64)) {
			return "", errors.New("integer overflow")
		}
	default:
		if b == 0 {
			return "", errors.New("division by zero")
		}
		if a == math.MinInt64 && b == -1 {
			return "", errors.New("integer overflow")
		}
		if op == "/" {
			v = a / b
		} else {
			v = a % b
		}
	}
	return strconv.FormatInt(v, 10), nil
}

func (p *parser) additive() (string, error) {
	l, err := p.multiplicative()
	for err == nil {
		op, ok := p.accept("+", "-")
		if !ok {
			break
		}
		var r string
		if r, err = p.multiplicative(); err == nil {
			l, err = arith(op, l, r)
		}
	}
	return l, err
}

func (p *parser) multiplicative() (string, error) {
	l, err := p.match()
	for err == nil {
		op, ok := p.accept("*", "/", "%")
		if !ok {
			break
		}
		var r string
		if r, err = p.match(); err == nil {
			l, err = arith(op, l, r)
		}
	}
	return l, err
}

// Match applies the anchored basic regexp re to s like expr's ":".
func Match(s, re string) (string, error) {
	m, err := search.CompileBRE(re)
	if err != nil {
		return "", err
	}
	loc := m.FindStringSubmatchIndex(s)
	if m.NumSubexp() > 0 {
		if loc == nil || loc[0] != 0 || loc[2] < 0 {
			return "", nil
		}
		return s[loc[2]:loc[3]], nil
	}
	if loc == nil || loc[0] != 0 {
		return "0", nil
	}
	return strconv.Itoa(utf8.RuneCountInString(s[:loc[1]])), nil
}

func (p *parser) match() (string, error) {
	l, err := p.unary()
	for err == nil {
		if _, ok := p.accept(":"); !ok {
			break
		}
		var r string
		if r, err = p.unary(); err == nil {
			l, err = Match(l, r)
		}
	}
	return l, err
}

func (p *parser) operand() (string, error) {
	t, ok := p.peek()
	if !ok {
		return "", fmt.Errorf("%w: missing argument", ErrSyntax)
	}
	p.pos++
	return t, nil
}

func (p *parser) unary() (string, error) {
	t, ok := p.peek()
	if !ok {
		return "", fmt.Errorf("%w: missing argument", ErrSyntax)
	}
	switch t {
	case "(":
		p.pos++
		v, err := p.or()
		if err != nil {
			return "", err
		}
		if _, ok := p.accept(")"); !ok {
			return "", fmt.Errorf("%w: expected )", ErrSyntax)
		}
		return v, nil
	case "+":
		p.pos++
		return p.operand()
	case "length":
		p.pos++
		s, err := p.unary()
		return strconv.Itoa(utf8.RuneCountInString(s)), err
	case "match", "index":
		p.pos++
		s, err := p.unary()
		if err != nil {
			return "", err
		}
		r, err := p.unary()
		if err != nil {
			return "", err
		}
		if t == "match" {
			return Match(s, r)
		}
		for i, c := range []rune(s) {
			if strings.ContainsRune(r, c) {
				return strconv.Itoa(i + 1), nil
			}
		}
		return "0", nil
	case "substr":
		p.pos++
		var vals [3]string
		for i := range vals {
			v, err := p.unary()
			if err != nil {
				return "", err
			}
			vals[i] = v
		}
		rs := []rune(vals[0])
		pos, pok := toInt(vals[1])
		n, nok := toInt(vals[2])
		if !pok || !nok || pos < 1 || n < 1 || pos > int64(len(rs)) {
			return "", nil
		}
		end := min(int64(len(rs)), pos-1+n)
		return string(rs[pos-1 : end]), nil
	case ")":
		return "", fmt.Errorf("%w: unexpected )", ErrSyntax)
	}
	return p.operand()
}
