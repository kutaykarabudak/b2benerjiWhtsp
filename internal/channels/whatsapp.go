package channels

import (
	"context"

	"github.com/shridarpatil/whatomate/pkg/whatsapp"
)

// waTextSender is the slice of *whatsapp.Client the WhatsApp adapter needs.
// Depending on an interface (rather than the concrete client) keeps the adapter
// unit-testable without performing real HTTP calls.
type waTextSender interface {
	SendTextMessage(ctx context.Context, account *whatsapp.Account, rcpt whatsapp.Recipient, text string, replyToMsgID ...string) (string, error)
}

// WhatsAppAdapter adapts the existing pkg/whatsapp client to the channel Adapter
// interface. It carries the connected account it sends on behalf of.
type WhatsAppAdapter struct {
	client  waTextSender
	account *whatsapp.Account
}

// NewWhatsAppAdapter builds an adapter for a specific WhatsApp account.
func NewWhatsAppAdapter(client waTextSender, account *whatsapp.Account) *WhatsAppAdapter {
	return &WhatsAppAdapter{client: client, account: account}
}

// Type reports the WhatsApp channel type.
func (a *WhatsAppAdapter) Type() Type { return TypeWhatsApp }

// SendText delivers a plain-text WhatsApp message, delegating to the underlying
// client. A non-empty ReplyToID is forwarded as the message being replied to.
func (a *WhatsAppAdapter) SendText(ctx context.Context, out OutboundText) (string, error) {
	rcpt := whatsapp.Recipient{Phone: out.Recipient.Phone, BSUID: out.Recipient.BSUID}
	if out.ReplyToID != "" {
		return a.client.SendTextMessage(ctx, a.account, rcpt, out.Text, out.ReplyToID)
	}
	return a.client.SendTextMessage(ctx, a.account, rcpt, out.Text)
}

// Ensure the adapter satisfies the Adapter interface at compile time.
var _ Adapter = (*WhatsAppAdapter)(nil)
