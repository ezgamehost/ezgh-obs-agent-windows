package eventlog

import (
	"encoding/xml"
	"errors"
	"fmt"
	"github.com/ezgamehost/ezgh-obs-agent-windows/internal/telemetry"
	logs "go.opentelemetry.io/proto/otlp/logs/v1"
	"strconv"
	"time"
)

type Event struct {
	System struct {
		Provider struct {
			Name string `xml:"Name,attr"`
			GUID string `xml:"Guid,attr"`
		} `xml:"Provider"`
		ID       string `xml:"EventID"`
		Level    int    `xml:"Level"`
		Version  string `xml:"Version"`
		Task     string `xml:"Task"`
		Opcode   string `xml:"Opcode"`
		Keywords string `xml:"Keywords"`
		RecordID string `xml:"EventRecordID"`
		Channel  string `xml:"Channel"`
		Time     struct {
			Value string `xml:"SystemTime,attr"`
		} `xml:"TimeCreated"`
		Execution struct {
			PID string `xml:"ProcessID,attr"`
			TID string `xml:"ThreadID,attr"`
		} `xml:"Execution"`
		Correlation struct {
			ID string `xml:"ActivityID,attr"`
		} `xml:"Correlation"`
	} `xml:"System"`
}

func Map(raw, message string, observed time.Time) (*logs.LogRecord, error) {
	if len(raw) > 1<<20 {
		return nil, errors.New("event XML too large")
	}
	var e Event
	if err := xml.Unmarshal([]byte(raw), &e); err != nil {
		return nil, errors.New("invalid event XML")
	}
	s := e.System
	at, err := time.Parse(time.RFC3339Nano, s.Time.Value)
	if err != nil {
		at = observed
	}
	levels := map[int]logs.SeverityNumber{1: 21, 2: 17, 3: 13, 4: 9, 5: 5}
	attrs := map[string]string{"windows.event.channel": s.Channel, "windows.event.provider": s.Provider.Name, "windows.event.provider_guid": s.Provider.GUID, "windows.event.id": s.ID, "windows.event.record_id": s.RecordID, "windows.event.level": strconv.Itoa(s.Level), "windows.event.version": s.Version, "windows.event.task": s.Task, "windows.event.opcode": s.Opcode, "windows.event.keywords": s.Keywords, "windows.event.process_id": s.Execution.PID, "windows.event.thread_id": s.Execution.TID, "windows.event.activity_id": s.Correlation.ID}
	if message == "" {
		message = raw
		attrs["windows.event.format_failed"] = "true"
	}
	r := telemetry.Log(message, at, levels[s.Level], attrs)
	r.ObservedTimeUnixNano = uint64(observed.UnixNano())
	return r, nil
}

type Checkpoint struct {
	Initialized bool   `json:"initialized"`
	Bookmark    string `json:"bookmark"`
	Query       string `json:"query"`
}

func SourceKey(id string, generation int) string {
	return fmt.Sprintf("eventlog:%s:%d", id, generation)
}
func ValidateCheckpoint(c Checkpoint, query string) error {
	if c.Initialized && c.Query != query {
		return errors.New("event query changed: increment source generation")
	}
	return nil
}
