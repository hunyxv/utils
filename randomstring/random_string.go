package randomstring

import (
	"math/rand"
	"time"
)

const (
	digitsAsciiLetters = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ-."
	mark               = 1<<6 - 1 // 63
)

func RandomString(l int) []byte {
	rand.Seed(time.Now().UnixNano())
	randomBytes := make([]byte, 0, l)
	for i := 0; i < l; {
		num := rand.Int63()
		n := num & mark
		num = num >> 6
		randomBytes = append(randomBytes, digitsAsciiLetters[n])
		i++
		for num > 0 && i < l {
			n = num & mark
			num = num >> 6
			randomBytes = append(randomBytes, digitsAsciiLetters[n])
			i++
		}
	}
	return randomBytes
}
