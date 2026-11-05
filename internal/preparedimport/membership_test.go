package preparedimport

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/sidsharma96/concordance/internal/catalog"
	"github.com/sidsharma96/concordance/internal/gitsource"
	"github.com/sidsharma96/concordance/internal/publishedsearch"
	"github.com/sidsharma96/concordance/internal/searchtarget"
	"github.com/sidsharma96/concordance/internal/termsearch"
	"github.com/sidsharma96/concordance/internal/textterms"
)

// These are authored HTML fixtures, not the output of a Markdown renderer. Each
// document has one body passage so expected membership and quotes are explicit.
type snapshotDocument struct{ path, text string }

func (f storageFixture) commitSnapshot(t *testing.T, documents []snapshotDocument) []Edition {
	t.Helper()
	if err := os.RemoveAll(filepath.Join(f.repo, "docs")); err != nil {
		t.Fatal(err)
	}
	editions := f.clone()
	c := &editions[0].Captures[0]
	c.Pages = nil
	for _, d := range documents {
		f.write(t, d.path, "# Guide\n\n"+d.text+"\n")
		c.Pages = append(c.Pages, Page{Path: d.path, HTML: []byte(`<main class="fixture"><h1>Guide</h1><p>` + d.text + `</p></main>`)})
	}
	f.git(t, "add", "--all")
	f.git(t, "commit", "-m", "synthetic complete membership")
	c.Revision = f.git(t, "rev-parse", "HEAD")
	return editions
}

