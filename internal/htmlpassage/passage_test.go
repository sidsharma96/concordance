package htmlpassage

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/sidsharma96/concordance/internal/htmlarticle"
)

func budgets() Limits {
	return Limits{MaxBlocks: 100, MaxGroups: 200, MaxInputBytes: 10000, MaxPassages: 100, MaxBodyBytes: 1000, MaxReferences: 1000, MaxContextBytes: 10000}
}
func deadline(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	t.Cleanup(cancel)
	return ctx
}
func fixture() htmlarticle.Article {
	return htmlarticle.Article{Processor: htmlarticle.Processor, Profile: "fixture/v1", Blocks: []htmlarticle.Block{
		{Kind: htmlarticle.Heading, Level: 1, Text: "Guide"},
		{Kind: htmlarticle.Heading, Level: 2, Text: "Restarts"},
		{Kind: htmlarticle.Prose, Text: "Beta; enabled by default"},
		{Kind: htmlarticle.Prose, Text: "Feature explanation"},
		{Kind: htmlarticle.Code, Text: "  restartPolicy: Always\n"},
		{Kind: htmlarticle.Heading, Level: 2, Text: "States"},
		{Kind: htmlarticle.Prose, Text: "state"},
		{Kind: htmlarticle.Prose, Text: "description"},
		{Kind: htmlarticle.Separator},
		{Kind: htmlarticle.Heading, Level: 3, Text: "Tail"},
	}, Groups: []htmlarticle.Group{
		{Kind: htmlarticle.Notice, First: 2, After: 3, Label: "Feature Gate: Example"},
		{Kind: htmlarticle.Paragraph, First: 2, After: 3},
		{Kind: htmlarticle.Paragraph, First: 3, After: 4},
		{Kind: htmlarticle.Table, First: 6, After: 8},
		{Kind: htmlarticle.Row, First: 6, After: 8},
		{Kind: htmlarticle.Cell, First: 6, After: 7, Header: true},
		{Kind: htmlarticle.Paragraph, First: 6, After: 7},
		{Kind: htmlarticle.Cell, First: 7, After: 8},
		{Kind: htmlarticle.Paragraph, First: 7, After: 8},
		{Kind: htmlarticle.Notice, First: 10, After: 10, Label: "Empty EOF notice"},
	}}
}

func TestWholeContainersAndCommonHeadingContext(t *testing.T) {
	a := fixture()
	got, err := Build(deadline(t), a, budgets())
	if err != nil {
		t.Fatal(err)
	}
	want := []Passage{
		{Body: Range{0, 3}, Groups: []int{0, 1}},
		{Body: Range{3, 4}, Headings: []int{0, 1}, Groups: []int{2}},
		{Body: Range{4, 5}, Headings: []int{0, 1}},
		{Body: Range{5, 8}, Headings: []int{0}, Groups: []int{3, 4, 5, 6, 7, 8}},
		{Body: Range{8, 10}, Headings: []int{0, 5}, Groups: []int{9}},
	}
	if got.Processor != Processor || !reflect.DeepEqual(got.Passages, want) {
		t.Fatalf("got %+v; want %+v", got.Passages, want)
	}
	assertOracle(t, a, got)
	// Metadata is independent across passages and does not mutate the Article.
	got.Passages[1].Headings[0] = 42
	got.Passages[0].Groups[0] = 42
	if got.Passages[2].Headings[0] != 0 || a.Groups[0].First != 2 || a.Blocks[0].Text != "Guide" {
		t.Fatal("metadata aliases another passage or article")
	}
}

func TestHeadingsInsideContainersAndEmptyBoundaries(t *testing.T) {
	a := htmlarticle.Article{Processor: htmlarticle.Processor, Profile: "fixture/v1", Blocks: []htmlarticle.Block{
		{Kind: htmlarticle.Heading, Level: 1, Text: "Title"},
		{Kind: htmlarticle.Heading, Level: 2, Text: "Old peer"},
		{Kind: htmlarticle.Prose, Text: "before"},
		{Kind: htmlarticle.Heading, Level: 2, Text: "New peer"},
		{Kind: htmlarticle.Prose, Text: "inside"},
		{Kind: htmlarticle.Prose, Text: "after"},
	}, Groups: []htmlarticle.Group{
		{Kind: htmlarticle.Paragraph, First: 2, After: 3},
		{Kind: htmlarticle.Notice, First: 3, After: 3, Label: "At boundary"},
		{Kind: htmlarticle.Quotation, First: 3, After: 5},
		{Kind: htmlarticle.Paragraph, First: 4, After: 5},
	}}
	got, err := Build(deadline(t), a, budgets())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Passages) != 3 || !slices.Equal(got.Passages[1].Headings, []int{0}) || !slices.Equal(got.Passages[2].Headings, []int{0, 3}) || !slices.Equal(got.Passages[1].Groups, []int{1, 2, 3}) {
		t.Fatalf("stale peer heading or lost boundary group: %+v", got.Passages)
	}
	assertOracle(t, a, got)
}

