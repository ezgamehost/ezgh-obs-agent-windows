//go:build windows

package eventlog

import (
	"context"
	"github.com/ezgamehost/ezgh-obs-agent-windows/internal/config"
	"github.com/ezgamehost/ezgh-obs-agent-windows/internal/telemetry"
	"golang.org/x/sys/windows/svc/eventlog"
	"strings"
	"testing"
	"time"
)

func TestNativeSubscriptionBookmark(t *testing.T) {
	const source = "EZGHAgentNativeTest"
	if e := eventlog.InstallAsEventCreate(source, eventlog.Info); e != nil {
		t.Fatal(e)
	}
	defer eventlog.Remove(source)
	log, e := eventlog.Open(source)
	if e != nil {
		t.Fatal(e)
	}
	defer log.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	ready := make(chan struct{}, 1)
	rows := make(chan string, 10)
	degraded := make(chan string, 16)
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, config.Channel{ID: "test", Name: "Application", Query: "*[System[Provider[@Name='" + source + "']]]", StartAt: "end", Service: "test"}, "id", "host", "test", func(string) ([]byte, error) { return nil, nil }, func(_ context.Context, r []*telemetry.LogItem, m map[string][]byte) error {
			for _, v := range r {
				rows <- v.Record.Body.GetStringValue()
			}
			for _, b := range m {
				if len(b) == 0 {
					t.Error("missing checkpoint")
				}
			}
			return nil
		}, func(s string) {
			if s != "ready" {
				select {
				case degraded <- s:
				default:
				}
			}
			if s == "ready" {
				select {
				case ready <- struct{}{}:
				default:
				}
			}
		}, func() {})
	}()
	select {
	case <-ready:
	case e := <-done:
		t.Fatal(e)
	case <-ctx.Done():
		t.Fatal("subscription not ready")
	}
	for i := 0; i < 2; i++ {
		if e = log.Info(1, "EZGH native subscription fixture"); e != nil {
			t.Fatal(e)
		}
		select {
		case body := <-rows:
			if !strings.Contains(body, "EZGH native subscription fixture") {
				t.Fatal("missing message")
			}
		case <-ctx.Done():
			t.Fatal("event not received")
		}
		select {
		case state := <-degraded:
			t.Fatalf("subscription degraded: %s", state)
		case <-time.After(2 * time.Second):
		}
	}
	cancel()
	select {
	case e = <-done:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("native reader leaked")
	}
}
