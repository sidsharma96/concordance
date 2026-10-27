package textterms

import (
	"context"
	"errors"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
	"unicode"
	"unsafe"
)

func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func testLimits() Limits {
	return Limits{MaxInputBytes: 1 << 20, MaxTerms: 10000, MaxTermBytes: 10000, MaxOutputBytes: 1 << 20}
}

func TestPrepareTerms(t *testing.T) {
	for _, tc := range []struct {
		text string
		want []string
	}{
		{"ContainerRestartRules C++ C C# C++17", []string{"containerrestartrules", "c++", "c", "c#", "c++17"}},
		{"readiness-probe v1.35 net/http kubernetes.io/name.", []string{"readiness-probe", "v1.35", "net/http", "kubernetes.io/name"}},
		{"(C++), [readiness-probe]; `v1.35`!", []string{"c++", "readiness-probe", "v1.35"}},
		{"a..b a--b a//b x+y a##b C++/CLI", []string{"a", "b", "a", "b", "a", "b", "x+y", "a##b", "c++/cli"}},
		{"--dry-run -1 +2 #tag .NET _ _private", []string{"dry-run", "1", "2", "tag", "net", "_", "_private"}},
		{"https://example.com/a?key=value foo:bar a—b", []string{"https", "example.com/a", "key", "value", "foo", "bar", "a", "b"}},
		{"\u0301É E\u0301 ÜBER ΚΟΣΜΟΣ 日本語 ١٢٣", []string{"é", "e\u0301", "über", "κοσμοσ", "日本語", "١٢٣"}},
		{"Straße STRASSE Σ ς", []string{"straße", "strasse", "σ", "ς"}},
		{"One one ONE", []string{"one", "one", "one"}},
		{"a\ufffdb", []string{"a", "b"}},
		{"", nil},
		{" \n\t.,--/++#\u0301", nil},
	} {
		t.Run(tc.text, func(t *testing.T) {
			got, err := Prepare(testContext(t), tc.text, testLimits())
			if err != nil || got.Version != Version || !reflect.DeepEqual(got.Terms, tc.want) {
				t.Fatalf("got %#v, %v; want %q", got, err, tc.want)
			}
		})
	}
	if !strings.Contains(Version, unicode.Version) {
		t.Fatal("compatibility tag omits Unicode table version")
	}
}

func TestPrepareRejectsMalformedInput(t *testing.T) {
	for _, text := range []string{"\xff", "valid \xff", "word\x00", " \x00 ", "é\xc3", "word.\xed\xa0\x80"} {
		got, err := Prepare(testContext(t), text, testLimits())
		if !errors.Is(err, ErrInvalid) || !reflect.DeepEqual(got, Prepared{}) {
			t.Fatalf("input %q: %#v, %v", text, got, err)
		}
	}
	for _, limits := range []Limits{
		{}, {MaxInputBytes: -1, MaxTerms: 1, MaxTermBytes: 1, MaxOutputBytes: 1},
		{MaxInputBytes: 1, MaxTerms: 0, MaxTermBytes: 1, MaxOutputBytes: 1},
		{MaxInputBytes: 1, MaxTerms: 1, MaxTermBytes: 0, MaxOutputBytes: 1},
		{MaxInputBytes: 1, MaxTerms: 1, MaxTermBytes: 1, MaxOutputBytes: 0},
	} {
		got, err := Prepare(testContext(t), "", limits)
		if !errors.Is(err, ErrInvalid) || !reflect.DeepEqual(got, Prepared{}) {
			t.Fatal(got, err, limits)
		}
	}
}

