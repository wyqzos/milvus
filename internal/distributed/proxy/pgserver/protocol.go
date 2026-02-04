package pgserver

import (
	"encoding/binary"
	"fmt"
	"io"
)

// Protocol handles reading and writing PostgreSQL wire protocol messages.

// ReadStartupMessage reads the initial startup message from the client.
func ReadStartupMessage(r io.Reader) (*StartupMessage, error) {
	// Read message length (4 bytes, includes itself)
	var length int32
	if err := binary.Read(r, binary.BigEndian, &length); err != nil {
		return nil, fmt.Errorf("failed to read startup message length: %w", err)
	}

	if length < 8 || length > 10000 {
		return nil, fmt.Errorf("invalid startup message length: %d", length)
	}

	// Read protocol version (4 bytes)
	var version int32
	if err := binary.Read(r, binary.BigEndian, &version); err != nil {
		return nil, fmt.Errorf("failed to read protocol version: %w", err)
	}

	msg := &StartupMessage{
		ProtocolVersion: version,
		Parameters:      make(map[string]string),
	}

	// Check for SSL request
	if version == SSLRequestCode {
		return msg, nil
	}

	// Read parameters (key=value pairs, null-terminated)
	remaining := int(length) - 8
	if remaining > 0 {
		params := make([]byte, remaining)
		if _, err := io.ReadFull(r, params); err != nil {
			return nil, fmt.Errorf("failed to read startup parameters: %w", err)
		}

		// Parse null-terminated key-value pairs
		msg.Parameters = parseStartupParameters(params)
	}

	return msg, nil
}

// parseStartupParameters parses null-terminated key-value pairs.
func parseStartupParameters(data []byte) map[string]string {
	params := make(map[string]string)
	var key, value string
	isKey := true

	start := 0
	for i, b := range data {
		if b == 0 {
			s := string(data[start:i])
			if isKey {
				key = s
				isKey = false
			} else {
				value = s
				if key != "" {
					params[key] = value
				}
				isKey = true
			}
			start = i + 1
		}
	}

	return params
}

// ReadMessage reads a single protocol message.
func ReadMessage(r io.Reader) (*Message, error) {
	// Read message type (1 byte)
	var msgType [1]byte
	if _, err := io.ReadFull(r, msgType[:]); err != nil {
		return nil, err
	}

	// Read message length (4 bytes, includes itself but not type)
	var length int32
	if err := binary.Read(r, binary.BigEndian, &length); err != nil {
		return nil, fmt.Errorf("failed to read message length: %w", err)
	}

	if length < 4 {
		return nil, fmt.Errorf("invalid message length: %d", length)
	}

	// Read payload
	payloadLen := int(length) - 4
	payload := make([]byte, payloadLen)
	if payloadLen > 0 {
		if _, err := io.ReadFull(r, payload); err != nil {
			return nil, fmt.Errorf("failed to read message payload: %w", err)
		}
	}

	return &Message{
		Type:    msgType[0],
		Length:  length,
		Payload: payload,
	}, nil
}

// WriteMessage writes a protocol message to the writer.
func WriteMessage(w io.Writer, msgType byte, payload []byte) error {
	// Message type
	if _, err := w.Write([]byte{msgType}); err != nil {
		return err
	}

	// Length (includes itself, 4 bytes)
	length := int32(len(payload) + 4)
	if err := binary.Write(w, binary.BigEndian, length); err != nil {
		return err
	}

	// Payload
	if len(payload) > 0 {
		if _, err := w.Write(payload); err != nil {
			return err
		}
	}

	return nil
}

// WriteAuthenticationOK writes an authentication success message.
func WriteAuthenticationOK(w io.Writer) error {
	payload := make([]byte, 4)
	binary.BigEndian.PutUint32(payload, 0) // 0 = AuthenticationOK
	return WriteMessage(w, MsgAuthenticationOK, payload)
}

// WriteParameterStatus writes a parameter status message.
func WriteParameterStatus(w io.Writer, name, value string) error {
	payload := make([]byte, 0, len(name)+len(value)+2)
	payload = append(payload, []byte(name)...)
	payload = append(payload, 0)
	payload = append(payload, []byte(value)...)
	payload = append(payload, 0)
	return WriteMessage(w, MsgParameterStatus, payload)
}

// WriteReadyForQuery writes a ready for query message.
func WriteReadyForQuery(w io.Writer, txStatus byte) error {
	return WriteMessage(w, MsgReadyForQuery, []byte{txStatus})
}

// WriteRowDescription writes a row description message.
func WriteRowDescription(w io.Writer, fields []FieldDescription) error {
	// TODO: implement proper encoding
	return nil
}

// WriteDataRow writes a data row message.
func WriteDataRow(w io.Writer, values [][]byte) error {
	// TODO: implement proper encoding
	return nil
}

// WriteCommandComplete writes a command complete message.
func WriteCommandComplete(w io.Writer, tag string) error {
	payload := append([]byte(tag), 0)
	return WriteMessage(w, MsgCommandComplete, payload)
}

// WriteErrorResponse writes an error response message.
func WriteErrorResponse(w io.Writer, err *ErrorResponse) error {
	// Build error response with field codes
	var payload []byte

	// Severity (S)
	payload = append(payload, 'S')
	payload = append(payload, []byte(err.Severity)...)
	payload = append(payload, 0)

	// SQLSTATE code (C)
	payload = append(payload, 'C')
	payload = append(payload, []byte(err.Code)...)
	payload = append(payload, 0)

	// Message (M)
	payload = append(payload, 'M')
	payload = append(payload, []byte(err.Message)...)
	payload = append(payload, 0)

	// Terminator
	payload = append(payload, 0)

	return WriteMessage(w, MsgErrorResponse, payload)
}

// WriteSSLResponse writes the SSL response byte.
func WriteSSLResponse(w io.Writer, accept bool) error {
	var response byte = 'N' // Decline
	if accept {
		response = 'S' // Accept
	}
	_, err := w.Write([]byte{response})
	return err
}
