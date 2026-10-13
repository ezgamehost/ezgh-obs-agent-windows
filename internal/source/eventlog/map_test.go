package eventlog

import (
	"strings"
	"testing"
	"time"
)

func TestMapLevelsAndFallback(t *testing.T) {
	xml := `<Event><System><Provider Name="Test" Guid="g"/><EventID>42</EventID><Level>2</Level><TimeCreated SystemTime="2026-10-12T01:00:00Z"/><EventRecordID>23</EventRecordID><Channel>System</Channel></System></Event>`
	r, e := Map(xml, "", time.Now())
	if e != nil {
		t.Fatal(e)
	}
	if int(r.SeverityNumber) != 17 || !strings.Contains(r.Body.GetStringValue(), "<Event>") {
		t.Fatalf("%+v", r)
	}
	if len(r.Attributes) == 0 {
		t.Fatal("missing attributes")
	}
}
func TestBookmarkQueryIdentity(t *testing.T) {
	a := SourceKey("sys", 0)
	if a == SourceKey("sys", 1) {
		t.Fatal("generation collision")
	}
	if ValidateCheckpoint(Checkpoint{Initialized: true, Query: "old"}, "new") == nil {
		t.Fatal("silently reused changed query")
	}
}
