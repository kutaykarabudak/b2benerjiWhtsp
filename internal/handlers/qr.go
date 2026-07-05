package handlers

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/shridarpatil/whatomate/internal/waqr"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

const (
	qrChannelType = "whatsapp_qr"
	qrAccountName = "qr"
)

// InitQR wires up the WhatsApp Web (whatsmeow) connector and its inbound
// pipeline. Inbound messages are persisted as a whatsapp_qr conversation and
// run through the keyword chatbot for auto-replies.
func (a *App) InitQR(ctx context.Context) error {
	sqlDB, err := a.DB.DB()
	if err != nil {
		return err
	}

	// Single-tenant deployment: bind the connector to the first (default) org.
	var org models.Organization
	if err := a.DB.Order("created_at asc").First(&org).Error; err != nil {
		return err
	}
	orgID := org.ID

	a.QR = waqr.NewManager(sqlDB, func(in waqr.Inbound) {
		a.handleQRInbound(orgID, in)
	})
	return a.QR.Init(ctx)
}

// handleQRInbound persists an inbound QR message and sends a keyword auto-reply.
func (a *App) handleQRInbound(orgID uuid.UUID, in waqr.Inbound) {
	now := time.Now()

	var contact models.Contact
	err := a.DB.Where("organization_id = ? AND phone_number = ? AND channel_type = ?",
		orgID, in.FromPhone, qrChannelType).First(&contact).Error
	if err != nil {
		contact = models.Contact{
			BaseModel:       models.BaseModel{ID: uuid.New()},
			OrganizationID:  orgID,
			PhoneNumber:     in.FromPhone,
			WhatsAppAccount: qrAccountName,
			ChannelType:     qrChannelType,
		}
	}
	if in.PushName != "" {
		contact.ProfileName = in.PushName
	}
	contact.LastMessageAt = &now
	contact.LastInboundAt = &now
	contact.LastMessagePreview = qrTruncate(in.Text, 100)
	contact.IsRead = false
	if err := a.DB.Save(&contact).Error; err != nil {
		a.Log.Error("QR: failed to save contact", "error", err)
		return
	}

	msg := models.Message{
		BaseModel:         models.BaseModel{ID: uuid.New()},
		OrganizationID:    orgID,
		WhatsAppAccount:   qrAccountName,
		ChannelType:       qrChannelType,
		ContactID:         contact.ID,
		WhatsAppMessageID: in.MessageID,
		Direction:         models.DirectionIncoming,
		MessageType:       models.MessageTypeText,
		Content:           in.Text,
		Status:            models.MessageStatusDelivered,
	}
	if err := a.DB.Create(&msg).Error; err != nil {
		a.Log.Error("QR: failed to save message", "error", err)
	}

	// Keyword chatbot auto-reply (channel-agnostic rules).
	reply := a.matchKeywordReply(orgID, in.Text)
	if reply == "" {
		return
	}
	if err := a.QR.SendText(context.Background(), in.FromPhone, reply); err != nil {
		a.Log.Error("QR: failed to send auto-reply", "error", err)
		return
	}
	out := models.Message{
		BaseModel:       models.BaseModel{ID: uuid.New()},
		OrganizationID:  orgID,
		WhatsAppAccount: qrAccountName,
		ChannelType:     qrChannelType,
		ContactID:       contact.ID,
		Direction:       models.DirectionOutgoing,
		MessageType:     models.MessageTypeText,
		Content:         reply,
		Status:          models.MessageStatusSent,
	}
	_ = a.DB.Create(&out).Error
	replyAt := time.Now()
	contact.LastMessageAt = &replyAt
	contact.LastMessagePreview = qrTruncate(reply, 100)
	_ = a.DB.Save(&contact).Error
}

