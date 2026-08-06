package handlers

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/shridarpatil/whatomate/pkg/whatsapp"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

// SendContactCard sends a saved-contact card over the WhatsApp Cloud API.
func (a *App) SendContactCard(r *fastglue.Request) error {
	orgID, userID, err := a.getOrgAndUserID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}
	contactID, err := parsePathUUID(r, "id", "contact")
	if err != nil {
		return nil
	}
	var req struct {
		Name    string `json:"name"`
		Phone   string `json:"phone"`
		Email   string `json:"email"`
		Company string `json:"company"`
	}
	if err := a.decodeRequest(r, &req); err != nil {
		return nil
	}
	req.Name, req.Phone = strings.TrimSpace(req.Name), strings.TrimSpace(req.Phone)
	if req.Name == "" || req.Phone == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Ad ve telefon zorunludur.", nil, "")
	}

	var contact models.Contact
	q := a.scopeAssignedContact(a.DB.Where("id = ? AND organization_id = ?", contactID, orgID), userID, orgID)
	if err := q.First(&contact).Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, "Contact not found", nil, "")
	}
	if !requireAgentServiceWindow(r, &contact) {
		return nil
	}
	if contact.ChannelType == qrChannelType {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Kişi kartı sadece Cloud API kanalında gönderilebilir.", nil, "")
	}
	account, err := a.resolveWhatsAppAccount(orgID, contact.WhatsAppAccount)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Failed to resolve WhatsApp account", nil, "")
	}
	rcpt := whatsapp.Recipient{Phone: contact.PhoneNumber, BSUID: contact.BSUID}
	wamid, err := a.WhatsApp.SendContactMessage(context.Background(), account.ToWAAccount(), rcpt, req.Name, req.Phone, strings.TrimSpace(req.Email), strings.TrimSpace(req.Company))
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadGateway, "Kişi kartı gönderilemedi: "+err.Error(), nil, "")
	}

	msg := models.Message{
		BaseModel:         models.BaseModel{ID: uuid.New()},
		OrganizationID:    orgID,
		WhatsAppAccount:   account.Name,
		ChannelType:       contact.ChannelType,
		ContactID:         contact.ID,
		WhatsAppMessageID: wamid,
		Direction:         models.DirectionOutgoing,
		MessageType:       models.MessageTypeContact,
		Content:           "👤 " + req.Name + "\n" + req.Phone,
		Status:            models.MessageStatusSent,
		SentByUserID:      &userID,
		Metadata:          models.JSONB{"name": req.Name, "phone": req.Phone, "email": req.Email, "company": req.Company},
	}
	_ = a.DB.Create(&msg).Error
	now := time.Now()
	contact.LastMessageAt = &now
	contact.LastMessagePreview = "👤 " + req.Name
	contact.IsRead = true
	_ = a.DB.Save(&contact).Error
	return r.SendEnvelope(map[string]any{"message_id": msg.ID.String()})
}
