package pgserver

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"net"
	"strings"
)

// Conn represents a single PostgreSQL client connection.
type Conn struct {
	netConn net.Conn
	reader  *bufio.Reader
	writer  *bufio.Writer
	proxy   ProxyComponent
	config  *Config

	// Connection state
	authenticated bool
	database      string
	user          string

	// Protocol state
	parameterStatus map[string]string
}

// NewConn creates a new connection handler.
func NewConn(netConn net.Conn, proxy ProxyComponent, config *Config) *Conn {
	return &Conn{
		netConn:         netConn,
		reader:          bufio.NewReader(netConn),
		writer:          bufio.NewWriter(netConn),
		proxy:           proxy,
		config:          config,
		parameterStatus: make(map[string]string),
	}
}

// Serve handles the connection lifecycle.
func (c *Conn) Serve(ctx context.Context) error {
	// Step 1: Handle startup and authentication
	if err := c.handleStartup(ctx); err != nil {
		return err
	}

	log.Printf("Client connected: user=%s database=%s", c.user, c.database)

	// Step 2: Enter query loop
	return c.queryLoop(ctx)
}

// handleStartup processes the initial connection handshake.
func (c *Conn) handleStartup(ctx context.Context) error {
	// Read startup message
	msg, err := c.readStartupMessage()
	if err != nil {
		return fmt.Errorf("read startup: %w", err)
	}

	// Handle SSL request if present
	if msg.IsSSLRequest() {
		// Send 'N' to decline SSL (for now)
		if err := c.writeSSLResponse(false); err != nil {
			return fmt.Errorf("write ssl response: %w", err)
		}
		// Read actual startup message
		msg, err = c.readStartupMessage()
		if err != nil {
			return fmt.Errorf("read startup after ssl: %w", err)
		}
	}

	// Extract connection parameters
	c.user = msg.GetParameter("user")
	c.database = msg.GetParameter("database")
	if c.database == "" {
		c.database = c.config.Database
	}

	// Authenticate (trust mode - always succeeds)
	if err := c.authenticate(ctx); err != nil {
		return fmt.Errorf("authenticate: %w", err)
	}

	// Send authentication OK
	if err := c.sendAuthenticationOK(); err != nil {
		return fmt.Errorf("send auth ok: %w", err)
	}

	// Send parameter status messages
	if err := c.sendParameterStatus(); err != nil {
		return fmt.Errorf("send param status: %w", err)
	}

	// Send backend key data (process ID and secret key)
	if err := c.sendBackendKeyData(); err != nil {
		return fmt.Errorf("send backend key: %w", err)
	}

	// Send ready for query
	if err := c.sendReadyForQuery(); err != nil {
		return fmt.Errorf("send ready: %w", err)
	}

	c.authenticated = true
	return nil
}

// queryLoop processes queries until the connection closes.
func (c *Conn) queryLoop(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		msg, err := c.readMessage()
		if err != nil {
			return err
		}

		switch msg.Type {
		case MsgQuery: // Simple query
			if err := c.handleSimpleQuery(ctx, msg); err != nil {
				// Log error but don't disconnect - send error to client
				log.Printf("Query error: %v", err)
			}
		case MsgTerminate: // Client disconnecting
			log.Printf("Client disconnected gracefully")
			return nil
		default:
			// Send error for unsupported message types
			c.sendError(fmt.Sprintf("Unsupported message type: %c", msg.Type))
		}
	}
}

// handleSimpleQuery executes a simple query.
func (c *Conn) handleSimpleQuery(ctx context.Context, msg *Message) error {
	query := msg.GetQueryString()
	log.Printf("Query: %s", query)

	// Handle some basic queries for testing
	switch {
	case matchesQuery(query, "SELECT 1"):
		return c.handleSelect1()
	case matchesQuery(query, "SELECT VERSION()"):
		return c.handleSelectVersion()
	default:
		// Extract statement type (first 1-2 words) for error message
		stmtType := getStatementType(query)
		return c.sendErrorWithCode(SQLStateFeatureNotSupported,
			fmt.Sprintf("%s is not yet supported", stmtType))
	}
}

// getStatementType extracts the statement type from a SQL query.
// e.g. "DROP TABLE foo" → "DROP TABLE", "INSERT INTO bar" → "INSERT INTO"
func getStatementType(query string) string {
	query = strings.TrimSpace(query)
	query = strings.TrimSuffix(query, ";")
	upper := strings.ToUpper(query)

	// Two-word statement types
	twoWord := []string{"DROP TABLE", "CREATE TABLE", "INSERT INTO", "DELETE FROM",
		"ALTER TABLE", "CREATE INDEX", "DROP INDEX", "SHOW TABLES"}
	for _, tw := range twoWord {
		if strings.HasPrefix(upper, tw) {
			return tw
		}
	}

	// Single-word fallback
	parts := strings.Fields(upper)
	if len(parts) > 0 {
		return parts[0]
	}
	return "UNKNOWN"
}

// matchesQuery does a simple case-insensitive query match.
func matchesQuery(query, pattern string) bool {
	// Trim spaces and semicolon, convert to uppercase
	query = strings.TrimSpace(query)
	query = strings.TrimSuffix(query, ";")
	query = strings.ToUpper(query)
	pattern = strings.ToUpper(pattern)
	return query == pattern
}

