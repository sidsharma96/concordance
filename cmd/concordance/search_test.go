package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sidsharma96/concordance/internal/catalog"
	"github.com/sidsharma96/concordance/internal/objectstore"
	"github.com/sidsharma96/concordance/internal/searchtarget"
)

func configEnvironment() map[string]string {
	return map[string]string{"PGSERVICE": "fixture", "CONCORDANCE_COLLECTION": "guide", "CONCORDANCE_R2_ENDPOINT": "https://fixture.r2.cloudflarestorage.com", "CONCORDANCE_R2_BUCKET": "fixture", "R2_ACCESS_KEY_ID": "synthetic-access", "R2_SECRET_ACCESS_KEY": "synthetic-secret"}
}
func environment(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}
func cliConnector(f fixture, opened, closed *int) searchConnector {
	return func(_ context.Context, c searchConfig, _ int64) (searchStores, error) {
		*opened++
		return searchStores{catalog: f.cat, objects: f.objects, close: func() { *closed++ }}, nil
	}
}
func activateFixture(t *testing.T, f fixture, generation string) {
	t.Helper()
	_, err := searchtarget.Activate(testContext(t), f.cat, f.objects, catalog.TargetUpdate{Name: "kubernetes", Value: catalog.TargetValue{Collection: "guide", Generation: generation, DefaultEdition: "one"}}, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	f.remote.reset()
}

func TestSearchCommandRealPipelineAndText(t *testing.T) {
	f := setup(t)
	f.publish(t, makeBundle(t, "generation-a", "Retry uploads safely."))
	activateFixture(t, f, "generation-a")
	var out bytes.Buffer
	opened, closed := 0, 0
	err := runSearch(testContext(t), []string{"kubernetes", "RETRY"}, &out, environment(configEnvironment()), cliConnector(f, &opened, &closed))
	if err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, want := range []string{`Searching "kubernetes" | edition "one"`, `Collection "guide" | generation "generation-a"`, `Showing 2 of 2 indexed matches`, `1. "docs":"docs/pods.md"`, `2. "docs":"docs/other.md"`, `revision "` + strings.Repeat("a", 40) + `"`, `retained blocks [0,2)`, "   Pods\n   Retry uploads safely.\n"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in output:\n%s", want, text)
		}
	}
	if opened != 1 || closed != 1 || f.remote.counts()["GET"] != 4 {
		t.Fatal("lifetime/request count", opened, closed, f.remote.counts())
	}
	if strings.Contains(text, "synthetic-secret") || strings.Contains(text, "RETRY") {
		t.Fatal("credentials or raw query echoed")
	}
	t.Log("command output follows (actual synthetic PostgreSQL/HTTP run):\n" + text)

	out.Reset()
	f.remote.reset()
	if err := runSearch(testContext(t), []string{"--edition", "empty", "kubernetes", "retry"}, &out, environment(configEnvironment()), cliConnector(f, &opened, &closed)); err != nil || !strings.Contains(out.String(), `edition "empty"`) || !strings.Contains(out.String(), "Showing 0 of 0 indexed matches") || f.remote.counts()["GET"] != 2 {
		t.Fatal("known empty", out.String(), err)
	}
	out.Reset()
	f.remote.reset()
	if err := runSearch(testContext(t), []string{"--edition", "missing", "kubernetes", "retry"}, &out, environment(configEnvironment()), cliConnector(f, &opened, &closed)); err == nil || out.Len() != 0 {
		t.Fatal("missing edition fell back", err)
	}
}

func TestSearchCommandFailureLeavesNoBodyAndCleansUp(t *testing.T) {
	for _, kind := range []string{"second quote", "format overflow", "corrupt article", "canceled second quote"} {
		t.Run(kind, func(t *testing.T) {
			f := setup(t)
			b := makeBundle(t, "generation-a", "Retry uploads safely.")
			f.publish(t, b)
			activateFixture(t, f, "generation-a")
			ctx, cancel := context.WithCancel(testContext(t))
			defer cancel()
			args := []string{"kubernetes", "retry"}
			f.remote.mu.Lock()
			switch kind {
			case "second quote":
				f.remote.failGETAt = 4
			case "format overflow":
				args = append([]string{"--max-output-bytes", "1"}, args...)
			case "corrupt article":
				body := f.remote.bodies[f.path(t, ref(b.article))]
				body[len(body)/2] ^= 1
			case "canceled second quote":
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
				t.Fatal("partial body or leaked ownership", out.String(), err, opened, closed)
			}
			if kind == "canceled second quote" && !errors.Is(err, context.Canceled) {
				t.Fatal("lost cancellation", err)
			}
			t.Logf("%s: stdout bytes=0; connection cleanup=1", kind)
		})
	}
}

