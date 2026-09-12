package storage

import (
	"crypto/rand"
	"encoding/hex"
)

// NewID returns a random hex identifier for records.
func NewID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
