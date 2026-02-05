// Simple test server for PostgreSQL wire protocol.
// Run with: go run ./internal/distributed/proxy/pgserver/cmd/
package main

import (
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/milvus-io/milvus/internal/distributed/proxy/pgserver"
)

func main() {
	config := pgserver.DefaultConfig()
	config.Port = 15432 // Use non-standard port to avoid conflicts

	// Create server
	server := pgserver.NewServer(nil, config)

	// Create listener
	addr := ":15432"
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("Failed to listen on %s: %v", addr, err)
	}

	log.Printf("PostgreSQL interface listening on %s", addr)
	log.Printf("Connect with: psql -h localhost -p 15432 -U test")

	// Start server
	if err := server.Start(listener); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down...")
	server.Stop()
}
