//go:build windows

package eventlog

import (
	"errors"
	"golang.org/x/sys/windows"
	"syscall"
	"unsafe"
)

var wevt = windows.NewLazySystemDLL("wevtapi.dll")
var evtQuery = wevt.NewProc("EvtQuery")
var evtNext = wevt.NewProc("EvtNext")
var evtSubscribe = wevt.NewProc("EvtSubscribe")
var evtRender = wevt.NewProc("EvtRender")
var evtClose = wevt.NewProc("EvtClose")
var evtCreateBookmark = wevt.NewProc("EvtCreateBookmark")
var evtUpdateBookmark = wevt.NewProc("EvtUpdateBookmark")
var evtOpenPublisher = wevt.NewProc("EvtOpenPublisherMetadata")
var evtFormatMessage = wevt.NewProc("EvtFormatMessage")
var errOversize = errors.New("event render size exceeds limit")

func closeEvent(h uintptr) {
	if h != 0 {
		evtClose.Call(h)
	}
}
func wide(s string) *uint16 {
	if s == "" {
		return nil
	}
	p, _ := windows.UTF16PtrFromString(s)
	return p
}
func winerr(e error) error {
	if e == nil || e == syscall.Errno(0) {
		return errors.New("Windows Event Log operation failed")
	}
	return e
}
func render(h uintptr, flag uintptr) (string, error) {
	var used, count uint32
	r, _, e := evtRender.Call(0, h, flag, 0, 0, uintptr(unsafe.Pointer(&used)), uintptr(unsafe.Pointer(&count)))
	if r == 0 && e != windows.ERROR_INSUFFICIENT_BUFFER {
		return "", winerr(e)
	}
	if used > 1<<20 {
		return "", errOversize
	}
	b := make([]uint16, (used+1)/2)
	if len(b) == 0 {
		return "", nil
	}
	r, _, e = evtRender.Call(0, h, flag, uintptr(len(b)*2), uintptr(unsafe.Pointer(&b[0])), uintptr(unsafe.Pointer(&used)), uintptr(unsafe.Pointer(&count)))
	if r == 0 {
		return "", winerr(e)
	}
	return windows.UTF16ToString(b), nil
}
func bookmark(event uintptr) (string, error) {
	h, _, e := evtCreateBookmark.Call(0)
	if h == 0 {
		return "", winerr(e)
	}
	defer closeEvent(h)
	r, _, e := evtUpdateBookmark.Call(h, event)
	if r == 0 {
		return "", winerr(e)
	}
	return render(h, 2)
}
func next(h uintptr) (uintptr, error) {
	var event uintptr
	var count uint32
	r, _, e := evtNext.Call(h, 1, uintptr(unsafe.Pointer(&event)), 0, 0, uintptr(unsafe.Pointer(&count)))
	if r == 0 {
		return 0, winerr(e)
	}
	return event, nil
}
func initialTail(channel, query string) (string, error) {
	h, _, e := evtQuery.Call(0, uintptr(unsafe.Pointer(wide(channel))), uintptr(unsafe.Pointer(wide(query))), 1|512)
	if h == 0 {
		return "", winerr(e)
	}
	defer closeEvent(h)
	event, e := next(h)
	if errors.Is(e, windows.ERROR_NO_MORE_ITEMS) {
		return "", nil
	}
	if e != nil {
		return "", e
	}
	defer closeEvent(event)
	return bookmark(event)
}
func subscribe(channel, query, bm string, oldest bool, signal windows.Handle) (uintptr, error) {
	var b uintptr
	var e error
	flags := uintptr(1)
	if oldest {
		flags = 2
	}
	if bm != "" {
		b, _, e = evtCreateBookmark.Call(uintptr(unsafe.Pointer(wide(bm))))
		if b == 0 {
			return 0, winerr(e)
		}
		defer closeEvent(b)
		flags = 3 | 0x10000
	}
	h, _, e := evtSubscribe.Call(0, uintptr(signal), uintptr(unsafe.Pointer(wide(channel))), uintptr(unsafe.Pointer(wide(query))), b, 0, 0, flags)
	if h == 0 {
		return 0, winerr(e)
	}
	return h, nil
}
func format(event uintptr, provider string) string {
	if provider == "" {
		return ""
	}
	h, _, _ := evtOpenPublisher.Call(0, uintptr(unsafe.Pointer(wide(provider))), 0, 0, 0)
	if h == 0 {
		return ""
	}
	defer closeEvent(h)
	var n uint32
	r, _, e := evtFormatMessage.Call(h, event, 0, 0, 0, 1, 0, 0, uintptr(unsafe.Pointer(&n)))
	if r == 0 && e != windows.ERROR_INSUFFICIENT_BUFFER {
		return ""
	}
	if n == 0 || n > 1<<19 {
		return ""
	}
	b := make([]uint16, n)
	r, _, _ = evtFormatMessage.Call(h, event, 0, 0, 0, 1, uintptr(n), uintptr(unsafe.Pointer(&b[0])), uintptr(unsafe.Pointer(&n)))
	if r == 0 {
		return ""
	}
	return windows.UTF16ToString(b)
}
