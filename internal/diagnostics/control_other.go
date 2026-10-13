//go:build !windows

package diagnostics

import (
	"context"
	"net"
	"path/filepath"
)

func Path(dir string) string                { return filepath.Join(dir, "control.sock") }
func listen(p string) (net.Listener, error) { return net.Listen("unix", p) }
func dial(ctx context.Context, p string) (net.Conn, error) {
	return (&net.Dialer{}).DialContext(ctx, "unix", p)
}
func Local(string) {}
