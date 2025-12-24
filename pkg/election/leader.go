package election

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"gorm.io/gorm"
)

// Elector manages leader election for a distributed system
type Elector struct {
	nodeID            string
	db                *gorm.DB
	currentStatus     NodeStatus
	currentTerm       int64
	heartbeatInterval time.Duration
	electionTimeout   time.Duration
	nodeTimeout       time.Duration
	priority          int
	mu                sync.RWMutex
	ctx               context.Context
	cancel            context.CancelFunc
	onBecomeLeader    func()
	onBecomeFollower  func()
}

// ElectorConfig holds configuration for the Elector
type ElectorConfig struct {
	NodeID            string
	DB                *gorm.DB
	HeartbeatInterval time.Duration
	ElectionTimeout   time.Duration
	NodeTimeout       time.Duration
	Priority          int
	OnBecomeLeader    func()
	OnBecomeFollower  func()
}

// NewElector creates a new Elector instance
func NewElector(config ElectorConfig) (*Elector, error) {
	if config.NodeID == "" {
		return nil, fmt.Errorf("nodeID is required")
	}
	if config.DB == nil {
		return nil, fmt.Errorf("database connection is required")
	}

	// Set defaults
	if config.HeartbeatInterval == 0 {
		config.HeartbeatInterval = 5 * time.Second
	}
	if config.ElectionTimeout == 0 {
		config.ElectionTimeout = 15 * time.Second
	}
	if config.NodeTimeout == 0 {
		config.NodeTimeout = 30 * time.Second
	}

	ctx, cancel := context.WithCancel(context.Background())

	elector := &Elector{
		nodeID:            config.NodeID,
		db:                config.DB,
		currentStatus:     StatusFollower,
		currentTerm:       0,
		heartbeatInterval: config.HeartbeatInterval,
		electionTimeout:   config.ElectionTimeout,
		nodeTimeout:       config.NodeTimeout,
		priority:          config.Priority,
		ctx:               ctx,
		cancel:            cancel,
		onBecomeLeader:    config.OnBecomeLeader,
		onBecomeFollower:  config.OnBecomeFollower,
	}

	// Auto-migrate the schema
	if err := elector.db.AutoMigrate(&NodeRecord{}); err != nil {
		return nil, fmt.Errorf("failed to migrate schema: %w", err)
	}

	return elector, nil
}

// Start begins the leader election process
func (e *Elector) Start() error {
	// Register this node in the database
	if err := e.registerNode(); err != nil {
		return fmt.Errorf("failed to register node: %w", err)
	}

	// Start background processes
	go e.heartbeatLoop()
	go e.electionLoop()
	go e.cleanupLoop()

	log.Printf("Node %s started as %s", e.nodeID, e.currentStatus)
	return nil
}

// Stop gracefully stops the elector
func (e *Elector) Stop() error {
	e.cancel()

	// Update node status to indicate it's shutting down
	return e.db.Model(&NodeRecord{}).
		Where("id = ?", e.nodeID).
		Updates(map[string]interface{}{
			"status":         StatusFollower,
			"last_heartbeat": time.Now().Add(-e.nodeTimeout),
		}).Error
}

// IsLeader returns whether this node is the current leader
func (e *Elector) IsLeader() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.currentStatus == StatusLeader
}

// GetStatus returns the current status of this node
func (e *Elector) GetStatus() NodeStatus {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.currentStatus
}

// GetLeader returns the current leader node ID, if any
func (e *Elector) GetLeader() (string, error) {
	var leader NodeRecord
	err := e.db.Where("status = ? AND last_heartbeat > ?",
		StatusLeader,
		time.Now().Add(-e.nodeTimeout)).
		First(&leader).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", nil
		}
		return "", err
	}

	return leader.ID, nil
}

// registerNode creates or updates the node record in the database
func (e *Elector) registerNode() error {
	node := NodeRecord{
		ID:            e.nodeID,
		Status:        StatusFollower,
		LastHeartbeat: time.Now(),
		Term:          0,
		Priority:      e.priority,
	}

	return e.db.Transaction(func(tx *gorm.DB) error {
		var existing NodeRecord
		err := tx.Where("id = ?", e.nodeID).First(&existing).Error

		if err == gorm.ErrRecordNotFound {
			// Create new record
			return tx.Create(&node).Error
		} else if err != nil {
			return err
		}

		// Update existing record
		return tx.Model(&NodeRecord{}).
			Where("id = ?", e.nodeID).
			Updates(map[string]interface{}{
				"status":         StatusFollower,
				"last_heartbeat": time.Now(),
				"priority":       e.priority,
			}).Error
	})
}

// heartbeatLoop sends periodic heartbeats when the node is a leader
func (e *Elector) heartbeatLoop() {
	ticker := time.NewTicker(e.heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-e.ctx.Done():
			return
		case <-ticker.C:
			if e.IsLeader() {
				if err := e.sendHeartbeat(); err != nil {
					log.Printf("Failed to send heartbeat: %v", err)
				}
			}
		}
	}
}

