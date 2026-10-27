package objectstore

import (
	"context"
	"crypto/rand"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// This explicitly opted-in probe writes synthetic bytes and leaves the immutable
// object in its unique namespace. It never provisions, exposes or deletes a bucket.
func TestR2LiveImmutableObject(t *testing.T) {
	if os.Getenv("CONCORDANCE_R2_LIVE_TEST") != "1" {
		t.Skip("set CONCORDANCE_R2_LIVE_TEST=1 with a private R2 bucket and scoped credentials")
	}
	endpoint, bucket := os.Getenv("CONCORDANCE_R2_ENDPOINT"), os.Getenv("CONCORDANCE_R2_BUCKET")
	access, secret := os.Getenv("R2_ACCESS_KEY_ID"), os.Getenv("R2_SECRET_ACCESS_KEY")
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || !strings.HasSuffix(u.Hostname(), ".r2.cloudflarestorage.com") ||
		u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" && u.Path != "/" || bucket == "" || access == "" || secret == "" {
		t.Fatal("configure an HTTPS R2 S3 endpoint, private bucket, R2_ACCESS_KEY_ID and R2_SECRET_ACCESS_KEY locally")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	collection := "concordance-live-probe-" + strings.ToLower(rand.Text())
	newStore := func() *Store {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		t.Cleanup(transport.CloseIdleConnections)
		client := s3.New(s3.Options{
			Region: "auto", BaseEndpoint: aws.String(endpoint), UsePathStyle: true,
			Credentials: aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
				return aws.Credentials{AccessKeyID: access, SecretAccessKey: secret, Source: "explicit R2 probe"}, nil
			}), HTTPClient: &http.Client{Transport: transport, Timeout: 20 * time.Second},
		})
		store, err := New(client, bucket, collection, 1024)
		if err != nil {
			t.Fatal(err)
		}
		return store
	}
	store := newStore()
	body := []byte("synthetic concordance immutable-object probe\n")
	first, err := store.Ensure(ctx, body)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Ensure(ctx, body)
	if err != nil || second != first {
		t.Fatalf("repeat ensure changed reference: %v", err)
	}
	// A new client reads by the reference; this is not a full host-loss/catalog
	// recovery experiment and must not be reported as one.
	got, err := newStore().Read(ctx, first)
	if err != nil || string(got) != string(body) {
		t.Fatalf("new-client read: %v", err)
	}
	t.Logf("retained one synthetic immutable object in unique collection %s", collection)
}
