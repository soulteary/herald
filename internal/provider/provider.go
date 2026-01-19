package provider

import (
	"context"
	"fmt"
)

// Channel represents the delivery channel
type Channel string

const (
	ChannelSMS   Channel = "sms"
	ChannelEmail Channel = "email"
)

// Message represents a message to be sent
type Message struct {
	To      string
	Subject string
	Body    string
	Code    string // Verification code
}

// Provider is the interface for message providers
type Provider interface {
	// Send sends a message via the provider
	Send(ctx context.Context, msg *Message) error

	// Channel returns the channel type this provider supports
	Channel() Channel

	// Validate checks if the provider is properly configured
	Validate() error
}

// Registry manages available providers
type Registry struct {
	providers map[Channel]Provider
}

// NewRegistry creates a new provider registry
func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[Channel]Provider),
	}
}

// Register registers a provider
func (r *Registry) Register(provider Provider) error {
	if err := provider.Validate(); err != nil {
		return fmt.Errorf("provider validation failed: %w", err)
	}
	r.providers[provider.Channel()] = provider
	return nil
}

// GetProvider returns a provider for the given channel
func (r *Registry) GetProvider(channel Channel) (Provider, error) {
	provider, ok := r.providers[channel]
	if !ok {
		return nil, fmt.Errorf("no provider registered for channel: %s", channel)
	}
	return provider, nil
}

// Send sends a message using the appropriate provider
func (r *Registry) Send(ctx context.Context, channel Channel, msg *Message) error {
	provider, err := r.GetProvider(channel)
	if err != nil {
		return err
	}
	return provider.Send(ctx, msg)
}
