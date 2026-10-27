package htmlsearch

import (
	"math/rand"
	"testing"

	"github.com/sidsharma96/concordance/internal/htmlarticle"
	"github.com/sidsharma96/concordance/internal/htmlterms"
	"github.com/sidsharma96/concordance/internal/termsearch"
	"github.com/sidsharma96/concordance/internal/textterms"
)

// Direct composition is deliberately outside Snapshot: flatten public canonical
// plans/fields, then call the existing independently-tested ranker. Compare exact
// location values, scores/order and independently joined original block text.
func checkComposition(t *testing.T, input []Edition, s *Snapshot, q termsearch.Query) {
	t.Helper()
	c := termsearch.Corpus{TermsVersion: textterms.Version}
	var locations []Location
	for ei, edition := range input {
		e := termsearch.Edition{ID: edition.ID}
		for ai, article := range edition.Articles {
			p, err := htmlterms.Build(deadline(t), article, budgets().Fields)
			if err != nil {
				t.Fatal(err)
			}
			e.Passages = append(e.Passages, p.Fields...)
			if edition.ID == q.Edition {
				for pi, passage := range p.Plan.Passages {
					locations = append(locations, Location{Edition: ei, Article: ai, Passage: pi, Body: passage.Body})
				}
			}
		}
		c.Editions = append(c.Editions, e)
	}
	want, err := termsearch.RankBM25(deadline(t), c, q, searchLimits())
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Search(deadline(t), q, searchLimits())
	if err != nil || got.EditionFound != want.EditionFound || got.RankingVersion != want.RankingVersion || len(got.Hits) != len(want.Matches) {
		t.Fatal(got, err, want)
	}
	for i, hit := range got.Hits {
		loc, err := s.Location(hit.Ref)
		if err != nil || loc != locations[want.Matches[i].Passage] || hit.Score != want.Matches[i].Score {
			t.Fatal(loc, err, want)
		}
		quote, err := s.Quote(deadline(t), hit.Ref, 10000)
		if err != nil || quote != oracleQuote(input, loc) {
			t.Fatal("quote authority mismatch", quote, err)
		}
	}
}

func mappedInput(data []byte) []Edition {
	input := []Edition{{ID: "chosen"}, {ID: "empty"}, {ID: "other"}}
	for i, b := range data {
		a := htmlarticle.Article{Processor: htmlarticle.Processor, Profile: "generated/v1"}
		for j := 0; j <= int(b)%4; j++ {
			a.Blocks = append(a.Blocks, htmlarticle.Block{Kind: htmlarticle.Prose, Text: []string{"alpha beta", "beta", "alpha alpha"}[(i+j)%3]})
		}
		if b%2 == 0 {
			a.Groups = []htmlarticle.Group{{Kind: htmlarticle.Notice, First: 0, After: len(a.Blocks), Label: "feature"}}
		}
		edition := []int{0, 2}[i%2]
		input[edition].Articles = append(input[edition].Articles, a)
	}
	return input
}

func TestGeneratedComposition(t *testing.T) {
	rng := rand.New(rand.NewSource(2048))
	for range 150 {
		data := make([]byte, rng.Intn(12))
		if _, err := rng.Read(data); err != nil {
			t.Fatal(err)
		}
		input := mappedInput(data)
		s := mustNew(t, input)
		for _, edition := range []string{"chosen", "other", "empty", "missing"} {
			checkComposition(t, input, s, query(t, edition, "alpha beta"))
		}
	}
}

func FuzzSnapshotComposition(f *testing.F) {
	f.Add([]byte{0, 2, 3, 255})
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 24 {
			return
		}
		input := mappedInput(data)
		s := mustNew(t, input)
		checkComposition(t, input, s, query(t, "chosen", "alpha beta"))
	})
}
