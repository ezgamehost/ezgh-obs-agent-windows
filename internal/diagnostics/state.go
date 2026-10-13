package diagnostics

import (
	"maps"
	"sync"
	"time"
)

type Snapshot struct {
	Started  time.Time          `json:"started"`
	Counters map[string]uint64  `json:"counters"`
	Gauges   map[string]float64 `json:"gauges"`
	States   map[string]string  `json:"states"`
}
type State struct {
	mu sync.Mutex
	v  Snapshot
}

func New() *State {
	return &State{v: Snapshot{Started: time.Now(), Counters: map[string]uint64{}, Gauges: map[string]float64{}, States: map[string]string{}}}
}
func (s *State) Add(k string, n uint64)    { s.mu.Lock(); defer s.mu.Unlock(); s.v.Counters[k] += n }
func (s *State) Gauge(k string, n float64) { s.mu.Lock(); defer s.mu.Unlock(); s.v.Gauges[k] = n }
func (s *State) Set(k, v string)           { s.mu.Lock(); defer s.mu.Unlock(); s.v.States[k] = v }
func (s *State) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Snapshot{s.v.Started, maps.Clone(s.v.Counters), maps.Clone(s.v.Gauges), maps.Clone(s.v.States)}
}
