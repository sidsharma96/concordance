package htmlarticlejson

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/sidsharma96/concordance/internal/htmlarticle"
)

func limits() Limits { return Limits{1 << 20, 100, 100, 10000, 100000, 20, 1 << 20} }
func deadline(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	t.Cleanup(cancel)
	return ctx
}
func sample() htmlarticle.Article {
	return htmlarticle.Article{Processor: "htmlarticle/v1", Profile: "fixture/v1", HTMLBytes: 1, Blocks: []htmlarticle.Block{{Kind: htmlarticle.Prose, Text: "hello"}}}
}
func equal(a, b htmlarticle.Article) bool {
	return a.Processor == b.Processor && a.Profile == b.Profile && a.HTMLSHA256 == b.HTMLSHA256 && a.HTMLBytes == b.HTMLBytes && slices.Equal(a.Blocks, b.Blocks) && slices.Equal(a.Groups, b.Groups)
}
func TestCanonicalGoldenAndIndependentJSONView(t *testing.T) {
	ctx := deadline(t)
	a := sample()
	got, e := Encode(ctx, a, limits())
	if e != nil {
		t.Fatal(e)
	}
	want := `{"format":"concordance.article/v1","kind":"html","processor":"htmlarticle/v1","profile":"fixture/v1","html_sha256":"` + strings.Repeat("0", 64) + `","html_bytes":1,"blocks":[{"kind":"prose","text":"hello","level":0,"anchor":"","source_url":""}],"groups":[]}`
	if string(got) != want {
		t.Fatalf("golden mismatch: %s", got)
	}
	var view map[string]any
	if e = json.Unmarshal(got, &view); e != nil {
		t.Fatal(e)
	}
	if len(view) != 8 || view["kind"] != "html" || view["html_bytes"] != float64(1) {
		t.Fatal(view)
	}
	block := view["blocks"].([]any)[0].(map[string]any)
	if len(block) != 5 || block["text"] != "hello" {
		t.Fatal(block)
	}
	out, e := Decode(ctx, got, limits())
	if e != nil || !equal(a, out) {
		t.Fatal(out, e)
	}
	got[0] = '!'
	if out.Blocks[0].Text != "hello" {
		t.Fatal("decoded strings alias input")
	}
	a.Groups = []htmlarticle.Group{}
	again, e := Encode(ctx, a, limits())
	if e != nil || string(again) != want {
		t.Fatal("nil/empty normalization", e)
	}
}
func TestExtractRoundTripPreservesEveryKindAndWhitespace(t *testing.T) {
	ctx := deadline(t)
	body := []byte(`<main class="article"><h2 id="heading">Pod</h2><p>中文 😀 � &amp; text</p><pre><code>if x &lt; 2 {&#10;  run()&#10;}</code></pre><hr><aside class="notice"><p>Careful</p></aside><ul><li>one</li></ul><figure><img src="image.svg" alt="diagram"><figcaption>caption</figcaption></figure><blockquote><p>quote</p></blockquote><table><tr><th>Head</th><td>Cell</td></tr></table><dl><dt>Term</dt><dd>Meaning</dd></dl></main>`)
	a, e := htmlarticle.Extract(ctx, body, htmlarticle.Profile{ID: "fixture/v1", ArticleClass: "article", NoticeClasses: []string{"notice"}}, htmlarticle.Limits{MaxInputBytes: 10000, MaxTokenBytes: 10000, MaxTokens: 1000, MaxNodes: 1000, MaxDepth: 100, MaxBlocks: 100, MaxGroups: 100, MaxBlockBytes: 10000, MaxOutputBytes: 100000})
	if e != nil {
		t.Fatal(e)
	}
	b, e := Encode(ctx, a, limits())
	if e != nil {
		t.Fatal(e)
	}
	out, e := Decode(ctx, b, limits())
	if e != nil || !equal(a, out) {
		t.Fatal(out, e)
	}
	out.Blocks[0].Text = "changed"
	if a.Blocks[0].Text == "changed" {
		t.Fatal("output aliases original article")
	}
}
func TestStrictJSONRejections(t *testing.T) {
	ctx := deadline(t)
	base, e := Encode(ctx, sample(), limits())
	if e != nil {
		t.Fatal(e)
	}
	replace := func(old, new string) []byte { return []byte(strings.Replace(string(base), old, new, 1)) }
	bad := [][]byte{
		append(bytes.Clone(base), []byte(` {}`)...), append(bytes.Clone(base), '!'), []byte(`null`),
		replace(`"format":`, `"Format":`), replace(`"format":`, `"extra":1,"format":`), replace(`"kind":"html"`, `"kind":"html","k\u0069nd":"html"`),
		replace(`"anchor":"",`, ``), replace(`"level":0`, `"level":null`), replace(`"level":0`, `"level":false`), replace(`"blocks":[`, `"blocks":null,"unused":[`),
		replace(`"text":"hello"`, `"text":"\ud800"`), replace(`"text":"hello"`, `"text":"\udc00"`), replace(`"text":"hello"`, `"text":"\ud800\u0041"`), replace(`"text":"hello"`, `"text":"\udc00\ud800"`),
		replace(`"text":"hello"`, `"text":"\u0000"`), replace(`"text":"hello"`, `"text":{}`), replace(`"groups":[]`, `"groups":[{}]`),
		replace(`"html_bytes":1`, `"html_bytes":-0`), replace(`"html_bytes":1`, `"html_bytes":1.0`), replace(`"html_bytes":1`, `"html_bytes":1e0`), replace(`"html_bytes":1`, `"html_bytes":999999999999999999999999999999`),
		replace(`"html_bytes":1`, `"html_bytes":0`), replace(`"profile":"fixture/v1"`, `"profile":"bad space"`), replace(`"source_url":""`, `"source_url":"not-an-image"`),
		replace(`"html_sha256":"`+strings.Repeat("0", 64)+`"`, `"html_sha256":"A`+strings.Repeat("0", 63)+`"`),
	}
	invalidUTF := bytes.Clone(base)
	invalidUTF[bytes.Index(invalidUTF, []byte("hello"))] = 0xff
	bad = append(bad, invalidUTF)
	for i, b := range bad {
		a, e := Decode(ctx, b, limits())
		if e == nil || !reflect.DeepEqual(a, htmlarticle.Article{}) {
			t.Fatalf("case %d accepted: %+v %v", i, a, e)
		}
	}
	for i := 0; i < len(base); i++ {
		a, e := Decode(ctx, base[:i], limits())
		if e == nil || !reflect.DeepEqual(a, htmlarticle.Article{}) {
			t.Fatalf("prefix %d accepted", i)
		}
	}
	for _, b := range [][]byte{replace(`concordance.article/v1`, `concordance.article/v2`), replace(`"kind":"html"`, `"kind":"pdf"`), replace(`htmlarticle/v1`, `htmlarticle/v2`), replace(`"kind":"prose"`, `"kind":"new-kind"`)} {
		if _, e := Decode(ctx, b, limits()); !errors.Is(e, ErrUnsupported) {
			t.Fatal(e)
		}
	}
}
func TestEquivalentSpellingsCanonicalizeWithoutRepair(t *testing.T) {
	ctx := deadline(t)
	base, _ := Encode(ctx, sample(), limits())
	for _, text := range []string{`\ud83d\ude00`, `😀`, `�`, `\ufffd`, `\\ud800`, `\"quoted\"`, `<>&\n\t`} {
		body := []byte(strings.Replace(string(base), `"hello"`, `"`+text+`"`, 1))
		a, e := Decode(ctx, body, limits())
		if e != nil {
			t.Fatal(text, e)
		}
		var independent map[string]any
		if e = json.Unmarshal(body, &independent); e != nil {
			t.Fatal(e)
		}
		want := independent["blocks"].([]any)[0].(map[string]any)["text"].(string)
		if a.Blocks[0].Text != want {
			t.Fatal(text, a.Blocks[0].Text, want)
		}
		canonical, e := Encode(ctx, a, limits())
		if e != nil {
			t.Fatal(e)
		}
		again, e := Decode(ctx, canonical, limits())
		if e != nil || !equal(a, again) {
			t.Fatal(e)
		}
	}
	var reorder map[string]any
	_ = json.Unmarshal(base, &reorder)
	ordered, _ := json.Marshal(reorder)
	if _, e := Decode(ctx, append([]byte(" \n"), append(ordered, []byte(" \n")...)...), limits()); e != nil {
		t.Fatal("arbitrary key order/whitespace rejected", e)
	}
}
func valueBytes(b []byte) (total, largest int) {
	var v any
	_ = json.Unmarshal(b, &v)
	var walk func(any)
	walk = func(v any) {
		switch v := v.(type) {
		case string:
			total += len(v)
			largest = max(largest, len(v))
		case []any:
			for _, x := range v {
				walk(x)
			}
		case map[string]any:
			for _, x := range v {
				walk(x)
			}
		}
	}
	walk(v)
	return
}
func TestExactAndExcessLimitsAndStructuralRules(t *testing.T) {
	ctx := deadline(t)
	a := sample()
	a.HTMLBytes = 42
	a.Blocks[0].Text = strings.Repeat("x", 80)
	a.Groups = []htmlarticle.Group{{Kind: htmlarticle.Notice, First: 0, After: 1, Label: "label"}, {Kind: htmlarticle.Paragraph, First: 0, After: 1}}
	raw, _ := Encode(ctx, a, limits())
	total, largest := valueBytes(raw)
	exact := Limits{len(raw), 1, 2, largest, total, 2, 42}
	if _, e := Encode(ctx, a, exact); e != nil {
		t.Fatal("exact encode", e)
	}
	if _, e := Decode(ctx, raw, exact); e != nil {
		t.Fatal("exact decode", e)
	}
	for _, field := range []string{"MaxEncodedBytes", "MaxGroups", "MaxStringBytes", "MaxValueBytes", "MaxGroupDepth", "MaxHTMLBytes"} {
		l := exact
		v := reflect.ValueOf(&l).Elem().FieldByName(field)
		v.SetInt(v.Int() - 1)
		if b, e := Encode(ctx, a, l); b != nil || !errors.Is(e, ErrLimit) {
			t.Fatal(field, e)
		}
		if b, e := Decode(ctx, raw, l); !reflect.DeepEqual(b, htmlarticle.Article{}) || !errors.Is(e, ErrLimit) {
			t.Fatal(field, e)
		}
	}
	for i := 0; i < reflect.TypeFor[Limits]().NumField(); i++ {
		l := exact
		reflect.ValueOf(&l).Elem().Field(i).SetInt(0)
		if _, e := Encode(ctx, a, l); !errors.Is(e, ErrInvalid) {
			t.Fatal(i, e)
		}
		if _, e := Decode(ctx, raw, l); !errors.Is(e, ErrInvalid) {
			t.Fatal(i, e)
		}
	}
	invalid := []htmlarticle.Article{}
	for _, groups := range [][]htmlarticle.Group{{{Kind: "new", First: 0, After: 1}}, {{Kind: htmlarticle.List, First: 0, After: 2}}, {{Kind: htmlarticle.Cell, First: 0, After: 1, Ordered: true}}, {{Kind: htmlarticle.Notice, First: 0, After: 1, Header: true}}, {{Kind: htmlarticle.Paragraph, First: 0, After: 1, Label: "bad"}}} {
		x := sample()
		x.Groups = groups
		invalid = append(invalid, x)
	}
	x := sample()
	x.Blocks[0].Level = 1
	invalid = append(invalid, x)
	x = sample()
	x.Profile = ""
	invalid = append(invalid, x)
	x = sample()
	x.Blocks = nil
	invalid = append(invalid, x)
	for i, x := range invalid {
		if b, e := Encode(ctx, x, limits()); e == nil || b != nil {
			t.Fatal(i, b, e)
		}
	}
}
func TestCancellationAndOwnership(t *testing.T) {
	ctx := deadline(t)
	a := sample()
	raw, _ := Encode(ctx, a, limits())
	c, cancel := context.WithCancel(ctx)
	cancel()
	if b, e := Encode(c, a, limits()); b != nil || !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if b, e := Decode(c, raw, limits()); !reflect.DeepEqual(b, htmlarticle.Article{}) || !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	for _, c := range []context.Context{nil, context.Background()} {
		if _, e := Encode(c, a, limits()); !errors.Is(e, ErrInvalid) {
			t.Fatal(e)
		}
		if _, e := Decode(c, raw, limits()); !errors.Is(e, ErrInvalid) {
			t.Fatal(e)
		}
	}
	saved := bytes.Clone(raw)
	out, e := Decode(ctx, raw, limits())
	if e != nil {
		t.Fatal(e)
	}
	for i := range raw {
		raw[i] = 'x'
	}
	again, e := Encode(ctx, out, limits())
	if e != nil || !bytes.Equal(saved, again) {
		t.Fatal("decoded ownership failed", e)
	}
}
func FuzzDecodeRoundTrip(f *testing.F) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	f.Cleanup(cancel)
	l := limits()
	l.MaxEncodedBytes = 8192
	l.MaxBlocks = 16
	l.MaxGroups = 16
	l.MaxStringBytes = 2048
	l.MaxValueBytes = 4096
	seed, _ := Encode(ctx, sample(), l)
	f.Add(seed)
	f.Add([]byte(`{"x":"\ud800"}`))
	f.Fuzz(func(t *testing.T, b []byte) {
		a, e := Decode(ctx, b, l)
		if e != nil {
			if !reflect.DeepEqual(a, htmlarticle.Article{}) {
				t.Fatal("partial article")
			}
			return
		}
		encoded, e := Encode(ctx, a, l)
		if errors.Is(e, ErrLimit) {
			return
		}
		if e != nil {
			t.Fatal(e)
		}
		again, e := Decode(ctx, encoded, l)
		if e != nil || !equal(a, again) {
			t.Fatal(e)
		}
	})
}

