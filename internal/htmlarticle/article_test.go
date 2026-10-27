package htmlarticle

import (
	"context"
	"crypto/sha256"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func testProfile() Profile {
	return Profile{ID: "fixture-v1", ArticleClass: "article-body", OmitClasses: []string{"chrome"}, OmitIDs: []string{"feedback"}, NoticeClasses: []string{"notice"}}
}

func testLimits() Limits {
	return Limits{MaxInputBytes: 1 << 20, MaxTokenBytes: 1 << 16, MaxTokens: 100000, MaxNodes: 100000, MaxDepth: 128,
		MaxBlocks: 10000, MaxGroups: 20000, MaxBlockBytes: 1 << 16, MaxOutputBytes: 1 << 20}
}

func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestStructureWhitespaceAndOwnership(t *testing.T) {
	body := []byte(`<nav>outer navigation</nav><main class="article-body" id="root">
<h1 id="title">Sample <em>guide</em><a class="chrome">#</a></h1>
<p>Use <code>run</code>&amp;stop. <b>Keep</b> this&nbsp;word.</p>
<div class="notice" title="Feature: example"><strong>Beta:</strong> enabled by default.<p>Read the limit.</p></div>
<div id="example"><pre><code>  key: &lt;value&gt;
    next: yes
</code></pre></div>
<ul><li>Parent<ul><li>Child</li></ul>After child</li></ul>
<dl><dt>Mode</dt><dd><p>The default</p></dd></dl>
<table><caption>Settings</caption><thead><tr><th>Name</th><th>Value</th></tr></thead><tbody><tr><td>Mode</td><td><code>safe</code></td></tr></tbody></table>
<figure><img src="/diagram.svg" alt="Two workers"><figcaption>Worker flow</figcaption></figure>
<div id="feedback"><h2>Not documentation</h2><button>Yes</button></div><script>secret_script</script>
</main><footer>outer footer</footer>`)
	ctx := testContext(t)
	a, err := Extract(ctx, body, testProfile(), testLimits())
	if err != nil {
		t.Fatal(err)
	}
	if a.HTMLSHA256 != sha256.Sum256(body) || a.HTMLBytes != len(body) || a.Processor != Processor || a.Profile != "fixture-v1" {
		t.Fatal("input identity/processing metadata lost")
	}
	want := []string{"Sample guide", "Use run&stop. Keep this word.", "Beta: enabled by default.", "Read the limit.", "  key: <value>\n    next: yes\n", "Parent", "Child", "After child", "Mode", "The default", "Settings", "Name", "Value", "Mode", "safe", "Two workers", "Worker flow"}
	var got []string
	for _, b := range a.Blocks {
		got = append(got, b.Text)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("block text: got %q want %q", got, want)
	}
	if a.Blocks[0].Kind != Heading || a.Blocks[0].Level != 1 || a.Blocks[0].Anchor != "title" ||
		a.Blocks[4].Kind != Code || a.Blocks[4].Anchor != "example" ||
		a.Blocks[15].Kind != Image || a.Blocks[15].SourceURL != "/diagram.svg" {
		t.Fatal("heading/code/image structure lost")
	}
	seen := make(map[GroupKind]int)
	for _, g := range a.Groups {
		seen[g.Kind]++
		if g.First < 0 || g.After < g.First || g.After > len(a.Blocks) {
			t.Fatal("invalid group interval")
		}
		if g.Kind == Notice && (g.First != 2 || g.After != 4 || g.Label != "Feature: example") {
			t.Fatalf("notice context lost: %+v", g)
		}
		if g.Kind == Cell && g.Header != (g.First == 11 || g.First == 12) {
			t.Fatalf("table header metadata: %+v", g)
		}
	}
	for _, kind := range []GroupKind{Notice, List, ListItem, DefinitionList, Term, Definition, Table, Row, Cell, Figure, Caption} {
		if seen[kind] == 0 {
			t.Fatalf("missing %s group", kind)
		}
	}
	before := strings.Join(got, "|")
	for i := range body {
		body[i] = 'x'
	}
	var after []string
	for _, b := range a.Blocks {
		after = append(after, b.Text)
	}
	if strings.Join(after, "|") != before {
		t.Fatal("output aliases input")
	}
}

func TestHTML5RepairAndInlineBoundaries(t *testing.T) {
	// The DOM parser closes each paragraph and foster-parents loose table text.
	// A tag-stripping or token-stack implementation would produce a different order.
	body := []byte(`<div class="article-body"><p>A<em>B</em>C<p>second<table>before<tr><td>cell</table><p>after<br>break</div>`)
	a, err := Extract(testContext(t), body, testProfile(), testLimits())
	if err != nil {
		t.Fatal(err)
	}
	var texts []string
	for _, b := range a.Blocks {
		texts = append(texts, b.Text)
	}
	if want := []string{"ABC", "secondbefore", "cell", "after break"}; !reflect.DeepEqual(texts, want) {
		t.Fatalf("DOM order/inline spacing: %q", texts)
	}
}

func TestSelectionAndUnsupportedStructuresFailWithoutPartialArticle(t *testing.T) {
	cases := []struct {
		name, html string
		err        error
	}{
		{"missing", `<div class="other">text</div>`, ErrSelection},
		{"ambiguous", `<div class="article-body">one</div><div class="article-body">two</div>`, ErrSelection},
		{"nested-main", `<div class="article-body">one<div class="article-body">two</div></div>`, ErrSelection},
		{"omitted-main", `<div class="article-body" id="feedback">not selected</div>`, ErrSelection},
		{"non-html-space", "<div class=\"article-body\u00a0other\">text</div>", ErrSelection},
		{"unknown-element", `<div class="article-body"><p>accepted</p><custom-widget>not silently discarded</custom-widget></div>`, ErrUnsupported},
		{"foreign", `<div class="article-body"><svg><text>diagram</text></svg></div>`, ErrUnsupported},
		{"table-span", `<div class="article-body"><table><tr><td colspan="2">merged</td></tr></table></div>`, ErrUnsupported},
		{"list-start", `<div class="article-body"><ol start="3"><li>third</li></ol></div>`, ErrUnsupported},
		{"list-reversed", `<div class="article-body"><ol reversed><li>last</li></ol></div>`, ErrUnsupported},
		{"list-value", `<div class="article-body"><ol><li value="2">second</li></ol></div>`, ErrUnsupported},
		{"empty", `<div class="article-body"><script>not text</script></div>`, ErrUnsupported},
		{"wrong-container", `<pre class="article-body">code</pre>`, ErrUnsupported},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, err := Extract(testContext(t), []byte(tc.html), testProfile(), testLimits())
			if !errors.Is(err, tc.err) || !reflect.DeepEqual(a, Article{}) {
				t.Fatalf("got article=%+v error=%v", a, err)
			}
		})
	}
}

