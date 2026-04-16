package mock

import (
	"encoding/json"
	"log"
	"math/rand"
	"net/http"
	"time"
)

// RecordPayload is the expected payload from the worker.
type RecordPayload struct {
	EmployeeID    string `json:"employee_id"`
	MinutesWorked int    `json:"minutes_worked"`
}

// StartMockServer simulates the unreliable external labor cost system.
func StartMockServer(addr string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/record", func(w http.ResponseWriter, r *http.Request) {
		var payload RecordPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}

		// Simulate random delay
		time.Sleep(time.Duration(rand.Intn(100)) * time.Millisecond)

		// Simulate unreliability (30% failure rate)
		if rand.Float32() < 0.3 {
			log.Printf("[Mock] Faking error for employee %s", payload.EmployeeID)
			http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
			return
		}

		log.Printf("[Mock] Successfully recorded shift for employee %s (%d min)", payload.EmployeeID, payload.MinutesWorked)
		w.WriteHeader(http.StatusOK)
	})

	log.Printf("Starting Mock System on %s/record", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("Mock server failed: %v", err)
	}
}
