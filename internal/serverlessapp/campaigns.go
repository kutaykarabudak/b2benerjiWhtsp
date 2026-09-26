package serverlessapp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	appcrypto "github.com/shridarpatil/whatomate/internal/crypto"
	"github.com/shridarpatil/whatomate/internal/firestorestore"
	"github.com/shridarpatil/whatomate/pkg/whatsapp"
	"github.com/valyala/fasthttp"
)

type campaignRequest struct {
	Name            string     `json:"name"`
	WhatsAppAccount string     `json:"whatsapp_account"`
	TemplateID      string     `json:"template_id"`
	ScheduledAt     *time.Time `json:"scheduled_at"`
}

func (a *App) ListCampaigns(ctx *fasthttp.RequestCtx) {
	claims, ok := a.authorize(ctx)
	if !ok {
		return
	}
	campaigns, err := a.administration().ListCampaigns(context.Background(), claims.OrganizationID.String())
	if err != nil {
		writeError(ctx, 500, "Failed to list campaigns")
		return
	}
	statusFilter := strings.ToLower(string(ctx.QueryArgs().Peek("status")))
	search := strings.ToLower(strings.TrimSpace(string(ctx.QueryArgs().Peek("search"))))
	filtered := make([]firestorestore.Campaign, 0, len(campaigns))
	for _, campaign := range campaigns {
		if statusFilter != "" && statusFilter != "all" && strings.ToLower(campaign.Status) != statusFilter {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(campaign.Name), search) {
			continue
		}
		filtered = append(filtered, campaign)
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].CreatedAt.After(filtered[j].CreatedAt) })
	limit, page := parseLimit(ctx, 20), 1
	if parsed, parseErr := strconv.Atoi(string(ctx.QueryArgs().Peek("page"))); parseErr == nil && parsed > 0 {
		page = parsed
	}
	start := (page - 1) * limit
	if start > len(filtered) {
		start = len(filtered)
	}
	end := start + limit
	if end > len(filtered) {
		end = len(filtered)
	}
	writeData(ctx, 200, map[string]any{"campaigns": filtered[start:end], "total": len(filtered), "page": page, "limit": limit})
}

func (a *App) CreateCampaign(ctx *fasthttp.RequestCtx) {
	claims, ok := a.authorize(ctx)
	if !ok {
		return
	}
	var request campaignRequest
	if !decodeJSON(ctx, &request) || strings.TrimSpace(request.Name) == "" || request.WhatsAppAccount == "" || request.TemplateID == "" {
		writeError(ctx, 400, "name, whatsapp_account and template_id are required")
		return
	}
	requestCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	template, err := a.store.Template(requestCtx, claims.OrganizationID.String(), request.TemplateID)
	if err != nil {
		writeError(ctx, 400, "Template not found")
		return
	}
	if template.WhatsAppAccount != request.WhatsAppAccount {
		writeError(ctx, 400, "Template does not belong to the selected WhatsApp account")
		return
	}
	if _, err := a.store.ResolveWhatsAppAccount(requestCtx, claims.OrganizationID.String(), request.WhatsAppAccount); err != nil {
		writeError(ctx, 400, "WhatsApp account not found")
		return
	}
	now := a.now().UTC()
	campaign := firestorestore.Campaign{ID: uuid.NewString(), OrganizationID: claims.OrganizationID.String(), WhatsAppAccount: request.WhatsAppAccount, Name: strings.TrimSpace(request.Name), TemplateID: template.ID, TemplateName: template.Name, Status: "draft", ScheduledAt: request.ScheduledAt, CreatedByID: claims.UserID.String(), UpdatedByID: claims.UserID.String(), CreatedAt: now, UpdatedAt: now}
	if err := a.administration().PutCampaign(requestCtx, campaign); err != nil {
		writeError(ctx, 500, "Failed to create campaign")
		return
	}
	writeData(ctx, 200, campaign)
}

func (a *App) GetCampaign(ctx *fasthttp.RequestCtx) {
	claims, ok := a.authorize(ctx)
	if !ok {
		return
	}
	campaign, err := a.administration().Campaign(context.Background(), claims.OrganizationID.String(), pathValue(ctx, "id"))
	if err != nil {
		writeError(ctx, 404, "Campaign not found")
		return
	}
	writeData(ctx, 200, campaign)
}

