package utils

import (
	"crypto/rand"
	"math/big"
)

const VERSION = "v1.0"
const EmptyNil = "nil"
const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

var charsetBig = big.NewInt(int64(len(charset)))

func stringWithCharset(length int) (string, error) {
	b := make([]byte, length)
	for i := range b {
		num, err := rand.Int(rand.Reader, charsetBig)
		if err != nil {
			return "", err
		}
		b[i] = charset[num.Int64()]
	}
	return string(b), nil
}
