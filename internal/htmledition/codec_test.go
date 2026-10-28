package htmledition

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"github.com/sidsharma96/concordance/internal/htmlarticle"
	"github.com/sidsharma96/concordance/internal/htmlarticlejson"
	"reflect"
	"testing"
	"time"

	"github.com/sidsharma96/concordance/internal/htmlpassage"
	"github.com/sidsharma96/concordance/internal/termsearch"
)

// An independent spec-shaped writer creates inconsistent records the canonical
// builder/encoder cannot produce. It deliberately performs no semantic validation.
type wireFixture struct {
	id           string
	tags         []string
	version      int
	articles     []ArticleRef
	locations    []Location
	counts       termsearch.EditionCounts
	postingDelta int
}

func fromEdition(t *testing.T, ctx context.Context, e *Edition) wireFixture {
	t.Helper()
	c, found, err := e.index.ExportEdition(ctx, e.id, countLimits(limits()))
	if err != nil || !found {
		t.Fatal(err)
	}
	return wireFixture{id: e.id, tags: []string{articleFormat, passageProcessor, fieldProcessor, termsVersion}, version: 1, articles: append([]ArticleRef{}, e.articles...), locations: append([]Location{}, e.locations...), counts: c}
}
func writeFixture(f wireFixture) []byte {
	out := []byte("CNDX\x00")
	number := func(n int) { out = binary.AppendUvarint(out, uint64(n)) }
	text := func(s string) { number(len(s)); out = append(out, s...) }
	number(f.version)
	text(f.id)
	for _, s := range f.tags {
		text(s)
	}
	rows := f.postingDelta
	for _, w := range f.counts.Words {
		rows += len(w.Rows)
	}
	for _, n := range []int{len(f.articles), len(f.locations), len(f.counts.Words), rows, f.counts.TotalTerms} {
		number(n)
	}
	for _, a := range f.articles {
		out = append(out, a.Object.SHA256[:]...)
		number(a.Object.Bytes)
		number(a.Blocks)
		number(a.Passages)
	}
	for i, r := range f.locations {
		number(r.Article)
		number(r.Body.First)
		number(r.Body.After)
		number(f.counts.Lengths[i])
	}
	for _, w := range f.counts.Words {
		text(w.Term)
		number(len(w.Rows))
		for _, r := range w.Rows {
			number(r.Passage)
			number(r.Body)
			number(r.Headings)
			number(r.Labels)
		}
	}
	return out
}
func TestRejectMalformedWireAfterMatchingChecksum(t *testing.T) {
	ctx, _, _, e, _ := fixture(t)
	cases := map[string]func(*wireFixture){
		"version": func(f *wireFixture) { f.version = 2 }, "tag": func(f *wireFixture) { f.tags[2] = "future/v2" },
		"bad unicode": func(f *wireFixture) { f.counts.Words[0].Term = "\xff" }, "NUL word": func(f *wireFixture) { f.counts.Words[0].Term = "a\x00b" }, "empty word": func(f *wireFixture) { f.counts.Words[0].Term = "" },
		"duplicate word": func(f *wireFixture) { f.counts.Words[1].Term = f.counts.Words[0].Term }, "unordered word": func(f *wireFixture) { f.counts.Words[0], f.counts.Words[1] = f.counts.Words[1], f.counts.Words[0] },
		"empty posting": func(f *wireFixture) { f.counts.Words[0].Rows = nil }, "outside posting": func(f *wireFixture) { f.counts.Words[0].Rows[0].Passage = len(f.locations) },
		"zero frequency": func(f *wireFixture) {
			f.counts.Words[0].Rows[0] = termsearch.FieldCount{Passage: f.counts.Words[0].Rows[0].Passage}
		},
		"huge frequency": func(f *wireFixture) { f.counts.Words[0].Rows[0].Body = int(^uint(0) >> 1) },
		"length":         func(f *wireFixture) { f.counts.Lengths[0]++ }, "total": func(f *wireFixture) { f.counts.TotalTerms++ }, "posting total": func(f *wireFixture) { f.postingDelta = 1 },
		"zero article bytes": func(f *wireFixture) { f.articles[0].Object.Bytes = 0 }, "zero blocks": func(f *wireFixture) { f.articles[0].Blocks = 0 }, "too many article passages": func(f *wireFixture) { f.articles[0].Passages++ },
		"no article": func(f *wireFixture) { f.articles = nil }, "no passage": func(f *wireFixture) { f.locations = nil; f.counts.Lengths = nil },
		"negative range": func(f *wireFixture) { f.locations[0].Body.First = -1 }, "range gap": func(f *wireFixture) { f.locations[0].Body.First = 1 }, "empty range": func(f *wireFixture) { f.locations[0].Body.After = 0 }, "outside range": func(f *wireFixture) { f.locations[0].Body.After = f.articles[0].Blocks + 1 },
		"wrong article": func(f *wireFixture) { f.locations[0].Article = 1 },
		"duplicate posting": func(f *wireFixture) {
			r := f.counts.Words[0].Rows[0]
			f.counts.Words[0].Rows = append([]termsearch.FieldCount{r}, f.counts.Words[0].Rows...)
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			f := fromEdition(t, ctx, e)
			mutate(&f)
			body := writeFixture(f)
			loaded, err := Open(ctx, body, reference(body), e.id, limits())
			if err == nil || loaded != nil {
				t.Fatal("accepted invalid", err)
			}
		})
	}
	canonical := writeFixture(fromEdition(t, ctx, e))
	for i := 0; i < len(canonical); i++ {
		body := canonical[:i]
		loaded, err := Open(ctx, body, reference(body), e.id, limits())
		if err == nil || loaded != nil {
			t.Fatalf("accepted truncation at %d: %v", i, err)
		}
	}
	for _, body := range [][]byte{append(bytes.Clone(canonical), 0), append(append([]byte("CNDX\x00"), 0x81, 0), canonical[6:]...), append([]byte("CNDX\x00"), bytes.Repeat([]byte{0xff}, 11)...)} {
		if loaded, err := Open(ctx, body, reference(body), e.id, limits()); err == nil || loaded != nil {
			t.Fatal("accepted noncanonical/overflow/trailing", err)
		}
	}
	// Tiny body, enormous cardinalities: reject before allocating declared tables.
	prefix := []byte("CNDX\x00\x01")
	for _, s := range []string{"one", articleFormat, passageProcessor, fieldProcessor, termsVersion} {
		prefix = binary.AppendUvarint(prefix, uint64(len(s)))
		prefix = append(prefix, s...)
	}
	for i := 0; i < 5; i++ {
		body := bytes.Clone(prefix)
		for j := 0; j < 5; j++ {
			n := uint64(0)
			if i == j {
				n = uint64(int(^uint(0) >> 1))
			}
			body = binary.AppendUvarint(body, n)
		}
		wide := limits()
		wide.MaxArticles = int(^uint(0) >> 1)
		wide.MaxPassages = wide.MaxArticles
		wide.MaxVocabulary = wide.MaxArticles
		wide.MaxPostings = wide.MaxArticles
		wide.MaxTerms = wide.MaxArticles
		if loaded, err := Open(ctx, body, reference(body), "one", wide); err == nil || loaded != nil {
			t.Fatal("accepted huge declaration", i, err)
		}
	}
}
func TestEveryFileBudgetAndCancellation(t *testing.T) {
	ctx, bodies, _, e, saved := fixture(t)
	typ := reflect.TypeFor[Limits]()
	for i := 0; i < typ.NumField(); i++ {
		name := typ.Field(i).Name
		t.Run(name, func(t *testing.T) {
			l := limits()
			reflect.ValueOf(&l).Elem().Field(i).SetInt(1)
			if x, err := Open(ctx, saved, reference(saved), "one", l); x != nil || !errors.Is(err, ErrLimit) {
				t.Fatal(x, err)
			}
			if b, err := Encode(ctx, e, l); b != nil || !errors.Is(err, ErrLimit) {
				t.Fatal(b, err)
			}
			l = limits()
			reflect.ValueOf(&l).Elem().Field(i).SetInt(0)
			if _, err := Build(ctx, "one", bodies, BuildLimits{l, articleLimits(), preparationLimits()}); !errors.Is(err, ErrInvalid) {
				t.Fatal(err)
			}
		})
	}
	for _, invoke := range []func(context.Context) error{
		func(ctx context.Context) error {
			e, err := Build(ctx, "one", bodies, buildLimits())
			if err != nil && e != nil {
				t.Fatal("partial build")
			}
			return err
		},
		func(ctx context.Context) error {
			e, err := Open(ctx, saved, reference(saved), "one", limits())
			if err != nil && e != nil {
				t.Fatal("partial open")
			}
			return err
		},
		func(ctx context.Context) error {
			b, err := Encode(ctx, e, limits())
			if err != nil && b != nil {
				t.Fatal("partial encode")
			}
			return err
		},
		func(ctx context.Context) error {
			b, err := Inspect(ctx, e, limits(), 1<<20)
			if err != nil && b != nil {
				t.Fatal("partial inspect")
			}
			return err
		},
		func(ctx context.Context) error {
			s, err := e.Quote(ctx, 0, bodies[0], articleLimits(), 10000)
			if err != nil && s != "" {
				t.Fatal("partial quote")
			}
			return err
		},
	} {
		parent, cancel := context.WithCancel(ctx)
		counter := &interruptContext{Context: parent, cancel: cancel}
		if err := invoke(counter); err != nil {
			t.Fatal(err)
		}
		cancel()
		for stop := 1; stop <= counter.calls; stop++ {
			parent, cancel := context.WithCancel(ctx)
			err := invoke(&interruptContext{Context: parent, cancel: cancel, stop: stop})
			cancel()
			if !errors.Is(err, context.Canceled) {
				t.Fatal(stop, err)
			}
		}
	}
}

