package rerank

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"unicode/utf8"

	"github.com/sidsharma96/concordance/internal/jsonunicode"
)

func readFrame(r io.Reader, max uint32) ([]byte, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(header[:])
	if n == 0 || n > max {
		return nil, ErrLimit
	}
	body := make([]byte, int(n))
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, err
	}
	return body, nil
}
func writeFrame(w io.Writer, body []byte) error {
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(body)))
	_, err := io.Copy(w, io.MultiReader(bytes.NewReader(header[:]), bytes.NewReader(body)))
	return err
}

// Check one bounded shallow JSON value before typed decoding. Field sets are
// closed by the receiver. Tokens are capped before any typed array append;
// duplicate decoded keys, null and malformed Unicode are never silently coerced.
func decode(ctx context.Context, body []byte, dst any) error {
	if !utf8.Valid(body) {
		return ErrInvalid
	}
	if err := jsonunicode.CheckSurrogates(ctx, body); err != nil {
		return ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(body))
	d.UseNumber()
	left := 2048
	var walk func(int) error
	walk = func(depth int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		left--
		if left < 0 || depth > 8 {
			return ErrLimit
		}
		token, err := d.Token()
		if err != nil {
			return ErrInvalid
		}
		if token == nil {
			return ErrInvalid
		}
		delim, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				left--
				if left < 0 {
					return ErrLimit
				}
				key, err := d.Token()
				if err != nil {
					return ErrInvalid
				}
				s, ok := key.(string)
				if !ok || seen[s] {
					return ErrInvalid
				}
				seen[s] = true
				if err := walk(depth + 1); err != nil {
					return err
				}
			}
			if end, err := d.Token(); err != nil || end != json.Delim('}') {
				return ErrInvalid
			}
		case '[':
			for d.More() {
				if err := walk(depth + 1); err != nil {
					return err
				}
			}
			if end, err := d.Token(); err != nil || end != json.Delim(']') {
				return ErrInvalid
			}
		default:
			return ErrInvalid
		}
		return nil
	}
	if err := walk(0); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return ErrInvalid
	}
	d = json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	if err := d.Decode(dst); err != nil {
		return ErrInvalid
	}
	return ctx.Err()
}

type request struct {
	Version     string `json:"version"`
	Nonce       string `json:"nonce"`
	RequestID   string `json:"request_id"`
	InputSHA256 string `json:"input_sha256"`
	Query       string `json:"query"`
	TimeoutMS   int64  `json:"timeout_ms"`
	Pairs       []pair `json:"pairs"`
}

// Encode one bounded record per pair; json.Marshal never constructs an unbounded
// whole-batch escaped intermediate before the frame limit is checked.
func encodeRequest(r request) ([]byte, error) {
	pairs := r.Pairs
	head, err := json.Marshal(struct {
		Version     string `json:"version"`
		Nonce       string `json:"nonce"`
		RequestID   string `json:"request_id"`
		InputSHA256 string `json:"input_sha256"`
		Query       string `json:"query"`
		TimeoutMS   int64  `json:"timeout_ms"`
	}{r.Version, r.Nonce, r.RequestID, r.InputSHA256, r.Query, r.TimeoutMS})
	if err != nil {
		return nil, err
	}
	body := append(head[:len(head)-1], []byte(`,"pairs":[`)...)
	for i, p := range pairs {
		wire, err := json.Marshal(p)
		if err != nil {
			return nil, err
		}
		extra := 0
		if i > 0 {
			extra = 1
		}
		if len(wire)+extra+2 > maxRequest-len(body) {
			return nil, ErrLimit
		}
		if i > 0 {
			body = append(body, ',')
		}
		body = append(body, wire...)
	}
	body = append(body, ']', '}')
	if len(body) > maxRequest {
		return nil, ErrLimit
	}
	return body, nil
}
