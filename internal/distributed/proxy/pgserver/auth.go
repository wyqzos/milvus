package pgserver

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
)

// Authenticator handles PostgreSQL authentication.
type Authenticator struct {
	method string
}

// NewAuthenticator creates a new authenticator with the specified method.
func NewAuthenticator(method string) *Authenticator {
	return &Authenticator{
		method: method,
	}
}

// Authenticate performs authentication based on the configured method.
func (a *Authenticator) Authenticate(ctx context.Context, user, password, database string) error {
	switch a.method {
	case "trust":
		return a.authenticateTrust(ctx, user, database)
	case "md5":
		return a.authenticateMD5(ctx, user, password, database)
	case "scram-sha-256":
		return a.authenticateSCRAM(ctx, user, password, database)
	default:
		return fmt.Errorf("unsupported authentication method: %s", a.method)
	}
}

// authenticateTrust always succeeds (no authentication).
func (a *Authenticator) authenticateTrust(ctx context.Context, user, database string) error {
	// Trust authentication - always allow
	// TODO: optionally check if user/database exists in Milvus
	return nil
}

// authenticateMD5 performs MD5 password authentication.
func (a *Authenticator) authenticateMD5(ctx context.Context, user, password, database string) error {
	// TODO: implement MD5 authentication
	// 1. Generate random salt
	// 2. Send AuthenticationMD5Password message with salt
	// 3. Receive password from client
	// 4. Verify: md5(md5(password + user) + salt)
	return fmt.Errorf("MD5 authentication not implemented")
}

// authenticateSCRAM performs SCRAM-SHA-256 authentication.
func (a *Authenticator) authenticateSCRAM(ctx context.Context, user, password, database string) error {
	// TODO: implement SCRAM-SHA-256 authentication
	// This is the most secure method, used by PostgreSQL 10+
	return fmt.Errorf("SCRAM-SHA-256 authentication not implemented")
}

// ComputeMD5Password computes the MD5 password hash.
// Format: "md5" + md5(md5(password + user) + salt)
func ComputeMD5Password(user, password string, salt []byte) string {
	// Step 1: md5(password + user)
	h1 := md5.New()
	h1.Write([]byte(password))
	h1.Write([]byte(user))
	hash1 := hex.EncodeToString(h1.Sum(nil))

	// Step 2: md5(hash1 + salt)
	h2 := md5.New()
	h2.Write([]byte(hash1))
	h2.Write(salt)
	hash2 := hex.EncodeToString(h2.Sum(nil))

	return "md5" + hash2
}

// GenerateSalt generates a random 4-byte salt for MD5 authentication.
func GenerateSalt() ([]byte, error) {
	// TODO: implement proper random salt generation
	return []byte{0x01, 0x02, 0x03, 0x04}, nil
}
