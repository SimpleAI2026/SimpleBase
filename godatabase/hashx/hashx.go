// Package hashx provides hexadecimal SHA-256 digests.
package hashx

import (
	"crypto/sha256"
	"encoding/hex"
)

// Sum returns the full SHA-256 digest of data.
func Sum(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// SumString returns the full SHA-256 digest of s.
func SumString(s string) string {
	return Sum([]byte(s))
}

// Short returns the hexadecimal encoding of the first n digest bytes.
// It returns an empty string for n <= 0 and caps n at 32.
func Short(data []byte, n int) string {
	if n <= 0 {
		return ""
	}
	sum := sha256.Sum256(data)
	if n > len(sum) {
		n = len(sum)
	}
	return hex.EncodeToString(sum[:n])
}
