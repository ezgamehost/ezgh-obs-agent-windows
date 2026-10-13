package file

import (
	"context"
	"github.com/ezgamehost/ezgh-obs-agent-windows/internal/config"
	"github.com/ezgamehost/ezgh-obs-agent-windows/internal/telemetry"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTailRestartAndRotation(t *testing.T) {
	p := filepath.Join(t.TempDir(), "app.log")
	os.WriteFile(p, []byte("old\n"), 0600)
	cp := map[string][]byte{}
	var bodies []string
	sink := func(_ context.Context, rows []*telemetry.LogItem, m map[string][]byte) error {
		for k, v := range m {
			cp[k] = v
		}
		for _, r := range rows {
			bodies = append(bodies, r.Record.Body.GetStringValue())
		}
		return nil
	}
	cfg := []config.File{{ID: "app", Paths: []string{p}, Service: "app", StartAt: "end", Encoding: "auto"}}
	pos := func(k string) ([]byte, error) { return cp[k], nil }
	s := New(cfg, 128, "id", "host", "prod", pos, sink)
	if e := s.Poll(context.Background(), time.Now()); e != nil {
		t.Fatal(e)
	}
	s.Close()
	f, _ := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0600)
	f.WriteString("new\n")
	f.Close()
	s = New(cfg, 128, "id", "host", "prod", pos, sink)
	defer s.Close()
	if e := s.Poll(context.Background(), time.Now()); e != nil {
		t.Fatal(e)
	}
	if len(bodies) != 1 || bodies[0] != "new" {
		t.Fatal(bodies)
	}
	os.Rename(p, p+".1")
	os.WriteFile(p, []byte("skip\n"), 0600)
	if e := s.Poll(context.Background(), time.Now()); e != nil {
		t.Fatal(e)
	}
	f, _ = os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0600)
	f.WriteString("after\n")
	f.Close()
	s.Poll(context.Background(), time.Now())
	if len(bodies) != 2 || bodies[1] != "after" {
		t.Fatal(bodies)
	}
}
