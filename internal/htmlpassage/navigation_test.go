package htmlpassage

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/sidsharma96/concordance/internal/htmlarticle"
)

func TestNavigationWholePassagesAndSections(t *testing.T) {
	a := fixture()
	for _, tc := range []struct {
		anchor                       Range
		scope                        Scope
		body, previous, next, parent Range
	}{
		{Range{4, 5}, Exact, Range{4, 5}, Range{3, 4}, Range{5, 8}, Range{0, 5}},
		{Range{4, 5}, Section, Range{0, 5}, Range{}, Range{5, 8}, Range{0, 10}},
		{Range{0, 5}, Parent, Range{0, 10}, Range{}, Range{}, Range{}},
		{Range{0, 5}, Next, Range{5, 8}, Range{4, 5}, Range{8, 10}, Range{5, 10}},
		{Range{5, 8}, Previous, Range{4, 5}, Range{3, 4}, Range{5, 8}, Range{0, 5}},
	} {
		n, err := Navigate(deadline(t), a, tc.anchor, tc.scope, budgets())
		if err != nil || n.Body != tc.body || n.Previous != tc.previous || n.Next != tc.next || n.Parent != tc.parent {
			t.Fatalf("%+v: %+v %v", tc, n, err)
		}
	}
	for _, scope := range []Scope{Parent, Previous, Next} {
		if n, err := Navigate(deadline(t), a, Range{0, 10}, scope, budgets()); !errors.Is(err, ErrNoContext) || !reflect.DeepEqual(n, Navigation{}) {
			t.Fatal(scope, n, err)
		}
	}
	// A table's interior is not a legal source locator even though its text exists.
	for _, anchor := range []Range{{6, 7}, {-1, 2}, {0, 99}, {5, 5}} {
		if n, err := Navigate(deadline(t), a, anchor, Exact, budgets()); err == nil || !reflect.DeepEqual(n, Navigation{}) {
			t.Fatal(anchor, n, err)
		}
	}
	n, err := Navigate(deadline(t), a, Range{4, 5}, Exact, budgets())
	if err != nil || !reflect.DeepEqual(n.Headings, []int{0, 1}) {
		t.Fatal(n, err)
	}
}

func TestNavigationIgnoresNoticeHeadingAndRejectsLimits(t *testing.T) {
	a := htmlarticle.Article{Processor: htmlarticle.Processor, Profile: "test/v1", Blocks: []htmlarticle.Block{
		{Kind: htmlarticle.Heading, Level: 1, Text: "Guide"}, {Kind: htmlarticle.Prose, Text: "Intro"},
		{Kind: htmlarticle.Heading, Level: 2, Text: "Rules"}, {Kind: htmlarticle.Prose, Text: "Rule"},
		{Kind: htmlarticle.Heading, Level: 1, Text: "Warning"}, {Kind: htmlarticle.Code, Text: "do not split\nthis code"},
		{Kind: htmlarticle.Prose, Text: "After notice"}, {Kind: htmlarticle.Heading, Level: 2, Text: "Other"}, {Kind: htmlarticle.Prose, Text: "Other text"},
	}, Groups: []htmlarticle.Group{{Kind: htmlarticle.Notice, First: 4, After: 6, Label: "Danger"}}}
	n, err := Navigate(deadline(t), a, Range{6, 7}, Section, budgets())
	if err != nil || n.Body != (Range{2, 7}) {
		t.Fatal(n, err)
	}
	n, err = Navigate(deadline(t), a, Range{6, 7}, Previous, budgets())
	if err != nil || n.Body != (Range{4, 6}) {
		t.Fatal(n, err)
	}
	ctx, cancel := context.WithCancel(deadline(t))
	cancel()
	if _, err := Navigate(ctx, a, Range{6, 7}, Exact, budgets()); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	l := budgets()
	l.MaxBlocks = 1
	if _, err := Navigate(deadline(t), a, Range{6, 7}, Exact, l); !errors.Is(err, ErrLimit) {
		t.Fatal(err)
	}
	if _, err := Navigate(nil, a, Range{6, 7}, Exact, budgets()); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := Navigate(deadline(t), a, Range{6, 7}, "unknown", budgets()); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
}
