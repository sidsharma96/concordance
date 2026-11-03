// Command fetchreranker obtains the exact public model assets used by the worker.
// Downloads are opt-in, streaming and checksum-checked; existing mismatches fail.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"time"
)

const baseURL = "https://huggingface.co/BAAI/bge-reranker-v2-m3/resolve/953dc6f6f85a1b2dbfca4c34a2796e7dde08d41e/"

type asset struct {
	Name   string
	Bytes  int64
	SHA256 string
}

var assets = []asset{
	{"config.json", 795, "13dcd6c31d9fec9d1d8e158702072f62d7fa7d312a64b9fe057bec9a08cfe41a"},
	{"model.safetensors", 2271071852, "d9e3e081faff1eefb84019509b2f5558fd74c1a05a2c7db22f74174fcedb5286"},
	{"special_tokens_map.json", 964, "8c785abebea9ae3257b61681b4e6fd8365ceafde980c21970d001e834cf10835"},
	{"tokenizer.json", 17098273, "69564b696052886ed0ac63fa393e928384e0f8caada38c1f4864a9bfbf379c15"},
	{"tokenizer_config.json", 1173, "7e4c1cc848840aeccdd763458c18dd525eb0f795c992e00ebe9c28554e7db2d4"},
}

func verify(ctx context.Context, path string, a asset) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	info, e := os.Lstat(path)
	if e != nil {
		return e
	}
	if !info.Mode().IsRegular() || info.Size() != a.Bytes {
		return errors.New("existing asset kind/size differs")
	}
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	h := sha256.New()
	reader := io.LimitReader(f, a.Bytes+1)
	var readBytes int64
	buf := make([]byte, 64<<10)
	for {
		if e = ctx.Err(); e != nil {
			break
		}
		var n int
		n, e = reader.Read(buf)
		readBytes += int64(n)
		if n > 0 {
			h.Write(buf[:n])
		}
		if e != nil {
			if e == io.EOF {
				e = nil
			}
			break
		}
	}
	e = errors.Join(e, f.Close())
	if e != nil {
		return e
	}
	if readBytes != a.Bytes || hex.EncodeToString(h.Sum(nil)) != a.SHA256 {
		return errors.New("existing asset checksum differs")
	}
	return nil
}
func fetch(ctx context.Context, client *http.Client, dir, url string, a asset) (result error) {
	if e := ctx.Err(); e != nil {
		return e
	}
	target := filepath.Join(dir, a.Name)
	if e := verify(ctx, target, a); e == nil {
		return nil
	} else if !errors.Is(e, os.ErrNotExist) {
		return e
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if e != nil {
		return e
	}
	resp, e := client.Do(req)
	if e != nil {
		return e
	}
	defer func() { result = errors.Join(result, resp.Body.Close()) }()
	if resp.StatusCode != http.StatusOK || resp.ContentLength >= 0 && resp.ContentLength != a.Bytes {
		return errors.New("model download status/length differs")
	}
	temp, e := os.CreateTemp(dir, ".model-download-")
	if e != nil {
		return e
	}
	name := temp.Name()
	defer func() { result = errors.Join(result, os.Remove(name)) }()
	h := sha256.New()
	n, e := io.CopyBuffer(io.MultiWriter(temp, h), io.LimitReader(resp.Body, a.Bytes+1), make([]byte, 64<<10))
	if e == nil && (n != a.Bytes || hex.EncodeToString(h.Sum(nil)) != a.SHA256) {
		e = errors.New("model download checksum/length differs")
	}
	if e == nil {
		e = temp.Sync()
	}
	e = errors.Join(e, temp.Close())
	if e != nil {
		return e
	}
	// Same-directory hard linking creates the named asset only if absent. A racing
	// winner is verified, never overwritten. The cache is recoverable, not a catalog.
	if e = os.Link(name, target); e != nil {
		if errors.Is(e, os.ErrExist) {
			return verify(ctx, target, a)
		}
		return e
	}
	return nil
}
func run(ctx context.Context, args []string, out io.Writer) error {
	f := flag.NewFlagSet("fetchreranker", flag.ContinueOnError)
	dir := f.String("dir", ".tools/reranking/model", "directory for pinned offline model assets")
	if e := f.Parse(args); e != nil {
		return e
	}
	if f.NArg() != 0 {
		return errors.New("unexpected model download arguments")
	}
	if e := os.MkdirAll(*dir, 0o700); e != nil {
		return e
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DisableCompression = true
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 30 * time.Minute, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 10 || req.URL.Scheme != "https" || req.URL.User != nil {
			return errors.New("unexpected model redirect")
		}
		return nil
	}}
	for _, a := range assets {
		if e := fetch(ctx, client, *dir, baseURL+a.Name, a); e != nil {
			return fmt.Errorf("obtain pinned asset %s failed", a.Name)
		}
		if _, e := fmt.Fprintln(out, "Verified", a.Name); e != nil {
			return e
		}
	}
	return nil
}
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if e := run(ctx, os.Args[1:], os.Stdout); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
