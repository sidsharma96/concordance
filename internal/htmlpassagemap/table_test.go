package htmlpassagemap

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sidsharma96/concordance/internal/htmlarticle"
	"github.com/sidsharma96/concordance/internal/htmlarticlejson"
	"github.com/sidsharma96/concordance/internal/htmlpassage"
	"github.com/sidsharma96/concordance/internal/htmlterms"
	"github.com/sidsharma96/concordance/internal/termsearch"
	"github.com/sidsharma96/concordance/internal/textterms"
)

func deadline(t *testing.T) context.Context {
	t.Helper()
	c, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	t.Cleanup(cancel)
	return c
}
func limits() Limits { return Limits{1 << 20, 1000, 1000, 10000, 1000, 1 << 20} }
func articleLimits() htmlarticlejson.Limits {
	return htmlarticlejson.Limits{MaxEncodedBytes: 1 << 20, MaxBlocks: 1000, MaxGroups: 1000, MaxStringBytes: 10000, MaxValueBytes: 1 << 20, MaxGroupDepth: 100, MaxHTMLBytes: 1 << 20}
}
func preparationLimits() htmlterms.Limits {
	return htmlterms.Limits{Passages: htmlpassage.Limits{MaxBlocks: 1000, MaxGroups: 1000, MaxInputBytes: 1 << 20, MaxPassages: 1000, MaxBodyBytes: 10000, MaxReferences: 10000, MaxContextBytes: 1 << 20}, Field: textterms.Limits{MaxInputBytes: 10000, MaxTerms: 10000, MaxTermBytes: 1000, MaxOutputBytes: 1 << 20}, MaxViewBytes: 1 << 20, MaxTerms: 10000, MaxOutputBytes: 1 << 20}
}
func captureLimits() CaptureLimits {
	return CaptureLimits{limits(), articleLimits(), preparationLimits()}
}
func openLimits() OpenLimits { return OpenLimits{limits(), articleLimits()} }
func fixture(t *testing.T) (context.Context, []byte, htmlarticle.Article, *Table, Fields, []byte) {
	t.Helper()
	ctx := deadline(t)
	a, err := htmlarticle.Extract(ctx, []byte(`<article class="doc"><h1 id="pod">Pod</h1><div class="notice" title="ContainerRestartRules"><p>Enabled by default.</p></div><pre>  restartPolicy: Never
</pre><p>中文 😀 �</p><img src="pod.svg" alt="Pod layout"></article>`), htmlarticle.Profile{ID: "fixture/v1", ArticleClass: "doc", NoticeClasses: []string{"notice"}}, htmlarticle.Limits{MaxInputBytes: 10000, MaxTokenBytes: 10000, MaxTokens: 1000, MaxNodes: 1000, MaxDepth: 100, MaxBlocks: 1000, MaxGroups: 1000, MaxBlockBytes: 10000, MaxOutputBytes: 100000})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := htmlarticlejson.Encode(ctx, a, articleLimits())
	if err != nil {
		t.Fatal(err)
	}
	table, f, err := Capture(ctx, raw, captureLimits())
	if err != nil {
		t.Fatal(err)
	}
	saved, err := Encode(ctx, table, limits())
	if err != nil {
		t.Fatal(err)
	}
	return ctx, raw, a, table, f, saved
}
func clone(f Fields) Fields {
	b, _ := json.Marshal(f)
	var out Fields
	_ = json.Unmarshal(b, &out)
	return out
}

