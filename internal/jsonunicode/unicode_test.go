package jsonunicode

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestCheckSurrogates(t *testing.T) {
	tests := []struct {
		name, body string
		valid      bool
	}{
		{"empty", "", true},
		{"ordinary UTF-8", `{"text":"hello 世界 �"}`, true},
		{"escaped replacement", `"\ufffd"`, true},
		{"BMP edges", `"\u0000\ud7ff\ue000\uFFFF"`, true},
		{"pair edges", `"\ud800\udc00\uDBFF\uDFFF"`, true},
		{"escaped backslash", `"\\ud800"`, true},
		{"escaped quote", `"\"\ud800\udc00"`, true},
		{"separate strings", `["\u0041","\ud800\udc00"]`, true},
		{"low alone", `"\udc00"`, false},
		{"high alone", `"\ud800"`, false},
		{"reversed pair", `"\udc00\ud800"`, false},
		{"two highs", `"\ud800\ud800"`, false},
		{"high then BMP", `"\ud800\u0041"`, false},
		{"pair split by strings", `["\ud800","\udc00"]`, false},
		{"pair split by text", `"\ud800x\udc00"`, false},
		{"escaped low is text", `"\ud800\\udc00"`, false},
		{"truncated unit", `"\u123`, false},
		{"nonhex unit", `"\u12xz"`, false},
		{"trailing escape", "\"\\", false},
		// This helper does not replace the caller's JSON grammar or UTF-8 checks.
		{"not JSON", `not JSON`, true},
		{"invalid UTF-8", "\"\xff\"", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(tt.body)
			err := CheckSurrogates(t.Context(), body)
			if tt.valid && err != nil || !tt.valid && !errors.Is(err, ErrInvalid) {
				t.Fatalf("valid=%v: %v", tt.valid, err)
			}
			if string(body) != tt.body {
				t.Fatal("input changed")
			}
		})
	}
}

// Cancel at a deterministic poll rather than coordinating with wall-clock sleeps.
type pollingContext struct {
	context.Context
	remaining int
	cancel    context.CancelFunc
}

func (c *pollingContext) Err() error {
	c.remaining--
	if c.remaining <= 0 {
		c.cancel()
	}
	return c.Context.Err()
}

func TestCheckSurrogatesCancellation(t *testing.T) {
	for _, body := range []string{"", `"plain"`, `"` + strings.Repeat("x", 4096) + `"`, `"` + strings.Repeat(`\ud800\udc00`, 1024) + `"`} {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if err := CheckSurrogates(ctx, []byte(body)); err != context.Canceled {
			t.Fatal(err)
		}
	}
	base, cancel := context.WithCancel(t.Context())
	defer cancel()
	ctx := &pollingContext{Context: base, remaining: 2, cancel: cancel}
	if err := CheckSurrogates(ctx, []byte(`"`+strings.Repeat("x", 4096)+`"`)); err != context.Canceled {
		t.Fatal("scan did not stop at its second cancellation poll", err)
	}
}
