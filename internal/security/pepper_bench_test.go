package security

import "testing"

func BenchmarkDigest(b *testing.B) {
	d := NewDigester([]byte("benchmark-server-pepper"))
	b.ReportAllocs()
	for b.Loop() {
		if len(d.Digest("person@example.com")) != 64 {
			b.Fatal("invalid digest")
		}
	}
}

func BenchmarkDigestParts(b *testing.B) {
	d := NewDigester([]byte("benchmark-server-pepper"))
	b.ReportAllocs()
	for b.Loop() {
		if len(d.DigestParts("user-123", "email", "person@example.com", "login")) != 64 {
			b.Fatal("invalid digest")
		}
	}
}

func BenchmarkDigestParallel(b *testing.B) {
	d := NewDigester([]byte("benchmark-server-pepper"))
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if len(d.Digest("person@example.com")) != 64 {
				b.Error("invalid digest")
			}
		}
	})
}
