//go:build !darwin && !linux

package rerank

func checkSocket(string) error { return ErrUnsupported }
