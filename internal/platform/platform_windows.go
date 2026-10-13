//go:build windows

package platform

import (
	"crypto/sha1"
	"encoding/binary"
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf16"
	"unsafe"
)

const ServiceName = "ezgh-obs-agent-windows"

func ServiceSID() (string, error) {
	u := utf16.Encode([]rune(strings.ToUpper(ServiceName)))
	b := make([]byte, 2*len(u))
	for i, v := range u {
		binary.LittleEndian.PutUint16(b[2*i:], v)
	}
	h := sha1.Sum(b)
	return fmt.Sprintf("S-1-5-80-%d-%d-%d-%d-%d", binary.LittleEndian.Uint32(h[0:]), binary.LittleEndian.Uint32(h[4:]), binary.LittleEndian.Uint32(h[8:]), binary.LittleEndian.Uint32(h[12:]), binary.LittleEndian.Uint32(h[16:])), nil
}
func NoReparse(p string) error {
	p = filepath.Clean(p)
	if !filepath.IsAbs(p) || strings.HasPrefix(p, `\\`) {
		return errors.New("local absolute path required")
	}
	for {
		u, e := windows.UTF16PtrFromString(p)
		if e != nil {
			return e
		}
		a, e := windows.GetFileAttributes(u)
		if e != nil {
			return e
		}
		if a&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
			return errors.New("reparse points are not supported")
		}
		parent := filepath.Dir(p)
		if parent == p {
			break
		}
		p = parent
	}
	return nil
}
func CheckStateFile(p string) error {
	if _, e := os.Lstat(p); os.IsNotExist(e) {
		return nil
	}
	return NoReparse(p)
}
func Available(p string) (uint64, error) {
	u, e := windows.UTF16PtrFromString(filepath.Dir(p))
	if e != nil {
		return 0, e
	}
	var free uint64
	e = windows.GetDiskFreeSpaceEx(u, &free, nil, nil)
	return free, e
}

// CheckACL rejects untrusted effective allow ACEs. Installer provisions protected DACLs.
func CheckACL(h windows.Handle, secret bool) error {
	sd, e := windows.GetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if e != nil {
		return e
	}
	a, _, e := sd.DACL()
	if e != nil || a == nil {
		return errors.New("missing restricted DACL")
	}
	service, _ := ServiceSID()
	trusted := func(s string) bool { return s == "S-1-5-18" || s == "S-1-5-32-544" || s == service }
	owner, _, e := sd.Owner()
	if e != nil || !trusted(owner.String()) {
		return errors.New("owner must be Administrators, SYSTEM or service")
	}
	for i := uint32(0); i < uint32(a.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if e = windows.GetAce(a, i, &ace); e != nil {
			return e
		}
		if ace.Header.AceFlags&windows.INHERIT_ONLY_ACE != 0 {
			continue
		}
		if ace.Header.AceType == windows.ACCESS_DENIED_ACE_TYPE {
			continue
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			return errors.New("unsupported access control entry")
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if trusted(sid.String()) {
			continue
		}
		danger := uint32(windows.GENERIC_ALL | windows.GENERIC_WRITE | windows.DELETE | windows.WRITE_DAC | windows.WRITE_OWNER | windows.FILE_WRITE_DATA | windows.FILE_APPEND_DATA | 0x0040 /* FILE_DELETE_CHILD */ | windows.FILE_WRITE_ATTRIBUTES | windows.FILE_WRITE_EA)
		if secret {
			danger |= windows.GENERIC_READ | windows.FILE_READ_DATA
		}
		if uint32(ace.Mask)&danger != 0 {
			return errors.New("file access is too broad")
		}
	}
	return nil
}
func secureOpen(p string, secret bool) (*os.File, error) {
	if e := NoReparse(p); e != nil {
		return nil, e
	}
	u, e := windows.UTF16PtrFromString(p)
	if e != nil {
		return nil, e
	}
	h, e := windows.CreateFile(u, windows.GENERIC_READ|windows.READ_CONTROL, windows.FILE_SHARE_READ|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if e != nil {
		return nil, e
	}
	f := os.NewFile(uintptr(h), p)
	var info windows.ByHandleFileInformation
	if e = windows.GetFileInformationByHandle(h, &info); e == nil && info.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DIRECTORY) != 0 {
		e = errors.New("regular file required")
	}
	if e == nil {
		e = CheckACL(h, secret)
	}
	if e != nil {
		f.Close()
		return nil, e
	}
	return f, nil
}
func OpenSecret(p string) (*os.File, error) { return secureOpen(p, true) }
func OpenConfig(p string) (*os.File, error) { return secureOpen(p, false) }
func Lock(dir string) (*os.File, error) {
	if e := NoReparse(dir); e != nil {
		return nil, e
	}
	u, e := windows.UTF16PtrFromString(dir)
	if e != nil {
		return nil, e
	}
	h, e := windows.CreateFile(u, windows.READ_CONTROL, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if e != nil {
		return nil, e
	}
	e = CheckACL(h, true)
	windows.CloseHandle(h)
	if e != nil {
		return nil, e
	}
	var fs [32]uint16
	root, _ := windows.UTF16PtrFromString(filepath.VolumeName(dir) + `\`)
	if windows.GetDriveType(root) != windows.DRIVE_FIXED {
		return nil, errors.New("state requires a fixed local drive")
	}
	if e = windows.GetVolumeInformation(root, nil, 0, nil, nil, nil, &fs[0], uint32(len(fs))); e != nil {
		return nil, e
	}
	if windows.UTF16ToString(fs[:]) != "NTFS" {
		return nil, errors.New("state requires NTFS")
	}
	p := filepath.Join(dir, "agent.lock")
	if e = CheckStateFile(p); e != nil {
		return nil, e
	}
	u, _ = windows.UTF16PtrFromString(p)
	h, e = windows.CreateFile(u, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_ALWAYS, windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if e != nil {
		return nil, errors.New("state is locked or unavailable")
	}
	return os.NewFile(uintptr(h), p), nil
}
