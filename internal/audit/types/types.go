package types

// EventType represents the type of audit event
type EventType string

const (
	EventChallengeCreated   EventType = "challenge_created"
	EventChallengeVerified  EventType = "challenge_verified"
	EventChallengeRevoked   EventType = "challenge_revoked"
	EventSendSuccess        EventType = "send_success"
	EventSendFailed         EventType = "send_failed"
	EventVerificationFailed EventType = "verification_failed"
)

// AuditRecord represents an audit log entry
// This type is defined in a separate package to avoid import cycles
type AuditRecord struct {
	EventType         EventType `json:"event_type"`
	ChallengeID       string    `json:"challenge_id,omitempty"`
	UserID            string    `json:"user_id,omitempty"`
	Channel           string    `json:"channel,omitempty"`
	Destination       string    `json:"destination,omitempty"` // May be masked
	Purpose           string    `json:"purpose,omitempty"`
	Result            string    `json:"result"` // "success" | "failure"
	Reason            string    `json:"reason,omitempty"`
	Provider          string    `json:"provider,omitempty"`
	ProviderMessageID string    `json:"provider_message_id,omitempty"`
	IP                string    `json:"ip,omitempty"`
	Timestamp         int64     `json:"timestamp"`
}
