// Package rerank binds complete candidate text to one local scorer exchange.
// Retrieval, authorization, citations and display selection remain with callers.
package rerank

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"hash"
	"strings"
	"unicode/utf8"
)

const (
	Protocol       = "concordance.local-rerank/v1"
	InputVersion   = "headings-notices-complete-body/v1"
	Model          = "BAAI/bge-reranker-v2-m3"
	Revision       = "953dc6f6f85a1b2dbfca4c34a2796e7dde08d41e"
	Runtime        = "python3.14.2/torch2.15.0/transformers5.22.1/mps-f32-sdpa-b1-t2-1/v1"
	MaxPairs       = 100
	MaxQueryBytes  = 4096
	MaxTextBytes   = 128 << 10
	MaxPairTokens  = 8192
	MaxTotalTokens = 32768
	maxRequest     = 16 << 20
	maxGreeting    = 16 << 10
	maxResponse    = 64 << 10
)

var (
	ErrInvalid     = errors.New("invalid reranking input or worker protocol")
	ErrLimit       = errors.New("reranking work limit exceeded")
	ErrBusy        = errors.New("local reranking worker is busy")
	ErrWorker      = errors.New("local reranking worker failed or is incompatible")
	ErrUnsupported = errors.New("local reranking socket is unsupported on this platform")
)

// Input borrows its strings/slices only through Prepare; they must stay unchanged.
// Context is separate from Body, the verbatim quote. Prepare owns formatted text.
type Input struct {
	Body             string
	Headings, Labels []string
}
type pair struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

// Batch owns immutable query/pair state. Copies share private read-only storage.
// No reader handle or citation is transported. The caller retains those in order.
type Batch struct {
	query, fingerprint string
	pairs              []pair
}

func (b Batch) Len() int      { return len(b.pairs) }
func validText(s string) bool { return utf8.ValidString(s) && !strings.ContainsRune(s, 0) }
func ready(ctx context.Context) error {
	if ctx == nil {
		return ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, ok := ctx.Deadline(); !ok {
		return ErrInvalid
	}
	return nil
}
func field(h hash.Hash, s string) {
	var n [4]byte
	binary.BigEndian.PutUint32(n[:], uint32(len(s)))
	h.Write(n[:])
	h.Write([]byte(s))
}
func pairID(i int, q, text string) string {
	h := sha256.New()
	h.Write([]byte("concordance.rerank-pair/v1\x00"))
	var n [4]byte
	binary.BigEndian.PutUint32(n[:], uint32(i))
	h.Write(n[:])
	field(h, q)
	field(h, text)
	return hex.EncodeToString(h.Sum(nil))
}
func fingerprint(q string, pairs []pair) string {
	h := sha256.New()
	h.Write([]byte("concordance.rerank-input/v1\x00"))
	field(h, InputVersion)
	field(h, Revision)
	field(h, q)
	var n [4]byte
	binary.BigEndian.PutUint32(n[:], uint32(len(pairs)))
	h.Write(n[:])
	for _, p := range pairs {
		id, _ := hex.DecodeString(p.ID)
		h.Write(id)
		field(h, p.Text)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Prepare preserves the evaluated Heading/Notice/Body serialization exactly.
// It admits counts and every text byte before construction. Empty input is valid
// and requires no scorer. Work/storage are O(total formatted bytes + pair count),
// excluding subsequent JSON encoding and model tokenization. Limits are not heap quotas.
func Prepare(ctx context.Context, query string, inputs []Input) (Batch, error) {
	if err := ready(ctx); err != nil {
		return Batch{}, err
	}
	if !validText(query) || query == "" {
		return Batch{}, ErrInvalid
	}
	if len(query) > MaxQueryBytes || len(inputs) > MaxPairs {
		return Batch{}, ErrLimit
	}
	out := Batch{query: strings.Clone(query), pairs: make([]pair, 0, len(inputs))}
	for i, input := range inputs {
		if err := ctx.Err(); err != nil {
			return Batch{}, err
		}
		n := len("Body:\n")
		if !validText(input.Body) {
			return Batch{}, ErrInvalid
		}
		if len(input.Body) > MaxTextBytes-n {
			return Batch{}, ErrLimit
		}
		n += len(input.Body)
		for j, field := range [][]string{input.Headings, input.Labels} {
			prefix := len("Heading: ")
			if j == 1 {
				prefix = len("Notice: ")
			}
			for _, s := range field {
				if !validText(s) {
					return Batch{}, ErrInvalid
				}
				if prefix+1 > MaxTextBytes-n || len(s) > MaxTextBytes-n-prefix-1 {
					return Batch{}, ErrLimit
				}
				n += prefix + len(s) + 1
			}
		}
		var text strings.Builder
		text.Grow(n)
		for _, s := range input.Headings {
			text.WriteString("Heading: ")
			text.WriteString(s)
			text.WriteByte('\n')
		}
		for _, s := range input.Labels {
			text.WriteString("Notice: ")
			text.WriteString(s)
			text.WriteByte('\n')
		}
		text.WriteString("Body:\n")
		text.WriteString(input.Body)
		s := text.String()
		out.pairs = append(out.pairs, pair{pairID(i, query, s), s})
	}
	out.fingerprint = fingerprint(query, out.pairs)
	if err := ctx.Err(); err != nil {
		return Batch{}, err
	}
	return out, nil
}
