//go:build !windows

package eventlog

import (
	"context"
	"errors"
	"github.com/ezgamehost/ezgh-obs-agent-windows/internal/config"
)

func Run(context.Context, config.Channel, string, string, string, Position, Sink, func(string), func()) error {
	return errors.New("Event Log requires Windows")
}
