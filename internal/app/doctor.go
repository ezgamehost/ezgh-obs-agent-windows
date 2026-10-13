package app

import (
	"errors"
	"github.com/ezgamehost/ezgh-obs-agent-windows/internal/config"
	"github.com/ezgamehost/ezgh-obs-agent-windows/internal/diagnostics"
	"github.com/ezgamehost/ezgh-obs-agent-windows/internal/source/eventlog"
	"os"
)

func doctor(c config.Config, s *diagnostics.State) error {
	if _, e := config.ReadCredential(c.CredentialFile); e != nil {
		return e
	}
	for _, v := range c.Logs.EventLog.Channels {
		if e := eventlog.Probe(v); e != nil {
			return errors.New("event channel access failed")
		}
	}
	f, e := os.CreateTemp(c.StateDir, ".doctor-")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if e = f.Sync(); e != nil {
		f.Close()
		return e
	}
	f.Close()
	for k, v := range s.Snapshot().States {
		if k == "files" || k == "metrics_collection" {
			if v != "ready" {
				return errors.New("source is degraded")
			}
		}
	}
	return nil
}
