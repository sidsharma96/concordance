package importjob

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"unicode/utf8"

	"github.com/sidsharma96/concordance/internal/capturefiles"
	"github.com/sidsharma96/concordance/internal/generationmanifest"
	"github.com/sidsharma96/concordance/internal/htmlarticle"
	"github.com/sidsharma96/concordance/internal/htmlpassage"
	"github.com/sidsharma96/concordance/internal/htmlterms"
	"github.com/sidsharma96/concordance/internal/jsonunicode"
	"github.com/sidsharma96/concordance/internal/textterms"
)

type sourceWire struct {
	ID         string `json:"id"`
	Repository string `json:"repository"`
}
type refWire struct {
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}
type recordWire struct {
	Format      string       `json:"format"`
	Key         string       `json:"key"`
	CandidateID string       `json:"candidate_id"`
	Collection  string       `json:"collection"`
	Sources     []sourceWire `json:"sources"`
	Original    refWire      `json:"original_descriptor"`
	Capture     refWire      `json:"capture_descriptor"`
	Article     string       `json:"article_processor"`
	Passage     string       `json:"passage_processor"`
	Fields      string       `json:"field_processor"`
	Terms       string       `json:"terms_version"`
	Fingerprint string       `json:"input_fingerprint_version"`
}

func wireRef(r capturefiles.DescriptorRef) refWire {
	return refWire{SHA256: hex.EncodeToString(r.SHA256[:]), Bytes: r.Bytes}
}
func parseRef(r refWire, l Limits) (capturefiles.DescriptorRef, error) {
	if len(r.SHA256) != 64 || r.Bytes <= 0 || r.Bytes > int64(l.Capture.MaxDescriptorBytes) {
		return capturefiles.DescriptorRef{}, ErrInvalid
	}
	var result capturefiles.DescriptorRef
	for _, c := range r.SHA256 {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return result, ErrInvalid
		}
	}
	_, err := hex.Decode(result.SHA256[:], []byte(r.SHA256))
	if err != nil {
		return capturefiles.DescriptorRef{}, ErrInvalid
	}
	result.Bytes = r.Bytes
	return result, nil
}
func encodeRecord(ctx context.Context, saved Saved, copied capturefiles.CopyInfo, l Limits) ([]byte, error) {
	sources := make([]sourceWire, len(saved.Sources))
	for i, s := range saved.Sources {
		sources[i] = sourceWire{ID: s.ID, Repository: s.Repository}
	}
	body, err := json.Marshal(recordWire{
		Format:      Format,
		Key:         saved.Key,
		CandidateID: saved.CandidateID,
		Collection:  saved.Collection,
		Sources:     sources,
		Original:    wireRef(copied.Original),
		Capture:     wireRef(copied.Descriptor),
		Article:     htmlarticle.Processor,
		Passage:     htmlpassage.Processor,
		Fields:      htmlterms.Processor,
		Terms:       textterms.Version,
		Fingerprint: generationmanifest.InputFingerprintVersion,
	})
	if err != nil {
		return nil, ErrInvalid
	}
	if len(body) > l.MaxRecordBytes {
		return nil, ErrLimit
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return body, nil
}

// Check closed, case-sensitive object shapes and source count before typed
// decoding. encoding/json's DisallowUnknownFields alone also accepts case-folded
// field names, so it cannot enforce this exact wire schema. Unicode and decoded
// duplicate keys are rejected here rather than silently repaired/overwritten.
func checkJSON(ctx context.Context, body []byte, l Limits) error {
	if !utf8.Valid(body) {
		return ErrInvalid
	}
	if err := jsonunicode.CheckSurrogates(ctx, body); err != nil {
		return ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(body))
	d.UseNumber()
	var walk func(string) error
	walk = func(shape string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		token, err := d.Token()
		if err != nil || token == nil {
			return ErrInvalid
		}
		if shape == "scalar" {
			if _, nested := token.(json.Delim); nested {
				return ErrInvalid
			}
			return nil // The typed decoder below checks string versus integer fields.
		}
		if shape == "sources" {
			if token != json.Delim('[') {
				return ErrInvalid
			}
			count := 0
			for d.More() {
				if count >= l.MaxSources {
					return ErrLimit
				}
				count++
				if err := walk("source"); err != nil {
					return err
				}
			}
			if end, err := d.Token(); err != nil || end != json.Delim(']') {
				return ErrInvalid
			}
			return nil
		}
		if token != json.Delim('{') {
			return ErrInvalid
		}
		var fields []string
		switch shape {
		case "record":
			fields = []string{
				"format", "key", "candidate_id", "collection", "sources",
				"original_descriptor", "capture_descriptor",
				"article_processor", "passage_processor", "field_processor",
				"terms_version", "input_fingerprint_version",
			}
		case "source":
			fields = []string{"id", "repository"}
		case "ref":
			fields = []string{"sha256", "bytes"}
		default:
			return ErrInvalid
		}
		seen := make(map[string]bool, len(fields))
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return ErrInvalid
			}
			name, ok := key.(string)
			if !ok || !slices.Contains(fields, name) || seen[name] {
				return ErrInvalid
			}
			seen[name] = true
			child := "scalar"
			if shape == "record" {
				switch name {
				case "sources":
					child = "sources"
				case "original_descriptor", "capture_descriptor":
					child = "ref"
				}
			}
			if err := walk(child); err != nil {
				return err
			}
		}
		if end, err := d.Token(); err != nil || end != json.Delim('}') || len(seen) != len(fields) {
			return ErrInvalid
		}
		return nil
	}
	if err := walk("record"); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return ErrInvalid
	}
	return nil
}
func decodeRecord(ctx context.Context, body []byte, l Limits) (Saved, capturefiles.DescriptorRef, error) {
	if len(body) > l.MaxRecordBytes {
		return Saved{}, capturefiles.DescriptorRef{}, ErrLimit
	}
	if err := checkJSON(ctx, body, l); err != nil {
		return Saved{}, capturefiles.DescriptorRef{}, err
	}
	var wire recordWire
	d := json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	if err := d.Decode(&wire); err != nil {
		return Saved{}, capturefiles.DescriptorRef{}, ErrInvalid
	}
	if wire.Format != Format || !validID(wire.Key, "import-") || !validID(wire.CandidateID, "generation-") {
		return Saved{}, capturefiles.DescriptorRef{}, ErrInvalid
	}
	for _, policy := range []struct{ name, recorded, current string }{
		{"article_processor", wire.Article, htmlarticle.Processor},
		{"passage_processor", wire.Passage, htmlpassage.Processor},
		{"field_processor", wire.Fields, htmlterms.Processor},
		{"terms_version", wire.Terms, textterms.Version},
		{"input_fingerprint_version", wire.Fingerprint, generationmanifest.InputFingerprintVersion},
	} {
		if policy.recorded != policy.current {
			return Saved{}, capturefiles.DescriptorRef{}, fmt.Errorf("%w: %s; use compatible processing rules", ErrPolicy, policy.name)
		}
	}
	if _, err := parseRef(wire.Original, l); err != nil {
		return Saved{}, capturefiles.DescriptorRef{}, err
	}
	ref, err := parseRef(wire.Capture, l)
	if err != nil {
		return Saved{}, capturefiles.DescriptorRef{}, err
	}
	sources := make([]Source, len(wire.Sources))
	for i, source := range wire.Sources {
		sources[i] = Source{ID: source.ID, Repository: source.Repository}
	}
	request, err := ownRequest(Request{Collection: wire.Collection, Sources: sources}, l)
	if err != nil {
		return Saved{}, capturefiles.DescriptorRef{}, err
	}
	return Saved{Key: wire.Key, CandidateID: wire.CandidateID, Collection: request.Collection, Sources: request.Sources}, ref, ctx.Err()
}
