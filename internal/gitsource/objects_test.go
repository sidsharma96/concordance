package gitsource

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestListingRejectsMalformedMetadata(t *testing.T) {
	valid := "100644 blob " + strings.Repeat("a", 40) + " 3\tguide.md\x00"
	for _, data := range []string{
		strings.TrimSuffix(valid, "\x00"),
		strings.Replace(valid, " 3\t", " -1\t", 1),
		strings.Replace(valid, " 3\t", " 9223372036854775808\t", 1),
		strings.Replace(valid, "guide.md", "../guide.md", 1),
		strings.Replace(valid, "100644 blob", "100644 tree", 1),
		strings.Replace(valid, "guide.md", string([]byte{0xff}), 1),
		valid + valid,
	} {
		if _, err := parseListing([]byte(data), 10, 40); !errors.Is(err, ErrCorrupt) {
			t.Fatalf("malformed record accepted: %q: %v", data, err)
		}
	}
	if _, err := parseListing([]byte(valid), 0, 40); !errors.Is(err, ErrLimit) {
		t.Fatal("entry bound not enforced:", err)
	}
}

func TestBlobIntegrityAndOutputLimit(t *testing.T) {
	if !matchesBlob(nil, "e69de29bb2d1d6434b8b29ae775ad8c2e48c5391") || matchesBlob([]byte("changed"), "e69de29bb2d1d6434b8b29ae775ad8c2e48c5391") {
		t.Fatal("Git blob header/content verification failed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := cappedOutput{remaining: 2, cancel: cancel}
	if n, err := w.Write([]byte("ab")); n != 2 || err != nil || ctx.Err() != nil {
		t.Fatalf("exact bound: %d, %v, %v", n, err, ctx.Err())
	}
	if n, err := w.Write([]byte("c")); n != 0 || !errors.Is(err, ErrLimit) || !w.exceeded || ctx.Err() != context.Canceled || w.buffer.String() != "ab" {
		t.Fatalf("overflow must cancel without successful truncation: %d, %v", n, err)
	}
}

func FuzzListing(f *testing.F) {
	f.Add([]byte("100644 blob " + strings.Repeat("a", 40) + " 0\tguide.md\x00"))
	f.Add([]byte("040000 tree " + strings.Repeat("b", 40) + " -\tdocs\x00"))
	f.Add([]byte("unterminated"))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 16<<10 {
			t.Skip()
		}
		entries, err := parseListing(data, 16, 40)
		if err != nil {
			return
		}
		if len(entries) > 16 {
			t.Fatal("entry limit exceeded")
		}
		for i, entry := range entries {
			if !cleanPath(entry.name, false) || !objectID(entry.id, 40) || entry.size < 0 || i > 0 && entries[i-1].name >= entry.name {
				t.Fatalf("invalid successful parse: %+v", entries)
			}
		}
	})
}
