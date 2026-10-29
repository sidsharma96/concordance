package capturefiles

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/sidsharma96/concordance/internal/htmlarticle"
	"github.com/sidsharma96/concordance/internal/preparedimport"
)

func testContext(t testing.TB) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)
	return ctx
}
func testLimits() Limits {
	return Limits{MaxDescriptorBytes: 1 << 20, MaxEditions: 4, MaxCaptures: 8, MaxPages: 32, MaxDependencies: 64, MaxFiles: 40, MaxStringBytes: 4096, MaxValueBytes: 1 << 18, MaxFileBytes: 1 << 20, MaxPayloadBytes: 8 << 20}
}
func reference(name string, body []byte) File {
	return File{Path: name, SHA256: sha256.Sum256(body), Bytes: int64(len(body))}
}

const htmlBody = `<main class="fixture"><h1>Pods</h1><p>Retry uploads safely.</p></main>`
const recipeBody = "authored synthetic fixture/v1; print excluded"

func description() Description {
	p := htmlarticle.Profile{ID: "fixture/v1", ArticleClass: "fixture", OmitClasses: []string{"chrome", "footer"}, OmitIDs: []string{}, NoticeClasses: []string{"alert", "notice"}}
	c := Capture{Source: "docs", Revision: strings.Repeat("a", 40), Directory: "docs", Preparation: "authored/v1", Toolchain: "no renderer", Recipe: reference("recipe.txt", []byte(recipeBody)), Dependencies: []string{"includes/sample.yaml"}, Profile: p, Pages: []Page{{"docs/b.md", reference("pages/shared.html", []byte(htmlBody))}, {"docs/a.md", reference("pages/shared.html", []byte(htmlBody))}}}
	return Description{Editions: []Edition{{ID: "one", Captures: []Capture{c}}, {ID: "empty", Captures: []Capture{}}}}
}
func writeFile(t testing.TB, root, name string, body []byte) {
	t.Helper()
	target := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, body, 0600); err != nil {
		t.Fatal(err)
	}
}
func writeCapture(t testing.TB, d Description) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, root, "recipe.txt", []byte(recipeBody))
	writeFile(t, root, "pages/shared.html", []byte(htmlBody))
	b, err := Encode(testContext(t), d, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, DescriptorName, b)
	return root
}
func expected() []preparedimport.Edition {
	c := description().Editions[0].Captures[0]
	return []preparedimport.Edition{{ID: "one", Captures: []preparedimport.Capture{{Source: c.Source, Revision: c.Revision, Directory: c.Directory, Preparation: c.Preparation, Toolchain: c.Toolchain, Recipe: []byte(recipeBody), Dependencies: c.Dependencies, Profile: c.Profile, Pages: []preparedimport.Page{{Path: "docs/b.md", HTML: []byte(htmlBody)}, {Path: "docs/a.md", HTML: []byte(htmlBody)}}}}}, {ID: "empty", Captures: []preparedimport.Capture{}}}
}

