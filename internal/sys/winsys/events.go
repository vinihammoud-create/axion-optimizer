//go:build windows

package winsys

import (
	"encoding/xml"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"axion/internal/sys"
)

var (
	modWevtapi    = windows.NewLazySystemDLL("wevtapi.dll")
	procEvtQuery  = modWevtapi.NewProc("EvtQuery")
	procEvtNext   = modWevtapi.NewProc("EvtNext")
	procEvtRender = modWevtapi.NewProc("EvtRender")
	procEvtClose  = modWevtapi.NewProc("EvtClose")
)

const (
	evtQueryChannelPath      = 0x1
	evtQueryReverseDirection = 0x200
	evtRenderEventXML        = 1
	errNoMoreItems           = 259
	errInsufficientBuffer    = 122
)

// EventLog implements sys.EventLog with the Windows Event Log API.
type EventLog struct{}

type evtXML struct {
	System struct {
		Provider struct {
			Name string `xml:"Name,attr"`
		} `xml:"Provider"`
		EventID     string `xml:"EventID"`
		Level       string `xml:"Level"`
		TimeCreated struct {
			SystemTime string `xml:"SystemTime,attr"`
		} `xml:"TimeCreated"`
		Channel string `xml:"Channel"`
	} `xml:"System"`
	EventData struct {
		Data []struct {
			Name  string `xml:"Name,attr"`
			Value string `xml:",chardata"`
		} `xml:"Data"`
	} `xml:"EventData"`
}

// Query implements sys.EventLog.
func (EventLog) Query(channel, xpath string, max int) ([]sys.Event, error) {
	if err := procEvtQuery.Find(); err != nil {
		return nil, err
	}
	ch, _ := windows.UTF16PtrFromString(channel)
	q, _ := windows.UTF16PtrFromString(xpath)
	h, _, e := procEvtQuery.Call(0, uintptr(unsafe.Pointer(ch)), uintptr(unsafe.Pointer(q)), evtQueryChannelPath|evtQueryReverseDirection)
	if h == 0 {
		if errno, ok := e.(syscall.Errno); ok && (errno == 15007 || errno == 2) { // channel not found
			return nil, nil
		}
		return nil, e
	}
	defer procEvtClose.Call(h)
	if max <= 0 {
		max = 50
	}
	var out []sys.Event
	handles := make([]uintptr, 16)
	buf := make([]uint16, 16*1024)
	for len(out) < max {
		var returned uint32
		r, _, e := procEvtNext.Call(h, uintptr(len(handles)), uintptr(unsafe.Pointer(&handles[0])), 2000, 0, uintptr(unsafe.Pointer(&returned)))
		if r == 0 {
			if errno, ok := e.(syscall.Errno); ok && errno == errNoMoreItems {
				break
			}
			if returned == 0 {
				break
			}
		}
		for i := uint32(0); i < returned; i++ {
			ev := handles[i]
			if len(out) < max {
				if x := renderXML(ev, &buf); x != "" {
					if parsed, ok := parseEventXML(x); ok {
						if parsed.Channel == "" {
							parsed.Channel = channel
						}
						out = append(out, parsed)
					}
				}
			}
			procEvtClose.Call(ev)
		}
		if returned == 0 {
			break
		}
	}
	return out, nil
}

func renderXML(ev uintptr, buf *[]uint16) string {
	var used, props uint32
	for tries := 0; tries < 3; tries++ {
		r, _, e := procEvtRender.Call(0, ev, evtRenderEventXML, uintptr(len(*buf)*2), uintptr(unsafe.Pointer(&(*buf)[0])), uintptr(unsafe.Pointer(&used)), uintptr(unsafe.Pointer(&props)))
		if r != 0 {
			return windows.UTF16ToString((*buf)[:used/2])
		}
		if errno, ok := e.(syscall.Errno); ok && errno == errInsufficientBuffer && used > 0 {
			*buf = make([]uint16, used/2+64)
			continue
		}
		return ""
	}
	return ""
}

func parseEventXML(x string) (sys.Event, bool) {
	var e evtXML
	if err := xml.Unmarshal([]byte(x), &e); err != nil {
		return sys.Event{}, false
	}
	id, _ := strconv.Atoi(strings.TrimSpace(e.System.EventID))
	lvl, _ := strconv.Atoi(strings.TrimSpace(e.System.Level))
	t, _ := time.Parse(time.RFC3339Nano, e.System.TimeCreated.SystemTime)
	ev := sys.Event{Channel: e.System.Channel, Provider: e.System.Provider.Name, ID: id, Level: lvl, Time: t.Local()}
	if len(e.EventData.Data) > 0 {
		ev.Data = map[string]string{}
		var parts []string
		for i, d := range e.EventData.Data {
			name := d.Name
			if name == "" {
				name = strconv.Itoa(i)
			}
			ev.Data[name] = strings.TrimSpace(d.Value)
			if v := strings.TrimSpace(d.Value); v != "" && len(parts) < 4 {
				parts = append(parts, v)
			}
		}
		ev.Message = strings.Join(parts, " · ")
	}
	return ev, true
}