func TestLimitsAtActualFailureBoundaries(t *testing.T) {
	body := []byte(`<div class="article-body"><p>abc</p><p>def</p></div>`)
	cases := []struct {
		name string
		edit func(*Limits)
	}{
		{"input", func(l *Limits) { l.MaxInputBytes = len(body) - 1; l.MaxTokenBytes = len(body) - 1 }},
		{"token", func(l *Limits) { l.MaxTokenBytes = 3 }},
		{"tokens", func(l *Limits) { l.MaxTokens = 2 }},
		{"nodes", func(l *Limits) { l.MaxNodes = 3 }},
		{"depth", func(l *Limits) { l.MaxDepth = 2 }},
		{"blocks", func(l *Limits) { l.MaxBlocks = 1 }},
		{"groups", func(l *Limits) { l.MaxGroups = 1 }},
		{"block-bytes", func(l *Limits) { l.MaxBlockBytes = 2 }},
		{"output-bytes", func(l *Limits) { l.MaxBlockBytes = 3; l.MaxOutputBytes = len(Processor) + len(testProfile().ID) + 5 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			limits := testLimits()
			tc.edit(&limits)
			a, err := Extract(testContext(t), body, testProfile(), limits)
			if !errors.Is(err, ErrLimit) || !reflect.DeepEqual(a, Article{}) {
				t.Fatalf("partial success or wrong error: %+v %v", a, err)
			}
		})
	}
	limits := testLimits()
	limits.MaxInputBytes, limits.MaxTokenBytes, limits.MaxBlockBytes = len(body), len(`<div class="article-body">`), 3
	limits.MaxOutputBytes = len(Processor) + len(testProfile().ID) + 6
	if _, err := Extract(testContext(t), body, testProfile(), limits); err != nil {
		t.Fatalf("exact inclusive limits rejected: %v", err)
	}
	// Long comments are bounded before DOM parsing, even outside the article.
	limits = testLimits()
	limits.MaxTokenBytes = 32
	_, err := Extract(testContext(t), []byte(`<!--`+strings.Repeat("x", 128)+`--><div class="article-body">ok</div>`), testProfile(), limits)
	if !errors.Is(err, ErrLimit) {
		t.Fatalf("unbounded comment accepted: %v", err)
	}
}

