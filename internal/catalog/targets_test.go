package catalog

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"testing"
)

func targetFixture(t *testing.T) fixture {
	t.Helper()
	f := newFixture(t)
	if err := f.store.CreateTargetSchema(f.ctx); err != nil {
		t.Fatal(err)
	}
	return f
}

func publishTargetGeneration(t *testing.T, f fixture, id string) TargetValue {
	t.Helper()
	r := request()
	r.Key = id
	if _, err := f.store.Bind(f.ctx, r); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.Publish(f.ctx, r, generation(id)); err != nil {
		t.Fatal(err)
	}
	// Catalog-only tests use trusted synthetic metadata. The separate application
	// tests verify the actual default edition through stored manifest/index bytes.
	return TargetValue{Collection: "guide", Generation: id, DefaultEdition: "1.35"}
}

func TestTargetChangesReplayAndSupersededRequests(t *testing.T) {
	f := targetFixture(t)
	a := publishTargetGeneration(t, f, "generation-a")
	b := publishTargetGeneration(t, f, "generation-b")
	b.DefaultEdition = "1.34"
	if _, err := f.store.ResolveTarget(f.ctx, "kubernetes"); !errors.Is(err, ErrNotFound) {
		t.Fatal("publication must not activate a target", err)
	}
	first := TargetUpdate{Name: "kubernetes", Value: a}
	got, err := f.store.SetTarget(f.ctx, first)
	want := Target{Name: first.Name, Value: a, Revision: 1}
	if err != nil || got != want {
		t.Fatal(got, err)
	}
	// Discard the successful caller result and reconnect; this is not a lost
	// PostgreSQL acknowledgment experiment.
	reopened, _ := f.reopen(t)
	retry, err := reopened.SetTarget(f.ctx, first)
	if err != nil || retry != want {
		t.Fatal("create retry changed state", retry, err)
	}
	update := TargetUpdate{Name: first.Name, Value: b, ExpectedRevision: 1}
	second, err := reopened.SetTarget(f.ctx, update)
	if err != nil || second.Value != b || second.Revision != 2 {
		t.Fatal(second, err)
	}
	if replay, err := reopened.SetTarget(f.ctx, update); err != nil || replay != second {
		t.Fatal("update retry changed revision", replay, err)
	}
	if result, err := reopened.SetTarget(f.ctx, first); !errors.Is(err, ErrTargetConflict) || result != (Target{}) {
		t.Fatal("old create restored a superseded choice", result, err)
	}
	back, err := reopened.SetTarget(f.ctx, TargetUpdate{Name: first.Name, Value: a, ExpectedRevision: 2})
	if err != nil || back.Value != a || back.Revision != 3 {
		t.Fatal(back, err)
	}
	if _, err := reopened.SetTarget(f.ctx, first); !errors.Is(err, ErrTargetConflict) {
		t.Fatal("same contents at a later revision are not the original result", err)
	}
	got, err = reopened.ResolveTarget(f.ctx, first.Name)
	if err != nil || got != back {
		t.Fatal(got, err)
	}
	got.Value.DefaultEdition = "caller-mutated"
	if current, err := reopened.ResolveTarget(f.ctx, first.Name); err != nil || current != back {
		t.Fatal("returned metadata was not owned", current, err)
	}
	if err := reopened.CreateTargetSchema(f.ctx); err == nil {
		t.Fatal("existing target schema must not be silently reused")
	}
}

func TestTargetConcurrentIdenticalAndDifferentUpdates(t *testing.T) {
	f := targetFixture(t)
	a := publishTargetGeneration(t, f, "a")
	if _, err := f.store.SetTarget(f.ctx, TargetUpdate{Name: "docs", Value: a}); err != nil {
		t.Fatal(err)
	}
	b := publishTargetGeneration(t, f, "b")
	secondStore, _ := f.reopen(t)
	const count = 16
	values := make([]TargetValue, count)
	for i := range values {
		values[i] = publishTargetGeneration(t, f, fmt.Sprintf("choice-%d", i))
		values[i].DefaultEdition = fmt.Sprintf("edition-%d", i)
	}
	type outcome struct {
		target Target
		err    error
	}
	run := func(identical bool, expected int64) []outcome {
		start := make(chan struct{})
		out := make(chan outcome, count)
		var workers sync.WaitGroup
		for i := range count {
			workers.Go(func() {
				<-start
				value := values[i]
				if identical {
					value = b
				}
				store := f.store
				if i%2 == 1 {
					store = secondStore
				}
				got, err := store.SetTarget(f.ctx, TargetUpdate{Name: "docs", Value: value, ExpectedRevision: expected})
				out <- outcome{got, err}
			})
		}
		close(start)
		workers.Wait()
		close(out)
		var results []outcome
		for result := range out {
			results = append(results, result)
		}
		return results
	}
	for _, result := range run(true, 1) {
		if result.err != nil || result.target.Value != b || result.target.Revision != 2 {
			t.Fatal("identical concurrent calls did not converge", result)
		}
	}
	success, conflict := 0, 0
	var winner Target
	for _, result := range run(false, 2) {
		switch {
		case result.err == nil:
			success++
			winner = result.target
		case errors.Is(result.err, ErrTargetConflict) && result.target == (Target{}):
			conflict++
		default:
			t.Fatal(result)
		}
	}
	if success != 1 || conflict != count-1 || winner.Revision != 3 {
		t.Fatal("lost or overlapping conditional writes", success, conflict, winner)
	}
	current, err := f.store.ResolveTarget(f.ctx, "docs")
	if err != nil || current != winner {
		t.Fatal("stored tuple differs from one complete winner", current, err)
	}
}

