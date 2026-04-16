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
Duplicates can cause lost timestamps (on check-in) or missing data errors (on check-out). I would fix this by implementing Idempotency Keys (using a unique `request_id` from the reader) and temporarily caching processed IDs to discard repeats.

**2. Clock trust — Each event includes a timestamp from the card reader hardware. Did you use it, or the server clock, or something else — and why? What are the tradeoffs of each approach?**  
I used the hardware clock to handle delayed retries and network outages (where server time would be wrong). The tradeoff is vulnerability to local clock drift/tampering. A hybrid approach (trusting hardware but bounding within reasonable server drift) is best.

**3. Concurrent requests — Go handles each HTTP request in its own goroutine. What could go wrong under concurrent load in your current implementation, and what did you do about it?**  
Concurrent map access causes data races resulting in crashes. I prevented this by protecting the in-memory state map with a `sync.RWMutex` to ensure thread-safe concurrent reads and writes.

**4. Scaling — The company opens 50 more factories. What would break first in your current design, and what would you change?**  
Horizontal scaling (adding more instances behind a load balancer) breaks the local in-memory state. I would change to a persistent distributed data store like PostgreSQL or Redis so state is shared across all API nodes.

**5. Data correction — An employee forgot to check out yesterday. How would you handle that without compromising the integrity of the existing data?**  
I would build an HR "Correction API" to insert a manual `check_out` event with an overridden backward-dated timestamp. To adjust past totals gracefully, the external labor backend must support idempotent overwrites or negative corrections.

---
**AI Transparency Notice:**
GitHub Copilot was used as a pair-programming assistant during this assignment. Specifically, it was utilized to:
* Scaffold the initial Go directory structure and boilerplate.
* Assist with standard library syntax (like parsing RFC3339 timestamps and ISO calendar weeks).
* Brainstorm and validate the technical trade-offs for the written design questions.
* Generate boilerplate SQL and the `docker-compose.yml` for the optional PostgreSQL database implementation.
* Draft the graceful shutdown channel context logic.

*All core logic and architecture decisions were reviewed and directed by me,,,, Mohannad Atta.*