// A deliberately slower oracle recomputes the heading stack from the beginning
// for every block, then intersects scopes across each body. It also scans all
// passages for every group instead of sharing the builder's cursor/outer map.
func assertOracle(t *testing.T, a htmlarticle.Article, p Plan) {
	t.Helper()
	next := 0
	for _, passage := range p.Passages {
		r := passage.Body
		if r.First != next || r.After <= r.First || r.After > len(a.Blocks) {
			t.Fatalf("not an exhaustive disjoint partition: %+v", p.Passages)
		}
		next = r.After
		var common []int
		for index := r.First; index < r.After; index++ {
			var stack []int
			for j, b := range a.Blocks[:index+1] {
				if b.Kind == htmlarticle.Heading {
					for len(stack) > 0 && a.Blocks[stack[len(stack)-1]].Level >= b.Level {
						stack = stack[:len(stack)-1]
					}
					stack = append(stack, j)
				}
			}
			if index == r.First {
				for _, h := range stack {
					if h < r.First {
						common = append(common, h)
					}
				}
			} else {
				common = slices.DeleteFunc(common, func(h int) bool { return !slices.Contains(stack, h) })
			}
		}
		if !slices.Equal(common, passage.Headings) {
			t.Fatalf("wrong common heading context for %+v: %v vs %v", r, passage.Headings, common)
		}
	}
	if next != len(a.Blocks) {
		t.Fatal("uncovered suffix")
	}
	seen := make([]int, len(a.Groups))
	for _, passage := range p.Passages {
		for _, index := range passage.Groups {
			if index < 0 || index >= len(a.Groups) {
				t.Fatal("out-of-range group reference")
			}
			seen[index]++
			g := a.Groups[index]
			r := passage.Body
			if g.First < r.First || g.After > r.After || g.First == r.After && !(g.First == g.After && r.After == len(a.Blocks)) {
				t.Fatalf("group split/misattributed: %+v %+v", g, r)
			}
		}
	}
	for _, count := range seen {
		if count != 1 {
			t.Fatal("lost/duplicated group", seen)
		}
	}
	// No passage boundary may cut the interior of any original nonempty group.
	for _, g := range a.Groups {
		for _, passage := range p.Passages {
			if passage.Body.First > g.First && passage.Body.First < g.After {
				t.Fatal("container split")
			}
		}
	}
}

func TestRejectMalformedInputWithoutPartialPlan(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*htmlarticle.Article)
		want   error
	}{
		{"unknown processor", func(a *htmlarticle.Article) { a.Processor = "future/v2" }, ErrUnsupported},
		{"empty", func(a *htmlarticle.Article) { a.Blocks = nil }, ErrInvalid},
		{"profile", func(a *htmlarticle.Article) { a.Profile = "invalid profile" }, ErrInvalid},
		{"unknown block", func(a *htmlarticle.Article) { a.Blocks[0].Kind = "future" }, ErrUnsupported},
		{"heading level", func(a *htmlarticle.Article) { a.Blocks[0].Level = 7 }, ErrInvalid},
		{"nonheading level", func(a *htmlarticle.Article) { a.Blocks[2].Level = 1 }, ErrInvalid},
		{"UTF8", func(a *htmlarticle.Article) { a.Blocks[2].Text = "\xff" }, ErrInvalid},
		{"NUL", func(a *htmlarticle.Article) { a.Groups[0].Label = "a\x00b" }, ErrInvalid},
		{"nonimage source", func(a *htmlarticle.Article) { a.Blocks[2].SourceURL = "/image" }, ErrInvalid},
		{"unknown group", func(a *htmlarticle.Article) { a.Groups[0].Kind = "future" }, ErrUnsupported},
		{"negative range", func(a *htmlarticle.Article) { a.Groups[0].First = -1 }, ErrInvalid},
		{"past end", func(a *htmlarticle.Article) { a.Groups[0].After = 11 }, ErrInvalid},
		{"reversed", func(a *htmlarticle.Article) { a.Groups[0].After = 1 }, ErrInvalid},
		{"not opening order", func(a *htmlarticle.Article) { a.Groups[2].First = 1 }, ErrInvalid},
		{"crossing", func(a *htmlarticle.Article) {
			a.Groups = []htmlarticle.Group{{Kind: htmlarticle.List, First: 1, After: 4}, {Kind: htmlarticle.Notice, First: 2, After: 5}}
		}, ErrInvalid},
		{"ordered paragraph", func(a *htmlarticle.Article) { a.Groups[1].Ordered = true }, ErrInvalid},
		{"header paragraph", func(a *htmlarticle.Article) { a.Groups[1].Header = true }, ErrInvalid},
		{"label paragraph", func(a *htmlarticle.Article) { a.Groups[1].Label = "unexpected" }, ErrInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := fixture()
			tc.change(&a)
			p, err := Build(deadline(t), a, budgets())
			if !errors.Is(err, tc.want) || !reflect.DeepEqual(p, Plan{}) {
				t.Fatalf("partial/wrong failure: %+v %v", p, err)
			}
		})
	}
}

