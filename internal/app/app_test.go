//go:build !windows

package app

import (
	"context"
	"encoding/pem"
	"github.com/ezgamehost/ezgh-obs-agent-windows/internal/config"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFileDeliveryAndStop(t *testing.T) {
	d, err := os.MkdirTemp("/tmp", "winagent-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(d)
	received := make(chan bool, 100)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { received <- true }))
	defer srv.Close()
	ca := filepath.Join(d, "ca.pem")
	os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0600)
	key := filepath.Join(d, "key")
	os.WriteFile(key, []byte("ezgh_ik_"+strings.Repeat("a", 43)), 0600)
	p := filepath.Join(d, "app.log")
	os.WriteFile(p, []byte("hello\n"), 0600)
	var c config.Config
	c.Target = "test"
	c.Endpoint = srv.URL
	c.Environment = "test"
	c.CredentialFile = key
	c.StateDir = filepath.Join(d, "state")
	c.CAFile = ca
	c.Storage.LogBytes = 16 << 20
	c.Storage.MetricBytes = 16 << 20
	c.Logs.MaxOpenFiles = 128
	c.Logs.Files = []config.File{{ID: "app", Paths: []string{p}, Service: "app", Encoding: "utf-8", StartAt: "beginning"}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- Run(ctx, c, func() {}) }()
	select {
	case <-received:
	case e := <-done:
		t.Fatal(e)
	case <-time.After(10 * time.Second):
		t.Fatal("no export")
	}
	cancel()
	select {
	case e := <-done:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown blocked")
	}
}
