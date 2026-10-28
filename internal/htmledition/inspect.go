package htmledition

import (
	"context"
	"encoding/hex"
	"encoding/json"
)

// Inspect returns a bounded owned JSON diagnostic, not a persistence/citation
// format. A complete count export coexists with output. Each record is marshaled
// separately before the output budget check (one word's rows can be large under
// admitted posting limits). Failure returns nil, never partial JSON. Inspecting a
// checked file does not establish the authority of the expected reference.
func Inspect(ctx context.Context, e *Edition, l Limits, maxBytes int) ([]byte, error) {
	if err := ready(ctx, l); err != nil {
		return nil, err
	}
	if maxBytes <= 0 || !e.valid() {
		return nil, ErrInvalid
	}
	if err := validateLocations(ctx, e, l); err != nil {
		return nil, err
	}
	c, found, err := e.index.ExportEdition(ctx, e.id, countLimits(l))
	if err != nil {
		return nil, countError(err)
	}
	if !found {
		return nil, ErrInvalid
	}
	out := []byte{}
	appendRaw := func(s string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if len(s) > maxBytes-len(out) {
			return ErrLimit
		}
		out = append(out, s...)
		return nil
	}
	appendRecord := func(v any) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		if len(b) > maxBytes-len(out) {
			return ErrLimit
		}
		out = append(out, b...)
		return nil
	}
	header := struct {
		Format           string `json:"format"`
		Edition          string `json:"edition"`
		ArticleFormat    string `json:"article_format"`
		PassageProcessor string `json:"passage_processor"`
		FieldProcessor   string `json:"field_processor"`
		TermsVersion     string `json:"terms_version"`
		TotalTerms       int    `json:"total_terms"`
	}{Format, e.id, articleFormat, passageProcessor, fieldProcessor, termsVersion, c.TotalTerms}
	if err := appendRecord(header); err != nil {
		return nil, err
	}
	// Replace only the final object brace, then emit explicit diagnostic fields.
	out = out[:len(out)-1]
	if err := appendRaw(",\n\"articles\":["); err != nil {
		return nil, err
	}
	for i, a := range e.articles {
		if i > 0 {
			if err := appendRaw(","); err != nil {
				return nil, err
			}
		}
		row := struct {
			SHA256   string `json:"sha256"`
			Bytes    int    `json:"bytes"`
			Blocks   int    `json:"blocks"`
			Passages int    `json:"passages"`
		}{hex.EncodeToString(a.Object.SHA256[:]), a.Object.Bytes, a.Blocks, a.Passages}
		if err := appendRaw("\n"); err != nil {
			return nil, err
		}
		if err := appendRecord(row); err != nil {
			return nil, err
		}
	}
	if err := appendRaw("],\n\"passages\":["); err != nil {
		return nil, err
	}
	for i, r := range e.locations {
		if i > 0 {
			if err := appendRaw(","); err != nil {
				return nil, err
			}
		}
		row := struct {
			Article int `json:"article"`
			First   int `json:"first"`
			After   int `json:"after"`
			Length  int `json:"length"`
		}{r.Article, r.Body.First, r.Body.After, c.Lengths[i]}
		if err := appendRaw("\n"); err != nil {
			return nil, err
		}
		if err := appendRecord(row); err != nil {
			return nil, err
		}
	}
	if err := appendRaw("],\n\"words\":["); err != nil {
		return nil, err
	}
	for _, w := range c.Words {
		if len(w.Term) > l.MaxTermBytes {
			return nil, ErrLimit
		}
	}
	for i, w := range c.Words {
		if i > 0 {
			if err := appendRaw(","); err != nil {
				return nil, err
			}
		}
		rows := make([][4]int, len(w.Rows))
		for j, r := range w.Rows {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			rows[j] = [4]int{r.Passage, r.Body, r.Headings, r.Labels}
		}
		row := struct {
			Term string   `json:"term"`
			Rows [][4]int `json:"rows"`
		}{w.Term, rows}
		if err := appendRaw("\n"); err != nil {
			return nil, err
		}
		if err := appendRecord(row); err != nil {
			return nil, err
		}
	}
	if err := appendRaw("]}\n"); err != nil {
		return nil, err
	}
	return out, nil
}
