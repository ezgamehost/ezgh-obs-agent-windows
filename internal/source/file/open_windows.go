//go:build windows

package file

import (
	"errors"
	"fmt"
	"github.com/ezgamehost/ezgh-obs-agent-windows/internal/platform"
	"golang.org/x/sys/windows"
	"os"
)

func open(p string) (*os.File, string, error) {
	if e := platform.NoReparse(p); e != nil {
		return nil, "", e
	}
	u, e := windows.UTF16PtrFromString(p)
	if e != nil {
		return nil, "", e
	}
	h, e := windows.CreateFile(u, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if e != nil {
		return nil, "", e
	}
	f := os.NewFile(uintptr(h), p)
	var s windows.ByHandleFileInformation
	if e = windows.GetFileInformationByHandle(h, &s); e != nil {
		f.Close()
		return nil, "", e
	}
	if s.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DIRECTORY) != 0 {
		f.Close()
		return nil, "", errors.New("regular file required")
	}
	return f, fmt.Sprintf("%x-%x-%x-%x-%x", s.VolumeSerialNumber, s.FileIndexHigh, s.FileIndexLow, s.CreationTime.HighDateTime, s.CreationTime.LowDateTime), nil
}
