//go:build !windows

package file

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

func open(p string) (*os.File, string, error) {
	s, e := os.Lstat(p)
	if e != nil {
		return nil, "", e
	}
	if !s.Mode().IsRegular() {
		return nil, "", errors.New("regular file required")
	}
	f, e := os.Open(p)
	if e != nil {
		return nil, "", e
	}
	st, e := f.Stat()
	if e != nil || !os.SameFile(s, st) {
		f.Close()
		return nil, "", errors.New("file changed during open")
	}
	i := st.Sys().(*syscall.Stat_t)
	return f, fmt.Sprintf("%d-%d", i.Dev, i.Ino), nil
}
