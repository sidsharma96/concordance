package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestSearchRequestedQuoteCount(t *testing.T) {
	f := setup(t)
	f.publish(t, makeBundleWithDocuments(t, "generation-a", "Retry uploads safely.", 12))
	activateFixture(t, f, "generation-a")
	for _, tc := range []struct {
		name  string
		flags []string
		shown int
	}{
		{"default", nil, 10},
		{"explicit", []string{"--limit=1"}, 1},
		{"larger than count", []string{"--limit=100"}, 12},
		{"all", []string{"--all"}, 12},
		{"disabled all", []string{"--all=false", "--limit=1"}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f.remote.reset()
			opened, closed := 0, 0
			var out bytes.Buffer
			args := append(append([]string{}, tc.flags...), "kubernetes", "retry")
			if err := runSearch(testContext(t), args, &out, environment(configEnvironment()), cliConnector(f, &opened, &closed)); err != nil {
				t.Fatal(err)
			}
			header := fmt.Sprintf("Showing %d of 12 indexed matches", tc.shown)
			if !strings.Contains(out.String(), header) || strings.Count(out.String(), "retained blocks") != tc.shown || f.remote.counts()["GET"] != tc.shown+2 || opened != 1 || closed != 1 {
				t.Fatal("wrong batch/count/lifetime", out.String(), f.remote.counts(), opened, closed)
			}
			t.Logf("%s: shown=%d, indexed matches=12, GETs=%d, cleanup=1", tc.name, tc.shown, f.remote.counts()["GET"])
		})
	}
}

func TestSearchLimitDoesNotBypassMatchCeilingOrLaterQuoteFailure(t *testing.T) {
	f := setup(t)
	f.publish(t, makeBundleWithDocuments(t, "generation-a", "Retry uploads safely.", 12))
	activateFixture(t, f, "generation-a")
	for _, kind := range []string{"complete match ceiling", "second selected quote", "cancel second selected quote"} {
		t.Run(kind, func(t *testing.T) {
			f.remote.reset()
			ctx, cancel := context.WithCancel(testContext(t))
			defer cancel()
			args := []string{"--limit=2", "kubernetes", "retry"}
			f.remote.mu.Lock()
			switch kind {
			case "complete match ceiling":
				args = []string{"--limit=1", "--max-matches=1", "kubernetes", "retry"}
			case "second selected quote":
				f.remote.failGETAt = 4
			case "cancel second selected quote":
				f.remote.failGETAt = 0
				f.remote.onGET = func(n int) {
					if n == 4 {
						cancel()
					}
				}
			}
			f.remote.mu.Unlock()
			opened, closed := 0, 0
			var out bytes.Buffer
			err := runSearch(ctx, args, &out, environment(configEnvironment()), cliConnector(f, &opened, &closed))
			if err == nil || out.Len() != 0 || opened != 1 || closed != 1 {
				t.Fatal("partial output or missing cleanup", err, out.String(), opened, closed)
			}
			if kind == "complete match ceiling" && f.remote.counts()["GET"] != 2 {
				t.Fatal("candidate overflow fetched articles", f.remote.counts())
			}
			if kind == "cancel second selected quote" && !errors.Is(err, context.Canceled) {
				t.Fatal("lost cancellation", err)
			}
		})
	}
}
