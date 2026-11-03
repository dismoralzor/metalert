package hash

import (
	"bytes"
	"testing"
)

func BenchmarkCompute(b *testing.B) {
	// Реалистичное тело батча ~4 КБ и непустой ключ.
	data := bytes.Repeat([]byte("abcd"), 1024)
	const key = "supersecretkey"

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Compute(data, key)
	}
}
