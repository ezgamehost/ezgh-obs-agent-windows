package app

import (
	"context"
	"github.com/ezgamehost/ezgh-obs-agent-windows/internal/diagnostics"
	exporter "github.com/ezgamehost/ezgh-obs-agent-windows/internal/export"
	"github.com/ezgamehost/ezgh-obs-agent-windows/internal/spool"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestUnexpectedSuccessRetainsQueue(t *testing.T) {
	q, e := spool.Open(filepath.Join(t.TempDir(), "q.db"), "test", 16<<20, 0)
	if e != nil {
		t.Fatal(e)
	}
	defer q.Close()
	if e = q.Enqueue([][]byte{{}}, nil); e != nil {
		t.Fatal(e)
	}
	seen := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(202)
		select {
		case seen <- struct{}{}:
		default:
		}
	}))
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	var generation atomic.Uint64
	state := diagnostics.New()
	go func() {
		done <- deliver(ctx, "logs", q, exporter.NewWithClient(srv.URL, srv.Client(), "test"), state, &generation)
	}()
	select {
	case <-seen:
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("no request")
	}
	deadline := time.Now().Add(time.Second)
	for state.Snapshot().Counters["logs_retries"] == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	cancel()
	if e = <-done; e != nil {
		t.Fatal(e)
	}
	if _, e = q.Peek(); e != nil {
		t.Fatal("unexpected success deleted batch", e)
	}
}
