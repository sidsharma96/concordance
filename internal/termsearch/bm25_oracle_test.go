package termsearch

import (
	"context"
	"math"
	"math/rand"
	"sort"
	"testing"
)

// Slow oracle: no production map, counting helper, MatchAll or sorting comparator.
// Independently rescan every passage for every distinct query term and derive its
// full-edition statistics, then insertion-sort qualifying scores. Keep small.
func oracleBM25(c Corpus, q Query) RankedResult {
	r := RankedResult{RankingVersion: BM25Version}
	var passages []Fields
	for _, e := range c.Editions {
		if e.ID == q.Edition {
			r.EditionFound = true
			passages = e.Passages
		}
	}
	terms := append([]string(nil), q.Terms...)
	sort.Strings(terms)
	unique := terms[:0]
	for _, term := range terms {
		if len(unique) == 0 || unique[len(unique)-1] != term {
			unique = append(unique, term)
		}
	}
	count := func(p Fields, key string) int {
		n := 0
		for _, field := range [][]string{p.Body, p.Headings, p.Labels} {
			for _, term := range field {
				if term == key {
					n++
				}
			}
		}
		return n
	}
	length := func(p Fields) int { return len(p.Body) + len(p.Headings) + len(p.Labels) }
	total := 0
	for _, p := range passages {
		total += length(p)
	}
	for index, p := range passages {
		score, matches := 0.0, true
		for _, term := range unique {
			tf := count(p, term)
			if tf == 0 {
				matches = false
				break
			}
			df := 0
			for _, other := range passages {
				if count(other, term) > 0 {
					df++
				}
			}
			rarity := math.Log(1 + (float64(len(passages))-float64(df)+.5)/(float64(df)+.5))
			avg := float64(total) / float64(len(passages))
			score += rarity * (float64(tf) * 2.2) / (float64(tf) + 1.2*(.25+.75*float64(length(p))/avg))
		}
		if matches {
			r.Matches = append(r.Matches, ScoredPassage{index, score})
		}
	}
	for i := 1; i < len(r.Matches); i++ {
		for j := i; j > 0 && r.Matches[j].Score > r.Matches[j-1].Score; j-- {
			r.Matches[j], r.Matches[j-1] = r.Matches[j-1], r.Matches[j]
		}
	}
	return r
}

func TestBM25GeneratedAgainstOracle(t *testing.T) {
	rng := rand.New(rand.NewSource(8192))
	vocab := []string{"a", "rare", "readiness-probe", "C++", "世界", "default"}
	for range 200 {
		c := Corpus{TermsVersion: "oracle/v1", Editions: []Edition{{ID: "chosen"}, {ID: "other"}}}
		for e := range c.Editions {
			for n := rng.Intn(12); n > 0; n-- {
				var p Fields
				for _, field := range []*[]string{&p.Body, &p.Headings, &p.Labels} {
					for k := rng.Intn(8); k > 0; k-- {
						*field = append(*field, vocab[rng.Intn(len(vocab))])
					}
				}
				c.Editions[e].Passages = append(c.Editions[e].Passages, p)
			}
		}
		q := Query{TermsVersion: c.TermsVersion, Edition: "chosen"}
		for n := 1 + rng.Intn(4); n > 0; n-- {
			q.Terms = append(q.Terms, vocab[rng.Intn(len(vocab))])
		}
		checkBM25Oracle(t, testContext(t), c, q)
	}
}

func FuzzBM25Oracle(f *testing.F) {
	f.Add([]byte{0, 1, 2, 3, 255, 2})
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 48 {
			return
		}
		c := Corpus{TermsVersion: "fuzz/v1", Editions: []Edition{{ID: "chosen", Passages: make([]Fields, 4)}, {ID: "other"}}}
		vocab := []string{"a", "b", "c", "absent"}
		for i, b := range data {
			p := &c.Editions[0].Passages[i%4]
			field := []*[]string{&p.Body, &p.Headings, &p.Labels}[int(b)%3]
			*field = append(*field, vocab[int(b)%3])
		}
		q := Query{TermsVersion: c.TermsVersion, Edition: "chosen", Terms: []string{"a"}}
		if len(data) > 0 {
			q.Terms = append(q.Terms, vocab[int(data[0])%4])
		}
		checkBM25Oracle(t, testContext(t), c, q)
	})
}

func checkBM25Oracle(t *testing.T, ctx context.Context, c Corpus, q Query) {
	t.Helper()
	got, err := RankBM25(ctx, c, q, testLimits())
	want := oracleBM25(c, q)
	if err != nil || got.EditionFound != want.EditionFound || got.RankingVersion != want.RankingVersion {
		t.Fatal(got, err, want)
	}
	assertScores(t, got.Matches, want.Matches)
}
