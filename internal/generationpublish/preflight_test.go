package generationpublish

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/sidsharma96/concordance/internal/catalog"
	gm "github.com/sidsharma96/concordance/internal/generationmanifest"
)

func TestPreflightAdmissionAndBindings(t *testing.T) {
	ctx := testContext(t)
	b := makeBundle(t)
	p := &Publisher{objects: referenceClient(t), limits: testLimits()}
	before := bytes.Clone(b.manifest)
	bodies := make([][]byte, len(b.artifacts))
	var total int64
	largest := len(b.manifest)
	total = int64(largest)
	for i, body := range b.artifacts {
		bodies[i] = bytes.Clone(body)
		total += int64(len(body))
		if len(body) > largest {
			largest = len(body)
		}
	}
	p.limits.MaxInputBytes = total
	p.limits.MaxObjects = len(b.artifacts) + 1
	p.limits.MaxBatchBytes = int64(largest)
	got, err := p.preflight(ctx, "guide", b.d.Generation, b.manifest, b.artifacts)
	if err != nil || got.root != storeRef(ref(b.manifest)) || len(got.artifacts) != len(b.artifacts) {
		t.Fatal(got, err)
	}
	if !bytes.Equal(before, b.manifest) || !reflect.DeepEqual(bodies, b.artifacts) {
		t.Fatal("mutated caller buffers")
	}
	slices.Reverse(b.artifacts)
	if _, err := p.preflight(ctx, "guide", b.d.Generation, b.manifest, b.artifacts); err != nil {
		t.Fatal("artifact order should be irrelevant", err)
	}
	for _, tc := range []struct {
		name   string
		change func(*Limits)
	}{
		{"total bytes", func(l *Limits) { l.MaxInputBytes-- }},
		{"objects including root", func(l *Limits) { l.MaxObjects-- }},
		{"single batch bytes", func(l *Limits) { l.MaxBatchBytes-- }},
		{"manifest bytes", func(l *Limits) { l.Manifest.MaxEncodedBytes = len(b.manifest) - 1 }},
		{"article bytes", func(l *Limits) { l.Article.MaxEncodedBytes = 1 }},
		{"edition bytes", func(l *Limits) { l.Edition.MaxEncodedBytes = 1 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			copy := *p
			tc.change(&copy.limits)
			out, err := copy.preflight(ctx, "guide", b.d.Generation, b.manifest, b.artifacts)
			if err == nil || !reflect.DeepEqual(out, prepared{}) {
				t.Fatal(out, err)
			}
		})
	}
	for _, tc := range []struct {
		name   string
		change func(*bundle)
	}{
		{"missing", func(b *bundle) { b.artifacts = b.artifacts[1:] }},
		{"extra", func(b *bundle) { b.artifacts = append(b.artifacts, []byte("extra")) }},
		{"duplicate", func(b *bundle) { b.artifacts[0] = b.artifacts[1] }},
		{"changed bytes", func(b *bundle) { b.artifacts[0][0] ^= 1 }},
		{"bad JSON", func(b *bundle) { b.manifest[0] = '[' }},
		{"profile association", func(b *bundle) { b.d.Editions[0].Documents[0].Profile = "other/v1"; b.encode(t) }},
		{"missing image", func(b *bundle) { b.d.Editions[0].Documents[0].Images = nil; b.encode(t) }},
		{"index swapped", func(b *bundle) {
			b.d.Editions[0].Index, b.d.Editions[1].Index = b.d.Editions[1].Index, b.d.Editions[0].Index
			b.encode(t)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := makeBundle(t)
			tc.change(&b)
			p := &Publisher{objects: referenceClient(t), limits: testLimits()}
			out, err := p.preflight(ctx, "guide", b.d.Generation, b.manifest, b.artifacts)
			if err == nil || !reflect.DeepEqual(out, prepared{}) {
				t.Fatal(out, err)
			}
		})
	}
	for _, pins := range [][2]string{{"other", "generation-1"}, {"guide", "other"}} {
		if _, err := p.preflight(ctx, pins[0], pins[1], b.manifest, b.artifacts); !errors.Is(err, gm.ErrMismatch) {
			t.Fatal("pin mismatch", err)
		}
	}
	c, cancel := context.WithCancel(ctx)
	cancel()
	if out, err := p.preflight(c, "guide", b.d.Generation, b.manifest, b.artifacts); !errors.Is(err, context.Canceled) || !reflect.DeepEqual(out, prepared{}) {
		t.Fatal(out, err)
	}
}
func TestZeroStateDeadlineAndPermit(t *testing.T) {
	ctx := testContext(t)
	b := makeBundle(t)
	for _, p := range []*Publisher{nil, {}} {
		if out, err := p.Publish(ctx, trustedRequest("key"), b.d.Generation, b.manifest, b.artifacts); !errors.Is(err, ErrInvalid) || out != (catalog.Publication{}) {
			t.Fatal(out, err)
		}
	}
	p := &Publisher{objects: referenceClient(t), limits: testLimits(), permit: make(chan struct{}, 1)}
	req := trustedRequest("key")
	req.Collection = "" // Match this zero namespace; preflight still rejects the pin.
	for _, c := range []context.Context{nil, context.Background()} {
		if _, err := p.Publish(c, req, b.d.Generation, b.manifest, b.artifacts); !errors.Is(err, ErrInvalid) {
			t.Fatal(err)
		}
	}
	p.permit <- struct{}{}
	if _, err := p.Publish(ctx, req, b.d.Generation, b.manifest, b.artifacts); !errors.Is(err, ErrBusy) {
		t.Fatal(err)
	}
	<-p.permit
	if _, err := p.Publish(ctx, req, b.d.Generation, b.manifest, b.artifacts); err == nil {
		t.Fatal("bad collection accepted")
	}
	if len(p.permit) != 0 {
		t.Fatal("permit leaked on preflight error")
	}
	if refs, err := p.preflight(ctx, "guide", b.d.Generation, b.manifest, b.artifacts); err != nil || len(refs.artifacts) == 0 {
		t.Fatal(err)
	}
	p.limits.MaxInputBytes = 0
	if _, err := New(nil, nil, catalog.Namespace{}, p.limits); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := New(nil, nil, catalog.Namespace{}, testLimits()); err == nil {
		t.Fatal("nil clients accepted")
	}
}
