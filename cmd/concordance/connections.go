package main

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sidsharma96/concordance/internal/catalog"
	"github.com/sidsharma96/concordance/internal/objectstore"
)

type searchConfig struct {
	service    string
	collection string
	endpoint   string
	bucket     string
	access     string
	secret     string
	objects    bool
}

func loadSearchConfig(getenv func(string) string, objects bool) (searchConfig, error) {
	c := searchConfig{service: getenv("PGSERVICE"), collection: getenv("CONCORDANCE_COLLECTION"), objects: objects}
	if !validName(c.service) || !validName(c.collection) {
		return searchConfig{}, errors.New("configure PGSERVICE and CONCORDANCE_COLLECTION locally")
	}
	if !objects {
		return c, nil
	}
	c.endpoint = getenv("CONCORDANCE_R2_ENDPOINT")
	c.bucket = getenv("CONCORDANCE_R2_BUCKET")
	c.access = getenv("R2_ACCESS_KEY_ID")
	c.secret = getenv("R2_SECRET_ACCESS_KEY")
	for _, s := range []string{c.endpoint, c.access, c.secret} {
		if s == "" || len(s) > 4096 || !utf8.ValidString(s) || strings.ContainsAny(s, "\x00\r\n") {
			return searchConfig{}, errors.New("configure bounded R2 endpoint and credentials locally")
		}
	}
	u, err := url.Parse(c.endpoint)
	if err != nil || u.Scheme != "https" ||
		!strings.HasSuffix(strings.ToLower(u.Hostname()), ".r2.cloudflarestorage.com") ||
		u.User != nil || u.Opaque != "" || (u.Path != "" && u.Path != "/") ||
		u.RawPath != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" ||
		(u.Port() != "" && u.Port() != "443") || !validName(c.bucket) || len(c.bucket) > 63 {
		return searchConfig{}, errors.New("configure an HTTPS R2 S3 root endpoint and valid bucket")
	}
	return c, nil
}

func connectSearch(ctx context.Context, c searchConfig, maxBytes int64) (searchStores, error) {
	// An encoded service selector preserves native profile/password-file handling
	// without interpolating a name into keyword syntax or embedding a password.
	cfg, err := pgxpool.ParseConfig("postgresql:///?" + url.Values{"service": []string{c.service}}.Encode())
	if err != nil {
		return searchStores{}, commandFailure("load PostgreSQL service configuration failed", err)
	}
	cfg.MaxConns = 2
	cfg.MinConns = 0
	cfg.MinIdleConns = 0
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return searchStores{}, commandFailure("create catalog pool failed", err)
	}
	cat, err := catalog.New(pool)
	if err != nil {
		pool.Close()
		return searchStores{}, commandFailure("initialize catalog failed", err)
	}
	if !c.objects {
		return searchStores{catalog: cat, close: pool.Close}, nil
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 4
	transport.MaxIdleConnsPerHost = 2
	transport.MaxResponseHeaderBytes = 64 << 10
	transport.ResponseHeaderTimeout = 20 * time.Second
	cleanup := func() { transport.CloseIdleConnections(); pool.Close() }
	client := s3.New(s3.Options{Region: "auto", BaseEndpoint: aws.String(c.endpoint), UsePathStyle: true, Credentials: aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
		return aws.Credentials{AccessKeyID: c.access, SecretAccessKey: c.secret, Source: "explicit operator configuration"}, nil
	}), HTTPClient: &http.Client{Transport: transport, Timeout: 20 * time.Second}})
	objects, err := objectstore.New(client, c.bucket, c.collection, maxBytes)
	if err != nil {
		cleanup()
		return searchStores{}, commandFailure("initialize object reader failed", err)
	}
	return searchStores{catalog: cat, objects: objects, close: cleanup}, nil
}
