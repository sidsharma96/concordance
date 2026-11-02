package generationpublish

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/sidsharma96/concordance/internal/catalog"
)

type recoveryChild struct {
	cmd         *exec.Cmd
	in          *os.File
	out         *os.File
	decode      *json.Decoder
	encode      *json.Encoder
	done        chan error
	cancel      context.CancelFunc
	waited      bool
	dir         string
	diagnostics recoveryDiagnostics
}

// os/exec drains this writer before Wait returns. A noisy failed child cannot
// retain unbounded stderr; race diagnostics must not disappear when the writer
// is deliberately killed before its normal test exit.
type recoveryDiagnostics struct{ data []byte }

func (d *recoveryDiagnostics) Write(p []byte) (int, error) {
	n := min(len(p), (16<<10)-len(d.data))
	d.data = append(d.data, p[:n]...)
	return len(p), nil
}

func startRecoveryChild(t *testing.T, config recoveryConfig) *recoveryChild {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal("locate test executable failed")
	}
	stdin, input, err := os.Pipe()
	if err != nil {
		t.Fatal("open worker input pipe failed")
	}
	output, stdout, err := os.Pipe()
	if err != nil {
		_ = stdin.Close()
		_ = input.Close()
		t.Fatal("open worker output pipe failed")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 40*time.Second)
	dir := t.TempDir()
	cmd := exec.CommandContext(ctx, executable, "-test.run=^TestRecoveryWorker$", "-test.count=1")
	cmd.Dir = dir
	// No home, credentials, accepted-state files or Go toolchain is inherited.
	cmd.Env = []string{"CONCORDANCE_RECOVERY_WORKER=1", "TMPDIR=" + dir, "GOTRACEBACK=single", "GORACE=halt_on_error=1 atexit_sleep_ms=0"}
	cmd.Stdin, cmd.Stdout = stdin, stdout
	cmd.WaitDelay = 2 * time.Second
	child := &recoveryChild{cmd: cmd, in: input, out: output, done: make(chan error, 1), cancel: cancel, dir: dir,
		decode: json.NewDecoder(io.LimitReader(output, 64<<10)), encode: json.NewEncoder(input)}
	cmd.Stderr = &child.diagnostics
	if err := cmd.Start(); err != nil {
		cancel()
		_ = stdin.Close()
		_ = stdout.Close()
		_ = input.Close()
		_ = output.Close()
		t.Fatal("start recovery worker failed")
	}
	_ = stdin.Close()
	_ = stdout.Close()
	go func() { child.done <- cmd.Wait() }()
	t.Cleanup(func() {
		child.cancel()
		_ = child.in.Close()
		_ = child.out.Close()
		if !child.waited {
			_ = child.cmd.Process.Kill()
			<-child.done
			child.waited = true
		}
	})
	if err := child.encode.Encode(config); err != nil {
		t.Fatal("send worker configuration failed")
	}
	return child
}

func (c *recoveryChild) event(t *testing.T, phase string) recoveryEvent {
	t.Helper()
	var event recoveryEvent
	if err := c.decode.Decode(&event); err != nil || event.Phase != phase {
		c.cancel()
		// Worker errors are deliberately fixed messages, never provider errors,
		// configuration or payload bytes. Bound diagnostics after cancellation.
		diagnostic, _ := io.ReadAll(io.LimitReader(io.MultiReader(c.decode.Buffered(), c.out), 4096))
		t.Fatalf("worker did not reach %s (got %q): %s", phase, event.Phase, diagnostic)
	}
	return event
}

func (c *recoveryChild) finish(t *testing.T, killed bool) {
	t.Helper()
	if killed {
		if err := c.cmd.Process.Kill(); err != nil {
			t.Fatal("writer did not remain alive until forced termination")
		}
	}
	err := <-c.done
	c.waited = true
	if len(c.diagnostics.data) != 0 {
		t.Fatalf("worker stderr (including race diagnostics): %s", c.diagnostics.data)
	}
	if killed {
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.Success() {
			t.Fatal("writer was not forcibly terminated and reaped")
		}
	} else if err != nil {
		t.Fatal("replacement worker failed")
	}
	_ = c.in.Close()
	_ = c.out.Close()
	c.cancel()
	if err := os.RemoveAll(c.dir); err != nil {
		t.Fatal("remove worker-local directory failed")
	}
	if _, err := os.Stat(c.dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("worker-local directory survived removal")
	}
}