func TestCanonicalWireGolden(t *testing.T) {
	ctx := deadline(t)
	table := &Table{articleHash: [32]byte{1}, fieldsHash: [32]byte{2}, articleBytes: 3, blocks: 2, rows: []htmlpassage.Range{{First: 0, After: 2}}}
	b, err := Encode(ctx, table, limits())
	if err != nil {
		t.Fatal(err)
	}
	golden := `{"format":"concordance.html-locations/v1","article_sha256":"01` + strings.Repeat("0", 62) + `","article_bytes":3,"passage_processor":"htmlpassage/v1","field_processor":"htmlterms/v1","terms_version":"technical/v1/unicode-17.0.0","fields_sha256":"02` + strings.Repeat("0", 62) + `","block_count":2,"locations":[{"first":0,"after":2}]}`
	if string(b) != golden {
		t.Fatal(string(b))
	}
	got, err := decode(ctx, b, limits())
	if err != nil || !reflect.DeepEqual(table, got) {
		t.Fatal(got, err)
	}
	var external map[string]any
	if err := json.Unmarshal(b, &external); err != nil || len(external) != 9 {
		t.Fatal(external, err)
	}
	// Different legal key order and whitespace preserve meaning, not raw byte hash.
	reordered, _ := json.Marshal(external)
	if _, err := decode(ctx, append(reordered, '\n'), limits()); err != nil {
		t.Fatal(err)
	}
	clear(b)
	if got.rows[0].After != 2 {
		t.Fatal("decoded aliases input")
	}
}
func TestCaptureOpenPreservesFieldsLocationsQuotesAndOwnership(t *testing.T) {
	ctx, raw, a, table, f, saved := fixture(t)
	current, err := htmlterms.Build(ctx, a, preparationLimits())
	if err != nil {
		t.Fatal(err)
	}
	if f.PassageProcessor != current.Plan.Processor || f.Processor != current.Processor || f.TermsVersion != current.TermsVersion || !reflect.DeepEqual(f.Passages, current.Fields) {
		t.Fatal("fields differ")
	}
	rawBefore, savedBefore := bytes.Clone(raw), bytes.Clone(saved)
	fieldsBefore := clone(f)
	reader, err := Open(ctx, saved, raw, f, openLimits())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(raw, rawBefore) || !bytes.Equal(saved, savedBefore) || !reflect.DeepEqual(f, fieldsBefore) {
		t.Fatal("mutated inputs")
	}
	clear(raw)
	clear(saved)
	for i, p := range current.Plan.Passages {
		location, err := reader.Location(i)
		if err != nil || location != p.Body {
			t.Fatal(location, err)
		}
		texts := []string{}
		for _, b := range a.Blocks[p.Body.First:p.Body.After] {
			texts = append(texts, b.Text)
		}
		quote, err := reader.Quote(ctx, i, 10000)
		if err != nil || quote != strings.Join(texts, "\n") {
			t.Fatal(quote, err)
		}
		if strings.Contains(quote, "ContainerRestartRules") || strings.Contains(quote, "pod.svg") {
			t.Fatal("context leaked into quote")
		}
		if len(quote) > 0 {
			if q, err := reader.Quote(ctx, i, len(quote)); err != nil || q != quote {
				t.Fatal(err)
			}
			if q, err := reader.Quote(ctx, i, max(1, len(quote)-1)); len(quote) > 1 && (q != "" || !errors.Is(err, ErrLimit)) {
				t.Fatal(q, err)
			}
		}
	}
	// Changing every caller field later cannot change the private saved table/reader.
	f.Passages[0].Body = []string{"changed"}
	encoded, err := Encode(ctx, table, limits())
	if err != nil || !bytes.Equal(encoded, savedBefore) {
		t.Fatal(err)
	}
	if _, err := Open(ctx, savedBefore, rawBefore, f, openLimits()); !errors.Is(err, ErrMismatch) {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for i := range table.rows {
				if _, err := reader.Quote(ctx, i, 10000); err != nil {
					t.Error(err)
				}
				if _, err := reader.Location(i); err != nil {
					t.Error(err)
				}
			}
		})
	}
	wg.Wait()
}
func TestArtifactMixupsAndFieldFraming(t *testing.T) {
	ctx, raw, a, _, f, saved := fixture(t)
	a.Profile = "other/v1"
	different, err := htmlarticlejson.Encode(ctx, a, articleLimits())
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range [][]byte{different, append(bytes.Clone(raw), '\n'), raw[:len(raw)-1]} {
		if r, err := Open(ctx, saved, b, f, openLimits()); r != nil || !errors.Is(err, ErrMismatch) {
			t.Fatal(r, err)
		}
	}
	for _, mutate := range []func(*Fields){
		func(f *Fields) { f.Passages[0].Body = append(f.Passages[0].Body, "extra") },
		func(f *Fields) { f.Passages[0].Headings = append(f.Passages[0].Headings, "pod") },
		func(f *Fields) { f.Passages[0].Labels = append(f.Passages[0].Labels, "notice") },
		func(f *Fields) { f.Passages[0], f.Passages[1] = f.Passages[1], f.Passages[0] },
		func(f *Fields) { f.Passages = f.Passages[:len(f.Passages)-1] },
	} {
		other := clone(f)
		mutate(&other)
		if r, err := Open(ctx, saved, raw, other, openLimits()); r != nil || !errors.Is(err, ErrMismatch) {
			t.Fatal(r, err)
		}
	}
	for _, mutate := range []func(*Fields){func(f *Fields) { f.PassageProcessor += "x" }, func(f *Fields) { f.Processor += "x" }, func(f *Fields) { f.TermsVersion += "x" }} {
		other := clone(f)
		mutate(&other)
		if _, err := Open(ctx, saved, raw, other, openLimits()); !errors.Is(err, ErrUnsupported) {
			t.Fatal(err)
		}
	}
	// These lists have ambiguous concatenations without explicit lengths/counts.
	seen := map[[32]byte]bool{}
	for _, p := range []termsearch.Fields{{Body: []string{"ab", "c"}}, {Body: []string{"a", "bc"}}, {Body: []string{"a"}, Headings: []string{"bc"}}, {Headings: []string{"a", "bc"}}, {Labels: []string{"a", "bc"}}, {Body: []string{"bc", "a"}}, {Body: []string{"a", "a", "bc"}}} {
		f := Fields{passageVersion, fieldVersion, termsVersion, []termsearch.Fields{p}}
		sum, err := fingerprint(ctx, f, limits())
		if err != nil || seen[sum] {
			t.Fatal(sum, err)
		}
		seen[sum] = true
	}
}
func TestStrictDecodeFailures(t *testing.T) {
	ctx, _, _, _, _, saved := fixture(t)
	replace := func(a, b string) []byte { return []byte(strings.Replace(string(saved), a, b, 1)) }
	for _, body := range [][]byte{[]byte(`null`), append(bytes.Clone(saved), '!'), append(bytes.Clone(saved), []byte(` {}`)...), replace(`"format":`, `"Format":`), replace(`"format":`, `"unknown":true,"format":`), replace(`"format":`, `"for\u006dat":"concordance.html-locations/v1","format":`), replace(`"first":0`, `"first":0,"first":0`), replace(`"first":0`, `"first":null`), replace(`"first":0`, `"first":false`), replace(`"first":0`, `"first":{}`), replace(`"first":0`, `"first":-0`), replace(`"first":0`, `"first":0e0`), replace(`"first":0`, `"first":0.0`), replace(`"first":0`, `"first":9999999999999999999999999`), replace(`"first":0`, `"first":1`), replace(`"first":0`, `"first":0,"missing":0`), replace(`"first":0,`, ``), replace(`"locations":[`, `"locations":null,"unused":[`), replace(`"format":"concordance.html-locations/v1"`, `"format":"\ud800"`), replace(`"format":"concordance.html-locations/v1"`, `"format":"�"`)} {
		if got, err := decode(ctx, body, limits()); got != nil || err == nil {
			t.Fatalf("accepted malformed input: %s", body)
		}
	}
	invalid := bytes.Clone(saved)
	invalid[bytes.Index(invalid, []byte("concordance"))] = 0xff
	if _, err := decode(ctx, invalid, limits()); err == nil {
		t.Fatal("accepted bad UTF8")
	}
	for i := range len(saved) {
		if got, err := decode(ctx, saved[:i], limits()); got != nil || err == nil {
			t.Fatal("accepted prefix", i)
		}
	}
	for _, body := range [][]byte{replace(`html-locations/v1`, `html-locations/v2`), replace(`htmlpassage/v1`, `htmlpassage/v2`), replace(`htmlterms/v1`, `htmlterms/v2`), replace(`unicode-17.0.0`, `unicode-99.0.0`)} {
		if _, err := decode(ctx, body, limits()); !errors.Is(err, ErrUnsupported) {
			t.Fatal(err)
		}
	}
}
func TestLogicalBoundsAndInvalidRanges(t *testing.T) {
	ctx, raw, _, table, f, saved := fixture(t)
	l := limits()
	l.MaxEncodedBytes = len(saved)
	l.MaxBlocks = table.blocks
	l.MaxPassages = len(table.rows)
	terms, total, largest := 0, len(f.PassageProcessor)+len(f.Processor)+len(f.TermsVersion), 0
	for _, p := range f.Passages {
		for _, field := range [3][]string{p.Body, p.Headings, p.Labels} {
			terms += len(field)
			for _, s := range field {
				total += len(s)
				largest = max(largest, len(s))
			}
		}
	}
	l.MaxTerms = terms
	l.MaxFieldBytes = total
	l.MaxTermBytes = largest
	if _, err := Encode(ctx, table, l); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(ctx, saved, raw, f, OpenLimits{l, articleLimits()}); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"MaxEncodedBytes", "MaxBlocks", "MaxPassages", "MaxTerms", "MaxFieldBytes", "MaxTermBytes"} {
		low := l
		v := reflect.ValueOf(&low).Elem().FieldByName(field)
		v.SetInt(v.Int() - 1)
		if r, err := Open(ctx, saved, raw, f, OpenLimits{low, articleLimits()}); r != nil || !errors.Is(err, ErrLimit) {
			t.Fatal(field, r, err)
		}
		zero := limits()
		reflect.ValueOf(&zero).Elem().FieldByName(field).SetInt(0)
		if b, err := Encode(ctx, table, zero); b != nil || !errors.Is(err, ErrInvalid) {
			t.Fatal(field, err)
		}
	}
	for _, rows := range [][]htmlpassage.Range{nil, {{First: 1, After: table.blocks}}, {{First: 0, After: 0}}, {{First: 0, After: table.blocks + 1}}, {{First: 0, After: 1}}, {{First: 0, After: 2}, {First: 1, After: table.blocks}}} {
		bad := *table
		bad.rows = rows
		if b, err := Encode(ctx, &bad, limits()); b != nil || !errors.Is(err, ErrInvalid) {
			t.Fatal(rows, err)
		}
	}
	badF := clone(f)
	badF.Passages[0].Body = []string{""}
	if _, err := Open(ctx, saved, raw, badF, openLimits()); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	badF = clone(f)
	badF.Passages[0].Body = []string{string([]byte{0xff})}
	if _, err := Open(ctx, saved, raw, badF, openLimits()); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	badArticle := openLimits()
	badArticle.Article.MaxBlocks = 0
	if _, err := Open(ctx, saved, raw, f, badArticle); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
}
func TestZeroValuesAndDeadlines(t *testing.T) {
	ctx, raw, _, table, f, saved := fixture(t)
	for _, c := range []context.Context{nil, context.Background()} {
		if b, err := Encode(c, table, limits()); b != nil || err == nil {
			t.Fatal(err)
		}
		if r, err := Open(c, saved, raw, f, openLimits()); r != nil || err == nil {
			t.Fatal(err)
		}
		if x, f, err := Capture(c, raw, captureLimits()); x != nil || !reflect.DeepEqual(f, Fields{}) || err == nil {
			t.Fatal(err)
		}
	}
	for _, r := range []*Reader{nil, {}} {
		if _, err := r.Location(0); err == nil {
			t.Fatal("zero reader")
		}
	}
	r, err := Open(ctx, saved, raw, f, openLimits())
	if err != nil {
		t.Fatal(err)
	}
	for _, i := range []int{-1, len(table.rows)} {
		if q, err := r.Quote(ctx, i, 1000); q != "" || err == nil {
			t.Fatal(q, err)
		}
	}
	if q, err := r.Quote(ctx, 0, 0); q != "" || err == nil {
		t.Fatal(err)
	}
	if b, err := Encode(ctx, &Table{}, limits()); b != nil || err == nil {
		t.Fatal(err)
	}
}