func measured(a htmlarticle.Article, p Plan) Limits {
	l := Limits{MaxBlocks: len(a.Blocks), MaxGroups: len(a.Groups), MaxPassages: len(p.Passages)}
	l.MaxInputBytes = len(a.Processor) + len(a.Profile)
	for _, b := range a.Blocks {
		l.MaxInputBytes += len(b.Kind) + len(b.Text) + len(b.Anchor) + len(b.SourceURL)
	}
	for _, g := range a.Groups {
		l.MaxInputBytes += len(g.Kind) + len(g.Label)
	}
	for _, p := range p.Passages {
		n := p.Body.After - p.Body.First - 1
		for _, b := range a.Blocks[p.Body.First:p.Body.After] {
			n += len(b.Text)
		}
		l.MaxBodyBytes = max(l.MaxBodyBytes, n)
		l.MaxReferences += len(p.Headings) + len(p.Groups)
		for _, h := range p.Headings {
			l.MaxContextBytes += len(a.Blocks[h].Text)
		}
		for _, g := range p.Groups {
			l.MaxContextBytes += len(a.Groups[g].Label)
		}
	}
	return l
}

func TestInclusiveBudgetsAndFailureAfterEarlierPassages(t *testing.T) {
	a := fixture()
	ctx := deadline(t)
	p, err := Build(ctx, a, budgets())
	if err != nil {
		t.Fatal(err)
	}
	exact := measured(a, p)
	if _, err := Build(ctx, a, exact); err != nil {
		t.Fatal("exact limits rejected", err)
	}
	for _, tc := range []struct {
		name  string
		lower func(*Limits)
	}{
		{"blocks", func(l *Limits) { l.MaxBlocks-- }},
		{"groups", func(l *Limits) { l.MaxGroups-- }},
		{"input", func(l *Limits) { l.MaxInputBytes-- }},
		{"passages", func(l *Limits) { l.MaxPassages-- }},
		{"body", func(l *Limits) { l.MaxBodyBytes-- }},
		{"references", func(l *Limits) { l.MaxReferences-- }},
		{"context", func(l *Limits) { l.MaxContextBytes-- }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l := exact
			tc.lower(&l)
			got, err := Build(ctx, a, l)
			if !errors.Is(err, ErrLimit) || !reflect.DeepEqual(got, Plan{}) {
				t.Fatalf("partial or missed bound: %+v %v", got, err)
			}
		})
	}
	if _, err := Build(context.Background(), a, exact); !errors.Is(err, ErrInvalid) {
		t.Fatal("missing deadline accepted", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := Build(cancelled, a, exact); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation lost", err)
	}
	l := exact
	l.MaxBodyBytes = 0
	if _, err := Build(ctx, a, l); !errors.Is(err, ErrInvalid) {
		t.Fatal("nonpositive limit accepted", err)
	}
}

func FuzzStructurePartition(f *testing.F) {
	f.Add([]byte{1, 2, 3, 4, 5, 6, 7, 8})
	f.Add([]byte{0})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) == 0 || len(data) > 64 {
			return
		}
		a := htmlarticle.Article{Processor: htmlarticle.Processor, Profile: "fuzz/v1"}
		for _, v := range data {
			b := htmlarticle.Block{Kind: htmlarticle.Prose, Text: "body"}
			switch v % 4 {
			case 0:
				b.Kind = htmlarticle.Heading
				b.Level = int(v%6) + 1
			case 1:
				b.Kind = htmlarticle.Code
			case 2:
				b.Kind = htmlarticle.Separator
				b.Text = ""
			}
			a.Blocks = append(a.Blocks, b)
		}
		for i := 0; i < len(data); {
			after := min(len(data), i+int(data[i]%4)+1)
			a.Groups = append(a.Groups, htmlarticle.Group{Kind: htmlarticle.Notice, First: i, After: after, Label: "notice"})
			for j := i; j < after; j++ {
				if data[j]%2 == 0 {
					a.Groups = append(a.Groups, htmlarticle.Group{Kind: htmlarticle.Paragraph, First: j, After: j + 1})
				}
			}
			i = after
		}
		a.Groups = append(a.Groups, htmlarticle.Group{Kind: htmlarticle.Notice, First: len(data), After: len(data), Label: "EOF"})
		p, err := Build(deadline(t), a, budgets())
		if err != nil {
			t.Fatal(err)
		}
		assertOracle(t, a, p)
	})
}
