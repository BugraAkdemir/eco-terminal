package storage

import (
	"crypto/rand"
	"fmt"
	"time"
)

// NewID returns a random UUIDv4 string.
func NewID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err) // crypto/rand failing is unrecoverable
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// Now returns the current time as an RFC3339 UTC string (the format used in every timestamp column).
func Now() string { return time.Now().UTC().Format(time.RFC3339) }

// Time formats t as RFC3339 UTC.
func Time(t time.Time) string { return t.UTC().Format(time.RFC3339) }
