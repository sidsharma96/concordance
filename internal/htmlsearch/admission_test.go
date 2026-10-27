package htmlsearch

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/sidsharma96/concordance/internal/htmlarticle"
	"github.com/sidsharma96/concordance/internal/htmlpassage"
)

func TestAggregateAdmission(t *testing.T) {
	input := []Edition{{ID: "one", Articles: []htmlarticle.Article{smallArticle()}}, {ID: "two", Articles: []htmlarticle.Article{smallArticle()}}}
	l := budgets()
	l.MaxEditions, l.MaxArticles, l.MaxBlocks, l.MaxGroups = 2, 2, 4, 2
	l.MaxPassages, l.MaxTerms, l.MaxOutputBytes = 2, 6, 32
	// Independent list of every charged fixture string, including duplicated
	// article metadata, labels and enum values. Not original HTMLBytes or hash.
	articleBytes := len(htmlarticle.Processor) + len("fixture/v1") + 2*len("prose") + len("AlphaBeta") + len("noticeFeature")
	l.MaxInputBytes = len("onetwo") + 2*articleBytes
	if _, err := New(deadline(t), input, l); err != nil {
		t.Fatal("exact aggregate ceilings", err)
	}
	for _, field := range []string{"MaxEditions", "MaxArticles", "MaxBlocks", "MaxGroups", "MaxInputBytes", "MaxPassages", "MaxTerms", "MaxOutputBytes"} {
		lower := l
		v := reflect.ValueOf(&lower).Elem().FieldByName(field)
		v.SetInt(v.Int() - 1)
		if got, err := New(deadline(t), input, lower); !errors.Is(err, ErrLimit) || got != nil {
			t.Fatal(field, got, err)
		}
	}
	l = budgets()
	l.Fields.Field.MaxTermBytes = 1
	if got, err := New(deadline(t), input, l); err == nil || got != nil {
		t.Fatal("nested text error lost", got, err)
	}
}

func TestInvalidEditionsAndLateArticle(t *testing.T) {
	for _, input := range [][]Edition{
		{{ID: ""}}, {{ID: "\x00"}}, {{ID: "\xff"}}, {{ID: "duplicate"}, {ID: "duplicate"}},
	} {
		if got, err := New(deadline(t), input, budgets()); !errors.Is(err, ErrInvalid) || got != nil {
			t.Fatal(got, err)
		}
	}
	a := smallArticle()
	a.Blocks[0].Kind = "unsupported"
	input := []Edition{{ID: "valid", Articles: []htmlarticle.Article{smallArticle(), a}}}
	if got, err := New(deadline(t), input, budgets()); !errors.Is(err, htmlpassage.ErrUnsupported) || got != nil {
		t.Fatal("late article returned partial state or lost cause", got, err)
	}
	if got, err := New(context.Background(), nil, budgets()); !errors.Is(err, ErrInvalid) || got != nil {
		t.Fatal(got, err)
	}
}

func TestEveryLimitPositiveEvenWithoutArticles(t *testing.T) {
	var paths [][]int
	var walk func(reflect.Type, []int)
	walk = func(typ reflect.Type, prefix []int) {
		for i := 0; i < typ.NumField(); i++ {
			path := append(append([]int(nil), prefix...), i)
			if typ.Field(i).Type.Kind() == reflect.Struct {
				walk(typ.Field(i).Type, path)
			} else {
				paths = append(paths, path)
			}
		}
	}
	walk(reflect.TypeFor[Limits](), nil)
	for _, path := range paths {
		l := budgets()
		reflect.ValueOf(&l).Elem().FieldByIndex(path).SetInt(0)
		if got, err := New(deadline(t), nil, l); !errors.Is(err, ErrInvalid) || got != nil {
			t.Fatal(path, got, err)
		}
	}
}

type countedContext struct {
	context.Context
	calls, stop int
}

func (c *countedContext) Err() error {
	c.calls++
	if c.stop > 0 && c.calls >= c.stop {
		return context.Canceled
	}
	return c.Context.Err()
}

func TestCancellationAtEveryCooperativeBoundary(t *testing.T) {
	input := []Edition{{ID: "chosen", Articles: []htmlarticle.Article{smallArticle()}}}
	s := mustNew(t, input)
	ref := independentRef(t, s)
	q := query(t, "chosen", "feature")
	for _, op := range []string{"new", "search", "quote", "no matches"} {
		run := func(ctx context.Context) error {
			switch op {
			case "new":
				got, err := New(ctx, input, budgets())
				if err != nil && got != nil {
					t.Fatal("partial constructor")
				}
				return err
			case "search", "no matches":
				query := q
				if op == "no matches" {
					query.Edition = "absent"
				}
				got, err := s.Search(ctx, query, searchLimits())
				if err != nil && !reflect.DeepEqual(got, Result{}) {
					t.Fatal("partial search")
				}
				return err
			default:
				got, err := s.Quote(ctx, ref, 10)
				if err != nil && got != "" {
					t.Fatal("partial quote")
				}
				return err
			}
		}
		count := &countedContext{Context: deadline(t)}
		if err := run(count); err != nil {
			t.Fatal(op, err)
		}
		for stop := 1; stop <= count.calls; stop++ {
			if err := run(&countedContext{Context: deadline(t), stop: stop}); !errors.Is(err, context.Canceled) {
				t.Fatal(op, stop, err)
			}
		}
	}
}
