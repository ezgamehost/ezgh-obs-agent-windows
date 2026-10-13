package eventlog

import (
	"context"
	"github.com/ezgamehost/ezgh-obs-agent-windows/internal/telemetry"
)

type Position func(string) ([]byte, error)
type Sink func(context.Context, []*telemetry.LogItem, map[string][]byte) error
