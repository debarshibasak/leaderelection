package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/debarshibasak/leaderelection/pkg/election"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func main() {
	// Get node ID from environment or use hostname
	nodeID := os.Getenv("NODE_ID")
	if nodeID == "" {
		hostname, _ := os.Hostname()
		nodeID = hostname
	}

	// Get database connection string from environment
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "host=localhost user=postgres password=postgres dbname=leaderelection port=5432 sslmode=disable"
		log.Printf("Using default DATABASE_URL. Set DATABASE_URL environment variable to override.")
	}

	// Connect to PostgreSQL
	db, err := gorm.Open(postgres.Open(dbURL), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		log.Fatalf("Failed to get database instance: %v", err)
	}
	defer sqlDB.Close()

	// Configure connection pool
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)

	// Get priority from environment (default: 0)
	priority := 0
	if p := os.Getenv("NODE_PRIORITY"); p != "" {
		fmt.Sscanf(p, "%d", &priority)
	}

	// Create elector
	elector, err := election.NewElector(election.ElectorConfig{
		NodeID:            nodeID,
		DB:                db,
		HeartbeatInterval: 5 * time.Second,
		ElectionTimeout:   15 * time.Second,
		NodeTimeout:       30 * time.Second,
		Priority:          priority,
		OnBecomeLeader: func() {
			log.Printf("🎉 Node %s became LEADER!", nodeID)
			// Add your leader-specific logic here
		},
		OnBecomeFollower: func() {
			log.Printf("📉 Node %s became FOLLOWER", nodeID)
			// Add your follower-specific logic here
		},
	})
	if err != nil {
		log.Fatalf("Failed to create elector: %v", err)
	}

	// Start the election process
	if err := elector.Start(); err != nil {
		log.Fatalf("Failed to start elector: %v", err)
	}

	// Status monitoring goroutine
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			status := elector.GetStatus()
			leader, _ := elector.GetLeader()
			nodes, _ := elector.GetAllNodes()

			log.Printf("Status: %s | Leader: %s | Active Nodes: %d",
				status, leader, len(nodes))

			if elector.IsLeader() {
				log.Printf("✓ This node is currently the LEADER")
			}
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down...")
	if err := elector.Stop(); err != nil {
		log.Printf("Error stopping elector: %v", err)
	}
	log.Println("Shutdown complete")
}