// sendHeartbeat updates the leader's heartbeat timestamp
func (e *Elector) sendHeartbeat() error {
	e.mu.RLock()
	term := e.currentTerm
	e.mu.RUnlock()

	return e.db.Model(&NodeRecord{}).
		Where("id = ?", e.nodeID).
		Updates(map[string]interface{}{
			"last_heartbeat": time.Now(),
			"status":         StatusLeader,
			"term":           term,
		}).Error
}

// electionLoop monitors for leader failures and triggers elections
func (e *Elector) electionLoop() {
	ticker := time.NewTicker(e.electionTimeout / 2)
	defer ticker.Stop()

	for {
		select {
		case <-e.ctx.Done():
			return
		case <-ticker.C:
			if err := e.checkAndStartElection(); err != nil {
				log.Printf("Election check failed: %v", err)
			}
		}
	}
}

// checkAndStartElection checks if an election is needed and starts one
func (e *Elector) checkAndStartElection() error {
	// Don't start election if we're already the leader
	if e.IsLeader() {
		return nil
	}

	// Check if there's a valid leader
	leaderExists, err := e.hasValidLeader()
	if err != nil {
		return err
	}

	if !leaderExists {
		return e.startElection()
	}

	return nil
}

// hasValidLeader checks if there's a leader with recent heartbeat
func (e *Elector) hasValidLeader() (bool, error) {
	var count int64
	err := e.db.Model(&NodeRecord{}).
		Where("status = ? AND last_heartbeat > ?",
			StatusLeader,
			time.Now().Add(-e.electionTimeout)).
		Count(&count).Error

	return count > 0, err
}

// startElection initiates a new election using a priority-based algorithm
func (e *Elector) startElection() error {
	log.Printf("Node %s starting election", e.nodeID)

	return e.db.Transaction(func(tx *gorm.DB) error {
		// Get all alive nodes
		var nodes []NodeRecord
		err := tx.Where("last_heartbeat > ?", time.Now().Add(-e.nodeTimeout)).
			Order("priority DESC, id ASC").
			Find(&nodes).Error
		if err != nil {
			return err
		}

		if len(nodes) == 0 {
			return fmt.Errorf("no alive nodes found")
		}

		// Find highest priority node
		highestPriorityNode := nodes[0]

		// If we're the highest priority node, become leader
		if highestPriorityNode.ID == e.nodeID {
			e.mu.Lock()
			e.currentTerm++
			newTerm := e.currentTerm
			e.mu.Unlock()

			// Update our status to leader
			err = tx.Model(&NodeRecord{}).
				Where("id = ?", e.nodeID).
				Updates(map[string]interface{}{
					"status":         StatusLeader,
					"term":           newTerm,
					"last_heartbeat": time.Now(),
				}).Error
			if err != nil {
				return err
			}

			// Demote any existing leaders
			err = tx.Model(&NodeRecord{}).
				Where("id != ? AND status = ?", e.nodeID, StatusLeader).
				Update("status", StatusFollower).Error
			if err != nil {
				return err
			}

			e.mu.Lock()
			oldStatus := e.currentStatus
			e.currentStatus = StatusLeader
			e.mu.Unlock()

			if oldStatus != StatusLeader && e.onBecomeLeader != nil {
				go e.onBecomeLeader()
			}

			log.Printf("Node %s became LEADER for term %d", e.nodeID, newTerm)
		} else {
			// We're not the highest priority, remain follower
			e.mu.Lock()
			oldStatus := e.currentStatus
			e.currentStatus = StatusFollower
			e.mu.Unlock()

			if oldStatus == StatusLeader && e.onBecomeFollower != nil {
				go e.onBecomeFollower()
			}

			err = tx.Model(&NodeRecord{}).
				Where("id = ?", e.nodeID).
				Updates(map[string]interface{}{
					"status":         StatusFollower,
					"last_heartbeat": time.Now(),
				}).Error
			if err != nil {
				return err
			}

			log.Printf("Node %s remains follower (higher priority node: %s)",
				e.nodeID, highestPriorityNode.ID)
		}

		return nil
	})
}

// cleanupLoop removes stale node records
func (e *Elector) cleanupLoop() {
	ticker := time.NewTicker(e.nodeTimeout)
	defer ticker.Stop()

	for {
		select {
		case <-e.ctx.Done():
			return
		case <-ticker.C:
			if err := e.cleanupStaleNodes(); err != nil {
				log.Printf("Failed to cleanup stale nodes: %v", err)
			}
		}
	}
}

// cleanupStaleNodes removes nodes that haven't sent heartbeats
func (e *Elector) cleanupStaleNodes() error {
	cutoff := time.Now().Add(-e.nodeTimeout * 2)

	return e.db.Where("last_heartbeat < ?", cutoff).
		Delete(&NodeRecord{}).Error
}

// GetAllNodes returns all active nodes in the cluster
func (e *Elector) GetAllNodes() ([]NodeRecord, error) {
	var nodes []NodeRecord
	err := e.db.Where("last_heartbeat > ?", time.Now().Add(-e.nodeTimeout)).
		Order("priority DESC, id ASC").
		Find(&nodes).Error
	return nodes, err
}
