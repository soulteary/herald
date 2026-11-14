package handlers

// maxChallengeIDBytes bounds a caller-supplied challenge ID. Generated IDs are
// 25 bytes ("ch_" and 22 characters); the bound leaves room for longer
// identifiers such as a 36-character UUID.
const maxChallengeIDBytes = 64

// validChallengeID reports whether id could be a challenge ID. Every release
// has generated "ch_" followed by 22 unpadded URL-safe base64 characters, so
// an ID outside that alphabet, or far longer than one, never names a
// challenge. Such a value is refused before it is used as a Redis key suffix,
// stored in an audit record or attached to a span. The prefix and exact
// length are deliberately not enforced.
func validChallengeID(id string) bool {
	if id == "" || len(id) > maxChallengeIDBytes {
		return false
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '-', c == '_':
		default:
			return false
		}
	}
	return true
}