// matchKeywordReply returns the text response of the first enabled keyword rule
// (highest priority) that matches the message.
func (a *App) matchKeywordReply(orgID uuid.UUID, text string) string {
	var rules []models.KeywordRule
	if err := a.DB.Where("organization_id = ? AND is_enabled = ?", orgID, true).
		Order("priority DESC, created_at ASC").Find(&rules).Error; err != nil {
		return ""
	}
	for _, rule := range rules {
		if rule.ResponseType != models.ResponseTypeText {
			continue
		}
		if keywordMatches(rule, text) {
			if body, ok := rule.ResponseContent["body"].(string); ok && body != "" {
				return body
			}
		}
	}
	return ""
}

func keywordMatches(rule models.KeywordRule, text string) bool {
	hay := text
	if !rule.CaseSensitive {
		hay = strings.ToLower(text)
	}
	for _, kw := range rule.Keywords {
		k := kw
		if !rule.CaseSensitive {
			k = strings.ToLower(kw)
		}
		switch rule.MatchType {
		case models.MatchTypeExact:
			if hay == k {
				return true
			}
		case models.MatchTypeStartsWith:
			if strings.HasPrefix(hay, k) {
				return true
			}
		case models.MatchTypeRegex:
			if re, err := regexp.Compile(kw); err == nil && re.MatchString(text) {
				return true
			}
		default: // contains
			if strings.Contains(hay, k) {
				return true
			}
		}
	}
	return false
}

func qrTruncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// sendQRMessage delivers an agent's outbound text over the WhatsApp Web
// connector and persists it as a whatsapp_qr message.
func (a *App) sendQRMessage(r *fastglue.Request, orgID, userID uuid.UUID, contact *models.Contact, body string) error {
	body = strings.TrimSpace(body)
	if body == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Message text is required", nil, "")
	}
	if a.QR == nil || a.QR.Status().State != "connected" {
		return r.SendErrorEnvelope(fasthttp.StatusServiceUnavailable, "WhatsApp Web bağlı değil. Yönetim → Kanallar'dan QR ile bağlanın.", nil, "")
	}
	if err := a.QR.SendText(context.Background(), contact.PhoneNumber, body); err != nil {
		a.Log.Error("QR: outbound send failed", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusBadGateway, "Mesaj gönderilemedi: "+err.Error(), nil, "")
	}

	msg := models.Message{
		BaseModel:       models.BaseModel{ID: uuid.New()},
		OrganizationID:  orgID,
		WhatsAppAccount: qrAccountName,
		ChannelType:     qrChannelType,
		ContactID:       contact.ID,
		Direction:       models.DirectionOutgoing,
		MessageType:     models.MessageTypeText,
		Content:         body,
		Status:          models.MessageStatusSent,
		SentByUserID:    &userID,
	}
	if err := a.DB.Create(&msg).Error; err != nil {
		a.Log.Error("QR: failed to save outbound message", "error", err)
	}

	now := time.Now()
	contact.LastMessageAt = &now
	contact.LastMessagePreview = qrTruncate(body, 100)
	contact.IsRead = true
	_ = a.DB.Save(contact).Error

	return r.SendEnvelope(a.buildMessagesResponse([]models.Message{msg})[0])
}

// QRStatus returns the current connector state (disconnected / qr / connected).
func (a *App) QRStatus(r *fastglue.Request) error {
	if _, _, err := a.getOrgAndUserID(r); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}
	if a.QR == nil {
		return r.SendEnvelope(map[string]any{"state": "disconnected"})
	}
	return r.SendEnvelope(a.QR.Status())
}

// QRConnect starts pairing and returns the current status (with a QR when ready).
func (a *App) QRConnect(r *fastglue.Request) error {
	if _, _, err := a.getOrgAndUserID(r); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}
	if a.QR == nil {
		return r.SendErrorEnvelope(fasthttp.StatusServiceUnavailable, "QR connector not available", nil, "")
	}
	if err := a.QR.Connect(context.Background()); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, err.Error(), nil, "")
	}
	return r.SendEnvelope(a.QR.Status())
}

// QRLogout unlinks the device.
func (a *App) QRLogout(r *fastglue.Request) error {
	if _, _, err := a.getOrgAndUserID(r); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}
	if a.QR != nil {
		_ = a.QR.Logout(context.Background())
	}
	return r.SendEnvelope(map[string]any{"state": "disconnected"})
}
