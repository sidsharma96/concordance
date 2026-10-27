package termsearch

import (
	"context"
	"math/rand/v2"
	"reflect"
	"testing"
)

// Deliberately uses no query/term lookup maps and searches anew for each term.
func scanOracle(c Corpus, q Query) Result {
	for _, edition := range c.Editions {
		if edition.ID != q.Edition {
			continue
		}
		result := Result{EditionFound: true}
		for index, p := range edition.Passages {
			all := true
			for _, wanted := range q.Terms {
				present := false
				for _, term := range p.Body {
					if term == wanted {
						present = true
					}
				}
				for _, term := range p.Headings {
					if term == wanted {
						present = true
					}
				}
				for _, term := range p.Labels {
					if term == wanted {
						present = true
					}
				}
				if !present {
					all = false
					break
				}
			}
			if all {
				result.Passages = append(result.Passages, index)
			}
		}
		return result
	}
	return Result{}
}
func TestGeneratedAgainstOracle(t *testing.T) {
	random := rand.New(rand.NewPCG(7, 103))
	ctx := testContext(t)
	for range 500 {
		c := Corpus{TermsVersion: "fixture/v1"}
		for _, id := range []string{"a", "b", "c"} {
			edition := Edition{ID: id}
			for n := random.IntN(15); n > 0; n-- {
				fields := Fields{}
				for _, target := range []*[]string{&fields.Body, &fields.Headings, &fields.Labels} {
					for m := random.IntN(8); m > 0; m-- {
						*target = append(*target, string(rune('a'+random.IntN(7))))
					}
				}
				edition.Passages = append(edition.Passages, fields)
			}
			c.Editions = append(c.Editions, edition)
		}
		q := Query{TermsVersion: c.TermsVersion, Edition: []string{"a", "b", "c", "missing"}[random.IntN(4)]}
		for n := 1 + random.IntN(6); n > 0; n-- {
			q.Terms = append(q.Terms, string(rune('a'+random.IntN(8))))
		}
		got, err := MatchAll(ctx, c, q, testLimits())
		if err != nil || !reflect.DeepEqual(got, scanOracle(c, q)) {
			t.Fatal(got, err, scanOracle(c, q))
		}
	}
}
func FuzzMatchAllAgainstOracle(f *testing.F) {
	f.Add([]byte{1, 2, 3, 7, 9, 0})
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 512 {
			t.Skip()
		}
		c := Corpus{TermsVersion: "fixture/v1", Editions: []Edition{{ID: "a"}, {ID: "b"}, {ID: "empty"}}}
		// At most32 passages * 3 terms, deliberately reusing a small vocabulary.
		for i, b := range data {
			if i >= 96 {
				break
			}
			edition := int(b % 2)
			for len(c.Editions[edition].Passages) <= i/3 {
				c.Editions[edition].Passages = append(c.Editions[edition].Passages, Fields{})
			}
			p := &c.Editions[edition].Passages[i/3]
			term := string(rune('a' + b%7))
			switch b % 3 {
			case 0:
				p.Body = append(p.Body, term)
			case 1:
				p.Headings = append(p.Headings, term)
			case 2:
				p.Labels = append(p.Labels, term)
			}
		}
		q := Query{TermsVersion: c.TermsVersion, Edition: "a", Terms: []string{"a"}}
		if len(data) > 0 {
			q.Edition = []string{"a", "b", "empty", "missing"}[int(data[0])%4]
			for _, b := range data[:min(8, len(data))] {
				q.Terms = append(q.Terms, string(rune('a'+b%8)))
			}
		}
		ctx, cancel := context.WithCancel(testContext(t))
		defer cancel()
		got, err := MatchAll(ctx, c, q, testLimits())
		if err != nil || !reflect.DeepEqual(got, scanOracle(c, q)) {
			t.Fatal(got, err, scanOracle(c, q))
		}
	})
}
