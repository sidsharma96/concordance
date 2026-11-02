package catalog

import (
	"errors"
	"strings"
	"testing"
)

func TestTargetExpectedCollectionIsPartOfConditionalWrite(t *testing.T) {
	f := targetFixture(t)
	a := publishTargetGeneration(t, f, "scope-a")
	// Catalog-only trusted metadata, as in the existing target tests. No object
	// body is asserted to exist by this test; application tests check real bodies.
	r := request()
	r.Collection, r.Key = "other", "scope-b"
	if _, err := f.store.Bind(f.ctx, r); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.Publish(f.ctx, r, generation("scope-b")); err != nil {
		t.Fatal(err)
	}
	b := TargetValue{Collection: "other", Generation: "scope-b", DefaultEdition: "1.35"}
	first, err := f.store.SetTarget(f.ctx, TargetUpdate{Name: "docs", Value: b})
	if err != nil {
		t.Fatal(err)
	}
	u := TargetUpdate{Name: "docs", Value: a, ExpectedRevision: 1, ExpectedCollection: "guide"}
	if got, err := f.store.SetTarget(f.ctx, u); !errors.Is(err, ErrTargetConflict) || got != (Target{}) {
		t.Fatal("same-revision collection takeover", got, err)
	}
	if got, err := f.store.ResolveTarget(f.ctx, "docs"); err != nil || got != first {
		t.Fatal("out-of-scope row changed", got, err)
	}
	// A trusted caller authorized for both sides can explicitly name the prior
	// collection. The CLI instead always constrains both sides to its own scope.
	u.ExpectedCollection = "other"
	second, err := f.store.SetTarget(f.ctx, u)
	if err != nil || second.Revision != 2 || second.Value != a {
		t.Fatal(second, err)
	}
	if got, err := f.store.SetTarget(f.ctx, u); err != nil || got != second {
		t.Fatal("exact state retry was lost after a scoped transition", got, err)
	}
	u.ExpectedRevision = 2
	if got, err := f.store.SetTarget(f.ctx, u); !errors.Is(err, ErrTargetConflict) || got != (Target{}) {
		t.Fatal("scope guard ignored on the next revision", got, err)
	}
	u.ExpectedCollection, u.Value = "", b
	if got, err := f.store.SetTarget(f.ctx, u); err != nil || got.Revision != 3 || got.Value != b {
		t.Fatal("trusted unscoped behavior changed", got, err)
	}
	for _, invalid := range []string{"bad\x00scope", "\xff", strings.Repeat("x", 257)} {
		u.ExpectedCollection = invalid
		if !errors.Is(u.Validate(), ErrInvalid) {
			t.Fatal("invalid collection guard admitted")
		}
	}
	t.Log("matching revision with wrong collection conflicts; exact scoped retry and trusted unscoped transitions remain valid")
}