// handleSelect1 handles "SELECT 1" query.
func (c *Conn) handleSelect1() error {
	// Send RowDescription (1 column)
	fields := []FieldDescription{
		{
			Name:         "?column?",
			TableOID:     0,
			ColumnAttrNo: 0,
			TypeOID:      23, // int4
			TypeSize:     4,
			TypeModifier: -1,
			Format:       0, // text
		},
	}
	if err := WriteRowDescription(c.writer, fields); err != nil {
		return err
	}

	// Send DataRow (value: "1")
	if err := WriteDataRow(c.writer, [][]byte{[]byte("1")}); err != nil {
		return err
	}

	// Send CommandComplete
	if err := WriteCommandComplete(c.writer, "SELECT 1"); err != nil {
		return err
	}

	if err := c.writer.Flush(); err != nil {
		return err
	}

	return c.sendReadyForQuery()
}

// handleSelectVersion handles "SELECT VERSION()" query.
func (c *Conn) handleSelectVersion() error {
	// Send RowDescription
	fields := []FieldDescription{
		{
			Name:         "version",
			TableOID:     0,
			ColumnAttrNo: 0,
			TypeOID:      25, // text
			TypeSize:     -1,
			TypeModifier: -1,
			Format:       0,
		},
	}
	if err := WriteRowDescription(c.writer, fields); err != nil {
		return err
	}

	// Send DataRow
	version := "PostgreSQL 15.0 (Milvus PostgreSQL Interface)"
	if err := WriteDataRow(c.writer, [][]byte{[]byte(version)}); err != nil {
		return err
	}

	// Send CommandComplete
	if err := WriteCommandComplete(c.writer, "SELECT 1"); err != nil {
		return err
	}

	if err := c.writer.Flush(); err != nil {
		return err
	}

	return c.sendReadyForQuery()
}

// readStartupMessage reads the initial startup message from the client.
func (c *Conn) readStartupMessage() (*StartupMessage, error) {
	return ReadStartupMessage(c.reader)
}

// readMessage reads a single protocol message.
func (c *Conn) readMessage() (*Message, error) {
	return ReadMessage(c.reader)
}

// writeSSLResponse writes the SSL response byte.
func (c *Conn) writeSSLResponse(accept bool) error {
	err := WriteSSLResponse(c.writer, accept)
	if err != nil {
		return err
	}
	return c.writer.Flush()
}

// authenticate performs authentication (trust mode for now).
func (c *Conn) authenticate(ctx context.Context) error {
	// Trust mode - always allow
	_ = ctx
	return nil
}

// sendAuthenticationOK sends authentication success message.
func (c *Conn) sendAuthenticationOK() error {
	err := WriteAuthenticationOK(c.writer)
	if err != nil {
		return err
	}
	return c.writer.Flush()
}

// sendParameterStatus sends server parameter status messages.
func (c *Conn) sendParameterStatus() error {
	params := map[string]string{
		"server_version":             "15.0 (Milvus PostgreSQL Interface)",
		"server_encoding":            "UTF8",
		"client_encoding":            "UTF8",
		"DateStyle":                  "ISO, MDY",
		"TimeZone":                   "UTC",
		"integer_datetimes":          "on",
		"standard_conforming_strings": "on",
	}

	for name, value := range params {
		if err := WriteParameterStatus(c.writer, name, value); err != nil {
			return err
		}
	}
	return c.writer.Flush()
}

// sendBackendKeyData sends the backend key data (process ID and secret key).
func (c *Conn) sendBackendKeyData() error {
	err := WriteBackendKeyData(c.writer, 12345, 67890) // Dummy values
	if err != nil {
		return err
	}
	return c.writer.Flush()
}

// sendReadyForQuery sends the ready for query message.
func (c *Conn) sendReadyForQuery() error {
	err := WriteReadyForQuery(c.writer, TxStatusIdle)
	if err != nil {
		return err
	}
	return c.writer.Flush()
}

// sendError sends an error response with default SQLSTATE code.
func (c *Conn) sendError(message string) error {
	return c.sendErrorWithCode(SQLStateInternalError, message)
}

// sendErrorWithCode sends an error response with a specific SQLSTATE code.
func (c *Conn) sendErrorWithCode(code, message string) error {
	err := WriteErrorResponse(c.writer, &ErrorResponse{
		Severity: "ERROR",
		Code:     code,
		Message:  message,
	})
	if err != nil {
		return err
	}
	if err := c.sendReadyForQuery(); err != nil {
		return err
	}
	return nil
}

// sendEmptyQueryResponse sends an empty query response.
func (c *Conn) sendEmptyQueryResponse() error {
	err := WriteEmptyQueryResponse(c.writer)
	if err != nil {
		return err
	}
	return c.writer.Flush()
}

// sendResult sends query results to the client.
func (c *Conn) sendResult(result *QueryResult) error {
	if result == nil {
		return c.sendEmptyQueryResponse()
	}

	// TODO: implement proper result sending
	// 1. Send RowDescription
	// 2. Send DataRow for each row
	// 3. Send CommandComplete

	return c.sendEmptyQueryResponse()
}
