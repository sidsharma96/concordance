package textnav

import (
	"context"
	"crypto/sha256"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"
)

func sessionLimits() Limits {
	return Limits{MaxCalls: 100, MaxReturnedBytes: 4096, MaxLiteralBytes: 64, MaxFindMatches: 8}
}

func textCorpus(text string) (*Corpus, Ref) {
	r := Ref{ID: "text", SHA256: sha256.Sum256([]byte(text))}
	return &Corpus{documents: map[string]document{r.ID: {ref: r, text: text}}, bytes: len(text)}, r
}

func TestSessionBudgetsAndBindings(t *testing.T) {
	ctx := testContext(t)
	c, ref := textCorpus("abc")
	l := sessionLimits()
	l.MaxCalls, l.MaxReturnedBytes = 6, 5
	s, _ := c.NewSession(l)
	if _, err := s.Lookup(ctx, ref.ID); err != nil {
		t.Fatal(err)
	}
	if x, err := s.Read(ctx, ref, Span{0, 3}); err != nil || x.Text != "abc" {
		t.Fatal(x, err)
	}
	if x, err := s.Read(ctx, ref, Span{0, 3}); !errors.Is(err, ErrLimit) || x != (Excerpt{}) {
		t.Fatalf("repeat read not charged: %+v %v", x, err)
	}
	if x, err := s.Read(ctx, ref, Span{0, 2}); err != nil || x.Text != "ab" {
		t.Fatal(x, err)
	}
	if x, err := s.Lookup(ctx, "missing"); !errors.Is(err, ErrMissing) || x != (Document{}) {
		t.Fatalf("missing: %+v %v", x, err)
	}
	bad := ref
	bad.SHA256[0] ^= 1
	if x, err := s.Read(ctx, bad, Span{0, 1}); !errors.Is(err, ErrBinding) || x != (Excerpt{}) {
		t.Fatalf("binding: %+v %v", x, err)
	}
	if _, err := s.Lookup(ctx, ref.ID); !errors.Is(err, ErrLimit) || s.Stats() != (Stats{Calls: 6, ReturnedBytes: 5}) {
		t.Fatalf("attempt budget: %+v %v", s.Stats(), err)
	}
}

func TestReadUTF8AndCancellation(t *testing.T) {
	ctx := testContext(t)
	c, ref := textCorpus("aé日🙂z")
	for _, span := range []Span{{1, 10}, {2, 3}, {1, 2}, {-1, 1}, {0, 0}, {0, 12}} {
		s, _ := c.NewSession(sessionLimits())
		x, err := s.Read(ctx, ref, span)
		if span == (Span{1, 10}) {
			if err != nil || x.Text != "é日🙂" || x.Span != span || x.Ref != ref || s.Stats().ReturnedBytes != 9 {
				t.Fatalf("exact UTF8: %+v %v", x, err)
			}
		} else if !errors.Is(err, ErrInvalid) || x != (Excerpt{}) || s.Stats() != (Stats{Calls: 1}) {
			t.Fatalf("split/invalid range: %+v %v %+v", span, err, s.Stats())
		}
	}
	s, _ := c.NewSession(sessionLimits())
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if x, err := s.Read(canceled, ref, Span{0, 1}); x != (Excerpt{}) || !errors.Is(err, context.Canceled) || s.Stats().Calls != 1 {
		t.Fatalf("canceled attempt: %+v %v", x, err)
	}
	if _, err := s.Lookup(context.Background(), ref.ID); !errors.Is(err, ErrInvalid) || s.Stats().Calls != 2 {
		t.Fatalf("missing deadline: %v %+v", err, s.Stats())
	}
	if _, err := s.Lookup(nil, ref.ID); !errors.Is(err, ErrInvalid) || s.Stats().Calls != 3 {
		t.Fatalf("nil context: %v %+v", err, s.Stats())
	}
}

