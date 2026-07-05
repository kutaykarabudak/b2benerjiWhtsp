package channels

import (
	"context"
	"errors"
	"testing"

	"github.com/shridarpatil/whatomate/pkg/whatsapp"
)

// fakeWASender records the arguments SendTextMessage was called with and returns
// canned results, so we can assert the adapter delegates correctly.
type fakeWASender struct {
	gotAccount *whatsapp.Account
	gotRcpt    whatsapp.Recipient
	gotText    string
	gotReplyTo []string

	retID  string
	retErr error
}

func (f *fakeWASender) SendTextMessage(_ context.Context, account *whatsapp.Account, rcpt whatsapp.Recipient, text string, replyToMsgID ...string) (string, error) {
	f.gotAccount = account
	f.gotRcpt = rcpt
	f.gotText = text
	f.gotReplyTo = replyToMsgID
	return f.retID, f.retErr
}

func TestWhatsAppAdapter_SendText(t *testing.T) {
	acct := &whatsapp.Account{PhoneID: "phone-1"}
	fake := &fakeWASender{retID: "wamid.123"}
	ad := NewWhatsAppAdapter(fake, acct)

	if ad.Type() != TypeWhatsApp {
		t.Fatalf("Type() = %q, want %q", ad.Type(), TypeWhatsApp)
	}

	id, err := ad.SendText(context.Background(), OutboundText{
		Recipient: Recipient{Phone: "+905551112233", BSUID: "bsuid-9"},
		Text:      "merhaba",
	})
	if err != nil {
		t.Fatalf("SendText error: %v", err)
	}
	if id != "wamid.123" {
		t.Fatalf("id = %q, want wamid.123", id)
	}
	if fake.gotAccount != acct {
		t.Fatalf("account not forwarded")
	}
	if fake.gotRcpt.Phone != "+905551112233" || fake.gotRcpt.BSUID != "bsuid-9" {
		t.Fatalf("recipient not mapped: %+v", fake.gotRcpt)
	}
	if fake.gotText != "merhaba" {
		t.Fatalf("text = %q", fake.gotText)
	}
	if len(fake.gotReplyTo) != 0 {
		t.Fatalf("expected no reply-to, got %v", fake.gotReplyTo)
	}
}

func TestWhatsAppAdapter_SendText_Reply(t *testing.T) {
	fake := &fakeWASender{retID: "wamid.reply"}
	ad := NewWhatsAppAdapter(fake, &whatsapp.Account{})

	if _, err := ad.SendText(context.Background(), OutboundText{
		Recipient: Recipient{Phone: "+1"},
		Text:      "yanit",
		ReplyToID: "wamid.orig",
	}); err != nil {
		t.Fatalf("SendText error: %v", err)
	}
	if len(fake.gotReplyTo) != 1 || fake.gotReplyTo[0] != "wamid.orig" {
		t.Fatalf("reply-to not forwarded: %v", fake.gotReplyTo)
	}
}

func TestWhatsAppAdapter_SendText_Error(t *testing.T) {
	fake := &fakeWASender{retErr: errors.New("boom")}
	ad := NewWhatsAppAdapter(fake, &whatsapp.Account{})
	if _, err := ad.SendText(context.Background(), OutboundText{Text: "x"}); err == nil {
		t.Fatal("expected error to propagate")
	}
}

func TestRegistry(t *testing.T) {
	r := NewRegistry()
	if _, ok := r.Get("acme"); ok {
		t.Fatal("expected empty registry")
	}
	ad := NewWhatsAppAdapter(&fakeWASender{}, &whatsapp.Account{})
	r.Register("acme", ad)
	got, ok := r.Get("acme")
	if !ok || got != ad {
		t.Fatal("adapter not registered")
	}
	r.Remove("acme")
	if _, ok := r.Get("acme"); ok {
		t.Fatal("adapter not removed")
	}
}

func TestType_Valid(t *testing.T) {
	for _, tp := range []Type{TypeWhatsApp, TypeInstagram, TypeMessenger, TypeTelegram} {
		if !tp.Valid() {
			t.Fatalf("%q should be valid", tp)
		}
	}
	if Type("carrier-pigeon").Valid() {
		t.Fatal("unknown type should be invalid")
	}
}