func (a *App) UpdateCampaign(ctx *fasthttp.RequestCtx) {
	claims, ok := a.authorize(ctx)
	if !ok {
		return
	}
	campaign, err := a.administration().Campaign(context.Background(), claims.OrganizationID.String(), pathValue(ctx, "id"))
	if err != nil {
		writeError(ctx, 404, "Campaign not found")
		return
	}
	if campaign.Status != "draft" {
		writeError(ctx, 400, "Only draft campaigns can be updated")
		return
	}
	var request campaignRequest
	if !decodeJSON(ctx, &request) {
		writeError(ctx, 400, "Invalid request")
		return
	}
	if strings.TrimSpace(request.Name) != "" {
		campaign.Name = strings.TrimSpace(request.Name)
	}
	if request.WhatsAppAccount != "" {
		campaign.WhatsAppAccount = request.WhatsAppAccount
	}
	if request.TemplateID != "" {
		template, templateErr := a.store.Template(context.Background(), claims.OrganizationID.String(), request.TemplateID)
		if templateErr != nil {
			writeError(ctx, 400, "Template not found")
			return
		}
		if template.WhatsAppAccount != campaign.WhatsAppAccount {
			writeError(ctx, 400, "Template does not belong to the selected WhatsApp account")
			return
		}
		campaign.TemplateID = template.ID
		campaign.TemplateName = template.Name
	}
	campaign.ScheduledAt = request.ScheduledAt
	campaign.UpdatedByID = claims.UserID.String()
	campaign.UpdatedAt = a.now().UTC()
	if err := a.administration().PutCampaign(context.Background(), *campaign); err != nil {
		writeError(ctx, 500, "Failed to update campaign")
		return
	}
	writeData(ctx, 200, campaign)
}

func (a *App) DeleteCampaign(ctx *fasthttp.RequestCtx) {
	claims, ok := a.authorize(ctx)
	if !ok {
		return
	}
	campaign, err := a.administration().Campaign(context.Background(), claims.OrganizationID.String(), pathValue(ctx, "id"))
	if err != nil {
		writeError(ctx, 404, "Campaign not found")
		return
	}
	if campaign.Status == "processing" || campaign.Status == "queued" {
		writeError(ctx, 400, "Cannot delete a running campaign")
		return
	}
	if err := a.administration().DeleteCampaign(context.Background(), *campaign, a.now().UTC()); err != nil {
		writeError(ctx, 500, "Failed to delete campaign")
		return
	}
	writeData(ctx, 200, map[string]string{"message": "Campaign deleted successfully"})
}

func (a *App) ListCampaignRecipients(ctx *fasthttp.RequestCtx) {
	claims, ok := a.authorize(ctx)
	if !ok {
		return
	}
	campaignID := pathValue(ctx, "id")
	if _, err := a.administration().Campaign(context.Background(), claims.OrganizationID.String(), campaignID); err != nil {
		writeError(ctx, 404, "Campaign not found")
		return
	}
	recipients, err := a.administration().ListCampaignRecipients(context.Background(), claims.OrganizationID.String(), campaignID)
	if err != nil {
		writeError(ctx, 500, "Failed to list recipients")
		return
	}
	sort.Slice(recipients, func(i, j int) bool { return recipients[i].CreatedAt.Before(recipients[j].CreatedAt) })
	writeData(ctx, 200, map[string]any{"recipients": recipients, "total": len(recipients)})
}

