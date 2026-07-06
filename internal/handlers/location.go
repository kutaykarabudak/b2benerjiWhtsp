package handlers

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/shridarpatil/whatomate/pkg/whatsapp"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

// SendLocation sends a location pin to a contact (Cloud API channels only).
func (a *App) SendLocation(r *fastglue.Request) error {
	orgID, userID, err := a.getOrgAndUserID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}
	contactID, err := parsePathUUID(r, "id", "contact")
	if err != nil {
		return nil
	}

	var req struct {
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
		Name      string  `json:"name"`
		Address   string  `json:"address"`
	}
	if err := a.decodeRequest(r, &req); err != nil {
		return nil
	}
	if req.Latitude == 0 && req.Longitude == 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "latitude and longitude are required", nil, "")
	}

	var contact models.Contact
	q := a.scopeAssignedContact(a.DB.Where("id = ? AND organization_id = ?", contactID, orgID), userID, orgID)
	if err := q.First(&contact).Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, "Contact not found", nil, "")
	}
	if contact.ChannelType == qrChannelType {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Konum sadece Cloud API kanalında gönderilebilir.", nil, "")
	}

	account, err := a.resolveWhatsAppAccount(orgID, contact.WhatsAppAccount)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Failed to resolve WhatsApp account", nil, "")
	}

	rcpt := whatsapp.Recipient{Phone: contact.PhoneNumber, BSUID: contact.BSUID}
	wamid, err := a.WhatsApp.SendLocationMessage(context.Background(), account.ToWAAccount(), rcpt, req.Latitude, req.Longitude, req.Name, req.Address)
	if err != nil {
		a.Log.Error("Failed to send location", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusBadGateway, "Konum gönderilemedi: "+err.Error(), nil, "")
	}

	mapsURL := fmt.Sprintf("https://maps.google.com/?q=%f,%f", req.Latitude, req.Longitude)
	msg := models.Message{
		BaseModel:         models.BaseModel{ID: uuid.New()},
		OrganizationID:    orgID,
		WhatsAppAccount:   account.Name,
		ChannelType:       contact.ChannelType,
		ContactID:         contact.ID,
		WhatsAppMessageID: wamid,
		Direction:         models.DirectionOutgoing,
		MessageType:       models.MessageTypeLocation,
		Content:           mapsURL,
		Status:            models.MessageStatusSent,
		SentByUserID:      &userID,
		Metadata:          models.JSONB{"latitude": req.Latitude, "longitude": req.Longitude},
	}
	_ = a.DB.Create(&msg).Error

	now := time.Now()
	contact.LastMessageAt = &now
	contact.LastMessagePreview = "📍 Konum"
	contact.IsRead = true
	_ = a.DB.Save(&contact).Error

	return r.SendEnvelope(map[string]any{"message_id": msg.ID.String(), "maps_url": mapsURL})
}
