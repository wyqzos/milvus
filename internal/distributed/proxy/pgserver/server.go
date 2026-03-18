// Package pgserver implements a PostgreSQL wire protocol compatible server
// that translates SQL queries to Milvus operations.
package pgserver

import (
	"context"
	"log"
	"net"
	"sync"
	"sync/atomic"

	"github.com/milvus-io/milvus-proto/go-api/v2/milvuspb"
)

// MilvusProxy defines the minimal Milvus interface needed by pgserver.
// The real Proxy satisfies this interface automatically since Go interfaces
// are implicit — no "implements" declaration needed.
type MilvusProxy interface {
	Search(ctx context.Context, req *milvuspb.SearchRequest) (*milvuspb.SearchResults, error)
	Insert(ctx context.Context, req *milvuspb.InsertRequest) (*milvuspb.MutationResult, error)
}

// Server handles PostgreSQL protocol connections and translates
// SQL queries to Milvus operations.
type Server struct {
	proxy    MilvusProxy
	listener net.Listener
	config   *Config

	wg      sync.WaitGroup
	stopped atomic.Bool
	ctx     context.Context
	cancel  context.CancelFunc
}

// NewServer creates a new PostgreSQL protocol server.
func NewServer(proxy MilvusProxy, config *Config) *Server {
	ctx, cancel := context.WithCancel(context.Background())
	return &Server{
		proxy:  proxy,
		config: config,
		ctx:    ctx,
		cancel: cancel,
	}
}

// Start begins accepting PostgreSQL connections on the given listener.
func (s *Server) Start(listener net.Listener) error {
	s.listener = listener
	log.Printf("PostgreSQL server starting on %s", listener.Addr().String())

	s.wg.Add(1)
	go s.acceptLoop()

	return nil
}

// acceptLoop continuously accepts new connections.
func (s *Server) acceptLoop() {
	defer s.wg.Done()

	for {
		if s.stopped.Load() {
			return
		}

		conn, err := s.listener.Accept()
		if err != nil {
			if s.stopped.Load() {
				return
			}
			log.Printf("Failed to accept connection: %v", err)
			continue
		}

		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.handleConnection(conn)
		}()
	}
}

// handleConnection processes a single client connection.
func (s *Server) handleConnection(netConn net.Conn) {
	defer netConn.Close()

	conn := NewConn(netConn, s.proxy, s.config)

	if err := conn.Serve(s.ctx); err != nil {
		log.Printf("Connection error: %v", err)
	}
}

// Stop gracefully shuts down the server.
func (s *Server) Stop() error {
	log.Println("PostgreSQL server stopping")

	s.stopped.Store(true)
	s.cancel()

	if s.listener != nil {
		s.listener.Close()
	}

	s.wg.Wait()

	log.Println("PostgreSQL server stopped")
	return nil
}

// GetName returns the server name for logging.
func (s *Server) GetName() string {
	return "PostgreSQLServer"
}
