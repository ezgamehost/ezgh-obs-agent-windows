//go:build windows

package eventlog

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"github.com/ezgamehost/ezgh-obs-agent-windows/internal/config"
	"github.com/ezgamehost/ezgh-obs-agent-windows/internal/telemetry"
	"golang.org/x/sys/windows"
	"strconv"
	"time"
	"unsafe"
)

func Run(ctx context.Context, c config.Channel, id, host, env string, position Position, sink Sink, status func(string), gap func()) error {
	key := SourceKey(c.ID, c.Generation)
	query := c.Query
	if query == "" {
		query = "*"
	}
	binding := c.Name + "|" + query
	data, e := position(key)
	if e != nil {
		return e
	}
	var cp Checkpoint
	if len(data) > 0 {
		if json.Unmarshal(data, &cp) != nil {
			return errors.New("invalid event checkpoint")
		}
	}
	if e = ValidateCheckpoint(cp, binding); e != nil {
		return e
	}
	save := func(rows []*telemetry.LogItem, next Checkpoint) error {
		b, _ := json.Marshal(next)
		if e := sink(ctx, rows, map[string][]byte{key: b}); e != nil {
			return e
		}
		cp = next
		return nil
	}
	signal, e := windows.CreateEvent(nil, 0, 0, nil)
	if e != nil {
		return e
	}
	defer windows.CloseHandle(signal)
	for ctx.Err() == nil {
		if !cp.Initialized {
			cp = Checkpoint{Initialized: true, Query: binding}
			if c.StartAt == "end" {
				cp.Bookmark, e = initialTail(c.Name, query)
				if e != nil {
					status("unavailable")
					if !pause(ctx) {
						break
					}
					cp.Initialized = false
					continue
				}
			}
			if e = save(nil, cp); e != nil {
				return e
			}
		}
		h, e := subscribe(c.Name, query, cp.Bookmark, true, signal)
		if e != nil {
			if errors.Is(e, windows.Errno(15011)) || errors.Is(e, windows.Errno(15012)) {
				gap()
				cp.Bookmark = ""
				if e = save(nil, cp); e != nil {
					return e
				}
			}
			status("unavailable")
			if !pause(ctx) {
				break
			}
			continue
		}
		status("ready")
		e = func() error {
			defer closeEvent(h)
			for ctx.Err() == nil {
				event, e := next(h)
				if errors.Is(e, windows.ERROR_NO_MORE_ITEMS) || errors.Is(e, windows.ERROR_TIMEOUT) {
					windows.WaitForSingleObject(signal, 250)
					continue
				}
				if e != nil {
					return e
				}
				e = func() error {
					defer closeEvent(event)
					bm, e := bookmark(event)
					if e != nil {
						return e
					}
					nextCP := Checkpoint{Initialized: true, Bookmark: bm, Query: binding}
					raw, e := render(event, 1)
					var row *telemetry.LogItem
					if errors.Is(e, errOversize) {
						row = &telemetry.LogItem{Resource: telemetry.Resource(id, host, env, c.Service), Record: telemetry.Log("Windows event exceeded render limit", time.Now(), 0, map[string]string{"windows.event.channel": c.Name, "windows.event.oversized": "true"})}
					} else if e != nil {
						return e
					} else {
						var ev Event
						if xml.Unmarshal([]byte(raw), &ev) != nil {
							return errors.New("invalid event XML")
						}
						if ev.System.Provider.Name == "ezgh-obs-agent-windows" {
							return save(nil, nextCP)
						}
						locale := messageLocale()
						message := format(event, ev.System.Provider.Name, locale)
						record, e := Map(raw, message, time.Now())
						if e != nil {
							return e
						}
						record.Attributes = append(record.Attributes, telemetry.Attribute("windows.event.locale", strconv.FormatUint(uint64(locale), 16)))
						row = &telemetry.LogItem{Resource: telemetry.Resource(id, host, env, c.Service), Record: record}
					}
					return save([]*telemetry.LogItem{row}, nextCP)
				}()
				if e != nil {
					return e
				}
			}
			return ctx.Err()
		}()
		if ctx.Err() != nil {
			return nil
		}
		if errors.Is(e, windows.Errno(15011)) || errors.Is(e, windows.Errno(15012)) {
			gap()
			cp.Bookmark = ""
			if e = save(nil, cp); e != nil {
				return e
			}
		}
		status("retrying")
		if !pause(ctx) {
			break
		}
	}
	return nil
}
func pause(ctx context.Context) bool {
	t := time.NewTimer(time.Second)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
func Probe(c config.Channel) error {
	query := c.Query
	if query == "" {
		query = "*"
	}
	h, _, e := evtQuery.Call(0, uintptr(unsafe.Pointer(wide(c.Name))), uintptr(unsafe.Pointer(wide(query))), 1)
	if h == 0 {
		return winerr(e)
	}
	closeEvent(h)
	return nil
}
