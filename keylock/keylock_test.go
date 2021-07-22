package keylock

import (
	"context"
	"math/rand"
	"testing"
	"time"
)


const (
	digitsAsciiLetters = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ-."
	mark               = 1<<6 - 1 // 63
)

func randomString(l int) []byte {
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



func BenchmarkLock(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	kl := NewKeyLock(ctx)

	keys := make([]string, 0, 10000)
	for i := 0; i < 10000; i++ {
		keys = append(keys, string(randomString(32)))
	}

	//b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(p *testing.PB) {
		for p.Next() {
			key := keys[rand.Intn(10000)]
			kl.Lock(key)
			kl.Unlock(key)
		}
	})
}

func TestLock(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	kl := NewKeyLock(ctx)

	key := string(randomString(32))
	kl.Lock(key)
	go func(){
		kl.Lock(key)
		t.Log("----------------")
	}()
	time.Sleep(1 * time.Second)
	t.Log("+++++++++++++++")
	kl.Unlock(key)
	time.Sleep(1 * time.Second)
}