func (a *App) ImportCampaignRecipients(ctx *fasthttp.RequestCtx) {
	claims, ok := a.authorize(ctx)
	if !ok {
		return
	}
	campaignID := pathValue(ctx, "id")
	campaign, err := a.administration().Campaign(context.Background(), claims.OrganizationID.String(), campaignID)
	if err != nil {
		writeError(ctx, 404, "Campaign not found")
		return
	}
	if campaign.Status != "draft" {
		writeError(ctx, 400, "Recipients can only be added to draft campaigns")
		return
	}
	var request struct {
		Recipients []struct {
			PhoneNumber    string         `json:"phone_number"`
			RecipientName  string         `json:"recipient_name"`
			TemplateParams map[string]any `json:"template_params"`
			HeaderParams   map[string]any `json:"header_params"`
		} `json:"recipients"`
	}
	if !decodeJSON(ctx, &request) || len(request.Recipients) == 0 {
		writeError(ctx, 400, "recipients are required")
		return
	}
	now := a.now().UTC()
	recipients := make([]firestorestore.CampaignRecipient, 0, len(request.Recipients))
	seen := map[string]struct{}{}
	for _, input := range request.Recipients {
		phone := normalizeCampaignPhone(input.PhoneNumber)
		if phone == "" {
			writeError(ctx, 400, "Every recipient must have a valid phone number")
			return
		}
		if _, exists := seen[phone]; exists {
			continue
		}
		seen[phone] = struct{}{}
		recipients = append(recipients, firestorestore.CampaignRecipient{ID: uuid.NewString(), CampaignID: campaignID, PhoneNumber: phone, RecipientName: input.RecipientName, TemplateParams: input.TemplateParams, HeaderParams: input.HeaderParams, Status: "pending", CreatedAt: now, UpdatedAt: now})
	}
	if err := a.administration().PutCampaignRecipients(context.Background(), claims.OrganizationID.String(), campaignID, recipients); err != nil {
		writeError(ctx, 500, "Failed to add recipients")
		return
	}
	all, err := a.administration().ListCampaignRecipients(context.Background(), claims.OrganizationID.String(), campaignID)
	if err != nil {
		writeError(ctx, 500, "Recipients were added but count could not be updated")
		return
	}
	campaign.TotalRecipients = len(all)
	campaign.UpdatedAt = now
	_ = a.administration().PutCampaign(context.Background(), *campaign)
	writeData(ctx, 200, map[string]any{"message": "Recipients added successfully", "added_count": len(recipients), "total_recipients": len(all)})
}

func normalizeCampaignPhone(value string) string {
	value = strings.TrimSpace(value)
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, value)
	if len(digits) < 10 || len(digits) > 15 {
		return ""
	}
	return digits
}

func (a *App) DeleteCampaignRecipient(ctx *fasthttp.RequestCtx) {
	claims, ok := a.authorize(ctx)
	if !ok {
		return
	}
	campaignID := pathValue(ctx, "id")
	campaign, err := a.administration().Campaign(context.Background(), claims.OrganizationID.String(), campaignID)
	if err != nil {
		writeError(ctx, 404, "Campaign not found")
		return
	}
	if campaign.Status != "draft" {
		writeError(ctx, 400, "Recipients can only be removed from draft campaigns")
		return
	}
	if err := a.administration().DeleteCampaignRecipient(context.Background(), claims.OrganizationID.String(), campaignID, pathValue(ctx, "recipientId")); err != nil {
		writeError(ctx, 500, "Failed to delete recipient")
		return
	}
	recipients, _ := a.administration().ListCampaignRecipients(context.Background(), claims.OrganizationID.String(), campaignID)
	campaign.TotalRecipients = len(recipients)
	campaign.UpdatedAt = a.now().UTC()
	_ = a.administration().PutCampaign(context.Background(), *campaign)
	writeData(ctx, 200, map[string]string{"message": "Recipient deleted successfully"})
}

func (a *App) UploadCampaignMedia(ctx *fasthttp.RequestCtx) {
	claims, ok := a.authorize(ctx)
	if !ok {
		return
	}
	campaign, err := a.administration().Campaign(context.Background(), claims.OrganizationID.String(), pathValue(ctx, "id"))
	if err != nil {
		writeError(ctx, 404, "Campaign not found")
		return
	}
	if campaign.Status != "draft" {
		writeError(ctx, 400, "Media can only be changed on draft campaigns")
		return
	}
	fileHeader, err := ctx.FormFile("file")
	if err != nil {
		writeError(ctx, 400, "No file provided")
		return
	}
	if fileHeader.Size > 32<<20 {
		writeError(ctx, 400, "File exceeds the 32 MiB limit")
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		writeError(ctx, 400, "Failed to open file")
		return
	}
	defer file.Close()
	data := make([]byte, fileHeader.Size)
	if _, err := io.ReadFull(file, data); err != nil {
		writeError(ctx, 400, "Failed to read file")
		return
	}
	mimeType := fileHeader.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = http.DetectContentType(data)
	}
	account, err := a.store.ResolveWhatsAppAccount(context.Background(), claims.OrganizationID.String(), campaign.WhatsAppAccount)
	if err != nil {
		writeError(ctx, 400, "WhatsApp account not found")
		return
	}
	token, err := appcrypto.Decrypt(account.AccessToken, a.config.App.EncryptionKey)
	if err != nil {
		writeError(ctx, 500, "WhatsApp credentials could not be opened")
		return
	}
	wa := &whatsapp.Account{PhoneID: account.PhoneID, BusinessID: account.BusinessID, AppID: account.AppID, APIVersion: account.APIVersion, AccessToken: token}
	requestCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	mediaID, err := a.sender.UploadMedia(requestCtx, wa, data, mimeType, fileHeader.Filename)
	if err != nil {
		writeError(ctx, 502, "Meta rejected campaign media: "+err.Error())
		return
	}
	storageKey := "campaign-media/" + campaign.ID + filepath.Ext(filepath.Base(fileHeader.Filename))
	if a.media != nil {
		if err := a.media.Upload(requestCtx, storageKey, bytes.NewReader(data), mimeType); err != nil {
			writeError(ctx, 502, "Campaign media could not be stored")
			return
		}
	} else {
		storageKey = ""
	}
	campaign.HeaderMediaID = mediaID
	campaign.HeaderMediaFilename = filepath.Base(fileHeader.Filename)
	campaign.HeaderMediaMimeType = mimeType
	campaign.HeaderMediaKey = storageKey
	campaign.UpdatedAt = a.now().UTC()
	if err := a.administration().PutCampaign(context.Background(), *campaign); err != nil {
		writeError(ctx, 500, "Media uploaded but campaign could not be updated")
		return
	}
	writeData(ctx, 200, map[string]any{"header_media_id": mediaID, "filename": campaign.HeaderMediaFilename, "mime_type": mimeType})
}

