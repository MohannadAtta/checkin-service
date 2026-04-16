package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"checkin-service/internal/api"
	"checkin-service/internal/mock"
	"checkin-service/internal/models"
	"checkin-service/internal/store"
	"checkin-service/internal/worker"
)

// In a real application, these might come from env vars
const (
	APIPort  = ":8080"
	MockPort = ":9090"
	MockURL  = "http://localhost:9090/record"
)

func main() {
	var activeStore store.Store
	dsn := os.Getenv("DATABASE_URL")

	if dsn != "" {
		log.Println("Using PostgreSQL Database Store...")
		var err error
		activeStore, err = store.NewPostgresStore(dsn)
		if err != nil {
			log.Fatalf("Failed to connect to postgres: %v", err)
		}
	} else {
		log.Println("Using In-Memory Database Store...")
		activeStore = store.NewMemoryStore()
	}

	// Initialize the job queue (buffered channel with size 1000)
	queue := make(chan models.CheckOutJob, 1000)

	// Start Mock Server in a goroutine
	go mock.StartMockServer(MockPort)

	// Start the background Worker in a goroutine
	w := worker.NewWorker(queue, MockURL)
	var wg sync.WaitGroup
	wg.Add(1)
	go w.Start(&wg)

	// Initialize HTTP API Handler
	h := &api.Handler{
		Store:  activeStore,
		Queue:  queue,
		Worker: w,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/event", h.HandleEvent)     // The single endpoint for all events
	mux.HandleFunc("/metrics", h.HandleMetrics) // New endpoint for metrics

	server := &http.Server{
		Addr:         APIPort,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	// Start server in a separate goroutine
	go func() {
		log.Printf("Starting Check-In Service API on port %s", APIPort)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	// Wait for interrupt signal to gracefully shut down the server
	quit := make(chan os.Signal, 1)
	// kill (no param) default is syscall.SIGTERM
	// kill -2 is syscall.SIGINT
	// kill -9 is syscall.SIGKILL but can't be caught, so don't need to add it
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	// Give outstanding HTTP requests 5 seconds to finish
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited properly. Draining the remaining worker jobs...")
	close(queue) // Close the channel, worker loop will naturally finish remaining items
	wg.Wait()    // Wait for the worker to finish sending final records to corporate payroll

	log.Println("All background jobs finished. Service officially stopped.")
}
