// Test server for PostgreSQL wire protocol against a real Milvus cluster.
// Usage:
//   1. Start Milvus: docker compose up -d
//   2. Run:          go run ./internal/distributed/proxy/pgserver/cmd/
//   3. Connect:      psql -h localhost -p 15432 -U test
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/milvus-io/milvus-proto/go-api/v2/milvuspb"
	"github.com/milvus-io/milvus/internal/distributed/proxy/pgserver"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// grpcProxy wraps a Milvus gRPC client to satisfy pgserver.MilvusProxy.
type grpcProxy struct {
	client milvuspb.MilvusServiceClient
}

func (g *grpcProxy) Search(ctx context.Context, req *milvuspb.SearchRequest) (*milvuspb.SearchResults, error) {
	return g.client.Search(ctx, req)
}

func (g *grpcProxy) Insert(ctx context.Context, req *milvuspb.InsertRequest) (*milvuspb.MutationResult, error) {
	return g.client.Insert(ctx, req)
}

func main() {
	milvusAddr := flag.String("milvus", "localhost:19530", "Milvus gRPC address")
	pgPort := flag.Int("port", 15432, "PostgreSQL listen port")
	flag.Parse()

	// Connect to Milvus via gRPC
	log.Printf("Connecting to Milvus at %s ...", *milvusAddr)
	conn, err := grpc.Dial(*milvusAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(64*1024*1024)),
	)
	if err != nil {
		log.Fatalf("Failed to connect to Milvus: %v", err)
	}
	defer conn.Close()

	proxy := &grpcProxy{client: milvuspb.NewMilvusServiceClient(conn)}
	log.Printf("Connected to Milvus at %s", *milvusAddr)

	// Start pgserver
	config := pgserver.DefaultConfig()
	config.Port = *pgPort

	server := pgserver.NewServer(proxy, config)

	addr := fmt.Sprintf(":%d", *pgPort)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("Failed to listen on %s: %v", addr, err)
	}

	log.Printf("PostgreSQL interface listening on %s", addr)
	log.Printf("Connect with: psql -h localhost -p %d -U test", *pgPort)

	if err := server.Start(listener); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down...")
	server.Stop()
}