func TestFindLiteralPagesAndOwnedRanges(t *testing.T) {
	ctx := testContext(t)
	c, ref := textCorpus("ééé aa aa AA")
	s, _ := c.NewSession(sessionLimits())
	page, err := s.Find(ctx, ref, "éé", 0, 1)
	if err != nil || !reflect.DeepEqual(page.Matches, []Span{{0, 4}}) || page.More {
		t.Fatalf("non-overlap: %+v %v", page, err)
	}
	page, err = s.Find(ctx, ref, "aa", 0, 1)
	if err != nil || !reflect.DeepEqual(page.Matches, []Span{{7, 9}}) || !page.More || page.Next != 9 {
		t.Fatalf("first page: %+v %v", page, err)
	}
	page.Matches[0].Start = -100
	page, err = s.Find(ctx, ref, "aa", page.Next, 1)
	if err != nil || !reflect.DeepEqual(page.Matches, []Span{{10, 12}}) || page.More || page.Next != len("ééé aa aa AA") {
		t.Fatalf("second page/case sensitivity: %+v %v", page, err)
	}
	if s.Stats().ReturnedBytes != 0 {
		t.Fatal("find emitted body text")
	}
	for _, literal := range []string{"", "\xff", "\x00", strings.Repeat("x", 65)} {
		got, err := s.Find(ctx, ref, literal, 0, 1)
		if err == nil || !reflect.DeepEqual(got, FindResult{}) {
			t.Fatalf("invalid literal accepted: %q %+v %v", literal, got, err)
		}
	}
	if got, err := s.Find(ctx, ref, "é", 1, 1); !errors.Is(err, ErrInvalid) || !reflect.DeepEqual(got, FindResult{}) {
		t.Fatalf("split start: %+v %v", got, err)
	}
}

// The wrapper closes a real context synchronously at a chosen check. This tests
// late cancellation deterministically without timers, sleeps or racing mutation.
type triggerContext struct {
	context.Context
	cancel context.CancelFunc
	left   int
}

func (c *triggerContext) Err() error {
	if c.left == 0 {
		c.cancel()
	} else {
		c.left--
	}
	return c.Context.Err()
}

func TestFindCancellationNeverReturnsPartialRanges(t *testing.T) {
	c, ref := textCorpus("a a a")
	for n := 0; n < 8; n++ {
		ctx, cancel := context.WithCancel(testContext(t))
		probe := &triggerContext{Context: ctx, cancel: cancel, left: n}
		s, _ := c.NewSession(sessionLimits())
		got, err := s.Find(probe, ref, "a", 0, 8)
		cancel()
		if err != nil && (!errors.Is(err, context.Canceled) || !reflect.DeepEqual(got, FindResult{})) {
			t.Fatalf("partial/error after check%d: %+v %v", n, got, err)
		}
	}
}

func TestIndependentSessionsShareImmutableCorpus(t *testing.T) {
	c, ref := textCorpus("é a a a")
	ctx := testContext(t)
	var wg sync.WaitGroup
	errorsOut := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s, err := c.NewSession(sessionLimits())
			if err != nil {
				errorsOut <- err
				return
			}
			for range 20 {
				part, err := s.Read(ctx, ref, Span{0, 2})
				if err != nil || part.Text != "é" {
					errorsOut <- errors.New("independent read failed")
					return
				}
				page, err := s.Find(ctx, ref, "a", 0, 2)
				if err != nil || len(page.Matches) != 2 || !page.More {
					errorsOut <- errors.New("independent find failed")
					return
				}
				page.Matches[0].Start = -1
			}
		}()
	}
	wg.Wait()
	close(errorsOut)
	for err := range errorsOut {
		t.Fatal(err)
	}
}

func FuzzFindPages(f *testing.F) {
	f.Add("ééé aa aa AA", "aa", uint8(1))
	f.Add("🙂x🙂x", "🙂", uint8(2))
	f.Add("aaaa", "aa", uint8(1))
	f.Fuzz(func(t *testing.T, text, literal string, pageSize uint8) {
		if len(text) > 512 || literal == "" || len(literal) > 64 || !utf8.ValidString(text) || !utf8.ValidString(literal) || strings.ContainsRune(text, 0) || strings.ContainsRune(literal, 0) {
			t.Skip()
		}
		var expected []Span
		for i := 0; i+len(literal) <= len(text); {
			if text[i:i+len(literal)] == literal {
				expected = append(expected, Span{i, i + len(literal)})
				i += len(literal)
			} else {
				i++
			}
		}
		c, ref := textCorpus(text)
		l := sessionLimits()
		l.MaxCalls = 2 + len(text)
		s, _ := c.NewSession(l)
		var got []Span
		from := 0
		for {
			page, err := s.Find(testContext(t), ref, literal, from, 1+int(pageSize)%8)
			if err != nil {
				t.Fatal(err)
			}
			got = append(got, page.Matches...)
			if !page.More {
				break
			}
			if page.Next <= from {
				t.Fatal("non-progressing page")
			}
			from = page.Next
		}
		if !reflect.DeepEqual(got, expected) {
			t.Fatalf("find versus simple scan: %+v %+v", got, expected)
		}
	})
}
