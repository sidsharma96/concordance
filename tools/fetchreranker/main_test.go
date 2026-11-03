package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestStreamingDownloadReuseAndMismatch(t *testing.T) {
	body := "synthetic model bytes"
	a := asset{"weights", int64(len(body)), fmt.Sprintf("%x", sha256.Sum256([]byte(body)))}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); fmt.Fprint(w, body) }))
	defer server.Close()
	dir := t.TempDir()
	for range 2 {
		if e := fetch(t.Context(), server.Client(), dir, server.URL, a); e != nil {
			t.Fatal(e)
		}
	}
	if calls.Load() != 1 {
		t.Fatal("valid local asset redownloaded", calls.Load())
	}
	if e := os.WriteFile(filepath.Join(dir, a.Name), []byte(strings.Repeat("x", len(body))), 0o600); e != nil {
		t.Fatal(e)
	}
	if e := fetch(t.Context(), server.Client(), dir, server.URL, a); e == nil || calls.Load() != 1 {
		t.Fatal("mismatch overwritten", e, calls.Load())
	}
}
func TestBadDownloadAndCancelledContextLeaveNoAsset(t *testing.T) {
	for _, body := range []string{"bad", "longer"} {
		t.Run(body, func(t *testing.T) {
			dir := t.TempDir()
			a := asset{"weights", 3, fmt.Sprintf("%x", sha256.Sum256([]byte("yes")))}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
			defer server.Close()
			if e := fetch(t.Context(), server.Client(), dir, server.URL, a); e == nil {
				t.Fatal("bad body accepted")
			}
			entries, e := os.ReadDir(dir)
			if e != nil || len(entries) != 0 {
				t.Fatal("partial asset/temp escaped", entries, e)
			}
		})
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if e := fetch(ctx, http.DefaultClient, t.TempDir(), "http://invalid", asset{Name: "weights"}); e == nil {
		t.Fatal("cancelled download accepted")
	}
}
