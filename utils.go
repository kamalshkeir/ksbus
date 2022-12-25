package ksbus

import (
	"crypto/rand"
	"encoding/base64"
)

func GenerateRandomString(s int) string {
	b, _ := GenerateRandomBytes(s)
	return base64.URLEncoding.EncodeToString(b)
}

// Generate Random Bytes
func GenerateRandomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	// err == nil only if len(b) == n
	if err != nil {
		return nil, err
	}

	return b, nil
}
