package search

import (
	"strings"
	"unicode"
)

const (
	esc     = '\x1b'
	bel     = '\x07'
	c1CSI   = '\u009b'
	c1OSC   = '\u009d'
	csiLow  = 0x40 // CSI final bytes are 0x40–0x7E
	csiHigh = 0x7e
)

// SanitizeText makes registry-supplied text safe to print on a terminal.
// Invalid UTF-8 becomes U+FFFD; ANSI escape sequences (CSI, OSC, DCS and
// the other ESC-introduced string sequences, plus their 8-bit C1 forms) are
// removed whole, an unterminated one up to the end of the text; \n, \r and
// \t each become one space; every other control character is dropped. The
// result is trimmed of surrounding whitespace.
func SanitizeText(s string) string {
	rs := []rune(strings.ToValidUTF8(s, "�"))
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		switch {
		case r == esc:
			if i+1 >= len(rs) {
				continue
			}
			i++
			switch rs[i] {
			case '[':
				i = skipCSI(rs, i+1)
			case ']':
				i = skipString(rs, i+1, true)
			case 'P', 'X', '^', '_':
				i = skipString(rs, i+1, false)
			}
			// Any other rune after ESC is dropped together with it.
		case r == c1CSI:
			i = skipCSI(rs, i+1)
		case r == c1OSC:
			i = skipString(rs, i+1, true)
		case r == '\n' || r == '\r' || r == '\t':
			b.WriteByte(' ')
		case unicode.IsControl(r):
			// dropped
		default:
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

// skipCSI returns the index of the final byte of the CSI sequence whose
// parameters start at i, or the last index when it is unterminated.
func skipCSI(rs []rune, i int) int {
	for ; i < len(rs); i++ {
		if rs[i] >= csiLow && rs[i] <= csiHigh {
			return i
		}
	}
	return len(rs) - 1
}

// skipString returns the index of the last rune of the terminator (ESC \,
// or BEL when belEnds) of the string sequence whose body starts at i, or
// the last index when it is unterminated.
func skipString(rs []rune, i int, belEnds bool) int {
	for ; i < len(rs); i++ {
		if (belEnds && rs[i] == bel) || rs[i] == '\u009c' {
			return i
		}
		if rs[i] == esc && i+1 < len(rs) && rs[i+1] == '\\' {
			return i + 1
		}
	}
	return len(rs) - 1
}

// sanitizeResult returns r with its registry-supplied text sanitized. Extra
// is copied, never modified, because the lookup cache may share it.
func sanitizeResult(r Result) Result {
	r.Name = SanitizeText(r.Name)
	r.Info = SanitizeText(r.Info)
	if r.Extra != nil {
		extra := make(map[string]string, len(r.Extra))
		for k, v := range r.Extra {
			extra[k] = SanitizeText(v)
		}
		r.Extra = extra
	}
	return r
}

// sanitizedError presents a registry error with its text sanitized (error
// strings can carry response bodies) while still wrapping the original for
// errors.Is and errors.As.
type sanitizedError struct{ err error }

func (e sanitizedError) Error() string { return SanitizeText(e.err.Error()) }
func (e sanitizedError) Unwrap() error { return e.err }
