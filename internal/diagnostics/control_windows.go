//go:build windows

package diagnostics

import (
	"context"
	"github.com/Microsoft/go-winio"
	"github.com/ezgamehost/ezgh-obs-agent-windows/internal/platform"
	"golang.org/x/sys/windows/svc/eventlog"
	"net"
)

func Path(string) string { return `\\.\pipe\ezgh-obs-agent-windows` }
func listen(p string) (net.Listener, error) {
	sid, e := platform.ServiceSID()
	if e != nil {
		return nil, e
	}
	return winio.ListenPipe(p, &winio.PipeConfig{SecurityDescriptor: "D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;GA;;;" + sid + ")", InputBufferSize: 1024, OutputBufferSize: 65536})
}
func dial(ctx context.Context, p string) (net.Conn, error) { return winio.DialPipeContext(ctx, p) }
func Local(s string) {
	l, e := eventlog.Open("ezgh-obs-agent-windows")
	if e == nil {
		defer l.Close()
		l.Info(1, s)
	}
}