func TestSearchAdmissionBeforeConnections(t *testing.T) {
	args := [][]string{{}, {"kubernetes"}, {"kubernetes", "retry", "extra"}, {"--edition=", "kubernetes", "retry"}, {"--timeout=0s", "kubernetes", "retry"}, {"--max-matches=0", "kubernetes", "retry"}, {"--limit=0", "kubernetes", "retry"}, {"--limit=-1", "kubernetes", "retry"}, {"--all", "--limit=10", "kubernetes", "retry"}, {"--max-query-bytes=1", "kubernetes", "retry"}, {"kubernetes", "..."}, {"kubernetes", "\xff"}, {"kubernetes", "private\x00query"}, {"--unknown-private-query", "kubernetes", "retry"}}
	for _, a := range args {
		var out bytes.Buffer
		calls := 0
		connect := func(context.Context, searchConfig, int64) (searchStores, error) {
			calls++
			return searchStores{}, errors.New("unexpected connection")
		}
		err := runSearch(context.Background(), a, &out, environment(configEnvironment()), connect)
		if err == nil || out.Len() != 0 || calls != 0 {
			t.Fatal("bad admission", a, err, calls)
		}
		if strings.Contains(err.Error(), "private") {
			t.Fatal("raw query/flag leaked", err)
		}
	}
	for _, a := range [][]string{nil, {"version"}, {"--help"}, {"search", "--help"}, {"target", "--help"}, {"target", "activate", "--help"}, {"target", "show", "--help"}} {
		var out bytes.Buffer
		if err := run(a, &out); err != nil || out.Len() == 0 {
			t.Fatal(a, err)
		}
	}
}

