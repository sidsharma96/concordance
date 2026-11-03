package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// Explicit allowlist: the model process receives no catalog, R2 or inherited
// Python/Hugging Face credentials/configuration. This is not an OS sandbox.
func workerEnvironment(path, tmp, cache string) []string {
	return []string{"PATH=" + path, "TMPDIR=" + tmp, "HF_HOME=" + cache, "HF_HUB_OFFLINE=1", "TRANSFORMERS_OFFLINE=1", "TOKENIZERS_PARALLELISM=false", "PYTORCH_ENABLE_MPS_FALLBACK=0"}
}
func runWorker(ctx context.Context, args []string, out io.Writer) error {
	if ctx == nil || out == nil || len(args) > 16 {
		return errors.New("invalid worker invocation")
	}
	f := flag.NewFlagSet("worker", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	python := f.String("python", ".tools/reranking/venv/bin/python", "pinned Python environment executable")
	model := f.String("model-dir", ".tools/reranking/model", "offline pinned model asset directory")
	socket := f.String("socket", ".tools/reranking/run/worker.sock", "private Unix socket; existing paths are never unlinked at startup")
	if err := f.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return searchHelp(f, out, "Usage: concordance worker [flags]")
		}
		return commandFailure("invalid worker flags; use worker --help", err)
	}
	if f.NArg() != 0 || runtime.GOOS != "darwin" {
		return errors.New("this pinned model worker requires the tested macOS/MPS environment; use worker --help")
	}
	sock, err := workerSocket(*socket)
	if err != nil {
		return err
	}
	executable, err := filepath.Abs(*python)
	if err != nil {
		return err
	}
	assets, err := filepath.Abs(*model)
	if err != nil {
		return err
	}
	script, err := filepath.Abs("worker/rerank.py")
	if err != nil {
		return err
	}
	info, err := os.Lstat(script)
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("start worker from a checkout containing worker/rerank.py")
	}
	tmp, err := filepath.Abs(".tools/tmp")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(tmp, 0o700); err != nil {
		return err
	}
	cache := filepath.Join(filepath.Dir(assets), "hf")
	cmd := exec.CommandContext(ctx, executable, "-I", "-B", script, "--socket", sock, "--model-dir", assets)
	cmd.Env = workerEnvironment(os.Getenv("PATH"), tmp, cache)
	cmd.Stdout = out
	cmd.Stderr = os.Stderr
	// Context requests graceful draining; Wait joins the actual child. There is no
	// automatic hard-kill deadline that would falsely claim cooperative preemption.
	cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
	if err := cmd.Run(); err != nil {
		return commandFailure("local model worker stopped", err)
	}
	return nil
}
