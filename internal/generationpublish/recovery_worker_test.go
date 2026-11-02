package generationpublish

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sidsharma96/concordance/internal/catalog"
	"github.com/sidsharma96/concordance/internal/objectstore"
	"github.com/sidsharma96/concordance/internal/publishedsearch"
	"github.com/sidsharma96/concordance/internal/termsearch"
	"github.com/sidsharma96/concordance/internal/textterms"
)

// Deployment configuration and original input identity only. No accepted
// generation/reference, artifact bytes, object map or writer-local path crosses.
type recoveryConfig struct{ DSN, Schema, ObjectURL, ProxyAddress, Role, Fault string }
type recoveryEvent struct {
	Phase       string
	Resume      bool
	Publication catalog.Publication
}

func recoverySend(t *testing.T, out *json.Encoder, e recoveryEvent) {
	t.Helper()
	if err := out.Encode(e); err != nil {
		t.Fatal("worker output failed")
	}
}
func recoveryResume(t *testing.T, in *json.Decoder) {
	t.Helper()
	var e recoveryEvent
	if err := in.Decode(&e); err != nil || !e.Resume {
		t.Fatal("worker barrier interrupted")
	}
}

// Trace only the final publication UPDATE, not earlier namespace or receipt
// transactions. pgx invokes TraceQueryStart before sending this SQL. The parent
// arms the wire proxy, or kills the writer, while the final row is still absent.
type recoveryTracer struct {
	t    *testing.T
	in   *json.Decoder
	out  *json.Encoder
	seen bool
}

func (r *recoveryTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	if strings.HasPrefix(d.SQL, "UPDATE concordance_imports SET generation_id=$3,") {
		if r.seen {
			r.t.Fatal("unexpected second final publication UPDATE")
		}
		r.seen = true
		recoverySend(r.t, r.out, recoveryEvent{Phase: "publishing"})
		recoveryResume(r.t, r.in)
	}
	return ctx
}
func (*recoveryTracer) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

