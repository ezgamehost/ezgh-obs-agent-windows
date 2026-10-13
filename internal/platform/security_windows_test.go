//go:build windows

package platform_test

import (
	"github.com/ezgamehost/ezgh-obs-agent-windows/internal/install"
	"github.com/ezgamehost/ezgh-obs-agent-windows/internal/platform"
	"os"
	"path/filepath"
	"testing"
)

func TestProtectedSecret(t *testing.T) {
	p := filepath.Join(t.TempDir(), "secret")
	os.WriteFile(p, []byte("fixture"), 0600)
	if e := install.Permission(p, false); e != nil {
		t.Fatal(e)
	}
	f, e := platform.OpenSecret(p)
	if e != nil {
		t.Fatal(e)
	}
	f.Close()
}