func (a *App) ServeCampaignMedia(ctx *fasthttp.RequestCtx) {
	claims, ok := a.authorize(ctx)
	if !ok {
		return
	}
	campaign, err := a.administration().Campaign(context.Background(), claims.OrganizationID.String(), pathValue(ctx, "id"))
	if err != nil || campaign.HeaderMediaKey == "" || a.media == nil {
		writeError(ctx, 404, "Campaign media not found")
		return
	}
	url, err := a.media.GetPresignedURL(context.Background(), campaign.HeaderMediaKey, 15*time.Minute)
	if err != nil {
		writeError(ctx, 500, "Failed to load campaign media")
		return
	}
	ctx.Redirect(url, http.StatusTemporaryRedirect)
}

func (a *App) PauseCampaign(ctx *fasthttp.RequestCtx) {
	a.setCampaignStatus(ctx, "paused", []string{"processing", "queued"})
}
func (a *App) CancelCampaign(ctx *fasthttp.RequestCtx) {
	a.setCampaignStatus(ctx, "cancelled", []string{"draft", "scheduled", "processing", "queued", "paused", "failed"})
}
func (a *App) setCampaignStatus(ctx *fasthttp.RequestCtx, status string, allowed []string) {
	claims, ok := a.authorize(ctx)
	if !ok {
		return
	}
	campaign, err := a.administration().Campaign(context.Background(), claims.OrganizationID.String(), pathValue(ctx, "id"))
	if err != nil {
		writeError(ctx, 404, "Campaign not found")
		return
	}
	valid := false
	for _, candidate := range allowed {
		if campaign.Status == candidate {
			valid = true
			break
		}
	}
	if !valid {
		writeError(ctx, 400, "Campaign cannot change to the requested status")
		return
	}
	campaign.Status = status
	campaign.UpdatedAt = a.now().UTC()
	if err := a.administration().PutCampaign(context.Background(), *campaign); err != nil {
		writeError(ctx, 500, "Failed to update campaign")
		return
	}
	writeData(ctx, 200, map[string]any{"message": "Campaign updated", "status": status})
}

func (a *App) StartCampaign(ctx *fasthttp.RequestCtx)       { a.processCampaign(ctx, false) }
func (a *App) RetryFailedCampaign(ctx *fasthttp.RequestCtx) { a.processCampaign(ctx, true) }

