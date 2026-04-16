package store

import (
	"database/sql"
	"fmt"
	"time"

	"checkin-service/internal/models"

	_ "github.com/lib/pq"
)

// PostgresStore implements the Store interface using PostgreSQL.
type PostgresStore struct {
	db *sql.DB
}

func NewPostgresStore(dsn string) (*PostgresStore, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}
	return &PostgresStore{db: db}, nil
}

func (s *PostgresStore) CheckIn(employeeID string, event models.Event) error {
	query := `
		INSERT INTO active_checkins (employee_id, factory_id, location_tz, check_in_time)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (employee_id) DO UPDATE 
		SET factory_id = EXCLUDED.factory_id, location_tz = EXCLUDED.location_tz, check_in_time = EXCLUDED.check_in_time`

	_, err := s.db.Exec(query, employeeID, event.FactoryID, event.Location, event.Timestamp)
	return err
}

func (s *PostgresStore) CheckOut(employeeID string, outTime time.Time, loc *time.Location) (int, int, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()

	// 1. Get the check-in time
	var checkInTime time.Time
	err = tx.QueryRow(`SELECT check_in_time FROM active_checkins WHERE employee_id = $1`, employeeID).Scan(&checkInTime)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, 0, nil // No active check-in found
		}
		return 0, 0, err
	}

	// 2. Calculate shift minutes
	shiftDuration := outTime.Sub(checkInTime)
	shiftMinutes := int(shiftDuration.Minutes())
	if shiftMinutes < 0 {
		shiftMinutes = 0
	}

	// 3. Calculate week key and update weekly_totals safely
	year, week := outTime.In(loc).ISOWeek()
	weekKey := fmt.Sprintf("%s-%d-%d", employeeID, year, week)

	var totalWeekly int
	updateQuery := `
		INSERT INTO weekly_totals (week_key, total_minutes)
		VALUES ($1, $2)
		ON CONFLICT (week_key) DO UPDATE 
		SET total_minutes = weekly_totals.total_minutes + EXCLUDED.total_minutes
		RETURNING total_minutes`
	err = tx.QueryRow(updateQuery, weekKey, shiftMinutes).Scan(&totalWeekly)
	if err != nil {
		return 0, 0, err
	}

	// 4. Delete the active check-in
	_, err = tx.Exec(`DELETE FROM active_checkins WHERE employee_id = $1`, employeeID)
	if err != nil {
		return 0, 0, err
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return 0, 0, err
	}

	return shiftMinutes, totalWeekly, nil
}

func (s *PostgresStore) GetActiveCount() int {
	var count int
	s.db.QueryRow(`SELECT COUNT(*) FROM active_checkins`).Scan(&count)
	return count
}
