//go:build windows

package service

import (
	"context"
	"errors"
	"github.com/ezgamehost/ezgh-obs-agent-windows/internal/diagnostics"
	"golang.org/x/sys/windows/svc"
	"time"
)

const Name = "ezgh-obs-agent-windows"

type handler struct {
	run func(context.Context, func()) error
}

func (h handler) Execute(_ []string, requests <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	status <- svc.Status{State: svc.StartPending, WaitHint: 30000}
	ready := make(chan struct{})
	done := make(chan error, 1)
	go func() { done <- h.run(ctx, func() { close(ready) }) }()
	current := svc.Status{State: svc.StartPending, WaitHint: 30000}
	for {
		select {
		case <-ready:
			ready = nil
			current = svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
			status <- current
		case r := <-requests:
			switch r.Cmd {
			case svc.Interrogate:
				status <- current
			case svc.Stop, svc.Shutdown:
				status <- svc.Status{State: svc.StopPending, WaitHint: 25000}
				cancel()
				select {
				case e := <-done:
					if e != nil {
						return true, 1
					}
					return false, 0
				case <-time.After(23 * time.Second):
					return true, 2
				}
			}
		case e := <-done:
			if e != nil {
				diagnostics.Local("Agent stopped with a local configuration, storage or collection error")
				return true, 1
			}
			return false, 0
		}
	}
}
func Host(run func(context.Context, func()) error) (bool, error) {
	is, e := svc.IsWindowsService()
	if e != nil {
		return false, e
	}
	if !is {
		return false, nil
	}
	e = svc.Run(Name, handler{run})
	if e != nil {
		return true, errors.New("SCM host failed")
	}
	return true, nil
}
