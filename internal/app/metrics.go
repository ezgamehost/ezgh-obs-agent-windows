package app

import (
	"context"
	"errors"
	"github.com/ezgamehost/ezgh-obs-agent-windows/internal/config"
	"github.com/ezgamehost/ezgh-obs-agent-windows/internal/diagnostics"
	"github.com/ezgamehost/ezgh-obs-agent-windows/internal/source/windowsmetrics"
	"github.com/ezgamehost/ezgh-obs-agent-windows/internal/spool"
	"github.com/ezgamehost/ezgh-obs-agent-windows/internal/telemetry"
	dto "github.com/prometheus/client_model/go"

	"math/rand/v2"

	"time"
)

func collectMetrics(ctx context.Context, c config.Config, id, host string, q *spool.Queue, s *diagnostics.State) error {
	s.Set("metrics_collection", "initializing")
	collector, e := windowsmetrics.New(c.Metrics.Collectors)
	if e != nil {
		s.Set("metrics_collection", "initialization_failed")
		<-ctx.Done()
		return nil
	}
	type result struct {
		f  []*dto.MetricFamily
		e  error
		at time.Time
	}
	done := make(chan result, 1)
	t := time.NewTimer(time.Duration(rand.Int64N(int64(c.Metrics.Interval.Duration))))
	defer t.Stop()
	health := time.NewTicker(time.Second)
	defer health.Stop()
	var started time.Time
	for {
		select {
		case <-ctx.Done():
			if started.IsZero() {
				_ = collector.Close()
			} else {
				go func() { <-done; _ = collector.Close() }()
			}
			return nil
		case <-t.C:
			t.Reset(c.Metrics.Interval.Duration)
			if !started.IsZero() || !q.CanAccept() {
				s.Add("metrics_skipped_scrapes", 1)
				continue
			}
			started = time.Now()
			go func(at time.Time) { f, e := collector.Gather(); done <- result{f, e, at} }(started)
		case r := <-done:
			started = time.Time{}
			s.Gauge("metrics_collection_age_seconds", 0)
			if r.e != nil {
				s.Add("metrics_collection_errors", 1)
			}
			healthy := collectorHealth(s, r.f, c.Metrics.Collectors) && r.e == nil
			for _, f := range r.f {
				if f.GetType() != dto.MetricType_GAUGE && f.GetType() != dto.MetricType_COUNTER && f.GetType() != dto.MetricType_UNTYPED {
					s.Add("metrics_unsupported_families", 1)
				}
			}
			payloads, e := telemetry.EncodeMetrics(r.f, telemetry.Resource(id, host, c.Environment, "windows-exporter"), r.at, c.Metrics.MaxPoints, c.Metrics.MaxBytes)
			if e != nil {
				s.Add("metrics_rejected_scrapes", 1)
				continue
			}
			if e = q.Enqueue(payloads, nil); errors.Is(e, spool.ErrFull) {
				s.Add("metrics_skipped_scrapes", 1)
			} else if e != nil {
				return e
			} else {
				for _, p := range payloads {
					s.Add("metrics_collected", uint64(telemetry.Count("metrics", p)))
				}
				if healthy {
					s.Set("metrics_collection", "ready")
				} else {
					s.Set("metrics_collection", "degraded")
				}
			}
		case <-health.C:
			if !started.IsZero() {
				age := time.Since(started)
				s.Gauge("metrics_collection_age_seconds", age.Seconds())
				if age > 60*time.Second {
					s.Set("metrics_collection", "blocked")
				}
			}

		}
	}
}

// Collection errors are encoded by the upstream collectors as success gauges,
// even when the registry itself gathers without error.
func collectorHealth(s *diagnostics.State, families []*dto.MetricFamily, names []string) bool {
	success := make(map[string]bool, len(names))
	for _, family := range families {
		if family.GetName() != "windows_exporter_collector_success" {
			continue
		}
		for _, metric := range family.Metric {
			for _, label := range metric.Label {
				if label.GetName() == "collector" {
					success[label.GetValue()] = metric.GetGauge().GetValue() == 1
				}
			}
		}
	}
	healthy := true
	for _, name := range names {
		value := 0.0
		if success[name] {
			value = 1
		} else {
			healthy = false
		}
		s.Gauge("collector_success_"+name, value)
	}
	return healthy
}
