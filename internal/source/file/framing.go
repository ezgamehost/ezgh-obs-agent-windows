package file

import (
	"encoding/binary"
	"strings"
	"unicode/utf16"
)

type Line struct {
	Body      string
	Bytes     int64
	Truncated bool
}
type Framer struct {
	encoding    string
	pending     []byte
	raw         []byte
	consumed    int64
	truncated   bool
	initialized bool
}

func NewFramer(encoding string) *Framer { return &Framer{encoding: encoding} }
func (f *Framer) Feed(b []byte) []Line {
	f.pending = append(f.pending, b...)
	var out []Line
	if !f.initialized {
		if len(f.pending) < 3 {
			return nil
		}
		f.initialized = true
		if f.encoding == "auto" {
			f.encoding = "utf-8"
			if f.pending[0] == 255 && f.pending[1] == 254 {
				f.encoding = "utf-16le"
			}
		}
		if f.pending[0] == 255 && f.pending[1] == 254 && f.encoding == "utf-16le" {
			f.pending = f.pending[2:]
			f.consumed += 2
		} else if len(f.pending) >= 3 && string(f.pending[:3]) == "\xef\xbb\xbf" {
			f.pending = f.pending[3:]
			f.consumed += 3
		}
	}
	width := 1
	if f.encoding == "utf-16le" {
		width = 2
	}
	for len(f.pending) >= width {
		unit := f.pending[:width]
		isLF := unit[0] == 10 && (width == 1 || unit[1] == 0)
		f.consumed += int64(width)
		if isLF {
			out = append(out, f.line())
		} else if len(f.raw)+width <= 131072 {
			f.raw = append(f.raw, unit...)
		} else {
			f.truncated = true
		}
		f.pending = f.pending[width:]
	}
	if len(f.pending) == 0 {
		f.pending = nil
	}
	return out
}
func (f *Framer) line() Line {
	var s string
	if f.encoding == "utf-16le" {
		u := make([]uint16, len(f.raw)/2)
		for i := range u {
			u[i] = binary.LittleEndian.Uint16(f.raw[i*2:])
		}
		s = string(utf16.Decode(u))
		if len(f.raw)%2 != 0 {
			s += "�"
		}
	} else {
		s = strings.ToValidUTF8(string(f.raw), "�")
	}
	s = strings.TrimSuffix(s, "\r")
	s, tr := bound(s)
	v := Line{s, f.consumed, f.truncated || tr}
	f.raw = nil
	f.consumed = 0
	f.truncated = false
	return v
}
func bound(s string) (string, bool) {
	if len(s) <= 65536 {
		return s, false
	}
	s = s[:65536]
	for len(s) > 0 && (s[len(s)-1]&0xc0) == 0x80 {
		s = s[:len(s)-1]
	}
	return strings.ToValidUTF8(s, ""), true
}
func (f *Framer) Flush() []Line {
	var out []Line
	if !f.initialized {
		f.initialized = true
		p := f.pending
		f.pending = nil
		if f.encoding == "auto" {
			f.encoding = "utf-8"
			if len(p) >= 2 && p[0] == 255 && p[1] == 254 {
				f.encoding = "utf-16le"
			}
		}
		if len(p) >= 2 && p[0] == 255 && p[1] == 254 && f.encoding == "utf-16le" {
			p = p[2:]
			f.consumed += 2
		}
		out = append(out, f.Feed(p)...)
	}
	if len(f.raw)+len(f.pending) == 0 && f.consumed == 0 {
		return out
	}
	f.raw = append(f.raw, f.pending...)
	f.consumed += int64(len(f.pending))
	f.pending = nil
	return append(out, f.line())
}