func TestBlockCountCrossingAndEmptyGroups(t *testing.T) {
	ctx := deadline(t)
	a := sample()
	a.Blocks = append(a.Blocks, a.Blocks[0], a.Blocks[0])
	raw, e := Encode(ctx, a, limits())
	if e != nil {
		t.Fatal(e)
	}
	l := limits()
	l.MaxBlocks = 2
	if b, e := Encode(ctx, a, l); b != nil || !errors.Is(e, ErrLimit) {
		t.Fatal(e)
	}
	if _, e := Decode(ctx, raw, l); !errors.Is(e, ErrLimit) {
		t.Fatal(e)
	}
	a.Groups = []htmlarticle.Group{{Kind: "notice", First: 0, After: 2}, {Kind: "list", First: 1, After: 3}}
	if _, e := Encode(ctx, a, limits()); !errors.Is(e, ErrInvalid) {
		t.Fatal("crossing groups accepted", e)
	}
	a.Groups = []htmlarticle.Group{{Kind: "paragraph", First: 1, After: 2}, {Kind: "paragraph", First: 0, After: 1}}
	if _, e := Encode(ctx, a, limits()); !errors.Is(e, ErrInvalid) {
		t.Fatal("unordered groups accepted", e)
	}
	a.Groups = []htmlarticle.Group{{Kind: "notice", First: 0, After: 3}, {Kind: "paragraph", First: 1, After: 1}, {Kind: "notice", First: 3, After: 3, Label: "empty"}}
	l = limits()
	l.MaxGroupDepth = 1
	raw, e = Encode(ctx, a, l)
	if e != nil {
		t.Fatal(e)
	}
	out, e := Decode(ctx, raw, l)
	if e != nil || !equal(a, out) {
		t.Fatal(out, e)
	}
}

func TestCanonicalEscapesAndEncodeInputUnchanged(t *testing.T) {
	ctx := deadline(t)
	a := sample()
	a.Blocks[0].Text = "<>&\u2028\u2029😀�\n\"\\"
	before, _ := json.Marshal(a)
	encoded, e := Encode(ctx, a, limits())
	if e != nil {
		t.Fatal(e)
	}
	expected := []byte(`"text":"\u003c\u003e\u0026\u2028\u2029😀�\n\"\\"`)
	if !bytes.Contains(encoded, expected) {
		t.Fatalf("canonical escaping changed: %s", encoded)
	}
	after, _ := json.Marshal(a)
	if !bytes.Equal(before, after) {
		t.Fatal("encoder mutated input")
	}
}