func TestPrepareExactAndExcessiveLimits(t *testing.T) {
	// U+023A lowercases to U+2C65: two input bytes become three output bytes.
	for _, tc := range []struct {
		name, text string
		exact      Limits
		reduce     func(*Limits)
	}{
		{"input", " x ", Limits{3, 1, 1, 1}, func(l *Limits) { l.MaxInputBytes-- }},
		{"repeated occurrences", "a a", Limits{3, 2, 1, 2}, func(l *Limits) { l.MaxTerms-- }},
		{"raw term", "Ⱥ", Limits{2, 1, 3, 3}, func(l *Limits) { l.MaxTermBytes = 1 }},
		{"lowercase term expansion", "Ⱥ", Limits{2, 1, 3, 3}, func(l *Limits) { l.MaxTermBytes-- }},
		{"raw term contraction", "K", Limits{3, 1, 3, 1}, func(l *Limits) { l.MaxTermBytes-- }},
		{"total lowercase bytes", "Ⱥ A", Limits{4, 2, 3, 4}, func(l *Limits) { l.MaxOutputBytes-- }},
		{"long technical term", "C++17", Limits{5, 1, 5, 5}, func(l *Limits) { l.MaxTermBytes-- }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Prepare(testContext(t), tc.text, tc.exact)
			if err != nil || got.Version != Version {
				t.Fatal("exact limit rejected", got, err)
			}
			lower := tc.exact
			tc.reduce(&lower)
			got, err = Prepare(testContext(t), tc.text, lower)
			if !errors.Is(err, ErrLimit) || !reflect.DeepEqual(got, Prepared{}) {
				t.Fatal("over-limit returned result", got, err)
			}
		})
	}
	limits := testLimits()
	limits.MaxTerms, limits.MaxTermBytes, limits.MaxOutputBytes = 1, 1, 1
	if got, err := Prepare(testContext(t), strings.Repeat(".", 100), limits); err != nil || len(got.Terms) != 0 {
		t.Fatal("separators charged as output", got, err)
	}
}

func TestPrepareOutputDoesNotRetainInput(t *testing.T) {
	text := strings.Repeat(" ", 1<<16) + "already_lowercase C++"
	got, err := Prepare(testContext(t), text, testLimits())
	if err != nil || !reflect.DeepEqual(got.Terms, []string{"already_lowercase", "c++"}) {
		t.Fatal(got, err)
	}
	// Observe addresses only: no unsafe string construction or mutation. This catches the
	// small lowercase substring retaining the large original allocation.
	start := uintptr(unsafe.Pointer(unsafe.StringData(text)))
	for _, term := range got.Terms {
		address := uintptr(unsafe.Pointer(unsafe.StringData(term)))
		if address >= start && address-start < uintptr(len(text)) {
			t.Fatal("term retains input backing storage")
		}
	}
	runtime.KeepAlive(text)
	got.Terms[0] = "changed"
	again, err := Prepare(testContext(t), text, testLimits())
	if err != nil || again.Terms[0] != "already_lowercase" {
		t.Fatal("output aliases another call", again, err)
	}
}

type countedContext struct {
	context.Context
	checks, cancelAt int
}

func (c *countedContext) Err() error {
	c.checks++
	if c.cancelAt > 0 && c.checks >= c.cancelAt {
		return context.Canceled
	}
	return c.Context.Err()
}

func TestPrepareCancellation(t *testing.T) {
	deadline := testContext(t)
	canceled, cancel := context.WithCancel(deadline)
	cancel()
	expired, expire := context.WithDeadline(t.Context(), time.Time{})
	defer expire()
	for _, tc := range []struct {
		ctx  context.Context
		want error
	}{
		{t.Context(), ErrInvalid},
		{canceled, context.Canceled},
		{expired, context.DeadlineExceeded},
	} {
		got, err := Prepare(tc.ctx, "C++", testLimits())
		if !errors.Is(err, tc.want) || !reflect.DeepEqual(got, Prepared{}) {
			t.Fatal(got, err)
		}
	}
	count := &countedContext{Context: deadline}
	if _, err := Prepare(count, "C++ probe", testLimits()); err != nil {
		t.Fatal(err)
	}
	// Inject at every cooperative boundary, including the final check after
	// output construction. The context seam avoids sleeps/timing assumptions.
	for boundary := 1; boundary <= count.checks; boundary++ {
		ctx := &countedContext{Context: deadline, cancelAt: boundary}
		got, err := Prepare(ctx, "C++ probe", testLimits())
		if !errors.Is(err, context.Canceled) || !reflect.DeepEqual(got, Prepared{}) {
			t.Fatal("partial result at boundary", boundary, got, err)
		}
	}
}
