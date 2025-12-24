# Leader Election with PostgreSQL

A robust leader election implementation for distributed systems using PostgreSQL as the coordination backend. It uses priority-based Bully algorithm.

## Features

- **Priority-based election**: Nodes with higher priority are preferred as leaders
- **Automatic failover**: Detects leader failures and triggers re-election
- **Heartbeat monitoring**: Leaders send periodic heartbeats to maintain leadership
- **PostgreSQL-backed state**: All node states stored in PostgreSQL for persistence
- **Graceful shutdown**: Clean node deregistration on shutdown
- **Automatic cleanup**: Removes stale node records

## Architecture

The implementation uses a priority-based Bully algorithm:
1. Each node has a unique ID and optional priority (default: 0)
2. Nodes with higher priority (or lower ID for tie-breaking) become leaders
3. Leaders send periodic heartbeats to the database
4. Followers monitor for leader heartbeats
5. If no valid leader exists, an election is triggered
6. The highest priority alive node becomes the new leader

## Database Schema

The `node_records` table stores:
- `id`: Unique node identifier
- `status`: Current status (follower/candidate/leader)
- `last_heartbeat`: Timestamp of last heartbeat
- `term`: Election term number
- `priority`: Node priority for leader election
- `created_at`, `updated_at`: Timestamps

## Usage

### Prerequisites

PostgreSQL database (local or remote)

### Environment Variables

- `NODE_ID`: Unique identifier for this node (defaults to hostname)
- `DATABASE_URL`: PostgreSQL connection string
- `NODE_PRIORITY`: Priority for this node (higher = preferred, default: 0)

### Running a Single Node

```bash
export DATABASE_URL="host=localhost user=postgres password=postgres dbname=leaderelection port=5432 sslmode=disable"
export NODE_ID="node-1"
export NODE_PRIORITY="100"

go run main.go
```

### Running Multiple Nodes (Simulating a Cluster)

Terminal 1 (High priority node):
```bash
export DATABASE_URL="host=localhost user=postgres password=postgres dbname=leaderelection port=5432 sslmode=disable"
export NODE_ID="node-1"
export NODE_PRIORITY="100"
go run main.go
```

Terminal 2 (Medium priority node):
```bash
export DATABASE_URL="host=localhost user=postgres password=postgres dbname=leaderelection port=5432 sslmode=disable"
export NODE_ID="node-2"
export NODE_PRIORITY="50"
go run main.go
```

Terminal 3 (Low priority node):
```bash
export DATABASE_URL="host=localhost user=postgres password=postgres dbname=leaderelection port=5432 sslmode=disable"
export NODE_ID="node-3"
export NODE_PRIORITY="10"
go run main.go
```

### Docker Compose Example

```yaml
version: '3.8'

services:
  postgres:
    image: postgres:16
    environment:
      POSTGRES_USER: postgres
      POSTGRES_PASSWORD: postgres
      POSTGRES_DB: leaderelection
    ports:
      - "5432:5432"
    volumes:
      - postgres_data:/var/lib/postgresql/data

  node1:
    build: .
    environment:
      DATABASE_URL: "host=postgres user=postgres password=postgres dbname=leaderelection port=5432 sslmode=disable"
      NODE_ID: "node-1"
      NODE_PRIORITY: "100"
    depends_on:
      - postgres

  node2:
    build: .
    environment:
      DATABASE_URL: "host=postgres user=postgres password=postgres dbname=leaderelection port=5432 sslmode=disable"
      NODE_ID: "node-2"
      NODE_PRIORITY: "50"
    depends_on:
      - postgres

  node3:
    build: .
    environment:
      DATABASE_URL: "host=postgres user=postgres password=postgres dbname=leaderelection port=5432 sslmode=disable"
      NODE_ID: "node-3"
      NODE_PRIORITY: "10"
    depends_on:
      - postgres

volumes:
  postgres_data:
```

## API Reference

### Creating an Elector

```go
elector, err := election.NewElector(election.ElectorConfig{
    NodeID:            "my-node-1",
    DB:                db, // *gorm.DB
    HeartbeatInterval: 5 * time.Second,
    ElectionTimeout:   15 * time.Second,
    NodeTimeout:       30 * time.Second,
    Priority:          100,
    OnBecomeLeader: func() {
        log.Println("I am now the leader!")
    },
    OnBecomeFollower: func() {
        log.Println("I am now a follower")
    },
})
```

### Methods

- `Start()`: Begin the election process
- `Stop()`: Gracefully shutdown and deregister
- `IsLeader()`: Check if this node is currently the leader
- `GetStatus()`: Get current node status (follower/candidate/leader)
- `GetLeader()`: Get the current leader's node ID
- `GetAllNodes()`: Get all active nodes in the cluster

## Configuration Parameters

- **HeartbeatInterval**: How often leaders send heartbeats (default: 5s)
- **ElectionTimeout**: How long to wait before declaring leader dead (default: 15s)
- **NodeTimeout**: How long before a node is considered completely dead (default: 30s)
- **Priority**: Node's priority in elections (higher = preferred)

## Testing

```bash
# Run tests
go test ./pkg/election/...

# Run with coverage
go test -cover ./pkg/election/...
```

## How It Works

1. **Startup**: Node registers itself in the database with follower status
2. **Heartbeat Loop**: If leader, sends heartbeats every `HeartbeatInterval`
3. **Election Loop**: Checks for valid leader every `ElectionTimeout/2`
4. **Leader Failure**: When no heartbeat detected, election is triggered
5. **Election**: Highest priority alive node becomes leader
6. **Cleanup Loop**: Removes stale nodes every `NodeTimeout`

## Guarantees

- **Safety**: At most one leader at any time (enforced by database transactions)
- **Liveness**: A leader will eventually be elected if nodes are alive
- **Fairness**: Higher priority nodes are preferred as leaders

## License

MIT
