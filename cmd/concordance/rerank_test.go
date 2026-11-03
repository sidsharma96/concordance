package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sidsharma96/concordance/internal/termsearch"
)

func fixtureWorker(t *testing.T, increasing bool) string {
	t.Helper()
	python, e := exec.LookPath("python3")
	if e != nil {
		t.Skip("Python stdlib required for worker integration")
	}
	root, e := filepath.Abs("../../.tools/tmp")
	if e != nil {
		t.Fatal(e)
	}
	os.MkdirAll(root, 0o700)
	dir, e := os.MkdirTemp(root, "cli-rw-")
	if e != nil {
		t.Fatal(e)
	}
	dir, e = filepath.EvalSymlinks(dir)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	socket := filepath.Join(dir, "s")
	script, e := filepath.Abs("../../worker/test_rerank.py")
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	args := []string{"-I", "-B", script, "--interop", socket}
	if increasing {
		args = append(args, "--increasing")
	}
	cmd := exec.CommandContext(ctx, python, args...)
	in, e := cmd.StdinPipe()
	if e != nil {
		t.Fatal(e)
	}
	stdout, e := cmd.StdoutPipe()
	if e != nil {
		t.Fatal(e)
	}
	var diagnostic bytes.Buffer
	cmd.Stderr = &diagnostic
	if e = cmd.Start(); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		io.WriteString(in, "stop\n")
		in.Close()
		defer cancel()
		if e := cmd.Wait(); e != nil {
			t.Errorf("fixture worker %v: %s", e, diagnostic.String())
		}
	})
	line, e := bufio.NewReader(stdout).ReadString('\n')
	if e != nil || line != "LISTENING\n" {
		t.Fatal("fixture worker startup", line, e)
	}
	return socket
}
func TestRerankCommandUsesCompleteCandidatesAndActualCitations(t *testing.T) {
	f := setup(t)
	f.publish(t, makeBundleWithDocuments(t, "generation-a", "Retry uploads safely.", 12))
	activateFixture(t, f, "generation-a")
	for _, mode := range []string{"ties", "reverse"} {
		t.Run(mode, func(t *testing.T) {
			socket := fixtureWorker(t, mode == "reverse")
			env := configEnvironment()
			env["CONCORDANCE_WORKER_SOCKET"] = socket
			f.remote.reset()
			opened, closed := 0, 0
			var out bytes.Buffer
			err := runSearch(testContext(t), []string{"--rerank", "--limit=1", "--max-total-quote-bytes=64", "kubernetes", "why retry"}, &out, environment(env), cliConnector(f, &opened, &closed))
			if err != nil || !strings.Contains(out.String(), "scored 12 keyword candidates") || !strings.Contains(out.String(), "Showing 1 of 12 indexed matches") || strings.Count(out.String(), "retained blocks") != 1 || f.remote.counts()["GET"] != 3 || opened != 1 || closed != 1 {
				t.Fatal(out.String(), err, f.remote.counts(), opened, closed)
			}
			want := "docs/pods.md"
			if mode == "reverse" {
				want = "docs/extra-11.md"
			}
			if !strings.Contains(out.String(), want) {
				t.Fatal("lost reranked citation/tie order", out.String())
			}
			t.Logf("%s: scored=12 shown=1 article GET=1 opening GET=2; retained exact source reference", mode)
		})
	}
}
func TestRerankFailureNoFallbackAndEmptyNeedsNoWorker(t *testing.T) {
	f := setup(t)
	f.publish(t, makeBundle(t, "generation-a", "Retry uploads safely."))
	activateFixture(t, f, "generation-a")
	for _, kind := range []string{"missing worker", "empty", "overflow", "output"} {
		t.Run(kind, func(t *testing.T) {
			env := configEnvironment()
			args := []string{"--rerank", "kubernetes", "retry absent"}
			var out bytes.Buffer
			switch kind {
			case "empty":
				args = []string{"--rerank", "kubernetes", "totallyabsent"}
			case "overflow":
				args = []string{"--rerank", "--max-matches=1", "kubernetes", "retry absent"}
			case "output":
				env["CONCORDANCE_WORKER_SOCKET"] = fixtureWorker(t, false)
				args = []string{"--rerank", "--max-output-bytes=1", "kubernetes", "retry"}
			}
			opened, closed := 0, 0
			f.remote.reset()
			err := runSearch(testContext(t), args, &out, environment(env), cliConnector(f, &opened, &closed))
			if kind == "empty" {
				if err != nil || !strings.Contains(out.String(), "scored 0 keyword candidates") || f.remote.counts()["GET"] != 2 {
					t.Fatal(out.String(), err, f.remote.counts())
				}
			} else if err == nil || out.Len() != 0 {
				t.Fatal("failure fell back or exposed partial display", err, out.String())
			}
			if kind == "overflow" && (!errors.Is(err, termsearch.ErrLimit) || f.remote.counts()["GET"] != 2) {
				t.Fatal("candidate overflow fetched articles", err, f.remote.counts())
			}
			if closed != 1 {
				t.Fatal("missing connection cleanup", closed)
			}
		})
	}
}
func TestRerankFlagsAndWorkerEnvironment(t *testing.T) {
	for _, flags := range [][]string{{"--rerank", "--all"}, {"--rerank", "--limit=101"}, {"--rerank", "--max-candidate-article-bytes=0"}, {"--worker-socket=anything"}} {
		args := append(flags, "kubernetes", "retry")
		var out bytes.Buffer
		opened, closed := 0, 0
		err := runSearch(testContext(t), args, &out, environment(configEnvironment()), func(context.Context, searchConfig, int64) (searchStores, error) {
			opened++
			return searchStores{}, errors.New("must not connect")
		})
		if err == nil || out.Len() != 0 || opened != 0 || closed != 0 {
			t.Fatal("invalid flags caused work", args, err, opened)
		}
	}
	t.Setenv("R2_SECRET_ACCESS_KEY", "must-not-inherit")
	t.Setenv("PGSERVICE", "must-not-inherit")
	t.Setenv("PYTHONPATH", "must-not-inherit")
	env := strings.Join(workerEnvironment("path", "tmp", "cache"), "\n")
	if strings.Contains(env, "must-not-inherit") || strings.Contains(env, "PGSERVICE") || strings.Contains(env, "R2_") || strings.Contains(env, "PYTHONPATH") {
		t.Fatal("worker received credentials/custom imports")
	}
}
