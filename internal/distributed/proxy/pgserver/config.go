package pgserver

import "time"

// Config holds PostgreSQL server configuration.
type Config struct {
	// Port is the port to listen on (default: 5432)
	Port int

	// Host is the address to bind to (default: 0.0.0.0)
	Host string

	// MaxConnections is the maximum number of concurrent connections
	MaxConnections int

	// ReadTimeout is the timeout for reading from connections
	ReadTimeout time.Duration

	// WriteTimeout is the timeout for writing to connections
	WriteTimeout time.Duration

	// AuthMethod is the authentication method (trust, md5, scram-sha-256)
	AuthMethod string

	// Database is the default database name (maps to Milvus database)
	Database string
}

// DefaultConfig returns the default configuration.
func DefaultConfig() *Config {
	return &Config{
		Port:           5432,
		Host:           "0.0.0.0",
		MaxConnections: 100,
		ReadTimeout:    30 * time.Second,
		WriteTimeout:   30 * time.Second,
		AuthMethod:     "trust", // No auth for development
		Database:       "default",
	}
}

// Validate checks if the configuration is valid.
func (c *Config) Validate() error {
	// TODO: implement validation
	return nil
}