func TestCompleteSnapshotsChangeMembershipWithoutChangingOldEvidence(t *testing.T) {
	f := setup(t)
	if err := f.cat.CreateTargetSchema(f.ctx); err != nil {
		t.Fatal(err)
	}
	l := testLimits()
	readerLimits := publishedsearch.Limits{Manifest: l.Manifest, Edition: l.Edition.File, Article: l.Edition.Article}
	objects := f.objects(t)
	beforeDocs := []snapshotDocument{
		{"docs/c.md", "Portable restart guidance."},
		{"docs/a.md", "Stable retry guidance."},
		{"docs/b.md", "Legacy rollback guidance."},
	}
	before := f.commitSnapshot(t, beforeDocs)
	first, err := f.importer.ImportPrepared(f.ctx, "before-key", "before", before)
	if err != nil || first.Duplicate {
		t.Fatal(first, err)
	}
	update := catalog.TargetUpdate{Name: "guide", ExpectedCollection: "guide", Value: catalog.TargetValue{
		Collection: "guide", Generation: first.Generation.ID, DefaultEdition: "one",
	}}
	initial, err := searchtarget.Activate(f.ctx, f.cat, objects, update, readerLimits)
	if err != nil || initial.Revision != 1 {
		t.Fatal(initial, err)
	}
	openTarget := func() *publishedsearch.Reader {
		t.Helper()
		selection, err := searchtarget.Resolve(f.ctx, f.cat, "guide", "")
		if err != nil {
			t.Fatal(err)
		}
		r, err := publishedsearch.Open(f.ctx, f.cat, objects, selection, readerLimits)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	query := func(word string) termsearch.Query {
		return termsearch.Query{Edition: "one", TermsVersion: textterms.Version, Terms: []string{word}}
	}
	// Compare paths and exact quote text, not just counts or rank positions: both
	// complete snapshots have three documents but only one logical key in common.
	check := func(r *publishedsearch.Reader, generation, revision string, docs []snapshotDocument) map[string]publishedsearch.Location {
		t.Helper()
		result, err := r.Search(f.ctx, query("guidance"), searchLimits())
		if err != nil || len(result.Hits) != len(docs) {
			t.Fatalf("membership: hits=%d want=%d error=%v", len(result.Hits), len(docs), err)
		}
		want := map[string]string{}
		for _, d := range docs {
			want[d.path] = "Guide\n" + d.text
		}
		locations := map[string]publishedsearch.Location{}
		for _, h := range result.Hits {
			loc, err := r.Location(f.ctx, h.Ref)
			if err != nil {
				t.Fatal(err)
			}
			path := loc.Document.NativeID
			text, exists := want[path]
			if !exists || loc.Selection != (publishedsearch.Selection{Collection: "guide", Generation: generation, Edition: "one"}) || loc.Document.Source.ID != "docs" || loc.Document.Revision != revision || loc.Document.Location != path || loc.Document.Input.Path != path {
				t.Fatal("wrong document, selection or source evidence", loc)
			}
			quote, err := r.Quote(f.ctx, h.Ref, 4096)
			if err != nil || quote != text {
				t.Fatalf("quote for %s: %q want %q error=%v", path, quote, text, err)
			}
			delete(want, path)
			locations[path] = loc
		}
		return locations
	}
	old := openTarget()
	oldLocations := check(old, "before", before[0].Captures[0].Revision, beforeDocs)
	legacy, err := old.Search(f.ctx, query("legacy"), searchLimits())
	if err != nil || len(legacy.Hits) != 1 {
		t.Fatal(legacy, err)
	}
	oldRef := legacy.Hits[0].Ref
	oldLocation, err := old.Location(f.ctx, oldRef)
	if err != nil {
		t.Fatal(err)
	}

	afterDocs := []snapshotDocument{
		{"docs/moved/c.md", "Portable restart guidance."},
		{"docs/a.md", "Stable retry guidance."},
		{"docs/0-added.md", "Newcomer scheduling guidance."},
	}
	after := f.commitSnapshot(t, afterDocs)
	// An old three-page handoff is still invalid against the new three-file tree.
	// Missing HTML or failed dependency acquisition must not imply removal either.
	for _, sample := range []struct {
		name string
		edit func(*Capture)
		want error
	}{
		{"stale paths", func(c *Capture) { c.Pages = before[0].Captures[0].Pages }, ErrMembership},
		{"missing page", func(c *Capture) { c.Pages = c.Pages[:2] }, ErrMembership},
		{"missing dependency", func(c *Capture) { c.Dependencies = append(c.Dependencies, "missing.yaml") }, gitsource.ErrMissing},
	} {
		t.Run(sample.name, func(t *testing.T) {
			input := f.fixture
			input.editions = after
			invalid := input.clone()
			sample.edit(&invalid[0].Captures[0])
			counts := f.remote.counts()
			out, err := f.importer.ImportPrepared(f.ctx, sample.name, "incomplete", invalid)
			if !errors.Is(err, sample.want) || out != (catalog.Publication{}) || f.remote.counts() != counts {
				t.Fatal("incomplete input reached remote storage", out, err)
			}
			if _, err := f.cat.Lookup(f.ctx, "guide", sample.name); !errors.Is(err, catalog.ErrNotFound) {
				t.Fatal("incomplete input bound operation", err)
			}
			if current, err := f.cat.ResolveTarget(f.ctx, "guide"); err != nil || current != initial {
				t.Fatal("incomplete input changed target", current, err)
			}
		})
	}
	counts := f.remote.counts()
	if out, err := f.importer.ImportPrepared(f.ctx, "before-key", "different", after); !errors.Is(err, catalog.ErrConflict) || out != (catalog.Publication{}) || f.remote.counts() != counts {
		t.Fatal("changed membership reused saved key", out, err)
	}
	second, err := f.importer.ImportPrepared(f.ctx, "after-key", "after", after)
	if err != nil || second.Duplicate {
		t.Fatal(second, err)
	}
	if current, err := f.cat.ResolveTarget(f.ctx, "guide"); err != nil || current != initial {
		t.Fatal("publication activated replacement", current, err)
	}
	check(openTarget(), "before", before[0].Captures[0].Revision, beforeDocs)
	update.Value.Generation, update.ExpectedRevision = second.Generation.ID, 1
	if activated, err := searchtarget.Activate(f.ctx, f.cat, objects, update, readerLimits); err != nil || activated.Revision != 2 {
		t.Fatal(activated, err)
	}
	current := openTarget()
	newLocations := check(current, "after", after[0].Captures[0].Revision, afterDocs)
	if result, err := current.Search(f.ctx, query("legacy"), searchLimits()); err != nil || len(result.Hits) != 0 {
		t.Fatal("removed document remained searchable", result, err)
	}
	if oldLocations["docs/c.md"].Document.Article != newLocations["docs/moved/c.md"].Document.Article || oldLocations["docs/c.md"].Document.Input.Object != newLocations["docs/moved/c.md"].Document.Input.Object {
		t.Fatal("exact renamed bytes lost sharing")
	}
	if quote, err := current.Quote(f.ctx, oldRef, 4096); !errors.Is(err, publishedsearch.ErrInvalid) || quote != "" {
		t.Fatal("old reference accepted by new reader", quote, err)
	}

	// Reusing a removed path supplies new evidence at that key; it must never
	// redirect an old reference to the replacement's bytes or captured revision.
	reusedDocs := append(append([]snapshotDocument(nil), afterDocs...), snapshotDocument{"docs/b.md", "Replacement recovery guidance."})
	reused := f.commitSnapshot(t, reusedDocs)
	third, err := f.importer.ImportPrepared(f.ctx, "reuse-key", "reused", reused)
	if err != nil || third.Duplicate {
		t.Fatal(third, err)
	}
	update.Value.Generation, update.ExpectedRevision = third.Generation.ID, 2
	if activated, err := searchtarget.Activate(f.ctx, f.cat, objects, update, readerLimits); err != nil || activated.Revision != 3 {
		t.Fatal(activated, err)
	}
	check(openTarget(), "reused", reused[0].Captures[0].Revision, reusedDocs)
	check(current, "after", after[0].Captures[0].Revision, afterDocs)
	check(old, "before", before[0].Captures[0].Revision, beforeDocs)
	oldQuote, err := old.Quote(f.ctx, oldRef, 4096)
	if err != nil || oldQuote != "Guide\nLegacy rollback guidance." {
		t.Fatal("old quote redirected or removed", oldQuote, err)
	}
	if loc, err := old.Location(f.ctx, oldRef); err != nil || !reflect.DeepEqual(loc, oldLocation) {
		t.Fatal("old provenance changed", loc, err)
	}
	for _, retry := range []struct {
		key      string
		editions []Edition
		want     catalog.Publication
	}{{"before-key", before, first}, {"after-key", after, second}, {"reuse-key", reused, third}} {
		counts := f.remote.counts()
		out, err := f.importer.ImportPrepared(f.ctx, retry.key, "retry-candidate", retry.editions)
		if err != nil || !out.Duplicate || out.Generation != retry.want.Generation || f.remote.counts() != counts {
			t.Fatal("completed retry changed generation or accessed objects", out, err)
		}
	}
	update.Value.Generation, update.ExpectedRevision = first.Generation.ID, 3
	if rollback, err := searchtarget.Activate(f.ctx, f.cat, objects, update, readerLimits); err != nil || rollback.Revision != 4 {
		t.Fatal(rollback, err)
	}
	check(openTarget(), "before", before[0].Captures[0].Revision, beforeDocs)
	t.Logf("retained evidence: %s at before commit; quote=%q", oldLocation.Document.NativeID, oldQuote)
	t.Log("membership: before=a,b,c; after=0-added,a,moved/c; reused=0-added,a,b,moved/c; rollback=a,b,c")
	t.Log("evidence: removed legacy quote survives path reuse; renamed bytes share storage with distinct paths; incomplete input never binds or accesses objects; three completed retries add no object requests")
}
