// Package format implements printf(1)-style formatting, echo escapes, and
// strftime-style date formatting.
package format

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

var simple = map[byte]byte{'\\': '\\', 'a': 7, 'b': 8, 'e': 27, 'f': 12, 'n': 10, 'r': 13, 't': 9, 'v': 11, '"': '"'}

func isOctal(c byte) bool { return c >= '0' && c <= '7' }
func isHex(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}

// digits parses up to limit digits of base starting at s[i].
func digits(s string, i, limit, base int) (byte, int) {
	j := i
	v := 0
	for ; j < len(s) && j < i+limit; j++ {
		d, err := strconv.ParseUint(s[j:j+1], base, 8)
		if err != nil {
			break
		}
		v = v*base + int(d)
	}
	return byte(v), j
}

// escape decodes the backslash escape at s[i] (s[i] == '\\'). With zeroOctal,
// octal must start with 0 and take up to 3 more digits (echo, %b); otherwise
// 1-3 octal digits follow directly (printf format). stop reports \c.
func escape(s string, i int, zeroOctal bool) (out string, next int, stop bool) {
	if i+1 >= len(s) {
		return `\`, i + 1, false
	}
	c := s[i+1]
	if v, ok := simple[c]; ok && (c != '"' || !zeroOctal) {
		return string(v), i + 2, false
	}
	switch {
	case c == 'c':
		return "", i + 2, true
	case zeroOctal && c == '0':
		v, j := digits(s, i+2, 3, 8)
		return string([]byte{v}), j, false
	case !zeroOctal && isOctal(c):
		v, j := digits(s, i+1, 3, 8)
		return string([]byte{v}), j, false
	case c == 'x' && i+2 < len(s) && isHex(s[i+2]):
		v, j := digits(s, i+2, 2, 16)
		return string([]byte{v}), j, false
	}
	return s[i : i+2], i + 2, false
}

// Escapes expands echo -e and printf %b escapes: \\ \a \b \c \e \f \n \r \t
// \v \0NNN \xHH. stop reports that \c ended output early.
func Escapes(s string) (string, bool) {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] != '\\' {
			b.WriteByte(s[i])
			i++
			continue
		}
		out, next, stop := escape(s, i, true)
		if stop {
			return b.String(), true
		}
		b.WriteString(out)
		i = next
	}
	return b.String(), false
}

var errInvalid = errors.New("invalid number")

func parseInt(s string) (int64, error) {
	s = strings.TrimLeft(s, " \t")
	if s == "" {
		return 0, nil
	}
	if s[0] == '\'' || s[0] == '"' {
		if len(s) == 1 {
			return 0, nil
		}
		r, _ := utf8.DecodeRuneInString(s[1:])
		return int64(r), nil
	}
	body := strings.TrimLeft(s, "+-")
	if strings.ContainsAny(body, "_") || len(body) > 1 && body[0] == '0' && strings.ContainsAny(body[1:2], "bBoO") {
		return 0, fmt.Errorf("%q: %w", s, errInvalid)
	}
	if v, err := strconv.ParseInt(s, 0, 64); err == nil {
		return v, nil
	}
	if v, err := strconv.ParseUint(strings.TrimPrefix(s, "+"), 0, 64); err == nil {
		return int64(v), nil
	}
	return 0, fmt.Errorf("%q: %w", s, errInvalid)
}

func parseFloat(s string) (float64, error) {
	t := strings.TrimLeft(s, " \t")
	if t == "" {
		return 0, nil
	}
	if t[0] == '\'' || t[0] == '"' {
		v, err := parseInt(t)
		return float64(v), err
	}
	v, err := strconv.ParseFloat(t, 64)
	if err != nil {
		return 0, fmt.Errorf("%q: %w", s, errInvalid)
	}
	return v, nil
}

// Printf formats args like printf(1). The format is reused while arguments
// remain; missing arguments are empty strings or zero. Invalid numbers print
// as 0 and are reported in the returned error alongside the full output.
// Formatting stops at \c in a %b argument or the format.
func Printf(format string, args []string) (string, error) {
	var b strings.Builder
	var errs []error
	used := 0
	next := func() (string, bool) {
		if used >= len(args) {
			return "", false
		}
		used++
		return args[used-1], true
	}
	for {
		start := used
		for i := 0; i < len(format); {
			c := format[i]
			if c == '\\' {
				out, n, stop := escape(format, i, false)
				if stop {
					return b.String(), errors.Join(errs...)
				}
				b.WriteString(out)
				i = n
				continue
			}
			if c != '%' {
				b.WriteByte(c)
				i++
				continue
			}
			if i+1 < len(format) && format[i+1] == '%' {
				b.WriteByte('%')
				i += 2
				continue
			}
			j := i + 1
			for j < len(format) && strings.IndexByte("-+ #0", format[j]) >= 0 {
				j++
			}
			flags := format[i+1 : j]
			star := func() string {
				arg, _ := next()
				n, err := parseInt(arg)
				if err != nil {
					errs = append(errs, err)
				}
				return strconv.FormatInt(n, 10)
			}
			width := ""
			if j < len(format) && format[j] == '*' {
				width, j = star(), j+1
				if strings.HasPrefix(width, "-") {
					flags, width = flags+"-", width[1:]
				}
			} else {
				k := j
				for j < len(format) && format[j] >= '0' && format[j] <= '9' {
					j++
				}
				width = format[k:j]
			}
			precision, hasPrecision := "", false
			if j < len(format) && format[j] == '.' {
				hasPrecision = true
				j++
				if j < len(format) && format[j] == '*' {
					precision, j = star(), j+1
					if strings.HasPrefix(precision, "-") {
						precision, hasPrecision = "", false
					}
				} else {
					k := j
					for j < len(format) && format[j] >= '0' && format[j] <= '9' {
						j++
					}
					precision = format[k:j]
					if precision == "" {
						precision = "0"
					}
				}
			}
			if j >= len(format) {
				return b.String(), errors.Join(append(errs, fmt.Errorf("missing conversion in %q", format[i:]))...)
			}
			verb := format[j]
			spec := "%" + flags + width
			if hasPrecision {
				spec += "." + precision
			}
			arg, _ := next()
			switch verb {
			case 'd', 'i':
				n, err := parseInt(arg)
				if err != nil {
					errs = append(errs, err)
				}
				fmt.Fprintf(&b, spec+"d", n)
			case 'u', 'o', 'x', 'X':
				n, err := parseInt(arg)
				if err != nil {
					errs = append(errs, err)
				}
				v := map[byte]string{'u': "d", 'o': "o", 'x': "x", 'X': "X"}[verb]
				fmt.Fprintf(&b, spec+v, uint64(n))
			case 'f', 'F', 'e', 'E', 'g', 'G':
				f, err := parseFloat(arg)
				if err != nil {
					errs = append(errs, err)
				}
				if !hasPrecision && (verb == 'g' || verb == 'G') {
					spec += ".6"
				}
				v := string(verb)
				if verb == 'F' {
					v = "f"
				}
				s := fmt.Sprintf(spec+v, f)
				if math.IsInf(f, 0) || math.IsNaN(f) {
					s = strings.NewReplacer("+Inf", "inf", "-Inf", "-inf", "Inf", "inf", "NaN", "nan").Replace(s)
					if verb == 'F' || verb == 'E' || verb == 'G' {
						s = strings.ToUpper(s)
					}
				}
				b.WriteString(s)
			case 'c':
				if arg != "" {
					arg = arg[:1]
				}
				fmt.Fprintf(&b, "%"+flags+width+"s", arg)
			case 's':
				fmt.Fprintf(&b, spec+"s", arg)
			case 'b':
				expanded, stop := Escapes(arg)
				fmt.Fprintf(&b, spec+"s", expanded)
				if stop {
					return b.String(), errors.Join(errs...)
				}
			default:
				return b.String(), errors.Join(append(errs, fmt.Errorf("invalid conversion %q", format[i:j+1]))...)
			}
			i = j + 1
		}
		if used == start || used >= len(args) {
			return b.String(), errors.Join(errs...)
		}
	}
}

// DefaultDateFormat is date(1)'s default output format in the C locale.
const DefaultDateFormat = "%a %b %e %H:%M:%S %Z %Y"

// Strftime formats t with strftime-style conversions. Supported: %a %A %b
// %B %c %C %d %D %e %F %g %G %h %H %I %j %k %l %m %M %n %N %p %P %r %R %s
// %S %t %T %u %U %V %w %W %x %X %y %Y %z %:z %Z %%. The flags '-' (no
// padding), '_' (space padding), and '0' (zero padding) may follow '%'.
// Unknown conversions are copied unchanged. Names are English.
func Strftime(t time.Time, layout string) string {
	var b strings.Builder
	for i := 0; i < len(layout); i++ {
		if layout[i] != '%' || i+1 >= len(layout) {
			b.WriteByte(layout[i])
			continue
		}
		start := i
		i++
		pad := byte(0)
		if strings.IndexByte("-_0", layout[i]) >= 0 && i+1 < len(layout) {
			pad = layout[i]
			i++
		}
		colon := layout[i] == ':' && i+1 < len(layout) && layout[i+1] == 'z'
		if colon {
			i++
		}
		num := func(v, width int, def byte) string {
			p := def
			if pad != 0 {
				p = pad
			}
			s := strconv.Itoa(v)
			if p == '-' {
				return s
			}
			fill := "0"
			if p == '_' {
				fill = " "
			}
			for len(s) < width {
				s = fill + s
			}
			return s
		}
		hour12 := t.Hour() % 12
		if hour12 == 0 {
			hour12 = 12
		}
		isoYear, isoWeek := t.ISOWeek()
		yday := t.YearDay() - 1
		wday := int(t.Weekday())
		var s string
		switch layout[i] {
		case 'a':
			s = t.Weekday().String()[:3]
		case 'A':
			s = t.Weekday().String()
		case 'b', 'h':
			s = t.Month().String()[:3]
		case 'B':
			s = t.Month().String()
		case 'c':
			s = Strftime(t, "%a %b %e %H:%M:%S %Y")
		case 'C':
			s = num(t.Year()/100, 2, '0')
		case 'd':
			s = num(t.Day(), 2, '0')
		case 'D', 'x':
			s = Strftime(t, "%m/%d/%y")
		case 'e':
			s = num(t.Day(), 2, '_')
		case 'F':
			s = Strftime(t, "%Y-%m-%d")
		case 'g':
			s = num(isoYear%100, 2, '0')
		case 'G':
			s = num(isoYear, 4, '0')
		case 'H':
			s = num(t.Hour(), 2, '0')
		case 'I':
			s = num(hour12, 2, '0')
		case 'j':
			s = num(t.YearDay(), 3, '0')
		case 'k':
			s = num(t.Hour(), 2, '_')
		case 'l':
			s = num(hour12, 2, '_')
		case 'm':
			s = num(int(t.Month()), 2, '0')
		case 'M':
			s = num(t.Minute(), 2, '0')
		case 'n':
			s = "\n"
		case 'N':
			s = num(t.Nanosecond(), 9, '0')
		case 'p':
			s = map[bool]string{true: "PM", false: "AM"}[t.Hour() >= 12]
		case 'P':
			s = map[bool]string{true: "pm", false: "am"}[t.Hour() >= 12]
		case 'r':
			s = Strftime(t, "%I:%M:%S %p")
		case 'R':
			s = Strftime(t, "%H:%M")
		case 's':
			s = strconv.FormatInt(t.Unix(), 10)
		case 'S':
			s = num(t.Second(), 2, '0')
		case 't':
			s = "\t"
		case 'T', 'X':
			s = Strftime(t, "%H:%M:%S")
		case 'u':
			s = strconv.Itoa((wday+6)%7 + 1)
		case 'U':
			s = num((yday+7-wday)/7, 2, '0')
		case 'V':
			s = num(isoWeek, 2, '0')
		case 'w':
			s = strconv.Itoa(wday)
		case 'W':
			s = num((yday+7-(wday+6)%7)/7, 2, '0')
		case 'y':
			s = num(t.Year()%100, 2, '0')
		case 'Y':
			s = num(t.Year(), 4, '0')
		case 'z':
			if colon {
				s = t.Format("-07:00")
			} else {
				s = t.Format("-0700")
			}
		case 'Z':
			s = t.Format("MST")
		case '%':
			s = "%"
		default:
			s = layout[start : i+1]
		}
		b.WriteString(s)
	}
	return b.String()
}

var dateLayouts = []string{
	time.RFC3339Nano,
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05.999999999 -0700",
	"2006-01-02 15:04:05 -0700",
	"2006-01-02 15:04:05",
	"2006-01-02 15:04",
	"2006-01-02T15:04",
	"2006-01-02",
	time.RFC1123Z,
	time.RFC1123,
	time.UnixDate,
	time.ANSIC,
}

// ParseDate parses "now", "@<unix seconds>", RFC 3339, "YYYY-MM-DD[ HH:MM[:SS]]",
// RFC 1123, and date(1)'s default output. Times without a zone are read in
// loc; the result is always expressed in loc.
func ParseDate(s string, now time.Time, loc *time.Location) (time.Time, error) {
	s = strings.TrimSpace(s)
	switch {
	case s == "now" || s == "":
		return now.In(loc), nil
	case strings.HasPrefix(s, "@"):
		secs, frac, _ := strings.Cut(s[1:], ".")
		sec, err := strconv.ParseInt(secs, 10, 64)
		if err != nil {
			break
		}
		var nsec int64
		if frac != "" {
			frac = (frac + "000000000")[:9]
			if nsec, err = strconv.ParseInt(frac, 10, 64); err != nil {
				break
			}
			if sec < 0 || strings.HasPrefix(secs, "-") {
				nsec = -nsec
			}
		}
		return time.Unix(sec, nsec).In(loc), nil
	}
	for _, layout := range dateLayouts {
		if t, err := time.ParseInLocation(layout, s, loc); err == nil {
			return t.In(loc), nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid date %q", s)
}
