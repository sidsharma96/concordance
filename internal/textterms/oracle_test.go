package textterms

import (
	"errors"
	"math/rand/v2"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

// Independent representation: a declarative regexp, with no production scanner
// or character predicates. Go's regexp implementation has no backtracking engine.
var oraclePattern = regexp.MustCompile(`[\p{L}\p{Nd}_](?:[\p{L}\p{Nd}\p{M}_+#]|[./-][\p{L}\p{Nd}_])*`)

func prepareOracle(text string) []string {
	terms := oraclePattern.FindAllString(text, -1)
	for i, term := range terms {
		terms[i] = strings.Map(unicode.ToLower, term)
	}
	return terms
}

func checkOracle(t *testing.T, text string) {
	t.Helper()
	got, err := Prepare(testContext(t), text, testLimits())
	if !utf8.ValidString(text) || strings.ContainsRune(text, 0) {
		if !errors.Is(err, ErrInvalid) || !reflect.DeepEqual(got, Prepared{}) {
			t.Fatal("invalid input returned terms", got, err)
		}
		return
	}
	want := prepareOracle(text)
	if err != nil || got.Version != Version || !reflect.DeepEqual(got.Terms, want) {
		t.Fatalf("%q: %#v, %v; oracle %q", text, got, err, want)
	}
	again, err := Prepare(testContext(t), strings.Join(got.Terms, " "), testLimits())
	if err != nil || !reflect.DeepEqual(again, got) {
		t.Fatal("preparation is not idempotent", got, again, err)
	}
}

func TestGeneratedPreparationAgainstOracle(t *testing.T) {
	random := rand.New(rand.NewPCG(101, 901))
	alphabet := []rune("aAzZ19_+##--//..,;:=() \n\tȺKÉé\u0301日本語١Σς\ufffd")
	for range 500 {
		var text strings.Builder
		for n := random.IntN(150); n > 0; n-- {
			text.WriteRune(alphabet[random.IntN(len(alphabet))])
		}
		checkOracle(t, text.String())
	}
}

func FuzzPrepareAgainstOracle(f *testing.F) {
	f.Add("ContainerRestartRules C++ readiness-probe v1.35.")
	f.Add("\xff words\x00")
	f.Add("É E\u0301 Ⱥ K C++/CLI a..b")
	f.Add("")
	f.Fuzz(func(t *testing.T, text string) {
		if len(text) > 512 {
			t.Skip()
		}
		checkOracle(t, text)
	})
}
