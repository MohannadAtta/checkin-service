package models

import "time"

// Event represents the incoming JSON request from a card reader.
type Event struct {
	EmployeeID string    `json:"employee_id"`
	FactoryID  string    `json:"factory_id"`
	Location   string    `json:"location"` // IANA timezone string
	Timestamp  time.Time `json:"timestamp"` // RFC3339 timestamp
	Type       EventType `json:"type"`     // "check_in" or "check_out"
}

type EventType string

const (
	TypeCheckIn  EventType = "check_in"
	TypeCheckOut EventType = "check_out"
)

// ShiftResult is the response returned to the card reader on check-out.
type ShiftResult struct {
	ShiftMinutes int `json:"shift_minutes"`
	WeeklyMinutes int `json:"weekly_minutes"`
}

// CheckOutJob represents the payload placed on the queue for the worker.
type CheckOutJob struct {
	EmployeeID    string `json:"employee_id"`
	MinutesWorked int    `json:"minutes_worked"`
}