func TestTargetCommandsAndScope(t *testing.T) {
	f := setup(t)
	f.publish(t, makeBundle(t, "generation-a", "Retry uploads safely."))
	f.publish(t, makeBundle(t, "generation-b", "Retry requests carefully."))
	opened, closed := 0, 0
	connect := cliConnector(f, &opened, &closed)
	env := configEnvironment()
	var out bytes.Buffer
	activate := []string{"activate", "--generation", "generation-a", "--edition", "one", "--expected-revision", "0", "kubernetes"}
	if err := runTarget(testContext(t), activate, &out, environment(env), connect); err != nil || !strings.Contains(out.String(), "revision 1") {
		t.Fatal(out.String(), err)
	}
	out.Reset()
	if err := runTarget(testContext(t), activate, &out, environment(env), connect); err != nil || !strings.Contains(out.String(), "revision 1") {
		t.Fatal("replay", out.String(), err)
	}
	f.remote.reset()
	out.Reset()
	for _, key := range []string{"CONCORDANCE_R2_ENDPOINT", "CONCORDANCE_R2_BUCKET", "R2_ACCESS_KEY_ID", "R2_SECRET_ACCESS_KEY"} {
		delete(env, key)
	}
	if err := runTarget(testContext(t), []string{"show", "kubernetes"}, &out, environment(env), connect); err != nil || !strings.Contains(out.String(), "generation-a") || len(f.remote.counts()) != 0 {
		t.Fatal("show depended on objects", err)
	}
	// A later output failure is not a rollback of activation's SQL update.
	activate = []string{"activate", "--generation", "generation-b", "--edition", "one", "--expected-revision", "1", "kubernetes"}
	err := runTarget(testContext(t), activate, shortOutput{}, environment(configEnvironment()), connect)
	current, readErr := f.cat.ResolveTarget(testContext(t), "kubernetes")
	if !errors.Is(err, io.ErrShortWrite) || readErr != nil || current.Revision != 2 || current.Value.Generation != "generation-b" {
		t.Fatal("write failure hid applied update", err, readErr, current)
	}

	// This other collection has trusted synthetic catalog metadata, but its body
	// is deliberately unavailable: scope denial must precede all object reads.
	r := catalog.Request{Collection: "other", Key: "other", FingerprintVersion: "fixture/v1", Fingerprint: sha256.Sum256([]byte("other"))}
	if _, err := f.cat.Bind(testContext(t), r); err != nil {
		t.Fatal(err)
	}
	if _, err := f.cat.Publish(testContext(t), r, catalog.Generation{ID: "other", ManifestKey: "not-read", ManifestSHA256: r.Fingerprint, ManifestBytes: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.cat.SetTarget(testContext(t), catalog.TargetUpdate{Name: "foreign", Value: catalog.TargetValue{Collection: "other", Generation: "other", DefaultEdition: "one"}}); err != nil {
		t.Fatal(err)
	}
	f.remote.reset()
	out.Reset()
	if err := runSearch(testContext(t), []string{"foreign", "retry"}, &out, environment(configEnvironment()), connect); err == nil || out.Len() != 0 || len(f.remote.counts()) != 0 {
		t.Fatal("cross-collection read", err)
	}
	if err := runTarget(testContext(t), []string{"show", "foreign"}, &out, environment(env), connect); err == nil || out.Len() != 0 {
		t.Fatal("cross-collection show", err)
	}
	if err := runTarget(testContext(t), []string{"activate", "--generation", "generation-a", "--edition", "one", "--expected-revision", "1", "foreign"}, &out, environment(configEnvironment()), connect); !errors.Is(err, catalog.ErrTargetConflict) || out.Len() != 0 {
		t.Fatal("cross-collection takeover", err)
	}
	foreign, err := f.cat.ResolveTarget(testContext(t), "foreign")
	if err != nil || foreign.Value.Collection != "other" || foreign.Revision != 1 {
		t.Fatal("foreign target changed", foreign, err)
	}
	if opened != closed {
		t.Fatal("target ownership leak", opened, closed)
	}
}

type shortOutput struct{}

func (shortOutput) Write(p []byte) (int, error) { return len(p) - 1, nil }

func TestTerminalEscapingOutputBoundsAndSafeErrors(t *testing.T) {
	if got := visible("ok\x1b[31m\r\t\u202e\\\""); strings.ContainsAny(got, "\x1b\r\t\u202e") || !strings.Contains(got, `\x1b`) {
		t.Fatal("terminal controls escaped incorrectly", got)
	}
	b := displayBuffer{max: 3}
	if n, err := b.Write([]byte("abc")); err != nil || n != 3 {
		t.Fatal(n, err)
	}
	if n, err := b.Write([]byte("d")); !errors.Is(err, errOutputLimit) || n != 0 || string(b.body) != "abc" {
		t.Fatal(n, err)
	}
	if err := writeCommand(shortOutput{}, []byte("abc")); !errors.Is(err, io.ErrShortWrite) {
		t.Fatal(err)
	}
	cause := errors.New("password=never-display-this; private query")
	err := commandFailure("catalog unavailable", cause)
	if strings.Contains(err.Error(), "never-display") || !errors.Is(err, cause) {
		t.Fatal("unsafe or lost cause", err)
	}
}

func TestNativeServiceConnectionAndOwnedClose(t *testing.T) {
	f := setup(t)
	f.publish(t, makeBundle(t, "generation-a", "Retry uploads safely."))
	activateFixture(t, f, "generation-a")
	c := f.pool.Config().ConnConfig
	dir := t.TempDir()
	service := filepath.Join(dir, "service.conf")
	content := fmt.Sprintf("[fixture]\nhost=%s\nport=%d\ndbname=%s\nuser=%s\npassword=%s\nsslmode=disable\noptions=-c search_path=%s\n", c.Host, c.Port, c.Database, c.User, c.Password, c.RuntimeParams["search_path"])
	if err := os.WriteFile(service, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PGSERVICEFILE", service)
	t.Setenv("PGSERVICE", "ignored-by-explicit-selector")
	stores, err := connectSearch(testContext(t), searchConfig{service: "fixture", collection: "guide"}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := stores.catalog.ResolveTarget(testContext(t), "kubernetes"); err != nil || got.Revision != 1 {
		stores.close()
		t.Fatal("native profile/query", got, err)
	}
	stores.close()
	if _, err := stores.catalog.ResolveTarget(testContext(t), "kubernetes"); err == nil {
		t.Fatal("pool stayed open")
	}
	if err := f.pool.Ping(testContext(t)); err != nil {
		t.Fatal("test database itself became unavailable", err)
	}
	// Construct the production R2 client without contacting its synthetic endpoint.
	// The configured object cap and owned-close behavior are still observable.
	full, err := loadSearchConfig(environment(configEnvironment()), true)
	if err != nil {
		t.Fatal(err)
	}
	stores, err = connectSearch(testContext(t), full, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if stores.objects.Binding().Endpoint != full.endpoint || stores.objects.Binding().Collection != full.collection {
		stores.close()
		t.Fatal("object routing changed")
	}
	if _, err := stores.objects.Key(objectstore.Ref{Bytes: 1025}); !errors.Is(err, objectstore.ErrInvalid) {
		stores.close()
		t.Fatal("object cap not propagated", err)
	}
	stores.close()
	if _, err := connectSearch(testContext(t), searchConfig{service: "missing-password=never-display", collection: "guide"}, 1); err == nil || strings.Contains(err.Error(), "never-display") {
		t.Fatal("unsafe configuration failure", err)
	}
}

func TestSearchConfigRejectsBadRoutesBeforeConnecting(t *testing.T) {
	for _, key := range []string{"PGSERVICE", "CONCORDANCE_COLLECTION", "CONCORDANCE_R2_ENDPOINT", "CONCORDANCE_R2_BUCKET", "R2_ACCESS_KEY_ID", "R2_SECRET_ACCESS_KEY"} {
		v := configEnvironment()
		delete(v, key)
		if _, err := loadSearchConfig(environment(v), true); err == nil {
			t.Fatal("missing", key)
		}
	}
	for _, key := range []string{"R2_ACCESS_KEY_ID", "R2_SECRET_ACCESS_KEY"} {
		v := configEnvironment()
		v[key] = "invalid\r\ncredential"
		if _, err := loadSearchConfig(environment(v), true); err == nil {
			t.Fatal("multiline credentials admitted")
		}
	}
	for _, endpoint := range []string{"http://fixture.r2.cloudflarestorage.com", "https://elsewhere.invalid", "https://u:p@fixture.r2.cloudflarestorage.com", "https://fixture.r2.cloudflarestorage.com/path", "https://fixture.r2.cloudflarestorage.com?secret=x", "https://fixture.r2.cloudflarestorage.com#x", "https://fixture.r2.cloudflarestorage.com:9000"} {
		v := configEnvironment()
		v["CONCORDANCE_R2_ENDPOINT"] = endpoint
		if _, err := loadSearchConfig(environment(v), true); err == nil {
			t.Fatal("route", endpoint)
		}
	}
	_, err := loadSearchConfig(func(key string) string {
		switch key {
		case "PGSERVICE":
			return "fixture"
		case "CONCORDANCE_COLLECTION":
			return "guide"
		default:
			t.Fatal("show read an object setting", key)
			return ""
		}
	}, false)
	if err != nil {
		t.Fatal(err)
	}
}

func TestPublicBinaryHelpAndLocalAdmission(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "concordance")
	if b, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatal(string(b), err)
	}
	for _, args := range [][]string{{"--help"}, {"version"}, {"search", "--help"}, {"target", "activate", "--help"}} {
		if b, err := exec.Command(binary, args...).CombinedOutput(); err != nil || len(b) == 0 {
			t.Fatal(args, err, string(b))
		}
	}
	cmd := exec.Command(binary, "search", "--private-query-do-not-echo", "kubernetes", "retry")
	b, err := cmd.CombinedOutput()
	if err == nil || strings.Contains(string(b), "private-query") {
		t.Fatal("bad flags leaked", string(b), err)
	}
}
