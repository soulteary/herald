package security

import "testing"

func TestDigester_StableAndNonReversible(t *testing.T) {
	d := NewDigester([]byte("pepper"))
	a := d.Digest("user@example.com")
	b := d.Digest("user@example.com")
	if a != b {
		t.Fatalf("digest not stable: %q != %q", a, b)
	}
	if a == "user@example.com" || contains(a, "user@example.com") {
		t.Fatalf("digest leaks raw value: %q", a)
	}
	if d.Digest("") != "" {
		t.Fatalf("empty input should return empty digest")
	}
}

func TestDigester_PepperChangesOutput(t *testing.T) {
	d1 := NewDigester([]byte("pepper-1"))
	d2 := NewDigester([]byte("pepper-2"))
	if d1.Digest("x") == d2.Digest("x") {
		t.Fatalf("different peppers must produce different digests")
	}
}

func TestDigester_PartsNoCollision(t *testing.T) {
	d := NewDigester([]byte("p"))
	if d.DigestParts("a", "bc") == d.DigestParts("ab", "c") {
		t.Fatalf("DigestParts must not collide on separator ambiguity")
	}
}

func contains(s, sub string) bool {
	return len(sub) > 0 && len(s) >= len(sub) && indexOf(s, sub) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
