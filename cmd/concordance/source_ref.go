package main

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/sidsharma96/concordance/internal/htmlpassage"
	"github.com/sidsharma96/concordance/internal/publishedsearch"
)

const sourceRefPrefix = "cs1."

// Three 256-byte IDs and two 8192-byte strings can each expand sixfold in
// JSON, then four-thirds in base64. 144 KiB also covers keys, integers and hash.
const maxSourceRefBytes = 144 << 10

var errSourceReference = errors.New("invalid captured-source reference; copy source_ref from a search or read result")

// This closed, canonical transport is a locator, not a signature or permission.
// Human provenance remains in the surrounding JSON. No routes or credentials.
type sourceRefWire struct {
	Collection string `json:"collection"`
	Generation string `json:"generation"`
	Edition    string `json:"edition"`
	Slot       int    `json:"slot"`
	Source     string `json:"source"`
	Document   string `json:"document"`
	SHA256     string `json:"sha256"`
	First      int    `json:"first"`
	After      int    `json:"after"`
}

func encodeSourceReference(ref publishedsearch.SourceReference) (string, error) {
	w := sourceRefWire{ref.Selection.Collection, ref.Selection.Generation, ref.Selection.Edition, ref.Slot, ref.SourceID, ref.NativeID, hex.EncodeToString(ref.ArticleSHA256[:]), ref.Body.First, ref.Body.After}
	if !validSourceWire(w) {
		return "", errSourceReference
	}
	body, err := json.Marshal(w)
	if err != nil {
		return "", err
	}
	if len(sourceRefPrefix)+base64.RawURLEncoding.EncodedLen(len(body)) > maxSourceRefBytes {
		return "", errSourceReference
	}
	return sourceRefPrefix + base64.RawURLEncoding.EncodeToString(body), nil
}
func decodeSourceReference(value string) (publishedsearch.SourceReference, error) {
	if len(value) > maxSourceRefBytes || !strings.HasPrefix(value, sourceRefPrefix) {
		return publishedsearch.SourceReference{}, errSourceReference
	}
	encoded := strings.TrimPrefix(value, sourceRefPrefix)
	body, err := base64.RawURLEncoding.Strict().DecodeString(encoded)
	if err != nil {
		return publishedsearch.SourceReference{}, errSourceReference
	}
	var w sourceRefWire
	if json.Unmarshal(body, &w) != nil || !validSourceWire(w) {
		return publishedsearch.SourceReference{}, errSourceReference
	}
	// Re-encoding rejects duplicate/unknown/missing/null fields, coercion, malformed
	// Unicode replacements and alternative encodings. Only our exact wire is v1.
	canonical, err := json.Marshal(w)
	if err != nil || !bytes.Equal(canonical, body) || base64.RawURLEncoding.EncodeToString(body) != encoded {
		return publishedsearch.SourceReference{}, errSourceReference
	}
	digest, err := hex.DecodeString(w.SHA256)
	if err != nil || len(digest) != 32 || w.SHA256 != hex.EncodeToString(digest) {
		return publishedsearch.SourceReference{}, errSourceReference
	}
	var sha [32]byte
	copy(sha[:], digest)
	return publishedsearch.SourceReference{Selection: publishedsearch.Selection{Collection: w.Collection, Generation: w.Generation, Edition: w.Edition}, Slot: w.Slot, SourceID: w.Source, NativeID: w.Document, ArticleSHA256: sha, Body: htmlpassage.Range{First: w.First, After: w.After}}, nil
}
func validSourceWire(w sourceRefWire) bool {
	if !validName(w.Collection) || !validName(w.Generation) || !validName(w.Source) || w.Slot < 0 || w.First < 0 || w.After <= w.First || len(w.SHA256) != 64 {
		return false
	}
	for _, s := range []string{w.Edition, w.Document} {
		if s == "" || len(s) > 8192 || !utf8.ValidString(s) || strings.ContainsRune(s, 0) {
			return false
		}
	}
	return true
}
