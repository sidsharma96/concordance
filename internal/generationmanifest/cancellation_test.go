package generationmanifest

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

// Deterministic cancellation at actual cooperative polls, without wall-clock sleeps.
type polledContext struct {
	context.Context
	calls, stop int
	cancel      context.CancelFunc
}

func (c *polledContext) Err() error {
	c.calls++
	if c.stop > 0 && c.calls == c.stop {
		c.cancel()
	}
	return c.Context.Err()
}

func TestEveryObservedCancellationPoll(t *testing.T) {
	f := fixture(t)
	body := encoded(t, f.d)
	m := opened(t, body)
	ops := map[string]func(context.Context) (bool, error){
		"encode": func(ctx context.Context) (bool, error) {
			b, err := Encode(ctx, f.d, testLimits())
			return b != nil, err
		},
		"open": func(ctx context.Context) (bool, error) {
			m, err := Open(ctx, body, reference(body), f.d.Collection, f.d.Generation, testLimits())
			return m != nil, err
		},
		"objects": func(ctx context.Context) (bool, error) { r, err := m.Objects(ctx); return r != nil, err },
		"describe": func(ctx context.Context) (bool, error) {
			d, err := m.Describe(ctx)
			return !reflect.DeepEqual(d, Description{}), err
		},
		"index": func(ctx context.Context) (bool, error) {
			x, err := m.OpenEdition(ctx, "one", f.index, indexLimits())
			return x != nil, err
		},
		"article": func(ctx context.Context) (bool, error) {
			a, err := m.VerifyArticle(ctx, "one", 0, f.article, articleLimits())
			return a.Blocks != nil, err
		},
		"resolve": func(ctx context.Context) (bool, error) {
			b, err := m.Resolve(ctx, "one", 0)
			return b.Source.ID != "" || b.Images != nil, err
		},
	}
	for name, op := range ops {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(testContext(t))
			c := &polledContext{Context: ctx, cancel: cancel}
			if _, err := op(c); err != nil {
				cancel()
				t.Fatal(err)
			}
			polls := c.calls
			cancel()
			for stop := 1; stop <= polls; stop++ {
				ctx, cancel := context.WithCancel(testContext(t))
				c := &polledContext{Context: ctx, stop: stop, cancel: cancel}
				partial, err := op(c)
				cancel()
				if !errors.Is(err, context.Canceled) || partial {
					t.Fatalf("poll %d/%d: partial=%v err=%v", stop, polls, partial, err)
				}
			}
		})
	}
}