type interruptContext struct {
	context.Context
	cancel      context.CancelFunc
	calls, stop int
}

func (c *interruptContext) Err() error {
	c.calls++
	if c.stop > 0 && c.calls == c.stop {
		c.cancel()
	}
	return c.Context.Err()
}

func TestSavedLocationsNotRegroupedAndActualArticleChecked(t *testing.T) {
	ctx := deadline(t)
	// A plausible saved table may differ from today's grouping. This synthetic
	// fixture demonstrates structural/integrity checks, not a historical writer.
	_, bodies, _, e, _ := fixture(t)
	f := fromEdition(t, ctx, e)
	f.articles[0].Blocks++
	last := f.articles[0].Passages - 1
	f.locations[last].Body.After++
	body := writeFixture(f)
	opened, err := Open(ctx, body, reference(body), "one", limits())
	if err != nil {
		t.Fatal(err)
	}
	if quote, err := opened.Quote(ctx, last, bodies[0], articleLimits(), 10000); quote != "" || !errors.Is(err, ErrMismatch) {
		t.Fatal("actual blocks not checked", quote, err)
	}
	// Today's outer notice stays whole; this synthetic saved fixture splits it.
	a := htmlarticle.Article{Processor: "htmlarticle/v1", Profile: "fixture/v1", HTMLBytes: 1, Blocks: []htmlarticle.Block{{Kind: htmlarticle.Prose, Text: "one"}, {Kind: htmlarticle.Prose, Text: "two"}, {Kind: htmlarticle.Prose, Text: "three"}}, Groups: []htmlarticle.Group{{Kind: htmlarticle.Notice, First: 0, After: 3, Label: "notice"}}}
	raw, err := htmlarticlejson.Encode(ctx, a, articleLimits())
	if err != nil {
		t.Fatal(err)
	}
	plan, err := htmlpassage.Build(ctx, a, preparationLimits().Passages)
	if err != nil || len(plan.Passages) != 1 {
		t.Fatal(err)
	}
	x, err := termsearch.NewIndex(ctx, termsearch.Corpus{TermsVersion: termsVersion, Editions: []termsearch.Edition{{ID: "one", Passages: []termsearch.Fields{{Body: []string{"one"}}, {Body: []string{"two", "three"}}}}}}, countLimits(limits()))
	if err != nil {
		t.Fatal(err)
	}
	counts, _, err := x.ExportEdition(ctx, "one", countLimits(limits()))
	if err != nil {
		t.Fatal(err)
	}
	f = wireFixture{id: "one", version: 1, tags: []string{articleFormat, passageProcessor, fieldProcessor, termsVersion}, articles: []ArticleRef{{reference(raw), 3, 2}}, locations: []Location{{0, htmlpassage.Range{First: 0, After: 1}}, {0, htmlpassage.Range{First: 1, After: 3}}}, counts: counts}
	body = writeFixture(f)
	opened, err = Open(ctx, body, reference(body), "one", limits())
	if err != nil {
		t.Fatal(err)
	}
	if quote, err := opened.Quote(ctx, 1, raw, articleLimits(), 100); err != nil || quote != "two\nthree" {
		t.Fatal(quote, err)
	}
}

