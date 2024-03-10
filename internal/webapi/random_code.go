package webapi

import (
	"crypto/rand"
)

const randomRuneCharset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*()"

func generateRandomBytes(count int) ([]byte, error) {
	result := make([]byte, count)
	charsetBytes := []byte(randomRuneCharset)
	for i := 0; i < count; i++ {
		// Generate a random index securely
		b := make([]byte, 1) // One byte is enough because we are indexing a byte array
		_, err := rand.Read(b)
		if err != nil {
			return nil, err
		}
		index := int(b[0]) % len(charsetBytes)

		// Get the byte at the calculated position
		result[i] = charsetBytes[index]
	}

	return result, nil
}
