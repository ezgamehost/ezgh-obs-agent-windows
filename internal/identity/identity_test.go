package identity

import (
	"testing"
)

func TestIdentityPersistsAndRejectsChange(t *testing.T) {
	d := t.TempDir()
	a, e := Load(d, "")
	if e != nil {
		t.Fatal(e)
	}
	b, e := Load(d, "")
	if e != nil || a != b {
		t.Fatal("identity changed")
	}
	if _, e = Load(d, "different"); e == nil {
		t.Fatal("accepted identity change")
	}
}
