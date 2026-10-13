//go:build windows

package windowsmetrics

import (
	"context"
	"errors"
	"github.com/alecthomas/kingpin/v2"
	upstream "github.com/prometheus-community/windows_exporter/pkg/collector"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"io"
	"log/slog"
	"sync"
	"time"
)

type Collector struct {
	mu       sync.Mutex
	registry *prometheus.Registry
	raw      upstream.Map
	owners   []*upstream.Collection
}

func New(names []string) (*Collector, error) {
	c := &Collector{registry: prometheus.NewRegistry(), raw: upstream.Map{}}
	flags := kingpin.New("embedded-windows-collectors", "")
	allowed := map[string]bool{"cpu": true, "memory": true, "logical_disk": true, "net": true, "os": true, "system": true}
	for _, name := range names {
		if !allowed[name] {
			return nil, errors.New("collector is not in allowlist")
		}
		c.raw[name] = upstream.BuildersWithFlags[name](flags)
	}
	if _, e := flags.Parse(nil); e != nil {
		return nil, e
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	for name, raw := range c.raw {
		owner := upstream.New(upstream.Map{name: raw})
		if e := owner.Build(context.Background(), logger); e != nil {
			return nil, errors.New("collector initialization failed")
		}
		c.owners = append(c.owners, owner)
	}
	c.registry.MustRegister(c)
	return c, nil
}
func (c *Collector) Describe(chan<- *prometheus.Desc) {}
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	for name, raw := range c.raw {
		start := time.Now()
		ok := func() (ok float64) {
			defer func() {
				if recover() != nil {
					ok = 0
				}
			}()
			if raw.Collect(ch) == nil {
				return 1
			}
			return 0
		}()
		ch <- prometheus.MustNewConstMetric(prometheus.NewDesc("windows_exporter_collector_success", "Collector success", []string{"collector"}, nil), prometheus.GaugeValue, ok, name)
		ch <- prometheus.MustNewConstMetric(prometheus.NewDesc("windows_exporter_collector_duration_seconds", "Collector duration", []string{"collector"}, nil), prometheus.GaugeValue, time.Since(start).Seconds(), name)
	}
}
func (c *Collector) Gather() ([]*dto.MetricFamily, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.registry.Gather()
}
func (c *Collector) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	var e error
	for _, v := range c.owners {
		e = errors.Join(e, v.Close())
	}
	return e
}