// Only explicitly launched test children read stdin or create clients here.
func TestRecoveryWorker(t *testing.T) {
	if os.Getenv("CONCORDANCE_RECOVERY_WORKER") != "1" {
		return
	}
	in := json.NewDecoder(io.LimitReader(os.Stdin, 64<<10))
	out := json.NewEncoder(os.Stdout)
	var cfg recoveryConfig
	if err := in.Decode(&cfg); err != nil {
		t.Fatal("worker configuration failed")
	}
	db, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		t.Fatal("worker database configuration failed")
	}
	if cfg.ProxyAddress != "" {
		host, port, err := net.SplitHostPort(cfg.ProxyAddress)
		if err != nil || !net.ParseIP(host).IsLoopback() || db.ConnConfig.TLSConfig != nil || len(db.ConnConfig.Fallbacks) != 0 {
			t.Fatal("unsafe protocol proxy")
		}
		n, err := strconv.ParseUint(port, 10, 16)
		if err != nil {
			t.Fatal("invalid proxy port")
		}
		db.ConnConfig.Host, db.ConnConfig.Port = host, uint16(n)
	}
	if cfg.Role == "writer" && (cfg.Fault == "commit-reply" || cfg.Fault == "before-publish") {
		db.ConnConfig.Tracer = &recoveryTracer{t: t, in: in, out: out}
	}
	db.ConnConfig.RuntimeParams["search_path"] = cfg.Schema
	db.MaxConns = 1
	db.ConnConfig.RuntimeParams["statement_timeout"] = "8000"
	// Require local commit WAL flush, without implying host-loss protection.
	db.ConnConfig.RuntimeParams["synchronous_commit"] = "on"
	ctx, cancel := context.WithTimeout(t.Context(), 35*time.Second)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(ctx, db)
	if err != nil {
		t.Fatal("worker pool failed")
	}
	defer pool.Close()
	c, err := catalog.New(pool)
	if err != nil {
		t.Fatal(err)
	}
	transport := &http.Transport{DialContext: (&net.Dialer{Timeout: 2 * time.Second}).DialContext, MaxConnsPerHost: 2, MaxIdleConnsPerHost: 2, ResponseHeaderTimeout: 3 * time.Second, MaxResponseHeaderBytes: 16 << 10}
	defer transport.CloseIdleConnections()
	client := s3.New(s3.Options{Region: "auto", BaseEndpoint: aws.String(cfg.ObjectURL), UsePathStyle: true, Credentials: aws.AnonymousCredentials{}, HTTPClient: &http.Client{Transport: transport, Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}})
	objects, err := objectstore.New(client, "fixture", "guide", 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	ns, err := c.BindNamespace(ctx, "fixture", objects.Binding())
	if err != nil {
		t.Fatal("worker namespace failed")
	}
	p, err := New(c, objects, ns, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	b := makeBundle(t) // Original synthetic producer inputs, reconstructed independently.
	request := trustedRequest("recovery")
	if cfg.Role == "writer" {
		result, err := p.Publish(ctx, request, b.d.Generation, b.manifest, b.artifacts)
		if cfg.Fault == "commit-reply" {
			if !errors.Is(err, catalog.ErrOutcomeUnknown) || result != (catalog.Publication{}) {
				t.Fatal("wire commit loss did not yield uncertain empty publication")
			}
			recoverySend(t, out, recoveryEvent{Phase: "uncertain"})
		} else {
			if err != nil || result.Duplicate {
				t.Fatal("writer publication failed")
			}
			recoverySend(t, out, recoveryEvent{Phase: "acknowledged", Publication: result})
		}
		recoveryResume(t, in)
		t.Fatal("writer survived without forced termination")
	}
	if cfg.Role != "replacement" {
		t.Fatal("unknown worker role")
	}
	record, err := c.Lookup(ctx, request.Collection, request.Key)
	if err != nil || record.Request != request {
		t.Fatal("replacement input binding changed")
	}
	accepted := cfg.Fault != "before-publish"
	if accepted {
		if record.Generation == nil {
			t.Fatal("published generation absent")
		}
		recoveryRead(t, ctx, c, objects, *record.Generation)
	} else {
		if record.Generation != nil {
			t.Fatal("unpublished root became published")
		}
		r, err := publishedsearch.Open(ctx, c, objects, publishedsearch.Selection{Collection: "guide", Generation: b.d.Generation, Edition: "one"}, recoveryReaderLimits())
		if r != nil || !errors.Is(err, catalog.ErrNotFound) {
			t.Fatal("unpublished generation became readable")
		}
	}
	recoverySend(t, out, recoveryEvent{Phase: "initial-read"})
	recoveryResume(t, in)
	// Same input/key, intentionally different candidate ID. Completed retries must
	// keep the original generation; unfinished ones may publish this new candidate.
	b.d.Generation = "replacement-candidate"
	b.encode(t)
	retry, err := p.Publish(ctx, request, b.d.Generation, b.manifest, b.artifacts)
	if err != nil || retry.Duplicate != accepted {
		t.Fatal("replacement retry outcome changed")
	}
	if accepted && retry.Generation != *record.Generation {
		t.Fatal("retry changed original generation")
	}
	if !accepted && retry.Generation.ID != b.d.Generation {
		t.Fatal("unpublished retry lost candidate")
	}
	recoverySend(t, out, recoveryEvent{Phase: "retried", Publication: retry})
	recoveryResume(t, in)
	changed := request
	changed.Fingerprint[0] ^= 1
	result, err := p.Publish(ctx, changed, b.d.Generation, b.manifest, b.artifacts)
	if result != (catalog.Publication{}) || !errors.Is(err, catalog.ErrConflict) {
		t.Fatal("changed inputs did not conflict")
	}
	recoverySend(t, out, recoveryEvent{Phase: "conflicted"})
	recoveryResume(t, in)
	got, err := c.Lookup(ctx, request.Collection, request.Key)
	if err != nil || got.Request != request || got.Generation == nil || *got.Generation != retry.Generation {
		t.Fatal("conflict changed catalog publication")
	}
	recoveryRead(t, ctx, c, objects, retry.Generation)
	recoverySend(t, out, recoveryEvent{Phase: "recovered", Publication: retry})
}
func recoveryReaderLimits() publishedsearch.Limits {
	l := testLimits()
	return publishedsearch.Limits{Manifest: l.Manifest, Edition: l.Edition, Article: l.Article}
}
func recoveryRead(t *testing.T, ctx context.Context, c *catalog.Store, o *objectstore.Store, g catalog.Generation) {
	t.Helper()
	r, err := publishedsearch.Open(ctx, c, o, publishedsearch.Selection{Collection: "guide", Generation: g.ID, Edition: "one"}, recoveryReaderLimits())
	if err != nil {
		t.Fatal("replacement strict reader failed")
	}
	q, err := textterms.Prepare(ctx, "retry", textterms.Limits{MaxInputBytes: 256, MaxTerms: 16, MaxTermBytes: 256, MaxOutputBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	hits, err := r.Search(ctx, termsearch.Query{TermsVersion: q.Version, Edition: "one", Terms: q.Terms}, termsearch.Limits{MaxEditions: 1, MaxPassages: 1000, MaxTerms: 20000, MaxQueryTerms: 16, MaxInputBytes: 1 << 20, MaxMatches: 1000})
	if err != nil || len(hits.Hits) != 2 {
		t.Fatal("replacement exact edition search changed")
	}
	for i, h := range hits.Hits {
		loc, err := r.Location(ctx, h.Ref)
		native := "docs/pods.md"
		if i == 1 {
			native = "docs/other.md"
		}
		if err != nil || loc.Selection.Generation != g.ID || loc.Selection.Edition != "one" || loc.Document.NativeID != native || loc.Document.Revision != strings.Repeat("a", 40) {
			t.Fatal("replacement provenance or stable ranking changed")
		}
	}
	quote, err := r.Quote(ctx, hits.Hits[0].Ref, 4096)
	if err != nil || quote != "Pods\nRetry uploads safely." {
		t.Fatalf("replacement exact synthetic quote: %q", quote)
	}
}
