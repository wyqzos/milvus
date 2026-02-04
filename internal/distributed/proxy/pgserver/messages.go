package pgserver

// PostgreSQL protocol message types (frontend -> backend)
const (
	MsgQuery     = 'Q' // Simple query
	MsgParse     = 'P' // Parse (prepared statement)
	MsgBind      = 'B' // Bind parameters
	MsgExecute   = 'E' // Execute
	MsgDescribe  = 'D' // Describe
	MsgSync      = 'S' // Sync
	MsgTerminate = 'X' // Terminate connection
	MsgClose     = 'C' // Close statement/portal
	MsgFlush     = 'H' // Flush
)

// PostgreSQL protocol message types (backend -> frontend)
const (
	MsgAuthenticationOK        = 'R' // Authentication response
	MsgParameterStatus         = 'S' // Parameter status
	MsgBackendKeyData          = 'K' // Backend key data
	MsgReadyForQuery           = 'Z' // Ready for query
	MsgRowDescription          = 'T' // Row description
	MsgDataRow                 = 'D' // Data row
	MsgCommandComplete         = 'C' // Command complete
	MsgErrorResponse           = 'E' // Error
	MsgNoticeResponse          = 'N' // Notice
	MsgEmptyQueryResponse      = 'I' // Empty query
	MsgParseComplete           = '1' // Parse complete
	MsgBindComplete            = '2' // Bind complete
	MsgCloseComplete           = '3' // Close complete
	MsgNoData                  = 'n' // No data
	MsgParameterDescription    = 't' // Parameter description
)

// Transaction status indicators
const (
	TxStatusIdle   = 'I' // Not in transaction
	TxStatusInTx   = 'T' // In transaction
	TxStatusFailed = 'E' // In failed transaction
)

// SSL request code
const SSLRequestCode = 80877103

// Message represents a PostgreSQL protocol message.
type Message struct {
	Type    byte
	Length  int32
	Payload []byte
}

// GetQueryString extracts the query string from a Query message.
func (m *Message) GetQueryString() string {
	if m.Type != MsgQuery || len(m.Payload) == 0 {
		return ""
	}
	// Query string is null-terminated
	if m.Payload[len(m.Payload)-1] == 0 {
		return string(m.Payload[:len(m.Payload)-1])
	}
	return string(m.Payload)
}

// StartupMessage represents the initial connection message.
type StartupMessage struct {
	ProtocolVersion int32
	Parameters      map[string]string
}

// IsSSLRequest checks if this is an SSL request.
func (s *StartupMessage) IsSSLRequest() bool {
	return s.ProtocolVersion == SSLRequestCode
}

// GetParameter returns a startup parameter value.
func (s *StartupMessage) GetParameter(key string) string {
	if s.Parameters == nil {
		return ""
	}
	return s.Parameters[key]
}

// FieldDescription describes a single field in a result set.
type FieldDescription struct {
	Name         string
	TableOID     int32
	ColumnAttrNo int16
	TypeOID      int32
	TypeSize     int16
	TypeModifier int32
	Format       int16 // 0 = text, 1 = binary
}

// RowDescription describes the structure of result rows.
type RowDescription struct {
	Fields []FieldDescription
}

// DataRow represents a single row of data.
type DataRow struct {
	Values [][]byte // nil means NULL
}

// ErrorResponse represents a PostgreSQL error.
type ErrorResponse struct {
	Severity string // ERROR, FATAL, PANIC
	Code     string // SQLSTATE code
	Message  string
	Detail   string
	Hint     string
	Position int32
}

// Common SQLSTATE error codes
const (
	SQLStateSuccessfulCompletion = "00000"
	SQLStateSyntaxError          = "42601"
	SQLStateUndefinedTable       = "42P01"
	SQLStateUndefinedColumn      = "42703"
	SQLStateInvalidParameter     = "22023"
	SQLStateConnectionException  = "08000"
	SQLStateInternalError        = "XX000"
)
