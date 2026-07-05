// Package channels provides a channel-agnostic messaging abstraction so the
// panel can present a unified inbox over multiple providers (WhatsApp today;
// Instagram, Messenger and Telegram planned). Each provider is wrapped by an
// Adapter that translates the common message shapes below into provider calls.
//
// This layer is intentionally thin: it wraps existing, tested provider clients
// (e.g. pkg/whatsapp) rather than reimplementing them.
package channels

import (
	"context"
	"sync"
	"time"
)

// Type identifies a messaging channel/provider.
type Type string

const (
	TypeWhatsApp  Type = "whatsapp"
	TypeInstagram Type = "instagram"
	TypeMessenger Type = "messenger"
	TypeTelegram  Type = "telegram"
)

// Valid reports whether t is a recognised channel type.
func (t Type) Valid() bool {
	switch t {
	case TypeWhatsApp, TypeInstagram, TypeMessenger, TypeTelegram:
		return true
	default:
		return false
	}
}

// Recipient identifies the destination user on a channel. Different providers
// key users differently, so this holds the union of identifiers; an adapter
// uses whichever fields apply to it.
//
//   - WhatsApp:  Phone (E.164), optionally BSUID
//   - Meta DM:   ExternalID (PSID for Messenger, IGSID for Instagram)
//   - Telegram:  ExternalID (chat id)
type Recipient struct {
	Phone      string
	BSUID      string
	ExternalID string
}

// OutboundText is the common-denominator outbound message: plain text to a
// recipient, optionally in reply to a previous message. Richer message types
// (media, templates, interactive) are added per-adapter as channels gain them.
type OutboundText struct {
	Recipient Recipient
	Text      string
	ReplyToID string // provider message id being replied to, if any
}

// InboundMessage is a provider webhook payload normalised to a
// channel-agnostic shape the inbox can persist and display.
type InboundMessage struct {
	ChannelType       Type
	AccountRef        string // which connected account received it (account name/id)
	ExternalUserID    string // sender's id on the channel (phone, PSID, IGSID, chat id)
	SenderName        string
	ProviderMessageID string
	MessageType       string // text, image, video, audio, document, ...
	Text              string
	MediaURL          string
	Timestamp         time.Time
	Raw               map[string]any // original provider fields, for adapters that need more
}

// Adapter is the send side of a channel. Every connected account has an Adapter.
type Adapter interface {
	// Type reports which channel this adapter speaks.
	Type() Type
	// SendText delivers a plain-text message and returns the provider message id.
	SendText(ctx context.Context, out OutboundText) (providerMessageID string, err error)
}

// InboundParser is implemented by adapters whose provider delivers messages via
// webhook. Kept separate from Adapter because the send and receive sides can be
// wired independently.
type InboundParser interface {
	// ParseInbound normalises a raw provider webhook body into inbound messages.
	ParseInbound(body []byte) ([]InboundMessage, error)
}

// Registry maps a connected-account reference to its Adapter. The inbox resolves
// the adapter for a conversation's account here before sending.
type Registry struct {
	mu       sync.RWMutex
	adapters map[string]Adapter
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{adapters: make(map[string]Adapter)}
}

// Register associates an adapter with a unique account reference. A repeat
// reference replaces the previous adapter (e.g. after a token refresh).
func (r *Registry) Register(accountRef string, a Adapter) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.adapters[accountRef] = a
}

// Get returns the adapter registered for accountRef.
func (r *Registry) Get(accountRef string) (Adapter, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.adapters[accountRef]
	return a, ok
}

// Remove drops the adapter for accountRef, if present.
func (r *Registry) Remove(accountRef string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.adapters, accountRef)
}
