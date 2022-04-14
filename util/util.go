package util

import "crypto/rand"

const randomStringOptions = "abcdefghijklmnopqrstuvwxyz0123456789"

func RandomString(n int) string {
	b := make([]byte, n)
	_, err := rand.Read(b)
	if err != nil {
		panic(err)
	}
	for i := range b {
		b[i] = randomStringOptions[b[i]%byte(len(randomStringOptions))]
	}
	return string(b)
}
