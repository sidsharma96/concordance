package htmlterms

import (
	"html"
	"math/rand/v2"
	"reflect"
	"strings"
	"testing"

	"github.com/sidsharma96/concordance/internal/htmlarticle"
	"github.com/sidsharma96/concordance/internal/termsearch"
	"github.com/sidsharma96/concordance/internal/textterms"
)

// Independent projection: materialize raw pieces and Join them, without the
// production walkField helper. Prepare's grammar has its own separate oracle.
func oracleViews(a htmlarticle.Article, got Prepared) [][3]string {
	var views [][3]string
	for _, p := range got.Plan.Passages {
		var body, headings, labels []string
		for _, b := range a.Blocks[p.Body.First:p.Body.After] {
			body = append(body, b.Text)
		}
		for _, h := range p.Headings {
			headings = append(headings, a.Blocks[h].Text)
		}
		for _, g := range p.Groups {
			if a.Groups[g].Label != "" {
				labels = append(labels, a.Groups[g].Label)
			}
		}
		views = append(views, [3]string{strings.Join(body, "\n"), strings.Join(headings, "\n"), strings.Join(labels, "\n")})
	}
	return views
}

func checkProjection(t *testing.T, source string) {
	t.Helper()
	a := extract(t, source)
	got, err := Build(testContext(t), a, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	views := oracleViews(a, got)
	var expected []termsearch.Fields
	viewBytes := 0
	for _, v := range views {
		field := termsearch.Fields{}
		for i, target := range [3]*[]string{&field.Body, &field.Headings, &field.Labels} {
			viewBytes += len(v[i])
			p, err := textterms.Prepare(testContext(t), v[i], testLimits().Field)
			if err != nil {
				t.Fatal(err)
			}
			*target = p.Terms
		}
		expected = append(expected, field)
	}
	if !reflect.DeepEqual(got.Fields, expected) {
		t.Fatal("projection mismatch", got.Fields, expected)
	}
	limits := testLimits()
	limits.MaxViewBytes = viewBytes
	if _, err := Build(testContext(t), a, limits); err != nil {
		t.Fatal("exact repeated-view charge", err)
	}
	if viewBytes > 1 {
		limits.MaxViewBytes--
		if result, err := Build(testContext(t), a, limits); err != ErrLimit || !reflect.DeepEqual(result, Prepared{}) {
			t.Fatal("missed repeated view/separator bytes", result, err)
		}
	}
}

func TestGeneratedProjection(t *testing.T) {
	random := rand.New(rand.NewPCG(13, 79))
	for range 100 {
		var source strings.Builder
		source.WriteString("<h1>Root</h1><p>base</p>")
		for n := random.IntN(8); n > 0; n-- {
			source.WriteString("<h2>Section</h2><p>")
			source.WriteString(html.EscapeString([]string{"C++", "readiness-probe", "a a", "v1.35", "é", "x/y"}[random.IntN(6)]))
			source.WriteString(`</p><div class="notice" title="Feature"><p>default</p></div>`)
		}
		checkProjection(t, source.String())
	}
}

func FuzzProjection(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0, 1, 2, 3, 6, 7})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 32 {
			t.Skip()
		}
		var source strings.Builder
		source.WriteString("<h1>Root</h1><p>base</p>")
		for _, b := range data {
			source.WriteString([]string{"<h2>Next</h2>", `<div class="notice" title="Feature"><p>enabled</p></div>`, "<pre>C++ v1.35</pre>", "<p>a a</p>", "<ul><li>alpha</li><li>beta</li></ul>", "<p>\u0301É e\u0301</p>", `<figure><img alt="plot" src="/image.svg"><figcaption>caption</figcaption></figure>`, "<p>readiness-probe</p>"}[int(b)%8])
		}
		checkProjection(t, source.String())
	})
}
