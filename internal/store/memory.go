package store

import (
	"sync"
	"time"

	"checkin-service/internal/models"
)

// Store interface allows us to swap in passing optional PostgreSQL eventually
type Store interface {
	CheckIn(employeeID string, event models.Event) error
	CheckOut(employeeID string, outTime time.Time, loc *time.Location) (int, int, error)
	GetActiveCount() int
}

// MemoryStore implements a thread-safe in-memory store.
type MemoryStore struct {
	mu sync.RWMutex
	// activeCheckIns maps employeeID to their current ongoing check-in event.
	activeCheckIns map[string]models.Event
	// weekMinutes maps a "week key" (e.g., "employeeID-2023-W45") to accumulated minutes.
	weekMinutes map[string]int
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		activeCheckIns: make(map[string]models.Event),
		weekMinutes:    make(map[string]int),
	}
}

// CheckIn records an active check-in for an employee.
func (s *MemoryStore) CheckIn(employeeID string, event models.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activeCheckIns[employeeID] = event
	return nil
}

// GetActiveCount returns the current number of active check-ins.
func (s *MemoryStore) GetActiveCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.activeCheckIns)
}

// CheckOut calculates elapsed time, updates weekly totals, and clears the active check-in.
func (s *MemoryStore) CheckOut(employeeID string, outTime time.Time, loc *time.Location) (int, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	inEvent, ok := s.activeCheckIns[employeeID]
	if !ok {
		return 0, 0, nil // Handle logic better in handler if check-in missing, returning 0 for now.
	}

	// Calculate shift minutes
	shiftDuration := outTime.Sub(inEvent.Timestamp)
	shiftMinutes := int(shiftDuration.Minutes())

	if shiftMinutes < 0 {
		shiftMinutes = 0 // Prevent negative time issues on weird clock syncs
	}

	// Calculate week key (ISO week)
	year, week := outTime.In(loc).ISOWeek()
	weekKey := employeeID + "-" + string(rune(year)) + "-" + string(rune(week))

	// Update total
	s.weekMinutes[weekKey] += shiftMinutes
	totalWeekly := s.weekMinutes[weekKey]

	// Remove active check-in
	delete(s.activeCheckIns, employeeID)

	return shiftMinutes, totalWeekly, nil
}
