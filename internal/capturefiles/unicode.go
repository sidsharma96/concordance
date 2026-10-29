package capturefiles

import "context"

// encoding/json replaces lone UTF-16 surrogates with U+FFFD. Reject malformed
// escaped pairs before that repair, while allowing real U+FFFD and valid pairs.
// This scan does not parse JSON grammar; the standard decoder still does that.
func surrogates(ctx context.Context, b []byte) error {
	inside := false
	for i := 0; i < len(b); i++ {
		if i%1024 == 0 {
			if e := ctx.Err(); e != nil {
				return e
			}
		}
		if b[i] == '"' {
			inside = !inside
			continue
		}
		if !inside || b[i] != '\\' {
			continue
		}
		i++
		if i >= len(b) {
			return ErrInvalid
		}
		if b[i] != 'u' {
			continue
		}
		u, ok := hexUnit(b, i+1)
		if !ok {
			return ErrInvalid
		}
		i += 4
		if u >= 0xdc00 && u <= 0xdfff {
			return ErrInvalid
		}
		if u >= 0xd800 && u <= 0xdbff {
			if i+2 >= len(b) || b[i+1] != '\\' || b[i+2] != 'u' {
				return ErrInvalid
			}
			low, ok := hexUnit(b, i+3)
			if !ok || low < 0xdc00 || low > 0xdfff {
				return ErrInvalid
			}
			i += 6
		}
	}
	return ctx.Err()
}
func hexUnit(b []byte, first int) (uint16, bool) {
	if first > len(b)-4 {
		return 0, false
	}
	var value uint16
	for _, c := range b[first : first+4] {
		value <<= 4
		switch {
		case c >= '0' && c <= '9':
			value |= uint16(c - '0')
		case c >= 'a' && c <= 'f':
			value |= uint16(c - 'a' + 10)
		case c >= 'A' && c <= 'F':
			value |= uint16(c - 'A' + 10)
		default:
			return 0, false
		}
	}
	return value, true
}
