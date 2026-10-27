package gitsource

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

// run waits on every started child, including cancellation/output-limit paths.
// Git stderr is discarded because it may contain repository paths or payloads.
func run(ctx context.Context, repository string, maxBytes int64, arguments ...string) ([]byte, error) {
	childContext, cancel := context.WithCancel(ctx)
	defer cancel()
	args := []string{"--no-replace-objects", "--no-lazy-fetch", "--no-optional-locks", "--literal-pathspecs", "-C", repository}
	cmd := exec.CommandContext(childContext, "git", append(args, arguments...)...)
	cmd.Env = gitEnvironment()
	cmd.Stderr = io.Discard
	cmd.WaitDelay = time.Second
	output := cappedOutput{remaining: maxBytes, cancel: cancel}
	cmd.Stdout = &output
	err := cmd.Run()
	if output.exceeded {
		return nil, errors.Join(ErrLimit, ctx.Err())
	}
	if err != nil {
		return nil, errors.Join(err, ctx.Err())
	}
	return output.buffer.Bytes(), nil
}

func gitEnvironment() []string {
	var result []string
	for _, variable := range os.Environ() {
		if !strings.HasPrefix(variable, "GIT_") && !strings.HasPrefix(variable, "LC_ALL=") {
			result = append(result, variable)
		}
	}
	return append(result, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_TERMINAL_PROMPT=0", "LC_ALL=C")
}

type cappedOutput struct {
	buffer    bytes.Buffer
	remaining int64
	cancel    context.CancelFunc
	exceeded  bool
}

func (w *cappedOutput) Write(data []byte) (int, error) {
	if int64(len(data)) > w.remaining {
		w.exceeded = true
		w.cancel()
		return 0, ErrLimit
	}
	w.remaining -= int64(len(data))
	return w.buffer.Write(data)
}
