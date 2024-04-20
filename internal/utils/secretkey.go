package utils

import (
	"crypto/rc4"
	"encoding/hex"
	"fmt"
	"strings"
)

var slatKey = GetEnviron("LESSDB_SLATKEY")

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
func NewRandomName() (string, string, error) {
	return randomName(20)
}

// NewRandomKey create random key
func NewRandomKey() (string, string, error) {
	return randomName(64)
}

func VerifyKey(ciphertexthex string) (string, bool) {
	ciphertext, err := hex.DecodeString(ciphertexthex)
	if err != nil {
		return "", false
	}

	plaintext, err := rc4Open(ciphertext, []byte(slatKey))
	if err != nil {
		return "", false
	}

	nameArr := strings.Split(string(plaintext), "-")
	if len(nameArr) == 3 && nameArr[1] == slatKey {
		return nameArr[2], true
	}
	return "", false
}

func randomName(l int) (string, string, error) {
	name, err := stringWithCharset(l)
	if err != nil {
		return "", "", err
	}

	ciphertext, err := rc4Cipher(
		[]byte(fmt.Sprintf("%v-%v-%v", VERSION, slatKey, name)),
		[]byte(slatKey))
	return hex.EncodeToString(ciphertext), name, err
}
