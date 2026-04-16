# Check-In Service

A simple HTTP backend service written in Go for processing factory employee check-ins and check-outs, calculating worked hours, and reliably forwarding data to an external recording system.

## Architecture

```text
                        ┌───────────────────┐
                        │   Card Readers    │
                        │  (HTTP Clients)   │
                        └─────────┬─────────┘
                                  │ POST /event
                                  ▼
                        ┌───────────────────┐               ┌───────────────────┐
                        │   HTTP Handler    │◄─────────────►│ In-Memory Store   │
                        │ (check-in/-out)   │               │ (sync.RWMutex)    │
                        └─────────┬─────────┘               └───────────────────┘
                                  │ Job (Minutes Worked)
                                  ▼
                        ┌───────────────────┐
                        │ Go Channel Queue  │ (Buffered)
                        └─────────┬─────────┘
                                  │ Wait for Job
                                  ▼
                        ┌───────────────────┐
                        │ Worker Goroutine  │ (Retries & Logging)
                        └─────────┬─────────┘
                                  │ POST /record
                                  ▼
                        ┌───────────────────┐
                        │    Mock System    │ (Fails randomly)
                        └───────────────────┘
```

## Setup & Running

This project uses the Go standard library with no external dependencies for core functionality. 

To run the application locally:

```bash
cd checkin-service
go run cmd/server/main.go
```

By default, the API will start on `localhost:8080/event` using the thread-safe **In-Memory Store**, and a mock backend server will start on `localhost:9090`.

### Running with PostgreSQL (Bonus Feature)
To persist state to a real database instead of memory, a `docker-compose.yml` and `schema.sql` are included.
1. Start the PostgreSQL database:
   ```bash
   docker-compose up -d
   ```
2. Start the server with the database URL (in PowerShell):
   ```powershell
   $env:DATABASE_URL="postgres://checkin_user:checkin_secret@localhost:5432/checkin_db?sslmode=disable"
   go run cmd/server/main.go
   ```
*(On Linux/Mac use: `DATABASE_URL="..." go run cmd/server/main.go`)*

### Extra Features Included
* **Graceful Shutdown**: Intercepts `SIGINT`/`SIGTERM` to stop incoming HTTP traffic, wait for outstanding HTTP requests, and fully drain the remaining jobs in the Go channel to Corporate Payroll before powering off.
* **Basic Metrics**: Use `curl http://localhost:8080/metrics` to see active check-ins, queue depth, and worker success/failure counts.

### Example Check-In
```bash
curl -X POST http://localhost:8080/event \
     -H "Content-Type: application/json" \
     -d '{
         "employee_id": "EMP-123",
         "factory_id": "FAC-1",
         "location": "America/Sao_Paulo",
         "timestamp": "2026-04-20T08:00:00Z",
         "type": "check_in"
     }'
```

---

## Design Questions

**1. Duplicate events — The card reader hardware sometimes retries a request when it does not receive a quick response. How would you protect against this? What could go wrong, and how did you address it (or how would you)?**  
If the card reader retries an identical `check_in` request, our system might overwrite the active check-in (resulting in losing the original timestamp). If it retries a `check_out` event, the state might have already been mutated, leading to missing data errors. To fix this, I would implement Idempotency Keys (either sending a unique `request_id` from the reader) and persisting the processed request IDs temporarily to discard identical duplicates.

**2. Clock trust — Each event includes a timestamp from the card reader hardware. Did you use it, or the server clock, or something else — and why? What are the tradeoffs of each approach?**  
I used the hardware clock from the payload. The tradeoff is that if the local factory hardware clock drifts or is maliciously tampered with, shift minutes will be wrong. However, using the server clock is fragile against high network latency or delayed retries (e.g. if the internet goes down at the factory, they will send the batch later, rendering the server clock useless). A hybrid approach (trusting hardware but validating against reasonable server drift) is ideal in production.

**3. Concurrent requests — Go handles each HTTP request in its own goroutine. What could go wrong under concurrent load in your current implementation, and what did you do about it?**  
Without proper synchronization, concurrent map writes and reads for the same `employee_id` (e.g., checking out while checking in simultaneously due to aggressive retries) would cause data races and crash the Go process. I prevented this by encapsulating the state map within `sync.RWMutex` which guarantees thread-safe reads and writes.

**4. Scaling — The company opens 50 more factories. What would break first in your current design, and what would you change?**  
Since the check-ins are stored in an in-memory map on a single instance, scaling horizontally (adding more instances of our API behind a load balancer) would instantly fail because session state (who checked in where) wouldn't be shared. I would replace the in-memory store with a persistent database like PostgreSQL or a fast KV store like Redis to ensure state is shared across all scaling nodes over the network.

**5. Data correction — An employee forgot to check out yesterday. How would you handle that without compromising the integrity of the existing data?**  
Currently, a missing checkout leaves the user in an "active" state forever. I would introduce an administrative "Correction API" that allows HR to append a manual `check_out` event to the worker channel directly, using the HR system's timestamp. To fix an old entry without breaking existing totals, the external labor cost backend should ideally support idempotent overwrites (or negative delta jobs). 

---
**AI Transparency Notice:**
GitHub Copilot was used as a pair-programming assistant during this assignment. Specifically, it was utilized to:
* Scaffold the initial Go directory structure and boilerplate.
* Assist with standard library syntax (like parsing RFC3339 timestamps and ISO calendar weeks).
* Brainstorm and validate the technical trade-offs for the written design questions.
* Generate boilerplate SQL and the `docker-compose.yml` for the optional PostgreSQL database implementation.
* Draft the graceful shutdown channel context logic.

*All core logic and architecture decisions were reviewed and directed by me, Mohannad Atta.*
