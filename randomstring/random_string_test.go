package randomstring

import "testing"

func BenchmarkRandomString(b *testing.B) {
	b.ResetTimer()
	b.RunParallel(func(p *testing.PB) {
		for p.Next(){
			_ = RandomString(32)
		}
	})
}