func TestTargetPublicationBoundsAndUnavailableAreDistinct(t *testing.T) {
	f := targetFixture(t)
	v := publishTargetGeneration(t, f, "published")
	missing := v
	missing.Generation = "not-published"
	if got, err := f.store.SetTarget(f.ctx, TargetUpdate{Name: "docs", Value: missing}); !errors.Is(err, ErrNotFound) || got != (Target{}) {
		t.Fatal("target accepted an unpublished generation", got, err)
	}
	if _, err := f.store.SetTarget(f.ctx, TargetUpdate{Name: "docs", Value: v, ExpectedRevision: 1}); !errors.Is(err, ErrTargetConflict) {
		t.Fatal("a nonzero revision created a missing target", err)
	}
	for _, invalid := range []TargetUpdate{
		{Name: "", Value: v}, {Name: "\x00", Value: v}, {Name: string([]byte{0xff}), Value: v},
		{Name: "docs", Value: TargetValue{Collection: "guide", Generation: "published"}},
		{Name: "docs", Value: v, ExpectedRevision: -1},
		{Name: strings.Repeat("x", 257), Value: v},
	} {
		if got, err := f.store.SetTarget(f.ctx, invalid); !errors.Is(err, ErrInvalid) || got != (Target{}) {
			t.Fatal("bad update", got, err)
		}
	}
	canceled, cancel := context.WithCancel(f.ctx)
	cancel()
	if _, err := f.store.SetTarget(canceled, TargetUpdate{Name: "docs", Value: v}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := f.store.ResolveTarget(f.ctx, "docs"); !errors.Is(err, ErrNotFound) {
		t.Fatal("failed operations changed the target", err)
	}
	if _, err := f.store.SetTarget(f.ctx, TargetUpdate{Name: "'; DROP TABLE concordance_imports; --", Value: v}); err != nil {
		t.Fatal("names are SQL parameters", err)
	}
	f.pool.Close()
	if _, err := f.store.ResolveTarget(f.ctx, "docs"); err == nil || errors.Is(err, ErrNotFound) {
		t.Fatal("database failure looked like absence", err)
	}
	if got, err := f.store.SetTarget(f.ctx, TargetUpdate{Name: "docs", Value: v}); !errors.Is(err, ErrTargetOutcomeUnknown) || got != (Target{}) {
		t.Fatal("write error lost conservative outcome category", got, err)
	}
}

func TestTargetRevisionCannotWrap(t *testing.T) {
	f := targetFixture(t)
	v := publishTargetGeneration(t, f, "published")
	if _, err := f.store.SetTarget(f.ctx, TargetUpdate{Name: "docs", Value: v}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(f.ctx, `UPDATE concordance_search_targets SET revision=$1 WHERE name='docs'`, int64(math.MaxInt64-1)); err != nil {
		t.Fatal(err)
	}
	u := TargetUpdate{Name: "docs", Value: v, ExpectedRevision: math.MaxInt64 - 1}
	for range 2 {
		got, err := f.store.SetTarget(f.ctx, u)
		if err != nil || got.Revision != math.MaxInt64 {
			t.Fatal(got, err)
		}
	}
	u.ExpectedRevision = math.MaxInt64
	if _, err := f.store.SetTarget(f.ctx, u); !errors.Is(err, ErrInvalid) {
		t.Fatal("exhausted revision was allowed to advance", err)
	}
	if got, err := f.store.ResolveTarget(f.ctx, "docs"); err != nil || got.Revision != math.MaxInt64 {
		t.Fatal("exhausted target must stay readable", got, err)
	}
}
