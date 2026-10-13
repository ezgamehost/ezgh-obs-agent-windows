package app

import (
	"context"
	"errors"
	"github.com/ezgamehost/ezgh-obs-agent-windows/internal/config"
	"github.com/ezgamehost/ezgh-obs-agent-windows/internal/diagnostics"
	exporter "github.com/ezgamehost/ezgh-obs-agent-windows/internal/export"
	"github.com/ezgamehost/ezgh-obs-agent-windows/internal/identity"
	"github.com/ezgamehost/ezgh-obs-agent-windows/internal/platform"
	"github.com/ezgamehost/ezgh-obs-agent-windows/internal/source/eventlog"
	files "github.com/ezgamehost/ezgh-obs-agent-windows/internal/source/file"
	"github.com/ezgamehost/ezgh-obs-agent-windows/internal/spool"
	"github.com/ezgamehost/ezgh-obs-agent-windows/internal/telemetry"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

func Run(parent context.Context, c config.Config, ready func()) error {
	lock, e := platform.Lock(c.StateDir)
	if e != nil {
		return e
	}
	cleanup := true
	defer func() {
		if cleanup {
			lock.Close()
		}
	}()
	id, e := identity.Load(c.StateDir, c.HostID)
	if e != nil {
		return e
	}
	host, e := os.Hostname()
	if e != nil {
		return e
	}
	key, e := config.ReadCredential(c.CredentialFile)
	if e != nil {
		return e
	}
	client, e := exporter.New(c.Endpoint, c.CAFile, key)
	if e != nil {
		return e
	}
	defer client.Close()
	if e = client.SetProxy(c.Proxy); e != nil {
		return e
	}
	binding := c.Target + "|" + c.Endpoint + "|" + c.Environment + "|" + id
	lq, e := spool.Open(filepath.Join(c.StateDir, "logs.db"), binding, c.Storage.LogBytes, c.Storage.ReserveBytes)
	if e != nil {
		return e
	}
	defer func() {
		if cleanup {
			lq.Close()
		}
	}()
	mq, e := spool.Open(filepath.Join(c.StateDir, "metrics.db"), binding, c.Storage.MetricBytes, c.Storage.ReserveBytes)
	if e != nil {
		return e
	}
	defer func() {
		if cleanup {
			mq.Close()
		}
	}()
	state := diagnostics.New()
	var generation atomic.Uint64
	reload := func() error {
		k, e := config.ReadCredential(c.CredentialFile)
		if e != nil {
			state.Set("credential", "reload_failed")
			return e
		}
		client.SetCredential(k)
		generation.Add(1)
		state.Set("credential", "loaded")
		return nil
	}
	closeControl, e := state.Serve(diagnostics.Path(c.StateDir), reload, func() error { return doctor(c, state) })
	if e != nil {
		return e
	}
	defer closeControl()
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	var wg sync.WaitGroup
	errs := make(chan error, 64)
	start := func(fn func() error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if e := fn(); e != nil && !errors.Is(e, context.Canceled) {
				select {
				case errs <- e:
				default:
				}
			}
		}()
	}
	for signal, q := range map[string]*spool.Queue{"logs": lq, "metrics": mq} {
		start(func() error { return deliver(ctx, signal, q, client, state, &generation) })
	}
	sink := func(ctx context.Context, rows []*telemetry.LogItem, cp map[string][]byte) error {
		payloads, e := telemetry.EncodeLogs(rows, telemetry.MaxBatch)
		if e != nil {
			return e
		}
		for {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			e = lq.Enqueue(payloads, cp)
			if e == nil {
				state.Add("logs_collected", uint64(len(rows)))
				for _, row := range rows {
					for _, a := range row.Record.Attributes {
						if a.Value.GetStringValue() == "true" {
							switch a.Key {
							case "ezgh.log.truncated":
								state.Add("logs_truncated", 1)
							case "windows.event.oversized":
								state.Add("logs_oversized", 1)
							case "ezgh.log.invalid_utf8":
								state.Add("logs_invalid_utf8", 1)
							}
						}
					}
				}
				state.Set("logs_storage", "ready")
				return nil
			}
			if !errors.Is(e, spool.ErrFull) {
				return e
			}
			state.Set("logs_storage", "full")
			if !sleep(ctx, time.Second) {
				return ctx.Err()
			}
		}
	}
	for _, channel := range c.Logs.EventLog.Channels {
		start(func() error {
			return eventlog.Run(ctx, channel, id, host, c.Environment, lq.Checkpoint, sink, func(v string) { state.Set("eventlog_"+channel.ID, v) }, func() { state.Add("eventlog_gaps", 1); diagnostics.Local("Event Log source gap: " + channel.ID) })
		})
	}
	if len(c.Logs.Files) > 0 {
		start(func() error {
			s := files.New(c.Logs.Files, c.Logs.MaxOpenFiles, id, host, c.Environment, lq.Checkpoint, sink)
			s.Status = func(v string) { state.Set("files", v) }
			defer s.Close()
			for {
				if e := s.Poll(ctx, time.Now()); e != nil {
					return e
				}
				if !sleep(ctx, 100*time.Millisecond) {
					return nil
				}
			}
		})
	}
	start(func() error { return selfMetrics(ctx, c, id, host, mq, state, 60*time.Second) })
	if c.Metrics.Enabled {
		start(func() error { return collectMetrics(ctx, c, id, host, mq, state) })
	}
	start(func() error {
		for {
			for signal, q := range map[string]*spool.Queue{"logs": lq, "metrics": mq} {
				v, e := q.Stats()
				if e != nil {
					return e
				}
				state.Gauge(signal+"_queue_bytes", float64(v.Bytes))
				state.Gauge(signal+"_queue_batches", float64(v.Batches))
				age := 0.
				if !v.Oldest.IsZero() {
					age = time.Since(v.Oldest).Seconds()
				}
				state.Gauge(signal+"_queue_oldest_seconds", age)
			}
			if !sleep(ctx, time.Second) {
				return nil
			}
		}
	})
	if ready != nil {
		ready()
	}
	diagnostics.Local("Agent started")
	var result error
	select {
	case <-parent.Done():
	case result = <-errs:
	}
	cancel()
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		cleanup = false
		return errors.New("shutdown deadline exceeded; process must exit")
	}
	diagnostics.Local("Agent stopped")
	return result
}
func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
