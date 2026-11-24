package accounting

import (
	"context"
	"slices"
	"sync"
	"time"
)

// StaticSource serves records held in memory. Tests drive the sync engine with
// it, and the bundled mock endpoint uses it as its backing store.
type StaticSource struct {
	name string

	mu      sync.RWMutex
	records []Record

	// FailWith makes the next FetchChanged fail, for testing a half-finished run.
	FailWith error
}

func NewStaticSource(name string, records []Record) *StaticSource {
	rs := slices.Clone(records)
	SortRecords(rs)
	return &StaticSource{name: name, records: rs}
}

func (s *StaticSource) Name() string { return s.name }

func (s *StaticSource) FetchChanged(_ context.Context, since time.Time, cursor string, limit int) (Page, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.FailWith != nil {
		return Page{}, s.FailWith
	}
	return PageRecords(s.records, since, cursor, limit)
}

func (s *StaticSource) Records() []Record {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Clone(s.records)
}

// Upsert adds or replaces a record: how a test says "upstream changed this".
func (s *StaticSource) Upsert(r Record) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if i := slices.IndexFunc(s.records, func(x Record) bool { return x.ExternalID == r.ExternalID }); i >= 0 {
		s.records[i] = r
	} else {
		s.records = append(s.records, r)
	}
	SortRecords(s.records)
}
