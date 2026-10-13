//go:build !windows

package windowsmetrics

import (
	"errors"
	dto "github.com/prometheus/client_model/go"
)

type Collector struct{}

func New([]string) (*Collector, error) { return nil, errors.New("host collectors require Windows") }
func (*Collector) Gather() ([]*dto.MetricFamily, error) {
	return nil, errors.New("host collectors require Windows")
}
func (*Collector) Close() error { return nil }
