package app

import (
	"context"
	"github.com/ezgamehost/ezgh-obs-agent-windows/internal/config"
	"github.com/ezgamehost/ezgh-obs-agent-windows/internal/diagnostics"
	"github.com/ezgamehost/ezgh-obs-agent-windows/internal/spool"
	"github.com/ezgamehost/ezgh-obs-agent-windows/internal/telemetry"
	"path/filepath"
	"testing"
	"time"
)

func TestSelfMetricsWithoutHostCollector(t *testing.T) {
	q, e := spool.Open(filepath.Join(t.TempDir(), "metrics.db"), "test", 16<<20, 0)
	if e != nil {
		t.Fatal(e)
	}
	defer q.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	if e = selfMetrics(ctx, config.Config{}, "id", "host", q, diagnostics.New(), time.Millisecond); e != nil {
		t.Fatal(e)
	}
	b, e := q.Peek()
	if e != nil || telemetry.Count("metrics", b.Payload) == 0 {
		t.Fatal("no independent health metrics", e)
	}
}
