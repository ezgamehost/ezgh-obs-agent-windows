package app

import (
	"context"
	"errors"
	"github.com/ezgamehost/ezgh-obs-agent-windows/internal/config"
	"github.com/ezgamehost/ezgh-obs-agent-windows/internal/diagnostics"
	"github.com/ezgamehost/ezgh-obs-agent-windows/internal/spool"
	"github.com/ezgamehost/ezgh-obs-agent-windows/internal/telemetry"
	dto "github.com/prometheus/client_model/go"
	"google.golang.org/protobuf/proto"
	"runtime"
	"time"
)

func selfMetrics(ctx context.Context, c config.Config, id, host string, q *spool.Queue, s *diagnostics.State, interval time.Duration) error {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:

			var mem runtime.MemStats
			runtime.ReadMemStats(&mem)
			s.Gauge("go_heap_bytes", float64(mem.Alloc))
			snap := s.Snapshot()
			var f []*dto.MetricFamily
			for name, value := range snap.Counters {
				f = append(f, &dto.MetricFamily{Name: proto.String("ezgh_agent_" + name + "_total"), Type: dto.MetricType_COUNTER.Enum(), Metric: []*dto.Metric{{Counter: &dto.Counter{Value: proto.Float64(float64(value))}}}})
			}
			for name, value := range snap.Gauges {
				f = append(f, &dto.MetricFamily{Name: proto.String("ezgh_agent_" + name), Type: dto.MetricType_GAUGE.Enum(), Metric: []*dto.Metric{{Gauge: &dto.Gauge{Value: proto.Float64(value)}}}})
			}
			payloads, e := telemetry.EncodeMetrics(f, telemetry.Resource(id, host, c.Environment, "ezgh-obs-agent-windows"), time.Now(), 50000, 16<<20)
			if e != nil {
				return e
			}
			if e = q.Enqueue(payloads, nil); e != nil && !errors.Is(e, spool.ErrFull) {
				return e
			}
		}
	}
}
