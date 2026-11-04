package importjob

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
)

// Only the durability boundaries this writer consumes are substitutable. There
// are no global hooks; a test's operations belong to that one preparation call.
type operations struct {
	write  func(context.Context, *os.Root, string, []byte) error
	sync   func(context.Context, *os.Root) error
	link   func(*os.Root, string, string) error
	remove func(*os.Root, string) error
}

func nativeOperations() operations {
	return operations{write: writeExclusive, sync: syncDirectory,
		link:   func(root *os.Root, old, next string) error { return root.Link(old, next) },
		remove: func(root *os.Root, name string) error { return root.Remove(name) },
	}
}

type syncFile interface {
	Write([]byte) (int, error)
	Sync() error
	Close() error
}

// writeAndSync consumes and always closes the newly created file. It never keeps
// body aliases. A Sync cannot be interrupted by context; checks surround it.
func writeAndSync(ctx context.Context, file syncFile, body []byte) (err error) {
	defer func() { err = errors.Join(err, file.Close()) }()
	if err := ctx.Err(); err != nil {
		return err
	}
	n, err := file.Write(body)
	if err != nil {
		return err
	}
	if n != len(body) {
		return io.ErrShortWrite
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	return ctx.Err()
}
func writeExclusive(ctx context.Context, root *os.Root, name string, body []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	file, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	return writeAndSync(ctx, file, body)
}
func syncDirectory(ctx context.Context, root *os.Root) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	file, err := root.Open(".")
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	if err := file.Sync(); err != nil {
		return err
	}
	return ctx.Err()
}
func readFile(ctx context.Context, root *os.Root, name string, max int) (body []byte, err error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	info, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, ErrInvalid
	}
	file, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer func() {
		err = errors.Join(err, file.Close())
		if err != nil {
			body = nil
		}
	}()
	info, err = file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, ErrInvalid
	}
	if info.Size() > int64(max) {
		return nil, ErrLimit
	}
	body, err = io.ReadAll(io.LimitReader(&contextReader{ctx: ctx, reader: file}, int64(max)+1))
	if err != nil {
		return nil, err
	}
	if len(body) > max {
		return nil, ErrLimit
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return body, nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(body []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(body)
}

// A job inside the producer would be deleted along with that producer. Compare
// actual directory identity up the existing parent chain, including symlink/case
// aliases. Protected stable routing is still assumed; this is no hostile-FS sandbox.
func jobLocation(ctx context.Context, producer, job string) (string, string, error) {
	if producer == "" || job == "" {
		return "", "", ErrInvalid
	}
	info, err := os.Stat(producer)
	if err != nil {
		return "", "", err
	}
	if !info.IsDir() {
		return "", "", ErrInvalid
	}
	absolute, err := filepath.Abs(job)
	if err != nil {
		return "", "", err
	}
	leaf := filepath.Base(absolute)
	parent := filepath.Dir(absolute)
	if leaf == "." || leaf == string(filepath.Separator) {
		return "", "", ErrInvalid
	}
	actual, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return "", "", err
	}
	for path := actual; ; path = filepath.Dir(path) {
		if err := ctx.Err(); err != nil {
			return "", "", err
		}
		current, err := os.Stat(path)
		if err != nil {
			return "", "", err
		}
		if !current.IsDir() || os.SameFile(info, current) {
			return "", "", ErrInvalid
		}
		if next := filepath.Dir(path); next == path {
			break
		}
	}
	return parent, leaf, nil
}
