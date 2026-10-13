// Package file reads explicitly configured local text files with durable byte checkpoints.
package file

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/ezgamehost/ezgh-obs-agent-windows/internal/config"
	"github.com/ezgamehost/ezgh-obs-agent-windows/internal/source/eventlog"
	"github.com/ezgamehost/ezgh-obs-agent-windows/internal/telemetry"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type checkpoint struct {
	Offset      int64
	Encoding    string
	Prefix      string
	PrefixBytes int
}
type tracked struct {
	f         *os.File
	key, path string
	cfg       config.File
	cp        checkpoint
	read      int64
	framer    *Framer
	last      time.Time
}
type Source struct {
	configs       []config.File
	max           int
	id, host, env string
	position      eventlog.Position
	sink          eventlog.Sink
	files         map[string]*tracked
	Status        func(string)
}

func New(c []config.File, max int, id, host, env string, p eventlog.Position, s eventlog.Sink) *Source {
	return &Source{c, max, id, host, env, p, s, map[string]*tracked{}, func(string) {}}
}
func (s *Source) Close() {
	for _, f := range s.files {
		f.f.Close()
	}
}
func prefix(f *os.File, n int) string {
	b := make([]byte, n)
	got, _ := f.ReadAt(b, 0)
	h := sha256.Sum256(b[:got])
	return hex.EncodeToString(h[:])
}
func (s *Source) commit(ctx context.Context, t *tracked, line *Line) error {
	next := t.cp
	var rows []*telemetry.LogItem
	if line != nil {
		next.Offset += line.Bytes
		attrs := map[string]string{"log.file.path": t.path}
		if line.Truncated {
			attrs["ezgh.log.truncated"] = "true"
		}
		rows = []*telemetry.LogItem{{Resource: telemetry.Resource(s.id, s.host, s.env, t.cfg.Service), Record: telemetry.Log(line.Body, time.Now(), 0, attrs)}}
	}
	b, _ := json.Marshal(next)
	if e := s.sink(ctx, rows, map[string][]byte{t.key: b}); e != nil {
		return e
	}
	t.cp = next
	return nil
}
func (s *Source) Poll(ctx context.Context, now time.Time) error {
	matched := map[string]bool{}
	for _, cfg := range s.configs {
		for _, pattern := range cfg.Paths {
			paths, e := filepath.Glob(pattern)
			if e != nil {
				return e
			}
			for _, p := range paths {
				excluded := false
				for _, x := range cfg.Exclude {
					ok, _ := filepath.Match(strings.ToLower(x), strings.ToLower(p))
					excluded = excluded || ok
				}
				if excluded {
					continue
				}
				f, id, e := open(p)
				if e != nil {
					s.Status("file_unavailable")
					continue
				}
				key := "file:" + cfg.ID + ":" + id
				matched[key] = true
				if s.files[key] != nil {
					f.Close()
					continue
				}
				if len(s.files) >= s.max {
					f.Close()
					s.Status("file_limit")
					continue
				}
				st, e := f.Stat()
				if e != nil {
					f.Close()
					continue
				}
				t := &tracked{f: f, key: key, path: p, cfg: cfg, last: now}
				b, e := s.position(key)
				if e != nil {
					f.Close()
					return e
				}
				if len(b) > 0 {
					if json.Unmarshal(b, &t.cp) != nil {
						f.Close()
						return errors.New("invalid file checkpoint")
					}
					if t.cp.Encoding != cfg.Encoding {
						f.Close()
						return errors.New("file encoding changed: use a new source ID")
					}
				} else {
					t.cp.Encoding = cfg.Encoding
					if cfg.StartAt == "end" {
						t.cp.Offset = st.Size()
					}
					t.cp.PrefixBytes = int(min(st.Size(), 64))
					t.cp.Prefix = prefix(f, t.cp.PrefixBytes)
					if e = s.commit(ctx, t, nil); e != nil {
						f.Close()
						return e
					}
				}
				t.read = t.cp.Offset
				t.framer = NewFramer(cfg.Encoding)
				if t.cp.Offset > 0 {
					t.framer.initialized = true
					if cfg.Encoding == "auto" {
						var bom [2]byte
						f.ReadAt(bom[:], 0)
						t.framer.encoding = "utf-8"
						if bom == [2]byte{255, 254} {
							t.framer.encoding = "utf-16le"
						}
					}
				}
				s.files[key] = t
			}
		}
	}
	for key, t := range s.files {
		st, e := t.f.Stat()
		if e != nil {
			s.Status("file_unavailable")
			continue
		}
		if st.Size() < t.read || (t.cp.PrefixBytes > 0 && prefix(t.f, t.cp.PrefixBytes) != t.cp.Prefix) {
			t.cp = checkpoint{Encoding: t.cfg.Encoding, PrefixBytes: int(min(st.Size(), 64))}
			t.cp.Prefix = prefix(t.f, t.cp.PrefixBytes)
			t.read = 0
			t.framer = NewFramer(t.cfg.Encoding)
			if e = s.commit(ctx, t, nil); e != nil {
				return e
			}
		}
		b := make([]byte, 64<<10)
		n, e := t.f.ReadAt(b, t.read)
		if e != nil && e != io.EOF {
			s.Status("file_read_failed")
			continue
		}
		if n > 0 {
			t.read += int64(n)
			t.last = now
			for _, l := range t.framer.Feed(b[:n]) {
				if e = s.commit(ctx, t, &l); e != nil {
					return e
				}
			}
		} else if now.Sub(t.last) >= 5*time.Second {
			if l := t.framer.Flush(); l != nil {
				if e = s.commit(ctx, t, l); e != nil {
					return e
				}
			}
			if !matched[key] {
				t.f.Close()
				delete(s.files, key)
			}
		}
	}
	return nil
}
