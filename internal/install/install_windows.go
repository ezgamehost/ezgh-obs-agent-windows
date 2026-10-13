//go:build windows

package install

import (
	"errors"
	"github.com/ezgamehost/ezgh-obs-agent-windows/internal/config"
	"github.com/ezgamehost/ezgh-obs-agent-windows/internal/platform"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
	"os"
	"path/filepath"
	"time"
	"unsafe"
)

const name = "ezgh-obs-agent-windows"

func Base() string {
	p, e := windows.KnownFolderPath(windows.FOLDERID_ProgramData, 0)
	if e != nil {
		return `C:\ProgramData\EZGH\ObsAgent`
	}
	return filepath.Join(p, "EZGH", "ObsAgent")
}
func Permission(path string, write bool) error {
	if e := platform.NoReparse(path); e != nil {
		return e
	}
	sid, e := platform.ServiceSID()
	if e != nil {
		return e
	}
	access := "GRGX"
	if write {
		access = "FA"
	}
	sd, e := windows.SecurityDescriptorFromString("O:BAG:BAD:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;" + access + ";;;" + sid + ")")
	if e != nil {
		return e
	}
	owner, _, _ := sd.Owner()
	dacl, _, _ := sd.DACL()
	return windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, owner, nil, dacl, nil)
}
func addGroup(sidText string) error {
	group, e := windows.StringToSid(sidText)
	if e != nil {
		return e
	}
	groupName, _, _, e := group.LookupAccount("")
	if e != nil {
		return e
	}
	member, _, _, e := windows.LookupSID("", `NT SERVICE\`+name)
	if e != nil {
		return e
	}
	g, _ := windows.UTF16PtrFromString(groupName)
	r, _, _ := windows.NewLazySystemDLL("netapi32.dll").NewProc("NetLocalGroupAddMembers").Call(0, uintptr(unsafe.Pointer(g)), 0, uintptr(unsafe.Pointer(&member)), 1)
	if r != 0 && r != 1378 {
		return windows.Errno(r)
	}
	return nil
}
func Handle(op string) error {
	m, e := mgr.Connect()
	if e != nil {
		return e
	}
	defer m.Disconnect()
	s, e := m.OpenService(name)
	if op == "install-prepare" {
		k, _, err := registry.CreateKey(registry.LOCAL_MACHINE, `SOFTWARE\EZGH\ObsAgent\Upgrade`, registry.SET_VALUE)
		if err != nil {
			return err
		}
		defer k.Close()
		k.SetDWordValue("Running", 0)
		k.SetDWordValue("Start", mgr.StartManual)
		k.SetDWordValue("Delayed", 0)
		if e != nil {
			return nil
		}
		defer s.Close()
		c, e := s.Config()
		if e != nil {
			return e
		}
		v, e := s.Query()
		if e != nil {
			return e
		}
		if e = k.SetDWordValue("Start", c.StartType); e != nil {
			return e
		}
		if c.DelayedAutoStart {
			k.SetDWordValue("Delayed", 1)
		}
		if v.State == svc.Running {
			return k.SetDWordValue("Running", 1)
		}
		return nil
	}
	if e != nil {
		return e
	}
	defer s.Close()
	if op == "repair-permissions" || op == "configure-install" {
		// MSI creates these directories. Existing retained data is never deleted.
		for _, p := range []string{Base(), filepath.Join(Base(), "config"), filepath.Join(Base(), "secrets"), filepath.Join(Base(), "state")} {
			if _, e := os.Stat(p); e != nil {
				return e
			}
			if e = Permission(p, filepath.Base(p) == "state"); e != nil {
				return e
			}
		}
		for _, folder := range []string{"config", "secrets", "state"} {
			if e = filepath.WalkDir(filepath.Join(Base(), folder), func(p string, d os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				return Permission(p, folder == "state")
			}); e != nil {
				return e
			}
		}
		if op == "repair-permissions" {
			return nil
		}
		for _, g := range []string{"S-1-5-32-573", "S-1-5-32-558"} {
			if e = addGroup(g); e != nil {
				return e
			}
		}
		c, e := s.Config()
		if e != nil {
			return e
		}
		c.SidType = windows.SERVICE_SID_TYPE_UNRESTRICTED
		if e = s.UpdateConfig(c); e != nil {
			return e
		}
		if e = s.SetRecoveryActions([]mgr.RecoveryAction{{Type: mgr.ServiceRestart, Delay: 30 * time.Second}, {Type: mgr.ServiceRestart, Delay: 60 * time.Second}, {Type: mgr.NoAction}}, 86400); e != nil {
			return e
		}
		if e = s.SetRecoveryActionsOnNonCrashFailures(false); e != nil {
			return e
		}
		return restore(s)
	}
	if op == "install-rollback" {
		return restore(s)
	}
	if op == "activate" {
		c, e := config.Load(filepath.Join(Base(), "config", "config.yaml"))
		if e != nil {
			return e
		}
		if _, e = config.ReadCredential(c.CredentialFile); e != nil {
			return e
		}
		sc, e := s.Config()
		if e != nil {
			return e
		}
		sc.StartType = mgr.StartAutomatic
		sc.DelayedAutoStart = true
		if e = s.UpdateConfig(sc); e != nil {
			return e
		}
		return s.Start()
	}
	return errors.New("unknown installer command")
}
func restore(s *mgr.Service) error {
	k, e := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\EZGH\ObsAgent\Upgrade`, registry.QUERY_VALUE)
	if e != nil {
		return nil
	}
	defer k.Close()
	start, _, e := k.GetIntegerValue("Start")
	if e != nil {
		return e
	}
	delayed, _, _ := k.GetIntegerValue("Delayed")
	running, _, _ := k.GetIntegerValue("Running")
	c, e := s.Config()
	if e != nil {
		return e
	}
	c.StartType = uint32(start)
	c.DelayedAutoStart = delayed != 0
	if e = s.UpdateConfig(c); e != nil {
		return e
	}
	if running == 1 {
		cfg, e := config.Load(filepath.Join(Base(), "config", "config.yaml"))
		if e != nil {
			return e
		}
		if _, e = config.ReadCredential(cfg.CredentialFile); e != nil {
			return e
		}
		return s.Start()
	}
	return nil
}