func TestCaptureFilesRetainOwnedVerifiedBytes(t *testing.T) {
	d := description()
	root := writeCapture(t, d)
	out, err := ReadDirectory(testContext(t), root, testLimits())
	if err != nil || !reflect.DeepEqual(out, expected()) {
		t.Fatal(out, err)
	}
	c := out[0].Captures[0]
	if &c.Pages[0].HTML[0] != &c.Pages[1].HTML[0] {
		t.Fatal("same payload path was not shared")
	}
	if err := os.RemoveAll(root); err != nil {
		t.Fatal(err)
	}
	if string(c.Recipe) != recipeBody || string(c.Pages[0].HTML) != htmlBody {
		t.Fatal("owned bytes changed after directory removal")
	}
	// Ownership belongs to the caller; aliases within this result share storage.
	c.Pages[0].HTML[0] = 'X'
	if c.Pages[1].HTML[0] != 'X' {
		t.Fatal("documented alias changed")
	}
	t.Log("verified payload survives removing capture directory")
}
func TestCaptureFilesIndependentWireAndRequiredEmptyLists(t *testing.T) {
	const empty = `{"format":"concordance.git-html-capture/v1","editions":[]}`
	ctx := testContext(t)
	b, err := Encode(ctx, Description{}, testLimits())
	if err != nil || string(b) != empty {
		t.Fatal(string(b), err)
	}
	if d, err := decode(ctx, []byte(empty), testLimits()); err != nil || len(d.Editions) != 0 {
		t.Fatal(d, err)
	}
	// Handwritten input, independent of Encode: reversed object key ordering,
	// escaped key spelling, whitespace and empty lists remain meaningful values.
	wire := fmt.Sprintf(`{"editions":[{"captures":[{"pages":[{"html":{"bytes":%d,"sha256":"%x","path":"pages/shared.html"},"path":"docs/a.md"}],"profile":{"notice_classes":[],"omit_ids":[],"omit_classes":[],"article_class":"fixture","id":"fixture/v1"},"dependencies":[],"recipe":{"bytes":%d,"sha256":"%x","path":"recipe.txt"},"toolchain":"no renderer","preparation":"authored/v1","directory":"docs","revision":"%s","source":"docs"}],"id":"one"}],"fo\u0072mat":"%s"}`, len(htmlBody), sha256.Sum256([]byte(htmlBody)), len(recipeBody), sha256.Sum256([]byte(recipeBody)), strings.Repeat("a", 40), Format)
	root := t.TempDir()
	writeFile(t, root, "pages/shared.html", []byte(htmlBody))
	writeFile(t, root, "recipe.txt", []byte(recipeBody))
	writeFile(t, root, DescriptorName, []byte(wire))
	out, err := ReadDirectory(ctx, root, testLimits())
	if err != nil || len(out) != 1 || len(out[0].Captures[0].Pages) != 1 || string(out[0].Captures[0].Pages[0].HTML) != htmlBody {
		t.Fatal(out, err)
	}
}
func TestCaptureDescriptorRejectsMalformedMetadata(t *testing.T) {
	ctx := testContext(t)
	base, err := Encode(ctx, description(), testLimits())
	if err != nil {
		t.Fatal(err)
	}
	replace := func(old, next string) []byte { return bytes.Replace(base, []byte(old), []byte(next), 1) }
	cases := []struct {
		name string
		body []byte
	}{
		{"duplicate key", replace(`"format":`, `"format":"x","format":`)},
		{"escaped duplicate", replace(`"format":`, `"fo\u0072mat":"x","format":`)},
		{"unknown key", replace(`"format":`, `"ignored":0,"format":`)},
		{"null list", replace(`"omit_ids":[]`, `"omit_ids":null`)},
		{"missing required empty list", replace(`"omit_ids":[],`, ``)},
		{"null file", replace(`"recipe":{`, `"recipe":null,"unknown":{`)},
		{"float bytes", replace(fmt.Sprintf(`"bytes":%d`, len(recipeBody)), `"bytes":1.0`)},
		{"exponent bytes", replace(fmt.Sprintf(`"bytes":%d`, len(recipeBody)), `"bytes":1e1`)},
		{"negative bytes", replace(fmt.Sprintf(`"bytes":%d`, len(recipeBody)), `"bytes":-1`)},
		{"overflow bytes", replace(fmt.Sprintf(`"bytes":%d`, len(recipeBody)), `"bytes":9223372036854775808`)},
		{"surrogate", replace(`"no renderer"`, `"\ud800"`)},
		{"raw invalid UTF8", replace(`"no renderer"`, "\"\xff\"")},
		{"trailing value", append(bytes.Clone(base), []byte(` {}`)...)},
		{"truncated", base[:len(base)-1]},
		{"uppercase digest", replace(fmt.Sprintf("%x", sha256.Sum256([]byte(recipeBody))), strings.ToUpper(fmt.Sprintf("%x", sha256.Sum256([]byte(recipeBody)))))},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if d, err := decode(ctx, tt.body, testLimits()); err == nil || !reflect.DeepEqual(d, Description{}) {
				t.Fatal(d, err)
			}
		})
	}
	unsupported := replace(Format, "future/v2")
	if _, err := decode(ctx, unsupported, testLimits()); !errors.Is(err, ErrUnsupported) {
		t.Fatal(err)
	}
	for _, valid := range []string{`"\ud83d\ude80"`, `"�"`} {
		if _, err := decode(ctx, replace(`"no renderer"`, valid), testLimits()); err != nil {
			t.Fatal("valid Unicode", err)
		}
	}
}
func TestCaptureMetadataFailurePrecedesPayloadReads(t *testing.T) {
	tests := []struct {
		name   string
		change func(*Description)
	}{
		{"traversal", func(d *Description) { d.Editions[0].Captures[0].Recipe.Path = "../private" }},
		{"absolute", func(d *Description) { d.Editions[0].Captures[0].Recipe.Path = "/private" }},
		{"reserved", func(d *Description) { d.Editions[0].Captures[0].Recipe.Path = DescriptorName }},
		{"conflicting payload", func(d *Description) { d.Editions[0].Captures[0].Pages[1].HTML.SHA256[0]++ }},
		{"duplicate edition", func(d *Description) { d.Editions[1].ID = "one" }},
		{"duplicate capture", func(d *Description) {
			d.Editions[0].Captures = append(d.Editions[0].Captures, d.Editions[0].Captures[0])
		}},
		{"duplicate page", func(d *Description) { d.Editions[0].Captures[0].Pages[1].Path = "docs/b.md" }},
		{"duplicate dependency", func(d *Description) {
			d.Editions[0].Captures[0].Dependencies = []string{"include.yaml", "include.yaml"}
		}},
		{"overlap", func(d *Description) { d.Editions[0].Captures[0].Dependencies = []string{"docs/a.md"} }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := description()
			tt.change(&d)
			if b, err := Encode(testContext(t), d, testLimits()); err == nil || b != nil {
				t.Fatal("encoder accepted invalid metadata", err)
			}
			if _, err := validate(testContext(t), d, testLimits()); err == nil {
				t.Fatal("validation passed")
			}
		})
	}
	// Reader must reject a malformed descriptor before trying its missing recipe.
	root := t.TempDir()
	b, err := Encode(testContext(t), description(), testLimits())
	if err != nil {
		t.Fatal(err)
	}
	b = bytes.Replace(b, []byte(`"recipe.txt"`), []byte(`"../private"`), 1)
	writeFile(t, root, DescriptorName, b)
	if out, err := ReadDirectory(testContext(t), root, testLimits()); !errors.Is(err, ErrInvalid) || out != nil {
		t.Fatal(out, err)
	}
}
func TestCaptureFilesRejectMissingCorruptOrUnsafePayload(t *testing.T) {
	for _, name := range []string{"missing", "same-sized corruption", "truncated", "oversized", "directory", "outside symlink", "ancestor outside symlink"} {
		t.Run(name, func(t *testing.T) {
			root := writeCapture(t, description())
			target := filepath.Join(root, "pages/shared.html")
			switch name {
			case "missing":
				if err := os.Remove(target); err != nil {
					t.Fatal(err)
				}
			case "same-sized corruption":
				changed := []byte(htmlBody)
				changed[1] = 'X'
				writeFile(t, root, "pages/shared.html", changed)
			case "truncated":
				writeFile(t, root, "pages/shared.html", []byte(htmlBody[:len(htmlBody)-1]))
			case "oversized":
				writeFile(t, root, "pages/shared.html", []byte(htmlBody+"x"))
			case "directory":
				if err := os.Remove(target); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(target, 0700); err != nil {
					t.Fatal(err)
				}
			case "outside symlink":
				outside := filepath.Join(t.TempDir(), "outside")
				if err := os.WriteFile(outside, []byte(htmlBody), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(target); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, target); err != nil {
					t.Skip(err)
				}
			case "ancestor outside symlink":
				outside := t.TempDir()
				writeFile(t, outside, "shared.html", []byte(htmlBody))
				if err := os.RemoveAll(filepath.Dir(target)); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, filepath.Dir(target)); err != nil {
					t.Skip(err)
				}
			}
			if out, err := ReadDirectory(testContext(t), root, testLimits()); err == nil || out != nil {
				t.Fatal(out, err)
			}
			t.Logf("rejected: %s", name)
		})
	}
}
func TestCaptureFileLimitsAndContext(t *testing.T) {
	d := description()
	root := writeCapture(t, d)
	l := testLimits()
	b, err := Encode(testContext(t), d, l)
	if err != nil {
		t.Fatal(err)
	}
	l.MaxDescriptorBytes = len(b)
	l.MaxPayloadBytes = int64(len(recipeBody) + 2*len(htmlBody))
	l.MaxFiles = 3
	l.MaxPages = 2
	l.MaxEditions = 2
	l.MaxCaptures = 1
	l.MaxDependencies = 1
	if _, err := ReadDirectory(testContext(t), root, l); err != nil {
		t.Fatal("exact budgets", err)
	}
	cases := []struct {
		name   string
		change func(*Limits)
	}{{"descriptor", func(l *Limits) { l.MaxDescriptorBytes-- }}, {"payload occurrences", func(l *Limits) { l.MaxPayloadBytes-- }}, {"file occurrences", func(l *Limits) { l.MaxFiles-- }}, {"pages", func(l *Limits) { l.MaxPages-- }}, {"editions", func(l *Limits) { l.MaxEditions-- }}, {"value bytes", func(l *Limits) { l.MaxValueBytes = 64 }}, {"file bytes", func(l *Limits) { l.MaxFileBytes = int64(len(htmlBody) - 1) }}}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			small := l
			tt.change(&small)
			if out, err := ReadDirectory(testContext(t), root, small); !errors.Is(err, ErrLimit) || out != nil {
				t.Fatal(out, err)
			}
		})
	}
	ctx, cancel := context.WithCancel(testContext(t))
	cancel()
	if _, err := ReadDirectory(ctx, root, l); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	for _, ctx := range []context.Context{nil, context.Background()} {
		if _, err := ReadDirectory(ctx, root, l); !errors.Is(err, ErrInvalid) {
			t.Fatal(err)
		}
	}
	invalid := l
	invalid.MaxDescriptorBytes = int(^uint(0) >> 1)
	if _, err := ReadDirectory(testContext(t), root, invalid); !errors.Is(err, ErrInvalid) {
		t.Fatal("probe overflow admitted", err)
	}
	// Git paths are metadata, not portable payload routes: a colon remains valid.
	d.Editions[0].Captures[0].Pages[0].Path = "docs/a:b.md"
	if _, err := Encode(testContext(t), d, testLimits()); err != nil {
		t.Fatal("Git path narrowed", err)
	}
}
func TestCaptureEncodeBorrowsAndReturnsIndependentBytes(t *testing.T) {
	d := description()
	before := description()
	b, err := Encode(testContext(t), d, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(d, before) {
		t.Fatal("input mutated")
	}
	clear(b)
	d.Editions[0].Captures[0].Profile.OmitClasses[0] = "changed"
	if slices.Equal(d.Editions[0].Captures[0].Profile.OmitClasses, before.Editions[0].Captures[0].Profile.OmitClasses) {
		t.Fatal("test fixture unexpectedly aliases")
	}
}
func FuzzCaptureDescriptor(f *testing.F) {
	b, err := Encode(testContext(f), description(), testLimits())
	if err != nil {
		f.Fatal(err)
	}
	f.Add(b)
	f.Add([]byte(`{"format":"concordance.git-html-capture/v1","editions":[]}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"format":"bad","editions":[]}`))
	f.Fuzz(func(t *testing.T, b []byte) {
		l := testLimits()
		l.MaxDescriptorBytes = 4096
		ctx := testContext(t)
		d, err := decode(ctx, b, l)
		if err != nil {
			return
		}
		if _, err := validate(ctx, d, l); err != nil {
			return
		}
		encoded, err := Encode(ctx, d, l)
		if err != nil {
			return
		}
		again, err := decode(ctx, encoded, l)
		if err != nil || !reflect.DeepEqual(d, again) {
			t.Fatal("round trip changed decoded metadata", err)
		}
	})
}