func TestWriterProcessRecovery(t *testing.T) {
	for _, fault := range []string{"acknowledged", "put-reply", "commit-reply", "before-publish"} {
		t.Run(fault, func(t *testing.T) {
			var peer *recoveryObjects
			f := setupPeer(t, func(h http.Handler) http.Handler {
				peer = &recoveryObjects{next: h, remote: h.(*remote), drop: fault == "put-reply"}
				return peer
			})
			cfg := recoveryConfig{DSN: f.config.ConnString(), Schema: f.config.ConnConfig.RuntimeParams["search_path"], ObjectURL: f.server.URL, Role: "writer", Fault: fault}
			var proxy *recoveryProxy
			if fault == "commit-reply" {
				proxy = newRecoveryProxy(t, f.config)
				cfg.ProxyAddress = proxy.address
			}
			child := startRecoveryChild(t, cfg)
			switch fault {
			case "before-publish", "commit-reply":
				child.event(t, "publishing")
				f.unpublished(t, "recovery")
				if fault == "commit-reply" {
					proxy.armed.Store(true)
					recoverySend(t, child.encode, recoveryEvent{Resume: true})
					child.event(t, "uncertain")
					if proxy.dropped.Load() != 1 {
						t.Fatal("final COMMIT and idle response were not withheld")
					}
				}
			default:
				child.event(t, "acknowledged")
			}
			child.finish(t, true) // Reap and remove cwd BEFORE starting replacement.
			b := makeBundle(t)    // Parent oracle is never passed to replacement.
			n := len(b.artifacts) + 1
			sum := int64(len(b.manifest))
			for _, body := range b.artifacts {
				sum += int64(len(body))
			}
			missing := int64(len("<Error><Code>NoSuchKey</Code></Error>"))
			before := peer.snapshot()
			want := recoveryTraffic{GET: 2 * n, PUT: n, Request: sum, Response: sum + int64(n)*missing}
			if before != want || peer.lost != (fault == "put-reply") {
				t.Fatalf("writer traffic/fault: got %+v want %+v", before, want)
			}
			assertRecoveryBodies(t, f, b, n)
			initial, err := f.cat.Lookup(f.ctx, "guide", "recovery")
			if err != nil || initial.Request != trustedRequest("recovery") {
				t.Fatal("original input binding absent")
			}
			accepted := fault != "before-publish"
			if (initial.Generation != nil) != accepted {
				t.Fatal("catalog visibility did not match actual commit")
			}
			root := storeRef(ref(b.manifest))
			key, err := f.objects.Key(root)
			if err != nil {
				t.Fatal(err)
			}
			expected := catalog.Generation{ID: b.d.Generation, ManifestKey: key, ManifestSHA256: root.SHA256, ManifestBytes: root.Bytes}
			if accepted && *initial.Generation != expected {
				t.Fatal("original generation reference changed")
			}
			cfg.Role, cfg.ProxyAddress = "replacement", ""
			replacement := startRecoveryChild(t, cfg)
			replacement.event(t, "initial-read")
			afterRead := peer.snapshot()
			want = recoveryTraffic{}
			if accepted {
				want = recoveryTraffic{GET: 3, Response: recoveryReadBytes(b)}
			}
			if got := afterRead.minus(before); got != want {
				t.Fatalf("initial strict read got %+v want %+v", got, want)
			}
			recoverySend(t, replacement.encode, recoveryEvent{Resume: true})
			retry := replacement.event(t, "retried").Publication
			afterRetry := peer.snapshot()
			want = recoveryTraffic{}
			replacementBundle := makeBundle(t)
			replacementBundle.d.Generation = "replacement-candidate"
			replacementBundle.encode(t)
			if !accepted {
				want = recoveryTraffic{GET: 2, HEAD: n - 1, PUT: 1, Request: int64(len(replacementBundle.manifest)), Response: missing + int64(len(replacementBundle.manifest))}
			}
			if got := afterRetry.minus(afterRead); got != want {
				t.Fatalf("retry got %+v want %+v", got, want)
			}
			if retry.Duplicate != accepted || (accepted && retry.Generation != *initial.Generation) {
				t.Fatal("retry changed original publication")
			}
			recoverySend(t, replacement.encode, recoveryEvent{Resume: true})
			replacement.event(t, "conflicted")
			afterConflict := peer.snapshot()
			if afterConflict != afterRetry {
				t.Fatal("conflict reached object storage")
			}
			recoverySend(t, replacement.encode, recoveryEvent{Resume: true})
			final := replacement.event(t, "recovered").Publication
			replacement.finish(t, false)
			afterFinal := peer.snapshot()
			selected := b
			if !accepted {
				selected = replacementBundle
			}
			want = recoveryTraffic{GET: 3, Response: recoveryReadBytes(selected)}
			if got := afterFinal.minus(afterConflict); got != want {
				t.Fatalf("final strict read got %+v want %+v", got, want)
			}
			if final != retry {
				t.Fatal("final read changed publication")
			}
			record, err := f.cat.Lookup(f.ctx, "guide", "recovery")
			if err != nil || record.Generation == nil || *record.Generation != final.Generation {
				t.Fatal("replacement publication not durable")
			}
			size := n
			if !accepted {
				size++
			}
			assertRecoveryBodies(t, f, b, size) // Old root remains retained after new publication.
			if !accepted {
				assertRecoveryBodies(t, f, replacementBundle, size)
			}
			t.Logf("writer killed/reaped; cwd removed; writer %+v; initial strict read %+v; retry %+v; conflict %+v; final strict read %+v; retained objects %d", before, afterRead.minus(before), afterRetry.minus(afterRead), afterConflict.minus(afterRetry), afterFinal.minus(afterConflict), size)
		})
	}
}
func recoveryReadBytes(b bundle) int64 {
	return int64(len(b.manifest)) + b.d.Editions[0].Index.Bytes + b.d.Editions[0].Documents[0].Article.Bytes
}
func assertRecoveryBodies(t *testing.T, f storageFixture, b bundle, size int) {
	t.Helper()
	f.remote.mu.Lock()
	defer f.remote.mu.Unlock()
	if len(f.remote.bodies) != size {
		t.Fatal("retained object count changed")
	}
	for _, body := range append(append([][]byte{}, b.artifacts...), b.manifest) {
		key, err := f.objects.Key(storeRef(ref(body)))
		if err != nil || !bytes.Equal(f.remote.bodies["/fixture/"+key], body) {
			t.Fatal("retained bytes changed")
		}
	}
}