func FuzzOpenCanonicalOrReject(f *testing.F) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	empty, err := Build(ctx, "empty", nil, buildLimits())
	if err != nil {
		f.Fatal(err)
	}
	body, err := Encode(ctx, empty, limits())
	if err != nil {
		f.Fatal(err)
	}
	f.Add(body)
	a := htmlarticle.Article{Processor: "htmlarticle/v1", Profile: "fuzz/v1", HTMLBytes: 1, Blocks: []htmlarticle.Block{{Kind: htmlarticle.Prose, Text: "one two one"}}}
	raw, err := htmlarticlejson.Encode(ctx, a, articleLimits())
	if err != nil {
		f.Fatal(err)
	}
	nonempty, err := Build(ctx, "empty", [][]byte{raw}, buildLimits())
	if err != nil {
		f.Fatal(err)
	}
	saved, err := Encode(ctx, nonempty, limits())
	if err != nil {
		f.Fatal(err)
	}
	f.Add(saved)
	f.Add([]byte("CNDX\x00\x81\x00"))
	f.Fuzz(func(t *testing.T, body []byte) {
		if len(body) > 8192 {
			t.Skip()
		}
		ctx := deadline(t)
		l := limits()
		l.MaxEncodedBytes = 8192
		l.MaxArticles = 50
		l.MaxPassages = 200
		l.MaxVocabulary = 200
		l.MaxPostings = 500
		l.MaxTerms = 2000
		e, err := Open(ctx, body, reference(body), "empty", l)
		if err != nil {
			if e != nil {
				t.Fatal("partial reader")
			}
			return
		}
		encoded, err := Encode(ctx, e, l)
		if err != nil || !bytes.Equal(body, encoded) {
			t.Fatal("accepted noncanonical representation", err)
		}
	})
}
