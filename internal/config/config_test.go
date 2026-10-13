package config

import (
	"strings"
	"testing"
)

const valid = `version: 1
target: prod
environment: production
credential_file: 'C:\ProgramData\EZGH\ObsAgent\secrets\ingest-key'
state_dir: 'C:\ProgramData\EZGH\ObsAgent\state'
`

func TestDefaults(t *testing.T) {
	c, e := Parse(strings.NewReader(valid))
	if e != nil {
		t.Fatal(e)
	}
	if len(c.Logs.EventLog.Channels) != 2 || c.Metrics.Interval.Seconds() != 30 || c.Storage.LogBytes != 256<<20 {
		t.Fatalf("bad defaults: %+v", c)
	}
}
func TestRejectsUnsafeConfiguration(t *testing.T) {
	for _, extra := range []string{"secret: DO_NOT_ECHO", "endpoint: http://example.com", "proxy: http://user:DO_NOT_ECHO@example.com", "metrics:\n  collectors: [process]", "logs:\n  eventlog:\n    channels:\n      - {id: a, name: System}\n      - {id: b, name: system}", "state_dir: '\\\\server\\state'"} {
		_, e := Parse(strings.NewReader(valid + extra + "\n"))
		if e == nil {
			t.Fatalf("accepted %s", extra)
		}
		if strings.Contains(e.Error(), "DO_NOT_ECHO") {
			t.Fatal("secret echoed")
		}
	}
}