func (a *App) processCampaign(ctx *fasthttp.RequestCtx, retryFailed bool) {
	claims, ok := a.authorize(ctx)
	if !ok {
		return
	}
	orgID, campaignID := claims.OrganizationID.String(), pathValue(ctx, "id")
	campaign, err := a.administration().Campaign(context.Background(), orgID, campaignID)
	if err != nil {
		writeError(ctx, 404, "Campaign not found")
		return
	}
	if retryFailed {
		if campaign.Status != "completed" && campaign.Status != "paused" && campaign.Status != "failed" {
			writeError(ctx, 400, "Failed messages cannot be retried in the current state")
			return
		}
	} else if campaign.Status != "draft" && campaign.Status != "scheduled" && campaign.Status != "paused" {
		writeError(ctx, 400, "Campaign cannot be started in the current state")
		return
	}
	template, err := a.store.Template(context.Background(), orgID, campaign.TemplateID)
	if err != nil {
		writeError(ctx, 400, "Campaign template no longer exists")
		return
	}
	if !strings.EqualFold(template.Status, "APPROVED") {
		writeError(ctx, 400, "Campaign template must be approved by Meta")
		return
	}
	if (strings.EqualFold(template.HeaderType, "IMAGE") || strings.EqualFold(template.HeaderType, "VIDEO") || strings.EqualFold(template.HeaderType, "DOCUMENT")) && campaign.HeaderMediaID == "" {
		writeError(ctx, 400, "This template requires campaign header media")
		return
	}
	account, err := a.store.ResolveWhatsAppAccount(context.Background(), orgID, campaign.WhatsAppAccount)
	if err != nil {
		writeError(ctx, 400, "WhatsApp account not found")
		return
	}
	token, err := appcrypto.Decrypt(account.AccessToken, a.config.App.EncryptionKey)
	if err != nil {
		writeError(ctx, 500, "WhatsApp credentials could not be opened")
		return
	}
	recipients, err := a.administration().ListCampaignRecipients(context.Background(), orgID, campaignID)
	if err != nil {
		writeError(ctx, 500, "Failed to load campaign recipients")
		return
	}
	pending := make([]firestorestore.CampaignRecipient, 0)
	for _, recipient := range recipients {
		if (!retryFailed && recipient.Status == "pending") || (retryFailed && recipient.Status == "failed") {
			recipient.Status = "pending"
			recipient.ErrorMessage = ""
			pending = append(pending, recipient)
		}
	}
	if len(pending) == 0 {
		writeError(ctx, 400, "Campaign has no recipients to send")
		return
	}
	now := a.now().UTC()
	campaign.Status = "processing"
	if campaign.StartedAt == nil {
		campaign.StartedAt = &now
	}
	campaign.CompletedAt = nil
	campaign.UpdatedAt = now
	if err := a.administration().PutCampaign(context.Background(), *campaign); err != nil {
		writeError(ctx, 500, "Failed to start campaign")
		return
	}
	wa := &whatsapp.Account{PhoneID: account.PhoneID, BusinessID: account.BusinessID, AppID: account.AppID, APIVersion: account.APIVersion, AccessToken: token}
	processed := 0
	for i := range pending {
		current, loadErr := a.administration().Campaign(context.Background(), orgID, campaignID)
		if loadErr != nil || current.Status == "paused" || current.Status == "cancelled" {
			campaign = current
			break
		}
		recipient := &pending[i]
		bodyParams := stringMap(recipient.TemplateParams)
		headerParams := stringMap(recipient.HeaderParams)
		components, buildErr := whatsapp.BuildTemplateComponents(bodyParams, template.HeaderType, template.HeaderContent, headerParams, campaign.HeaderMediaID, campaign.HeaderMediaFilename)
		if buildErr == nil {
			components = append(components, whatsapp.AutoButtonComponents(template.Buttons)...)
			var externalID string
			externalID, buildErr = a.sender.SendTemplateMessage(context.Background(), wa, whatsapp.Recipient{Phone: recipient.PhoneNumber}, template.Name, template.Language, components)
			if buildErr == nil {
				sentAt := a.now().UTC()
				recipient.Status = "sent"
				recipient.WhatsAppMessageID = externalID
				recipient.SentAt = &sentAt
				recipient.UpdatedAt = sentAt
				if saveErr := a.saveCampaignChatMessage(context.Background(), orgID, claims.UserID.String(), account.Name, campaign, template, recipient, externalID, sentAt); saveErr != nil {
					recipient.ErrorMessage = "Message was sent but could not be added to the chat history: " + saveErr.Error()
				}
			}
		}
		if buildErr != nil {
			recipient.Status = "failed"
			recipient.ErrorMessage = buildErr.Error()
			recipient.UpdatedAt = a.now().UTC()
		}
		_ = a.administration().PutCampaignRecipients(context.Background(), orgID, campaignID, []firestorestore.CampaignRecipient{*recipient})
		processed++
		time.Sleep(100 * time.Millisecond)
	}
	all, _ := a.administration().ListCampaignRecipients(context.Background(), orgID, campaignID)
	campaign, err = a.administration().Campaign(context.Background(), orgID, campaignID)
	if err != nil {
		writeError(ctx, 500, "Campaign sent but final status could not be loaded")
		return
	}
	campaign.TotalRecipients = len(all)
	campaign.SentCount, campaign.DeliveredCount, campaign.ReadCount, campaign.FailedCount = 0, 0, 0, 0
	remaining := 0
	for _, recipient := range all {
		switch recipient.Status {
		case "sent":
			campaign.SentCount++
		case "delivered":
			campaign.SentCount++
			campaign.DeliveredCount++
		case "read":
			campaign.SentCount++
			campaign.DeliveredCount++
			campaign.ReadCount++
		case "failed":
			campaign.FailedCount++
		default:
			remaining++
		}
	}
	finished := a.now().UTC()
	if campaign.Status == "processing" {
		if remaining == 0 {
			campaign.Status = "completed"
			campaign.CompletedAt = &finished
		} else {
			campaign.Status = "paused"
		}
	}
	campaign.UpdatedAt = finished
	_ = a.administration().PutCampaign(context.Background(), *campaign)
	writeData(ctx, 200, map[string]any{"message": "Campaign processing finished", "status": campaign.Status, "processed": processed, "sent_count": campaign.SentCount, "failed_count": campaign.FailedCount})
}

