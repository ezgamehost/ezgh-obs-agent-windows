package app

import (
	"github.com/ezgamehost/ezgh-obs-agent-windows/internal/diagnostics"
	dto "github.com/prometheus/client_model/go"
	"google.golang.org/protobuf/proto"
	"testing"
)

func TestCollectorHealthRequiresEveryConfiguredCollector(t *testing.T) {
	s := diagnostics.New()
	success := func(name string, value float64) *dto.Metric {
		return &dto.Metric{Label: []*dto.LabelPair{{Name: proto.String("collector"), Value: proto.String(name)}}, Gauge: &dto.Gauge{Value: proto.Float64(value)}}
	}
	family := &dto.MetricFamily{Name: proto.String("windows_exporter_collector_success"), Metric: []*dto.Metric{success("cpu", 1), success("memory", 0)}}
	if collectorHealth(s, []*dto.MetricFamily{family}, []string{"cpu", "memory"}) {
		t.Fatal("failed collector reported healthy")
	}
	family.Metric[1] = success("memory", 1)
	if !collectorHealth(s, []*dto.MetricFamily{family}, []string{"cpu", "memory"}) {
		t.Fatal("successful collectors reported unhealthy")
	}
	if collectorHealth(s, []*dto.MetricFamily{family}, []string{"cpu", "memory", "net"}) {
		t.Fatal("missing collector reported healthy")
	}
}
