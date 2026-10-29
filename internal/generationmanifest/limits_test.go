package generationmanifest

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/sidsharma96/concordance/internal/htmledition"
)

// Count logical values/reference sizes from the actual JSON tree independently of
// the codec's DTO traversal. Object member names are not value strings.
func wireTotals(t testing.TB, b []byte) (values int, referenced, maxObject int64) {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var tree any
	if err := dec.Decode(&tree); err != nil {
		t.Fatal(err)
	}
	var walk func(any)
	walk = func(v any) {
		switch v := v.(type) {
		case string:
			values += len(v)
		case []any:
			for _, x := range v {
				walk(x)
			}
		case map[string]any:
			if _, ok := v["sha256"]; ok {
				n, err := v["bytes"].(json.Number).Int64()
				if err != nil {
					t.Fatal(err)
				}
				referenced += n
				maxObject = max(maxObject, n)
			}
			for _, x := range v {
				walk(x)
			}
		}
	}
	walk(tree)
	return
}
func TestLimitAdmission(t *testing.T) {
	f := fixture(t)
	ctx := testContext(t)
	body := encoded(t, f.d)
	values, total, largest := wireTotals(t, body)
	cases := map[string]func(*Limits){
		"encoded":          func(l *Limits) { l.MaxEncodedBytes = len(body) - 1 },
		"editions":         func(l *Limits) { l.MaxEditions = 1 },
		"files":            func(l *Limits) { l.MaxFiles = 2 },
		"documents":        func(l *Limits) { l.MaxDocuments = 1 },
		"images":           func(l *Limits) { l.MaxImages = 1 },
		"source-files":     func(l *Limits) { l.MaxSourceFiles = 2 },
		"source-documents": func(l *Limits) { l.MaxSourceDocuments = 1 },
		"string":           func(l *Limits) { l.MaxStringBytes = 63 },
		"values":           func(l *Limits) { l.MaxValueBytes = values - 1 },
		"object":           func(l *Limits) { l.MaxObjectBytes = largest - 1 },
		"referenced":       func(l *Limits) { l.MaxReferencedBytes = total - 1 },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			l := testLimits()
			change(&l)
			if b, err := Encode(ctx, f.d, l); !errors.Is(err, ErrLimit) || b != nil {
				t.Fatalf("encode: %v", err)
			}
			if m, err := Open(ctx, body, reference(body), f.d.Collection, f.d.Generation, l); !errors.Is(err, ErrLimit) || m != nil {
				t.Fatalf("open: %v", err)
			}
		})
	}
	l := testLimits()
	l.MaxEncodedBytes = len(body)
	l.MaxValueBytes = values
	l.MaxStringBytes = 64
	l.MaxObjectBytes = largest
	l.MaxReferencedBytes = total
	if _, err := Encode(ctx, f.d, l); err != nil {
		t.Fatal("exact budgets", err)
	}
	if _, err := Open(ctx, body, reference(body), f.d.Collection, f.d.Generation, l); err != nil {
		t.Fatal("exact budgets", err)
	}
	// Every zero configured limit is invalid, even if this input is empty.
	for i := 0; i < reflect.TypeFor[Limits]().NumField(); i++ {
		l := testLimits()
		v := reflect.ValueOf(&l).Elem()
		v.Field(i).SetInt(0)
		if _, err := Encode(ctx, f.d, l); !errors.Is(err, ErrInvalid) {
			t.Fatalf("%s: %v", v.Type().Field(i).Name, err)
		}
	}
	// Independent source/capture ceilings are exercised with two actual records.
	g := fixture(t)
	g.d.Sources = append(g.d.Sources, Source{"other", "https://example.invalid/other.git"})
	c := g.d.Editions[0].Captures[0]
	c.Source = "other"
	g.d.Editions[0].Captures = append(g.d.Editions[0].Captures, c)
	b := encoded(t, g.d)
	for _, change := range []func(*Limits){func(l *Limits) { l.MaxSources = 1 }, func(l *Limits) { l.MaxCaptures = 1 }} {
		l := testLimits()
		change(&l)
		if _, err := Encode(ctx, g.d, l); !errors.Is(err, ErrLimit) {
			t.Fatal(err)
		}
		if _, err := Open(ctx, b, reference(b), g.d.Collection, g.d.Generation, l); !errors.Is(err, ErrLimit) {
			t.Fatal(err)
		}
	}
}

func TestKeysAcrossSourcesAndEditions(t *testing.T) {
	f := fixture(t)
	ctx := testContext(t)
	f.d.Sources = append(f.d.Sources, Source{"second", "https://example.invalid/docs.git"})
	c := f.d.Editions[0].Captures[0]
	c.Source = "second"
	f.d.Editions[0].Captures = append(f.d.Editions[0].Captures, c)
	doc := f.d.Editions[0].Documents[0]
	doc.Capture = 1
	f.d.Editions[0].Documents = append(f.d.Editions[0].Documents, doc)
	build := func(id string, articleBodies [][]byte) []byte {
		x, err := htmledition.Build(ctx, id, articleBodies, htmledition.BuildLimits{File: indexLimits(), Article: articleLimits(), Preparation: preparationLimits()})
		if err != nil {
			t.Fatal(err)
		}
		b, err := htmledition.Encode(ctx, x, indexLimits())
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	three := build("one", [][]byte{f.article, f.article, f.article})
	f.d.Editions[0].Index = reference(three)
	second := f.d.Editions[0]
	second.ID = "two"
	second.Captures = append([]Capture{}, second.Captures...)
	second.Captures[0].Revision = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	twoBytes := build("two", [][]byte{f.article, f.article, f.article})
	second.Index = reference(twoBytes)
	f.d.Editions = append(f.d.Editions, second)
	m := opened(t, encoded(t, f.d))
	if _, err := m.OpenEdition(ctx, "one", three, indexLimits()); err != nil {
		t.Fatal(err)
	}
	if _, err := m.OpenEdition(ctx, "two", twoBytes, indexLimits()); err != nil {
		t.Fatal(err)
	}
	d, err := m.Describe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if d.Editions[0].Captures[0].Revision == d.Editions[2].Captures[0].Revision {
		t.Fatal("revision aliases collapsed")
	}
}
