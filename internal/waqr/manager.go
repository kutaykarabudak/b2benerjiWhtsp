// Package waqr provides an unofficial WhatsApp connector over the multi-device
// (WhatsApp Web) protocol via whatsmeow. It links to a phone that stays on the
// WhatsApp Business app (QR pairing) and is used ONLY for 1:1 chat + chatbot —
// never bulk messaging, which belongs on the official Cloud API to avoid bans.
package waqr

import (
	"context"
	"database/sql"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

// Inbound is a normalised incoming text message from the QR-linked number.
type Inbound struct {
	FromPhone string // sender's phone number (JID user part, digits only)
	PushName  string
	Text      string
	MessageID string
	Timestamp time.Time
}

// InboundHandler receives each inbound text. Implementations persist it and run
// the chatbot; the return value (if non-empty) is sent back as an auto-reply.
type InboundHandler func(in Inbound)

// Status is a snapshot of the connector state for the UI.
type Status struct {
	State string `json:"state"` // "disconnected", "qr", "connected"
	Phone string `json:"phone"` // linked number once paired
	QR    string `json:"qr"`    // current QR payload to render, when state == "qr"
}

// Manager owns a single whatsmeow client and its lifecycle.
type Manager struct {
	mu        sync.RWMutex
	container *sqlstore.Container
	client    *whatsmeow.Client
	onInbound InboundHandler
	log       waLog.Logger

	qr    string
	state string
	phone string
}

// NewManager builds a manager backed by the given *sql.DB (the app's Postgres).
func NewManager(db *sql.DB, onInbound InboundHandler) *Manager {
	return &Manager{
		onInbound: onInbound,
		log:       waLog.Stdout("waqr", "INFO", true),
		state:     "disconnected",
		container: sqlstore.NewWithDB(db, "postgres", waLog.Stdout("waqr-db", "WARN", true)),
	}
}

// Init runs the store migrations and, if a session already exists, reconnects.
func (m *Manager) Init(ctx context.Context) error {
	if err := m.container.Upgrade(ctx); err != nil {
		return err
	}
	device, err := m.container.GetFirstDevice(ctx)
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.client = whatsmeow.NewClient(device, m.log)
	m.client.AddEventHandler(m.handleEvent)
	loggedIn := m.client.Store.ID != nil
	if loggedIn {
		m.phone = m.client.Store.ID.User
	}
	m.mu.Unlock()

	if loggedIn {
		return m.client.Connect()
	}
	return nil
}

// Connect starts a pairing session. If already logged in it just ensures the
// socket is up; otherwise it opens a QR channel and publishes codes to Status.
func (m *Manager) Connect(ctx context.Context) error {
	m.mu.RLock()
	client := m.client
	m.mu.RUnlock()
	if client == nil {
		return nil
	}

	if client.Store.ID != nil {
		if !client.IsConnected() {
			return client.Connect()
		}
		return nil
	}

	qrChan, err := client.GetQRChannel(ctx)
	if err != nil {
		return err
	}
	if err := client.Connect(); err != nil {
		return err
	}
	go func() {
		for evt := range qrChan {
			switch evt.Event {
			case "code":
				m.setQR(evt.Code)
			case "success":
				m.setState("connected", "")
			default: // timeout, err, etc.
				m.setState("disconnected", "")
			}
		}
	}()
	return nil
}

// SendText sends a plain-text message to a phone number (digits, no +).
func (m *Manager) SendText(ctx context.Context, toPhone, text string) error {
	m.mu.RLock()
	client := m.client
	m.mu.RUnlock()
	if client == nil {
		return whatsmeow.ErrClientIsNil
	}
	jid := types.NewJID(toPhone, types.DefaultUserServer)
	_, err := client.SendMessage(ctx, jid, &waE2E.Message{Conversation: proto.String(text)})
	return err
}

// Logout unlinks the device (requires a new QR scan afterwards).
func (m *Manager) Logout(ctx context.Context) error {
	m.mu.RLock()
	client := m.client
	m.mu.RUnlock()
	if client == nil {
		return nil
	}
	err := client.Logout(ctx)
	m.setState("disconnected", "")
	return err
}

// Status returns the current connector snapshot.
func (m *Manager) Status() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return Status{State: m.state, Phone: m.phone, QR: m.qr}
}

func (m *Manager) setQR(code string) {
	m.mu.Lock()
	m.qr = code
	m.state = "qr"
	m.mu.Unlock()
}

func (m *Manager) setState(state, phone string) {
	m.mu.Lock()
	m.state = state
	if state != "qr" {
		m.qr = ""
	}
	if phone != "" {
		m.phone = phone
	}
	if state == "connected" && m.client != nil && m.client.Store.ID != nil {
		m.phone = m.client.Store.ID.User
	}
	m.mu.Unlock()
}

// handleEvent processes whatsmeow events: connection state + inbound messages.
func (m *Manager) handleEvent(evt any) {
	switch v := evt.(type) {
	case *events.Connected, *events.PairSuccess:
		m.setState("connected", "")
	case *events.LoggedOut:
		m.setState("disconnected", "")
	case *events.Message:
		m.handleMessage(v)
	}
}

func (m *Manager) handleMessage(v *events.Message) {
	if v.Info.IsFromMe || v.Info.IsGroup {
		return
	}
	text := v.Message.GetConversation()
	if text == "" {
		text = v.Message.GetExtendedTextMessage().GetText()
	}
	if text == "" {
		return
	}
	if m.onInbound != nil {
		m.onInbound(Inbound{
			FromPhone: v.Info.Sender.User,
			PushName:  v.Info.PushName,
			Text:      text,
			MessageID: v.Info.ID,
			Timestamp: v.Info.Timestamp,
		})
	}
}