func TestValidationCancellationAndMetadataBudgets(t *testing.T) {
	body := []byte(`<div class="article-body">ok</div>`)
	ctx := testContext(t)
	if _, err := Extract(context.Background(), body, testProfile(), testLimits()); !errors.Is(err, ErrInvalid) {
		t.Fatal("deadline required")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := Extract(cancelled, body, testProfile(), testLimits()); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	for _, invalid := range [][]byte{{0xff}, []byte("\x00")} {
		if _, err := Extract(ctx, invalid, testProfile(), testLimits()); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid text accepted: %v", err)
		}
	}
	profile := testProfile()
	profile.OmitClasses = []string{profile.ArticleClass}
	if _, err := Extract(ctx, body, profile, testLimits()); !errors.Is(err, ErrInvalid) {
		t.Fatal("contradictory profile accepted")
	}
	for _, content := range []string{`<h1 id="` + strings.Repeat("x", 80) + `">h</h1>`, `<img alt="a" src="` + strings.Repeat("x", 80) + `">`, `<div class="notice" title="` + strings.Repeat("x", 80) + `">n</div>`} {
		limits := testLimits()
		limits.MaxBlockBytes, limits.MaxOutputBytes = 32, 64
		a, err := Extract(ctx, []byte(`<div class="article-body">`+content+`</div>`), testProfile(), limits)
		if !errors.Is(err, ErrLimit) || !reflect.DeepEqual(a, Article{}) {
			t.Fatalf("metadata escaped budget: %+v %v", a, err)
		}
	}
}

func FuzzExtract(f *testing.F) {
	f.Add([]byte(`<div class="article-body"><h1>Title</h1><p>one <code>x</code></p></div>`))
	f.Add([]byte(`<div class="article-body"><table><tr><td>x<td>y</table></div>`))
	f.Add([]byte(`<div class="article-body"><img alt="reference only" src="/diagram.svg"></div>`))
	f.Fuzz(func(t *testing.T, body []byte) {
		if len(body) > 16<<10 {
			t.Skip()
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		limits := testLimits()
		limits.MaxInputBytes, limits.MaxTokenBytes, limits.MaxTokens, limits.MaxNodes = 16<<10, 4<<10, 4096, 4096
		limits.MaxBlocks, limits.MaxGroups = 256, 512
		a, err := Extract(ctx, body, testProfile(), limits)
		if err != nil {
			if !reflect.DeepEqual(a, Article{}) {
				t.Fatal("partial output on failure")
			}
			return
		}
		if a.HTMLSHA256 != sha256.Sum256(body) || a.HTMLBytes != len(body) || len(a.Blocks) > limits.MaxBlocks || len(a.Groups) > limits.MaxGroups {
			t.Fatal("identity/cardinality invariant")
		}
		ownedBytes := len(a.Processor) + len(a.Profile)
		for _, block := range a.Blocks {
			ownedBytes += len(block.Text) + len(block.Anchor) + len(block.SourceURL)
			if len(block.Text) > limits.MaxBlockBytes {
				t.Fatal("per-block text budget invariant")
			}
		}
		for i, group := range a.Groups {
			ownedBytes += len(group.Label)
			if group.First < 0 || group.After < group.First || group.After > len(a.Blocks) {
				t.Fatal("group interval invariant")
			}
			if i > 0 && a.Groups[i-1].First > group.First {
				t.Fatal("group opening order invariant")
			}
		}
		if ownedBytes > limits.MaxOutputBytes {
			t.Fatal("owned string budget invariant")
		}
		b, err := Extract(ctx, body, testProfile(), limits)
		if err == nil && !reflect.DeepEqual(a, b) {
			t.Fatal("determinism invariant")
		}
	})
}
