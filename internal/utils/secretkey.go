package utils

import (
	"crypto/rc4"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

var slatNonce = []byte("OXFFFFFFFFFF")
var slatKey = GetEnviron("TAMI_SLATKEY")

func rc4Cipher(plaintext, key []byte) ([]byte, error) {
	if len(key) < 32 {
		zerokey := make([]byte, 32-len(key))
		key = append(key, zerokey...)
	}

	cipher, err := rc4.NewCipher(key)
	if err != nil {
		return nil, err
	}

	encrypted := make([]byte, len(plaintext))
	cipher.XORKeyStream(encrypted, plaintext)
	return encrypted, nil
}

func rc4Open(encrypted, key []byte) ([]byte, error) {
	if len(key) < 32 {
		zerokey := make([]byte, 32-len(key))
		key = append(key, zerokey...)
	}

	cipher, err := rc4.NewCipher(key)
	if err != nil {
		return nil, err
	}

	decrypted := make([]byte, len(encrypted))
	cipher.XORKeyStream(decrypted, encrypted)

	return decrypted, nil
}

// NewRandomName create random name
func NewRandomName() string {
	name, err := randomName(12)
	if err != nil {
		return fmt.Sprintln("default.%v", time.Now().UnixNano())
	}
	return name
}

// NewRandomKey create random key
func NewRandomKey() string {
	key, err := randomName(32)
	if err != nil {
		return fmt.Sprintln("default.%v", time.Now().UnixNano())
	}
	return key
}

func VerifyName(ciphertexthex string) bool {
	ciphertext, err := hex.DecodeString(ciphertexthex)
	if err != nil {
		return false
	}
	plaintext, err := rc4Open(ciphertext, []byte(slatKey))
	if err != nil {
		return false
	}
	return strings.Contains(string(plaintext), slatKey)
}

func randomName(l int) (string, error) {
	message, err := stringWithCharset(l)
	if err != nil {
		message = u.String()
	}
	ciphertext, err := rc4Cipher(
		[]byte(fmt.Sprintf("%v.%v", slatKey, message)),
		[]byte(slatKey))
	return hex.EncodeToString(ciphertext), err
}
