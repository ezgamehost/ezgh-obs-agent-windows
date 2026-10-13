//go:build !windows

package platform

import (
	"errors"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
)

func CheckStateFile(p string) error {
	s, e := os.Lstat(p)
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return e
	}
	if !s.Mode().IsRegular() || s.Mode().Perm()&0077 != 0 {
		return errors.New("state file must be private")
	}
	return nil
}
func Available(p string) (uint64, error) {
	var s unix.Statfs_t
	e := unix.Statfs(p, &s)
	return uint64(s.Bavail) * uint64(s.Bsize), e
}
func OpenSecret(p string) (*os.File, error) {
	if e := CheckStateFile(p); e != nil {
		return nil, e
	}
	return os.Open(p)
}
func OpenConfig(p string) (*os.File, error) { return OpenSecret(p) }
func Lock(dir string) (*os.File, error) {
	if e := os.MkdirAll(dir, 0700); e != nil {
		return nil, e
	}
	f, e := os.OpenFile(filepath.Join(dir, "agent.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	if e = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); e != nil {
		f.Close()
		return nil, e
	}
	return f, nil
}
