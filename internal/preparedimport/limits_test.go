package preparedimport

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/sidsharma96/concordance/internal/generationpublish"
)

func TestNewLimitDiagnostics(t *testing.T) {
	// The public configuration contract says every numeric budget is positive.
	// Walk the type independently of validateLimits' inventory to catch omissions
	// when a nested component later gains a budget.
	var visit func(reflect.Type, []int, string)
	visit = func(typ reflect.Type, index []int, prefix string) {
		for n := 0; n < typ.NumField(); n++ {
			field := typ.Field(n)
			path := append(append([]int(nil), index...), n)
			name := prefix + field.Name
			if field.Type.Kind() == reflect.Struct {
				visit(field.Type, path, name+".")
				continue
			}
			if field.Type.Kind() != reflect.Int && field.Type.Kind() != reflect.Int64 {
				t.Fatalf("unexpected limit type %s", name)
			}
			t.Run(name, func(t *testing.T) {
				for _, invalid := range []int64{0, -1} {
					limits := testLimits()
					reflect.ValueOf(&limits).Elem().FieldByIndex(path).SetInt(invalid)
					importer, err := New(&generationpublish.Publisher{}, "guide", nil, limits)
					if importer != nil || !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), name) {
						t.Fatalf("%s=%d: importer=%v error=%v", name, invalid, importer, err)
					}
				}
			})
		}
	}
	visit(reflect.TypeFor[Limits](), nil, "")
}

func TestNewLimitRelationships(t *testing.T) {
	tests := []struct {
		name, field string
		change      func(*Limits)
		valid       bool
	}{
		{"Git exact total", "Git.MaxTotalBytes", func(l *Limits) { l.Git.MaxTotalBytes = l.Git.MaxDocumentBytes }, true},
		{"Git short total", "Git.MaxTotalBytes", func(l *Limits) { l.Git.MaxTotalBytes = l.Git.MaxDocumentBytes - 1 }, false},
		{"files exact total", "Files.MaxTotalBytes", func(l *Limits) { l.Files.MaxTotalBytes = l.Files.MaxFileBytes }, true},
		{"files short total", "Files.MaxTotalBytes", func(l *Limits) { l.Files.MaxTotalBytes = l.Files.MaxFileBytes - 1 }, false},
		{"token below input", "Extract.MaxTokenBytes", func(l *Limits) { l.Extract.MaxTokenBytes = l.Extract.MaxInputBytes - 1 }, true},
		{"token equals input", "Extract.MaxTokenBytes", func(l *Limits) { l.Extract.MaxTokenBytes = l.Extract.MaxInputBytes }, false},
		{"token above input", "Extract.MaxTokenBytes", func(l *Limits) { l.Extract.MaxTokenBytes = l.Extract.MaxInputBytes + 1 }, false},
		{"depth ceiling", "Extract.MaxDepth", func(l *Limits) { l.Extract.MaxDepth = 512 }, true},
		{"depth exceeded", "Extract.MaxDepth", func(l *Limits) { l.Extract.MaxDepth = 513 }, false},
		{"output exact block", "Extract.MaxOutputBytes", func(l *Limits) { l.Extract.MaxOutputBytes = l.Extract.MaxBlockBytes }, true},
		{"output short block", "Extract.MaxOutputBytes", func(l *Limits) { l.Extract.MaxOutputBytes = l.Extract.MaxBlockBytes - 1 }, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limits := testLimits()
			tt.change(&limits)
			importer, err := New(&generationpublish.Publisher{}, "guide", nil, limits)
			if tt.valid {
				if err != nil || importer == nil {
					t.Fatal(importer, err)
				}
			} else if importer != nil || !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), tt.field) {
				t.Fatal(importer, err)
			}
		})
	}
}
