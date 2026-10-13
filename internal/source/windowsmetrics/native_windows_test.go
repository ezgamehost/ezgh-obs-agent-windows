//go:build windows

package windowsmetrics

import (
	"testing"
)

func TestNativeDefaultCollectors(t *testing.T) {
	c, e := New([]string{"cpu", "memory", "logical_disk", "net", "os", "system"})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	f, e := c.Gather()
	if e != nil {
		t.Fatal(e)
	}
	if len(f) == 0 {
		t.Fatal("no native metrics")
	}
	for _, m := range f {
		if m.GetName() == "windows_exporter_collector_success" {
			for _, v := range m.Metric {
				if v.GetGauge().GetValue() != 1 {
					t.Fatalf("collector failed: %v", v.Label)
				}
			}
		}
	}
}
