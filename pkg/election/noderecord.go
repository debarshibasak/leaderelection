package election

import "time"

// NodeStatus represents the state of a node in the cluster
type NodeStatus string

const (
	StatusFollower  NodeStatus = "follower"
	StatusCandidate NodeStatus = "candidate"
	StatusLeader    NodeStatus = "leader"
)

// NodeRecord represents a node's state in the database
type NodeRecord struct {
	ID            string     `gorm:"primaryKey"`
	Status        NodeStatus `gorm:"type:varchar(20)"`
	LastHeartbeat time.Time  `gorm:"index"`
	Term          int64      `gorm:"index"`
	Priority      int        `gorm:"default:0"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
}
