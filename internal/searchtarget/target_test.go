package searchtarget

import (
	"bytes"
	"context"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/sidsharma96/concordance/internal/catalog"
	"github.com/sidsharma96/concordance/internal/publishedsearch"
)

func TestExplicitTargetSwitchPinsResolvedAndOpenReaders(t *testing.T) {
	f := setup(t)
	a := makeBundle(t, "generation-a", "Retry uploads safely.")
	b := makeBundle(t, "generation-b", "Retry requests carefully.")
	f.publish(t, a)
	f.publish(t, b)
	ctx := testContext(t)
	if _, err := Resolve(ctx, f.cat, "kubernetes", ""); !errors.Is(err, catalog.ErrNotFound) {
		t.Fatal("publishing must not select a default", err)
	}
	u := catalog.TargetUpdate{Name: "kubernetes", Value: catalog.TargetValue{
		Collection: "guide", Generation: "generation-a", DefaultEdition: "one",
	}}
	f.remote.reset()
	first, err := Activate(ctx, f.cat, f.objects, u, testLimits())
	if err != nil || first.Revision != 1 {
		t.Fatal(first, err)
	}
	if !reflect.DeepEqual(f.remote.counts(), map[string]int{"GET": 2}) {
		t.Fatal("activation must only strictly read the manifest and default index", f.remote.counts())
	}
	selected, err := Resolve(ctx, f.cat, "kubernetes", "")
	if err != nil || selected != (publishedsearch.Selection{Collection: "guide", Generation: "generation-a", Edition: "one"}) {
		t.Fatal(selected, err)
	}
	old, err := publishedsearch.Open(ctx, f.cat, f.objects, selected, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	u.Value.Generation, u.ExpectedRevision = "generation-b", first.Revision
	second, err := Activate(ctx, f.cat, f.objects, u, testLimits())
	if err != nil || second.Revision != 2 {
		t.Fatal(second, err)
	}
	// A resolution captured before the switch is also stable if opening happens
	// later. The application must not resolve the name again midway through work.
	lateOld, err := publishedsearch.Open(ctx, f.cat, f.objects, selected, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	next, err := Resolve(ctx, f.cat, "kubernetes", "")
	if err != nil || next.Generation != "generation-b" || selected.Generation != "generation-a" {
		t.Fatal(selected, next, err)
	}
	current, err := publishedsearch.Open(ctx, f.cat, f.objects, next, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	for _, sample := range []struct {
		reader *publishedsearch.Reader
		text   string
	}{{old, "Retry uploads safely."}, {lateOld, "Retry uploads safely."}, {current, "Retry requests carefully."}} {
		result, err := sample.reader.Search(ctx, prepared(t, "retry"), queryLimits())
		if err != nil || len(result.Hits) != 2 {
			t.Fatal(result, err)
		}
		quote, err := sample.reader.Quote(ctx, result.Hits[0].Ref, 4096)
		if err != nil || !strings.Contains(quote, sample.text) {
			t.Fatal(quote, err)
		}
	}
	if replay, err := Activate(ctx, f.cat, f.objects, u, testLimits()); err != nil || replay != second {
		t.Fatal("identical retry changed selection/revision", replay, err)
	}
	stale := u
	stale.Value.Generation = "generation-a"
	if got, err := Activate(ctx, f.cat, f.objects, stale, testLimits()); !errors.Is(err, catalog.ErrTargetConflict) || got != (catalog.Target{}) {
		t.Fatal("stale operator replaced current default", got, err)
	}
	override, err := Resolve(ctx, f.cat, "kubernetes", "empty")
	if err != nil || override.Generation != "generation-b" || override.Edition != "empty" {
		t.Fatal(override, err)
	}
	if r, err := publishedsearch.Open(ctx, f.cat, f.objects, override, testLimits()); err != nil || r == nil {
		t.Fatal("known-empty edition must remain available", err)
	}
	missing, err := Resolve(ctx, f.cat, "kubernetes", "absent")
	if err != nil || missing.Edition != "absent" {
		t.Fatal("resolution silently replaced override", missing, err)
	}
	if r, err := publishedsearch.Open(ctx, f.cat, f.objects, missing, testLimits()); err == nil || r != nil {
		t.Fatal("missing requested edition fell back", err)
	}
	t.Log("target switch: future resolutions use B; resolved/open A readers keep A; default and explicit editions stay exact")
}

func TestActivationRejectsMissingAndCorruptDefaultsBeforeWrite(t *testing.T) {
	f := setup(t)
	b := makeBundle(t, "generation-a", "Retry uploads safely.")
	f.publish(t, b)
	ctx := testContext(t)
	good := catalog.TargetUpdate{Name: "kubernetes", Value: catalog.TargetValue{Collection: "guide", Generation: "generation-a", DefaultEdition: "one"}}
	before, err := Activate(ctx, f.cat, f.objects, good, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []catalog.TargetValue{
		{Collection: "guide", Generation: "not-published", DefaultEdition: "one"},
		{Collection: "guide", Generation: "generation-a", DefaultEdition: "absent"},
	} {
		u := catalog.TargetUpdate{Name: "kubernetes", ExpectedRevision: 1, Value: value}
		if got, err := Activate(ctx, f.cat, f.objects, u, testLimits()); err == nil || got != (catalog.Target{}) {
			t.Fatal("unchecked selection was activated", got, err)
		}
	}
	good.ExpectedRevision = 1
	for _, artifact := range []string{f.path(t, ref(b.manifest)), f.path(t, b.description.Editions[0].Index)} {
		for _, corrupt := range []bool{false, true} {
			f.remote.mu.Lock()
			original := bytes.Clone(f.remote.bodies[artifact])
			if corrupt {
				f.remote.bodies[artifact] = bytes.Repeat([]byte{'x'}, len(original))
			} else {
				delete(f.remote.bodies, artifact)
			}
			f.remote.mu.Unlock()
			got, err := Activate(ctx, f.cat, f.objects, good, testLimits())
			f.remote.mu.Lock()
			f.remote.bodies[artifact] = original
			f.remote.mu.Unlock()
			if err == nil || got != (catalog.Target{}) {
				t.Fatal("missing/corrupt artifact changed default", artifact, corrupt, got, err)
			}
		}
	}
	if after, err := f.cat.ResolveTarget(ctx, "kubernetes"); err != nil || after != before {
		t.Fatal("failed preflight changed target", after, err)
	}
	f.server.Close()
	good.ExpectedRevision = 0 // Replay of the already-applied original request.
	if got, err := Activate(ctx, f.cat, f.objects, good, testLimits()); err == nil || got != (catalog.Target{}) {
		t.Fatal("outage must not invent successful verification", got, err)
	}
	if after, err := f.cat.ResolveTarget(ctx, "kubernetes"); err != nil || after != before {
		t.Fatal("catalog result must remain discoverable during object outage", after, err)
	}
}

func TestInvalidTargetRequestsFailBeforeDatabaseOrObjectIO(t *testing.T) {
	f := setup(t)
	f.pool.Close()
	ctx := testContext(t)
	valid := catalog.TargetUpdate{Name: "docs", Value: catalog.TargetValue{Collection: "guide", Generation: "g", DefaultEdition: "one"}}
	for _, badContext := range []context.Context{nil, context.Background()} {
		if got, err := Activate(badContext, f.cat, f.objects, valid, testLimits()); !errors.Is(err, catalog.ErrInvalid) || got != (catalog.Target{}) {
			t.Fatal(got, err)
		}
		if _, err := Resolve(badContext, f.cat, "docs", ""); !errors.Is(err, catalog.ErrInvalid) {
			t.Fatal(err)
		}
	}
	for _, revision := range []int64{-1, math.MaxInt64} {
		bad := valid
		bad.ExpectedRevision = revision
		if got, err := Activate(ctx, f.cat, f.objects, bad, testLimits()); !errors.Is(err, catalog.ErrInvalid) || got != (catalog.Target{}) {
			t.Fatal(got, err)
		}
	}
	for _, edition := range []string{"\x00", "\xff", strings.Repeat("x", 257)} {
		if got, err := Resolve(ctx, f.cat, "docs", edition); !errors.Is(err, catalog.ErrInvalid) || got != (publishedsearch.Selection{}) {
			t.Fatal("bad override reached lookup", got, err)
		}
	}
	if got, err := Resolve(ctx, f.cat, "", ""); !errors.Is(err, catalog.ErrInvalid) || got != (publishedsearch.Selection{}) {
		t.Fatal(got, err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if got, err := Activate(canceled, f.cat, f.objects, valid, testLimits()); !errors.Is(err, context.Canceled) || got != (catalog.Target{}) {
		t.Fatal(got, err)
	}
	if got, err := Resolve(canceled, f.cat, "docs", ""); !errors.Is(err, context.Canceled) || got != (publishedsearch.Selection{}) {
		t.Fatal(got, err)
	}
	if len(f.remote.counts()) != 0 {
		t.Fatal("invalid request caused object access", f.remote.counts())
	}
}
