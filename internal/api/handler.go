package api

import (
	"encoding/json"
	"net/http"
	"sync/atomic"
	"time"

	"checkin-service/internal/models"
	"checkin-service/internal/store"
	"checkin-service/internal/worker"
)

type Handler struct {
	Store  store.Store
	Queue  chan<- models.CheckOutJob
	Worker *worker.Worker
}

func (h *Handler) HandleEvent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	var event models.Event
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	if event.EmployeeID == "" || event.Location == "" || event.Type == "" {
		http.Error(w, "Missing required fields", http.StatusBadRequest)
		return
	}

	loc, err := time.LoadLocation(event.Location)
	if err != nil {
		http.Error(w, "Invalid timezone", http.StatusBadRequest)
		return
	}

	switch event.Type {
	case models.TypeCheckIn:
		// Process Check-In
		err := h.Store.CheckIn(event.EmployeeID, event)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]string{"status": "checked_in"})

	case models.TypeCheckOut:
		// Process Check-Out
		shiftMins, weeklyMins, err := h.Store.CheckOut(event.EmployeeID, event.Timestamp, loc)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// Enqueue Job for the external system worker (non-blocking)
		select {
		case h.Queue <- models.CheckOutJob{EmployeeID: event.EmployeeID, MinutesWorked: shiftMins}:
			// Job queued successfully
		default:
			// Queue is full; in a real app, log error or drop
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(models.ShiftResult{
			ShiftMinutes:  shiftMins,
			WeeklyMinutes: weeklyMins,
		})

	default:
		http.Error(w, "Invalid event type", http.StatusBadRequest)
	}
}

// HandleMetrics provides a snapshot of current system health and performance
func (h *Handler) HandleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	queueLength := len(h.Queue)
	successes := atomic.LoadUint64(&h.Worker.Successes)
	failures := atomic.LoadUint64(&h.Worker.Failures)

	metrics := map[string]interface{}{
		"active_checkins":  h.Store.GetActiveCount(),
		"queue_depth":      queueLength,
		"worker_successes": successes,
		"worker_failures":  failures,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(metrics)
}
