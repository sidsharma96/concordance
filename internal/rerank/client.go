package rerank

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"net"
	"time"
)

type greeting struct {
	Version      string `json:"version"`
	Kind         string `json:"kind"`
	Nonce        string `json:"nonce"`
	Model        string `json:"model"`
	Revision     string `json:"revision"`
	InputVersion string `json:"input_version"`
	Runtime      string `json:"runtime"`
	Code         string `json:"code"`
}
type score struct {
	ID     string   `json:"id"`
	Score  *float64 `json:"score"`
	Tokens int      `json:"tokens"`
}
type response struct {
	Version      string  `json:"version"`
	Kind         string  `json:"kind"`
	Nonce        string  `json:"nonce"`
	RequestID    string  `json:"request_id"`
	InputSHA256  string  `json:"input_sha256"`
	Model        string  `json:"model"`
	Revision     string  `json:"revision"`
	InputVersion string  `json:"input_version"`
	Runtime      string  `json:"runtime"`
	Scores       []score `json:"scores"`
	Code         string  `json:"code"`
}

// Result owns scores/token counts in the original Batch order, never worker order.
// Copying the wrapper shares its slices. Scores are raw logits, not probabilities.
type Result struct {
	Scores []float64
	Tokens []int
}

func hexID(s string, n int) bool {
	if len(s) != n {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
func compatible(model, revision, input, runtime string) bool {
	return model == Model && revision == Revision && input == InputVersion && runtime == Runtime
}
func workerError(code string) error {
	switch code {
	case "busy":
		return ErrBusy
	case "limit":
		return ErrLimit
	case "invalid":
		return ErrInvalid
	case "timeout":
		return context.DeadlineExceeded
	case "cancelled", "stopping", "runtime":
		return ErrWorker
	default:
		return ErrInvalid
	}
}

// Score opens one private connection for one batch. It has no retry, connection
// pool or credential lookup. All errors return zero Result. Empty batches need
// no socket. Caller owns immutable Batch state and simultaneous-call admission.
// Context cancellation closes and joins its callback; it does not claim to stop
// remote tensor computation. The worker keeps its reservation until compute drains.
func Score(ctx context.Context, socket string, b Batch) (Result, error) {
	if err := ready(ctx); err != nil {
		return Result{}, err
	}
	if b.query == "" || b.fingerprint != fingerprint(b.query, b.pairs) {
		return Result{}, ErrInvalid
	}
	if len(b.pairs) == 0 {
		return Result{Scores: []float64{}, Tokens: []int{}}, nil
	}
	if err := checkSocket(socket); err != nil {
		return Result{}, fmt.Errorf("%w: check private socket: %w", ErrWorker, err)
	}
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", socket)
	if err != nil {
		return Result{}, fmt.Errorf("%w: connect local reranking worker: %w", ErrWorker, err)
	}
	defer conn.Close()
	end, _ := ctx.Deadline()
	if err := conn.SetDeadline(end); err != nil {
		return Result{}, err
	}
	joined := make(chan struct{})
	stop := context.AfterFunc(ctx, func() { conn.Close(); close(joined) })
	defer func() {
		if !stop() {
			<-joined
		}
	}()
	result, err := exchange(ctx, conn, b, end)
	if cause := ctx.Err(); cause != nil {
		return Result{}, cause
	}
	if err != nil {
		return Result{}, err
	}
	return result, nil
}
func exchange(ctx context.Context, conn net.Conn, b Batch, end time.Time) (Result, error) {
	wire, err := readFrame(conn, maxGreeting)
	if err != nil {
		return Result{}, fmt.Errorf("%w: read worker greeting: %w", ErrWorker, err)
	}
	var hello greeting
	if err := decode(ctx, wire, &hello); err != nil {
		return Result{}, err
	}
	if hello.Version != Protocol {
		return Result{}, ErrWorker
	}
	if hello.Kind == "error" {
		return Result{}, workerError(hello.Code)
	}
	if hello.Kind != "ready" || hello.Code != "" || !hexID(hello.Nonce, 32) || !compatible(hello.Model, hello.Revision, hello.InputVersion, hello.Runtime) {
		return Result{}, ErrWorker
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return Result{}, err
	}
	remaining := time.Until(end).Milliseconds()
	if remaining <= 0 {
		return Result{}, context.DeadlineExceeded
	}
	remaining = min(remaining, int64((5*time.Minute)/time.Millisecond))
	req := request{Version: Protocol, Nonce: hello.Nonce, RequestID: hex.EncodeToString(id[:]), InputSHA256: b.fingerprint, Query: b.query, TimeoutMS: remaining, Pairs: b.pairs}
	wire, err = encodeRequest(req)
	if err != nil {
		return Result{}, err
	}
	if err = writeFrame(conn, wire); err != nil {
		return Result{}, fmt.Errorf("%w: write worker request: %w", ErrWorker, err)
	}
	wire, err = readFrame(conn, maxResponse)
	if err != nil {
		return Result{}, fmt.Errorf("%w: read worker result: %w", ErrWorker, err)
	}
	var reply response
	if err := decode(ctx, wire, &reply); err != nil {
		return Result{}, err
	}
	if reply.Version != Protocol || reply.Nonce != hello.Nonce {
		return Result{}, ErrInvalid
	}
	if reply.Kind == "error" {
		if reply.RequestID != "" && reply.RequestID != req.RequestID || reply.InputSHA256 != "" && reply.InputSHA256 != req.InputSHA256 {
			return Result{}, ErrInvalid
		}
		return Result{}, workerError(reply.Code)
	}
	if reply.Kind != "result" || reply.Code != "" || reply.RequestID != req.RequestID || reply.InputSHA256 != req.InputSHA256 || !compatible(reply.Model, reply.Revision, reply.InputVersion, reply.Runtime) || len(reply.Scores) != len(b.pairs) {
		return Result{}, ErrWorker
	}
	out := Result{Scores: make([]float64, len(b.pairs)), Tokens: make([]int, len(b.pairs))}
	positions := make(map[string]int, len(b.pairs))
	for i, p := range b.pairs {
		positions[p.ID] = i
	}
	total := 0
	for _, s := range reply.Scores {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		i, ok := positions[s.ID]
		if !ok || s.Score == nil || math.IsNaN(*s.Score) || math.IsInf(*s.Score, 0) || s.Tokens <= 0 || s.Tokens > MaxPairTokens {
			return Result{}, ErrInvalid
		}
		delete(positions, s.ID)
		if s.Tokens > MaxTotalTokens-total {
			return Result{}, ErrLimit
		}
		total += s.Tokens
		out.Scores[i] = *s.Score
		out.Tokens[i] = s.Tokens
	}
	if len(positions) != 0 {
		return Result{}, errors.New("worker omitted expected scores")
	}
	return out, nil
}
