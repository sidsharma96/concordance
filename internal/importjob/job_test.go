package importjob

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sidsharma96/concordance/internal/capturefiles"
	"github.com/sidsharma96/concordance/internal/htmlarticle"
)

const fixtureHTML = `<main class="fixture"><h1>Pods</h1><p>Retry uploads safely.</p></main>`
const fixtureRecipe = "authored fixture; print copies excluded"

func testContext(t testing.TB) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)
	return ctx
}
func testLimits() Limits {
	return Limits{MaxRecordBytes: 8192, MaxSources: 4, Capture: capturefiles.Limits{
		MaxDescriptorBytes: 65536, MaxEditions: 4, MaxCaptures: 8, MaxPages: 32,
		MaxDependencies: 32, MaxFiles: 40, MaxStringBytes: 4096, MaxValueBytes: 65536,
		MaxFileBytes: 65536, MaxPayloadBytes: 1 << 20,
	}}
}
func fixtureRequest() Request {
	return Request{Collection: "guide", Sources: []Source{{ID: "docs", Repository: "https://example.invalid/docs"}}}
}
func writeCapture(t testing.TB, revision string) string {
	t.Helper()
	root := t.TempDir()
	ref := func(path string, body []byte) capturefiles.File {
		return capturefiles.File{Path: path, SHA256: sha256.Sum256(body), Bytes: int64(len(body))}
	}
	d := capturefiles.Description{Editions: []capturefiles.Edition{
		{ID: "one", Captures: []capturefiles.Capture{{
			Source: "docs", Revision: revision, Directory: "docs", Preparation: "authored/v1", Toolchain: "fixture/v1",
			Recipe: ref("recipe", []byte(fixtureRecipe)), Dependencies: []string{},
			Profile: htmlarticle.Profile{ID: "fixture/v1", ArticleClass: "fixture"},
			Pages:   []capturefiles.Page{{Path: "docs/a.md", HTML: ref("article", []byte(fixtureHTML))}, {Path: "docs/b.md", HTML: ref("article", []byte(fixtureHTML))}},
		}}},
		{ID: "empty", Captures: []capturefiles.Capture{}},
	}}
	descriptor, err := capturefiles.Encode(testContext(t), d, testLimits().Capture)
	if err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string][]byte{"recipe": []byte(fixtureRecipe), "article": []byte(fixtureHTML), capturefiles.DescriptorName: descriptor} {
		if err := os.WriteFile(filepath.Join(root, name), body, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}
func newJob(t testing.TB) (string, string, Saved) {
	t.Helper()
	producer := writeCapture(t, strings.Repeat("a", 40))
	directory := filepath.Join(t.TempDir(), "job")
	saved, err := Prepare(testContext(t), producer, directory, fixtureRequest(), testLimits())
	if err != nil {
		t.Fatal(err)
	}
	return producer, directory, saved
}
func TestSavedJobOwnsCaptureAndIdentity(t *testing.T) {
	producer, directory, saved := newJob(t)
	if err := os.RemoveAll(producer); err != nil {
		t.Fatal(err)
	}
	first, err := Open(testContext(t), directory, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	second, err := Open(testContext(t), directory, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first.Saved, saved) || !reflect.DeepEqual(second, first) || len(first.Editions) != 2 {
		t.Fatal(first, second, saved)
	}
	if string(first.Editions[0].Captures[0].Pages[0].HTML) != fixtureHTML {
		t.Fatal("body changed")
	}
	first.Editions[0].Captures[0].Pages[0].HTML[0] = 'X'
	first.Sources[0].ID = "changed"
	if second.Sources[0].ID != "docs" || string(second.Editions[0].Captures[0].Pages[0].HTML) != fixtureHTML {
		t.Fatal("separate opens shared mutable state")
	}
	body, err := os.ReadFile(filepath.Join(directory, recordName))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(body, []byte(producer)) || bytes.Contains(body, []byte(directory)) {
		t.Fatal("local routes retained")
	}
	if _, err := Prepare(testContext(t), writeCapture(t, strings.Repeat("a", 40)), directory, fixtureRequest(), testLimits()); !errors.Is(err, os.ErrExist) {
		t.Fatal(err)
	}
	third, err := Open(testContext(t), directory, testLimits())
	if err != nil || third.Key != saved.Key {
		t.Fatal(third, err)
	}
	info, err := os.Stat(filepath.Join(directory, recordName))
	if err != nil || info.Mode().Perm()&0077 != 0 {
		t.Fatal(info, err)
	}
	t.Log("original capture removed; fresh opens retain key, two pages and exact source identities")
}
func TestSavedJobRejectsNestedAliasedAndConcurrentPreparation(t *testing.T) {
	producer := writeCapture(t, strings.Repeat("a", 40))
	if _, err := Prepare(testContext(t), producer, filepath.Join(producer, "job"), fixtureRequest(), testLimits()); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "producer-link")
	if err := os.Symlink(producer, alias); err != nil {
		t.Fatal(err)
	}
	if _, err := Prepare(testContext(t), producer, filepath.Join(alias, "job"), fixtureRequest(), testLimits()); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "job")
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Go(func() {
			_, err := Prepare(testContext(t), producer, destination, fixtureRequest(), testLimits())
			results <- err
		})
	}
	wg.Wait()
	close(results)
	success, conflict := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, os.ErrExist) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatal(success, conflict)
	}
	if _, err := Open(testContext(t), destination, testLimits()); err != nil {
		t.Fatal(err)
	}
}
func TestJobAdmissionAndSourceBindings(t *testing.T) {
	producer := writeCapture(t, strings.Repeat("a", 40))
	for _, request := range []Request{{Collection: ""}, {Collection: "guide", Sources: []Source{{ID: "docs", Repository: "r"}, {ID: "docs", Repository: "r"}}}} {
		path := filepath.Join(t.TempDir(), "job")
		if _, err := Prepare(testContext(t), producer, path, request, testLimits()); err == nil {
			t.Fatal("bad request")
		}
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("invalid request claimed job", err)
		}
	}
	mismatch := fixtureRequest()
	mismatch.Sources[0].ID = "other"
	path := filepath.Join(t.TempDir(), "job")
	if _, err := Prepare(testContext(t), producer, path, mismatch, testLimits()); !errors.Is(err, ErrMismatch) {
		t.Fatal(err)
	}
	if _, err := Open(testContext(t), path, testLimits()); !errors.Is(err, ErrNotReady) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(testContext(t))
	cancel()
	if _, err := Prepare(ctx, producer, filepath.Join(t.TempDir(), "job"), fixtureRequest(), testLimits()); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := Open(context.Background(), path, testLimits()); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	small := testLimits()
	small.MaxSources = 0
	if _, err := Prepare(testContext(t), producer, filepath.Join(t.TempDir(), "job"), fixtureRequest(), small); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	small = testLimits()
	small.MaxRecordBytes = 100
	path = filepath.Join(t.TempDir(), "job")
	if _, err := Prepare(testContext(t), producer, path, fixtureRequest(), small); !errors.Is(err, ErrLimit) {
		t.Fatal(err)
	}
	if _, err := Open(testContext(t), path, testLimits()); !errors.Is(err, ErrNotReady) {
		t.Fatal(err)
	}
}
func TestJobPinsAllSavedBytes(t *testing.T) {
	for _, name := range []string{readyName, recordName, filepath.Join(captureName, capturefiles.DescriptorName), filepath.Join(captureName, "payload-00000001")} {
		t.Run(name, func(t *testing.T) {
			_, directory, _ := newJob(t)
			file := filepath.Join(directory, name)
			body, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			body[len(body)/2] ^= 1
			if err := os.WriteFile(file, body, 0600); err != nil {
				t.Fatal(err)
			}
			result, err := Open(testContext(t), directory, testLimits())
			if err == nil || !reflect.DeepEqual(result, Opened{}) {
				t.Fatal(result, err)
			}
		})
	}
}
func rewriteRecord(t testing.TB, directory string, body []byte) {
	t.Helper()
	digest := sha256.Sum256(body)
	for name, data := range map[string][]byte{recordName: body, readyName: []byte(readyPrefix + hex.EncodeToString(digest[:]) + "\n")} {
		if err := os.WriteFile(filepath.Join(directory, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
func TestJobStrictRecordAndPolicy(t *testing.T) {
	_, directory, _ := newJob(t)
	original, err := os.ReadFile(filepath.Join(directory, recordName))
	if err != nil {
		t.Fatal(err)
	}
	cases := [][]byte{
		bytes.Replace(original, []byte(`"collection":`), []byte(`"COLLECTION":`), 1),
		bytes.Replace(original, []byte(`"collection":"guide"`), []byte(`"collection":null`), 1),
		bytes.Replace(original, []byte(`"collection":"guide"`), []byte(`"collection":"guide","collection":"guide"`), 1),
		bytes.Replace(original, []byte(`"collection":"guide"`), []byte(`"collection":"\ud800"`), 1),
		bytes.Replace(original, []byte(`"collection":"guide"`), []byte(`"collection":"`+string([]byte{255})+`"`), 1),
		append(bytes.Clone(original), []byte(` {}`)...),
		bytes.Replace(original, []byte(`"bytes":`), []byte(`"unknown":`), 1),
		bytes.Replace(original, []byte(`"candidate_id":`), []byte(`"another":`), 1),
		bytes.Replace(original, []byte(`"bytes":`), []byte(`"bytes":1e3,"junk":`), 1),
	}
	for i, body := range cases {
		if bytes.Equal(body, original) {
			t.Fatalf("fixture replacement %d did not apply", i)
		}
		rewriteRecord(t, directory, body)
		if result, err := Open(testContext(t), directory, testLimits()); err == nil || !reflect.DeepEqual(result, Opened{}) {
			t.Fatal(i, result, err)
		}
	}
	var wire map[string]any
	if err := json.Unmarshal(original, &wire); err != nil {
		t.Fatal(err)
	}
	wire["article_processor"] = "changed"
	body, _ := json.Marshal(wire)
	rewriteRecord(t, directory, body)
	if _, err := Open(testContext(t), directory, testLimits()); !errors.Is(err, ErrPolicy) {
		t.Fatal(err)
	}
	wire["article_processor"] = htmlarticle.Processor
	wire["sources"] = []any{}
	body, _ = json.Marshal(wire)
	rewriteRecord(t, directory, body)
	if _, err := Open(testContext(t), directory, testLimits()); !errors.Is(err, ErrMismatch) {
		t.Fatal(err)
	}
}
func TestPreparationFaultsRespectReadyBoundary(t *testing.T) {
	producer := writeCapture(t, strings.Repeat("a", 40))
	fault := errors.New("injected local failure")
	// This fixture writes 2payloads, descriptor, record, pending marker, then syncs
	// capture/job/parent both before and after the one marker installation.
	for fail := 1; fail <= 12; fail++ {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			directory := filepath.Join(t.TempDir(), "job")
			ops := nativeOperations()
			count := 0
			installed := false
			step := func() error {
				count++
				if count == fail {
					return fault
				}
				return nil
			}
			originalWrite := ops.write
			ops.write = func(ctx context.Context, root *os.Root, name string, body []byte) error {
				if err := step(); err != nil {
					return err
				}
				return originalWrite(ctx, root, name, body)
			}
			originalSync := ops.sync
			ops.sync = func(ctx context.Context, root *os.Root) error {
				if err := step(); err != nil {
					return err
				}
				return originalSync(ctx, root)
			}
			originalLink := ops.link
			ops.link = func(root *os.Root, old, next string) error {
				if err := step(); err != nil {
					return err
				}
				err := originalLink(root, old, next)
				installed = err == nil
				return err
			}
			originalRemove := ops.remove
			ops.remove = func(root *os.Root, name string) error {
				if err := step(); err != nil {
					return err
				}
				return originalRemove(root, name)
			}
			result, err := prepare(testContext(t), producer, directory, fixtureRequest(), testLimits(), ops)
			if !errors.Is(err, fault) || !reflect.DeepEqual(result, Saved{}) {
				t.Fatal(count, result, err)
			}
			opened, openErr := Open(testContext(t), directory, testLimits())
			if installed {
				if openErr != nil || !validID(opened.Key, "import-") {
					t.Fatal(opened, openErr)
				}
			} else if openErr == nil {
				t.Fatal("failed preparation opened before marker")
			}
		})
	}
}

type faultFile struct {
	writeErr, syncErr, closeErr error
	short                       bool
	events                      []string
}

func (f *faultFile) Write(body []byte) (int, error) {
	f.events = append(f.events, "write")
	if f.short {
		return len(body) - 1, nil
	}
	return len(body), f.writeErr
}
func (f *faultFile) Sync() error  { f.events = append(f.events, "sync"); return f.syncErr }
func (f *faultFile) Close() error { f.events = append(f.events, "close"); return f.closeErr }
func TestFileErrorsAlwaysCloseAndNeverSkipSyncFailure(t *testing.T) {
	fault := errors.New("file failure")
	for _, file := range []*faultFile{{writeErr: fault}, {syncErr: fault}, {closeErr: fault}, {short: true}, {}} {
		err := writeAndSync(testContext(t), file, []byte("body"))
		if file.short {
			if !errors.Is(err, io.ErrShortWrite) {
				t.Fatal(err)
			}
		} else if file.writeErr != nil || file.syncErr != nil || file.closeErr != nil {
			if !errors.Is(err, fault) {
				t.Fatal(err)
			}
		} else if err != nil {
			t.Fatal(err)
		}
		if file.events[len(file.events)-1] != "close" {
			t.Fatal(file.events)
		}
		if file.writeErr != nil || file.short {
			if reflect.DeepEqual(file.events, []string{"write", "sync", "close"}) {
				t.Fatal("synced after failed write")
			}
		}
	}
}
func TestJobProcessHelper(t *testing.T) {
	mode := os.Getenv("CONCORDANCE_LOCAL_JOB_HELPER")
	if mode == "" {
		return
	}
	directory := os.Getenv("CONCORDANCE_LOCAL_JOB_PATH")
	if mode == "open" {
		job, err := Open(testContext(t), directory, testLimits())
		if err != nil {
			fmt.Fprintln(os.Stderr, "open failed")
			os.Exit(79)
		}
		fmt.Println(job.Key)
		os.Exit(0)
	}
	ops := nativeOperations()
	link := ops.link
	ops.link = func(root *os.Root, old, next string) error {
		if mode == "before" {
			os.Exit(73)
		}
		if err := link(root, old, next); err != nil {
			return err
		}
		os.Exit(74)
		return nil
	}
	_, err := prepare(testContext(t), os.Getenv("CONCORDANCE_LOCAL_JOB_SOURCE"), directory, fixtureRequest(), testLimits(), ops)
	t.Fatal("helper unexpectedly returned", err)
}
func TestFreshProcessInterruptionAndReopen(t *testing.T) {
	for _, mode := range []string{"before", "after"} {
		t.Run(mode, func(t *testing.T) {
			producer := writeCapture(t, strings.Repeat("a", 40))
			directory := filepath.Join(t.TempDir(), "job")
			command := func(mode string) *exec.Cmd {
				cmd := exec.CommandContext(testContext(t), os.Args[0], "-test.run=^TestJobProcessHelper$")
				cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "TMPDIR=" + os.Getenv("TMPDIR"), "CONCORDANCE_LOCAL_JOB_HELPER=" + mode, "CONCORDANCE_LOCAL_JOB_PATH=" + directory, "CONCORDANCE_LOCAL_JOB_SOURCE=" + producer}
				return cmd
			}
			output, err := command(mode).CombinedOutput()
			var exit *exec.ExitError
			code := 73
			if mode == "after" {
				code = 74
			}
			if !errors.As(err, &exit) || exit.ExitCode() != code {
				t.Fatal(string(output), err)
			}
			if err := os.RemoveAll(producer); err != nil {
				t.Fatal(err)
			}
			output, err = command("open").CombinedOutput()
			if mode == "before" {
				if !errors.As(err, &exit) || exit.ExitCode() != 79 {
					t.Fatal(string(output), err)
				}
				return
			}
			if err != nil {
				t.Fatal(string(output), err)
			}
			local, err := Open(testContext(t), directory, testLimits())
			if err != nil || strings.TrimSpace(string(output)) != local.Key {
				t.Fatal(string(output), local, err)
			}
			t.Log("preparer exited after marker install; fresh process reopened original key without producer")
		})
	}
}

func TestRecordAdmissionBeforeSourceAppend(t *testing.T) {
	_, directory, _ := newJob(t)
	original, err := os.ReadFile(filepath.Join(directory, recordName))
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(original, &wire); err != nil {
		t.Fatal(err)
	}
	sources := []map[string]string{}
	for i := 0; i < 5; i++ {
		sources = append(sources, map[string]string{"id": fmt.Sprint("source-", i), "repository": "r"})
	}
	wire["sources"] = sources
	body, _ := json.Marshal(wire)
	if _, _, err := decodeRecord(testContext(t), body, testLimits()); !errors.Is(err, ErrLimit) {
		t.Fatal(err)
	}
	for _, value := range []any{1e100, -1, 1.5} {
		ref := wire["capture_descriptor"].(map[string]any)
		ref["bytes"] = value
		wire["sources"] = []any{}
		body, _ := json.Marshal(wire)
		if _, _, err := decodeRecord(testContext(t), body, testLimits()); err == nil {
			t.Fatal("noninteger/negative/overflow size", value)
		}
	}
}
func FuzzLocalJobRecord(f *testing.F) {
	copied := capturefiles.CopyInfo{Original: capturefiles.DescriptorRef{Bytes: 32}, Descriptor: capturefiles.DescriptorRef{Bytes: 32}}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	body, err := encodeRecord(ctx, Saved{Key: "import-" + strings.Repeat("a", 32), CandidateID: "generation-" + strings.Repeat("b", 32), Collection: "guide", Sources: fixtureRequest().Sources}, copied, testLimits())
	cancel()
	if err != nil {
		f.Fatal(err)
	}
	f.Add(body)
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"sources":[null]}`))
	f.Fuzz(func(t *testing.T, body []byte) {
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		ownedBody := bytes.Clone(body)
		result, ref, err := decodeRecord(ctx, ownedBody, testLimits())
		if err != nil {
			return
		}
		if !validID(result.Key, "import-") || !validID(result.CandidateID, "generation-") || len(result.Sources) > testLimits().MaxSources || ref.Bytes <= 0 {
			t.Fatal("invalid accepted state", result, ref)
		}
		originalKey := result.Key
		for i := range ownedBody {
			ownedBody[i] = 0
		}
		if result.Key != originalKey {
			t.Fatal("retained mutable input bytes")
		}
		encoded, err := encodeRecord(ctx, result, capturefiles.CopyInfo{Original: ref, Descriptor: ref}, testLimits())
		if errors.Is(err, ErrLimit) {
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		reopened, pin, err := decodeRecord(ctx, encoded, testLimits())
		if err != nil || pin != ref || !reflect.DeepEqual(reopened, result) {
			t.Fatal(reopened, pin, err)
		}
	})
}

func TestJobRejectsOversizedRoutesBeforeFilesystemWork(t *testing.T) {
	long := strings.Repeat("x", testLimits().Capture.MaxStringBytes+1)
	if _, err := Prepare(testContext(t), long, "missing/job", fixtureRequest(), testLimits()); !errors.Is(err, ErrLimit) {
		t.Fatal(err)
	}
	if _, err := Open(testContext(t), long, testLimits()); !errors.Is(err, ErrLimit) {
		t.Fatal(err)
	}
}
