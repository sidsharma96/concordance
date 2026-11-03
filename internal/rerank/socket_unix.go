//go:build darwin || linux

package rerank

import (
	"os"
	"path/filepath"
	"syscall"
)

func checkSocket(path string) error {
	if !filepath.IsAbs(path) || len(path) > 103 {
		return ErrInvalid
	}
	parent := filepath.Dir(path)
	real, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return err
	}
	if real != parent {
		return ErrInvalid
	}
	for i, p := range []string{parent, path} {
		info, err := os.Lstat(p)
		if err != nil {
			return err
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || stat.Uid != uint32(os.Geteuid()) {
			return ErrInvalid
		}
		if i == 0 {
			if !info.IsDir() || info.Mode().Perm() != 0o700 {
				return ErrInvalid
			}
		} else if info.Mode()&os.ModeSocket == 0 || info.Mode().Perm() != 0o600 {
			return ErrInvalid
		}
	}
	return nil
}
