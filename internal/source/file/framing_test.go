package file

import (
	"encoding/binary"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestUTF16SplitAndTruncation(t *testing.T) {
	var b []byte
	for _, u := range utf16.Encode([]rune("hello 😀\r\n")) {
		b = binary.LittleEndian.AppendUint16(b, u)
	}
	f := NewFramer("utf-16le")
	var lines []Line
	for _, v := range b {
		lines = append(lines, f.Feed([]byte{v})...)
	}
	if len(lines) != 1 || lines[0].Body != "hello 😀" || lines[0].Bytes != int64(len(b)) {
		t.Fatalf("%+v", lines)
	}
	g := NewFramer("utf-8")
	got := g.Feed([]byte(strings.Repeat("x", 70000) + "\n"))
	if len(got) != 1 || len(got[0].Body) > 65536 || got[0].Bytes != 70001 || !got[0].Truncated {
		t.Fatal("unbounded framing")
	}
}