func (a *App) saveCampaignChatMessage(ctx context.Context, orgID, userID, accountName string, campaign *firestorestore.Campaign, template *firestorestore.Template, recipient *firestorestore.CampaignRecipient, externalID string, sentAt time.Time) error {
	phone := normalizeCampaignPhone(recipient.PhoneNumber)
	contact, err := a.store.ContactByPhone(ctx, orgID, accountName, phone)
	if errors.Is(err, firestorestore.ErrNotFound) {
		contact = &firestorestore.Contact{
			ID: uuid.NewString(), OrganizationID: orgID, PhoneNumber: phone,
			ProfileName:     firstNonEmpty(strings.TrimSpace(recipient.RecipientName), phone),
			WhatsAppAccount: accountName, ChannelType: "whatsapp", IsRead: true,
			Tags: []any{}, Metadata: map[string]any{}, CreatedAt: sentAt, UpdatedAt: sentAt,
		}
		contact.SearchTokens = firestorestore.BuildContactSearchTokens(*contact)
		if err := a.store.CreateContact(ctx, *contact); err != nil {
			if !errors.Is(err, firestorestore.ErrConflict) {
				return err
			}
			contact, err = a.store.ContactByPhone(ctx, orgID, accountName, phone)
			if err != nil {
				return err
			}
		}
	} else if err != nil {
		return err
	}
	content := "[Template: " + firstNonEmpty(template.DisplayName, template.Name) + "]"
	message := firestorestore.Message{
		ID: uuid.NewString(), OrganizationID: orgID, ContactID: contact.ID,
		WhatsAppAccount: accountName, ChannelType: "whatsapp", ExternalID: externalID,
		Direction: "outgoing", MessageType: "template", Content: content,
		TemplateName: template.Name, TemplateParams: recipient.TemplateParams, Status: "sent",
		SentByUserID: userID, Metadata: map[string]any{"campaign_id": campaign.ID, "campaign_recipient_id": recipient.ID},
		CreatedAt: sentAt, UpdatedAt: sentAt,
	}
	if campaign.HeaderMediaKey != "" {
		message.MediaURL = campaign.HeaderMediaKey
		message.MediaMimeType = campaign.HeaderMediaMimeType
		message.MediaFilename = campaign.HeaderMediaFilename
	}
	contact.WhatsAppAccount = accountName
	contact.LastMessageAt = &sentAt
	contact.LastMessagePreview = content
	contact.IsRead = true
	contact.UpdatedAt = sentAt
	contact.SearchTokens = firestorestore.BuildContactSearchTokens(*contact)
	return a.store.CreateOutgoingMessage(ctx, *contact, message)
}

func stringMap(values map[string]any) map[string]string {
	result := map[string]string{}
	for key, value := range values {
		result[key] = strings.TrimSpace(toString(value))
	}
	return result
}
func toString(value any) string {
	if value == nil {
		return ""
	}
	if typed, ok := value.(string); ok {
		return typed
	}
	return fmt.Sprint(value)
}
