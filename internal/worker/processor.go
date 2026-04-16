package worker

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"checkin-service/internal/models"
)

type Worker struct {
	Queue     <-chan models.CheckOutJob
	MockURL   string
	Client    *http.Client
	Successes uint64 // Metrics counter
	Failures  uint64 // Metrics counter
}

const maxRetries = 3

func NewWorker(queue <-chan models.CheckOutJob, url string) *Worker {
	return &Worker{
		Queue:   queue,
		MockURL: url,
		Client:  &http.Client{Timeout: 5 * time.Second},
	}
}

func (w *Worker) Start(wg *sync.WaitGroup) {
	defer wg.Done()
	for job := range w.Queue {
		w.processJob(job)
	}
	log.Println("[Worker] Queue closed, gracefully shutting down.")
}

func (w *Worker) processJob(job models.CheckOutJob) {
	payloadBytes, _ := json.Marshal(map[string]interface{}{
		"employee_id":    job.EmployeeID,
		"minutes_worked": job.MinutesWorked,
	})

	for attempt := 1; attempt <= maxRetries; attempt++ {
		resp, err := w.Client.Post(w.MockURL, "application/json", bytes.NewBuffer(payloadBytes))

		if err == nil {
			if resp.StatusCode == http.StatusOK {
				log.Printf("[Worker] Successfully synced job for %s", job.EmployeeID)
				atomic.AddUint64(&w.Successes, 1)
				resp.Body.Close()
				return
			}
			log.Printf("[Worker] Attempt %d failed for %s with status %v", attempt, job.EmployeeID, resp.StatusCode)
			resp.Body.Close()
		} else {
			log.Printf("[Worker] Attempt %d failed due to err: %v", attempt, err)
		}

		time.Sleep(time.Duration(attempt) * time.Second) // Exponential-ish backoff
	}

	log.Printf("[Worker] ERROR: Max retries reached for %s. Job discarded.", job.EmployeeID)
	atomic.AddUint64(&w.Failures, 1)
}
