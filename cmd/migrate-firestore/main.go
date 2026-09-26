package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/config"
	"github.com/shridarpatil/whatomate/internal/database"
	"github.com/shridarpatil/whatomate/internal/firestorestore"
	"github.com/shridarpatil/whatomate/internal/models"
	"gorm.io/gorm"
)

type report struct {
	Project          string           `json:"project"`
	Namespace        string           `json:"namespace"`
	Since            string           `json:"since,omitempty"`
	WriteEnabled     bool             `json:"write_enabled"`
	Organizations    int              `json:"organizations"`
	Users            int              `json:"users"`
	Accounts         int              `json:"whatsapp_accounts"`
	Templates        int              `json:"templates"`
	Tags             int              `json:"tags"`
	Contacts         int              `json:"contacts"`
	Messages         int              `json:"messages"`
	Notes            int              `json:"conversation_notes"`
	AgentTransfers   int              `json:"agent_transfers"`
	ChatbotSessions  int              `json:"chatbot_sessions"`
	CannedResponses  int              `json:"canned_responses"`
	FeatureInventory map[string]int64 `json:"feature_inventory"`
}

func main() {
	configPath := flag.String("config", "config.toml", "legacy configuration file")
	write := flag.Bool("write", false, "write to Firestore; default is a read-only dry run")
	onlyAgentTransfers := flag.Bool("only-agent-transfers", false, "write only agent transfer documents and their active projections")
	confirmProject := flag.String("confirm-project", "", "must exactly match the target project when --write is used")
	namespace := flag.String("namespace", "", "Firestore namespace override")
	sinceRaw := flag.String("since", "", "delta migration lower bound in RFC3339; only newer mutable rows are merged")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	check(err)
	projectID := cfg.Firestore.ProjectID
	if projectID == "" {
		projectID = os.Getenv("GOOGLE_CLOUD_PROJECT")
	}
	if projectID == "" {
		check(errors.New("Firestore project id is required"))
	}
	if *namespace != "" {
		cfg.Firestore.Namespace = *namespace
	}
	if *write && *confirmProject != projectID {
		check(fmt.Errorf("--confirm-project must exactly match %q", projectID))
	}
	var since *time.Time
	if strings.TrimSpace(*sinceRaw) != "" {
		parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(*sinceRaw))
		check(err)
		parsed = parsed.UTC()
		since = &parsed
	}

	db, err := database.NewPostgres(&cfg.Database, false)
	check(err)
	sqlDB, err := db.DB()
	check(err)
	defer sqlDB.Close()

	var organizations []models.Organization
	var users []models.User
	var accounts []models.WhatsAppAccount
	var templates []models.Template
	var tags []models.Tag
	var contacts []models.Contact
	var messages []models.Message
	var notes []models.ConversationNote
	var transfers []models.AgentTransfer
	var sessions []models.ChatbotSession
	var cannedResponses []models.CannedResponse
	check(filterSince(db, since).Find(&organizations).Error)
	check(filterSince(db.Preload("Role.Permissions"), since).Find(&users).Error)
	check(filterSince(db.Unscoped(), since).Find(&accounts).Error)
	check(filterSince(db.Unscoped(), since).Find(&templates).Error)
	check(filterSince(db, since).Find(&tags).Error)
	check(filterSince(db.Unscoped(), since).Find(&contacts).Error)
	check(filterSince(db.Unscoped(), since).Find(&messages).Error)
	check(filterSince(db.Preload("CreatedBy"), since).Find(&notes).Error)
	check(filterSince(db.Preload("Contact").Preload("Agent").Preload("Team").Preload("TransferredByUser").Preload("ResumedByUser"), since).Find(&transfers).Error)
	check(filterSince(db, since).Find(&sessions).Error)
	check(filterSince(db, since).Find(&cannedResponses).Error)
	tagDocs := collectTags(tags, contacts)
	featureInventory := map[string]int64{
		"campaigns":                 countRows(db, &models.BulkMessageCampaign{}),
		"campaigns_open":            countRowsWhere(db, &models.BulkMessageCampaign{}, "status NOT IN ?", []models.CampaignStatus{models.CampaignStatusCompleted, models.CampaignStatusCancelled, models.CampaignStatusFailed}),
		"campaign_recipients":       countRows(db, &models.BulkMessageRecipient{}),
		"chatbot_flows":             countRows(db, &models.ChatbotFlow{}),
		"chatbot_settings":          countRows(db, &models.ChatbotSettings{}),
		"chatbot_settings_enabled":  countRowsWhere(db, &models.ChatbotSettings{}, "is_enabled = ?", true),
		"chatbot_sessions":          countRows(db, &models.ChatbotSession{}),
		"chatbot_sessions_active":   countRowsWhere(db, &models.ChatbotSession{}, "status = ?", models.SessionStatusActive),
		"agent_transfers":           countRows(db, &models.AgentTransfer{}),
		"agent_transfers_active":    countRowsWhere(db, &models.AgentTransfer{}, "status = ?", models.TransferStatusActive),
		"active_transfers_assigned": countRowsWhere(db, &models.AgentTransfer{}, "status = ? AND agent_id IS NOT NULL", models.TransferStatusActive),
		"active_transfers_general":  countRowsWhere(db, &models.AgentTransfer{}, "status = ? AND agent_id IS NULL AND team_id IS NULL", models.TransferStatusActive),
		"active_transfer_contacts":  countDistinctWhere(db, &models.AgentTransfer{}, "contact_id", "status = ?", models.TransferStatusActive),
		"canned_responses":          countRows(db, &models.CannedResponse{}),
		"canned_with_buttons":       countRowsWhere(db, &models.CannedResponse{}, "jsonb_array_length(buttons) > 0"),
		"teams":                     countRows(db, &models.Team{}),
		"custom_actions":            countRows(db, &models.CustomAction{}),
		"outbound_webhooks":         countRows(db, &models.Webhook{}),
		"whatsapp_flows":            countRows(db, &models.WhatsAppFlow{}),
	}
	appendGroupedCounts(db, &models.BulkMessageCampaign{}, "status", "campaign_status_", featureInventory)
	appendGroupedCounts(db, &models.AgentTransfer{}, "status", "transfer_status_", featureInventory)
	appendGroupedCounts(db, &models.ChatbotSession{}, "status", "session_status_", featureInventory)
	appendGroupedCounts(db, &models.Template{}, "header_type", "template_header_", featureInventory)

	result := report{
		Project: projectID, Namespace: cfg.Firestore.Namespace, WriteEnabled: *write,
		Organizations: len(organizations), Users: len(users), Accounts: len(accounts), Templates: len(templates), Tags: len(tagDocs),
		Contacts: len(contacts), Messages: len(messages), Notes: len(notes), AgentTransfers: len(transfers),
		ChatbotSessions: len(sessions), CannedResponses: len(cannedResponses),
		FeatureInventory: featureInventory,
	}
	if since != nil {
		result.Since = since.Format(time.RFC3339Nano)
	}
	if !*write {
		printReport(result)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	store, err := firestorestore.New(ctx, projectID, cfg.Firestore.DatabaseID, cfg.Firestore.Namespace)
	check(err)
	defer store.Close()
	if *onlyAgentTransfers {
		transferDocs := make([]firestorestore.AgentTransfer, 0, len(transfers))
		for _, transfer := range transfers {
			transferDocs = append(transferDocs, mapAgentTransfer(transfer))
		}
		if since == nil {
			check(store.ImportAgentTransfers(ctx, transferDocs))
		} else {
			for _, document := range transferDocs {
				check(store.MergeAgentTransferIfNewer(ctx, document))
			}
		}
		printReport(result)
		return
	}

	for _, organization := range organizations {
		check(store.PutOrganization(ctx, mapOrganization(organization)))
	}
	for _, user := range users {
		check(store.PutUser(ctx, mapUser(user)))
	}
	accountDocs := make([]firestorestore.WhatsAppAccount, 0, len(accounts))
	for _, account := range accounts {
		accountDocs = append(accountDocs, mapWhatsAppAccount(account))
	}
	check(store.ImportWhatsAppAccounts(ctx, accountDocs))
	templateDocs := make([]firestorestore.Template, 0, len(templates))
	for _, template := range templates {
		templateDocs = append(templateDocs, mapTemplate(template))
	}
	check(store.ImportTemplates(ctx, templateDocs))
	for _, tag := range tagDocs {
		check(store.PutTag(ctx, tag))
	}

	contactDocs := make([]firestorestore.Contact, 0, len(contacts))
	for _, contact := range contacts {
		contactDocs = append(contactDocs, mapContact(contact))
	}
	if since == nil {
		check(store.ImportContacts(ctx, contactDocs))
	} else {
		for _, document := range contactDocs {
			check(store.MergeContactIfNewer(ctx, document))
		}
	}

	messageDocs := make([]firestorestore.Message, 0, len(messages))
	for _, message := range messages {
		messageDocs = append(messageDocs, mapMessage(message))
	}
	if since == nil {
		check(store.ImportMessages(ctx, messageDocs))
	} else {
		for _, document := range messageDocs {
			check(store.MergeMessageIfNewer(ctx, document))
		}
	}
	noteDocs := make([]firestorestore.ConversationNote, 0, len(notes))
	for _, note := range notes {
		noteDocs = append(noteDocs, mapConversationNote(note))
	}
	check(store.ImportConversationNotes(ctx, noteDocs))
	transferDocs := make([]firestorestore.AgentTransfer, 0, len(transfers))
	for _, transfer := range transfers {
		transferDocs = append(transferDocs, mapAgentTransfer(transfer))
	}
	if since == nil {
		check(store.ImportAgentTransfers(ctx, transferDocs))
	} else {
		for _, document := range transferDocs {
			check(store.MergeAgentTransferIfNewer(ctx, document))
		}
	}
	sessionDocs := make([]firestorestore.ChatbotSession, 0, len(sessions))
	for _, session := range sessions {
		sessionDocs = append(sessionDocs, mapChatbotSession(session))
	}
	if since == nil {
		check(store.ImportChatbotSessions(ctx, sessionDocs))
	} else {
		for _, document := range sessionDocs {
			check(store.MergeChatbotSessionIfNewer(ctx, document))
		}
	}
	cannedDocs := make([]firestorestore.CannedResponse, 0, len(cannedResponses))
	for _, response := range cannedResponses {
		cannedDocs = append(cannedDocs, mapCannedResponse(response))
	}
	check(store.ImportCannedResponses(ctx, cannedDocs))
	printReport(result)
}

func filterSince(query *gorm.DB, since *time.Time) *gorm.DB {
	if since == nil {
		return query
	}
	return query.Where("updated_at >= ?", *since)
}

func mapAgentTransfer(input models.AgentTransfer) firestorestore.AgentTransfer {
	output := firestorestore.AgentTransfer{
		ID: input.ID.String(), OrganizationID: input.OrganizationID.String(), ContactID: input.ContactID.String(),
		WhatsAppAccount: input.WhatsAppAccount, PhoneNumber: input.PhoneNumber, Status: string(input.Status), Source: string(input.Source),
		AgentID: uuidString(input.AgentID), TeamID: uuidString(input.TeamID), TransferredBy: uuidString(input.TransferredByUserID),
		Notes: input.Notes, TransferredAt: input.TransferredAt, ResumedAt: input.ResumedAt, ResumedBy: uuidString(input.ResumedBy),
		SLAResponseDeadline: input.SLA.ResponseDeadline, SLAResolutionDeadline: input.SLA.ResolutionDeadline,
		SLABreached: input.SLA.Breached, SLABreachedAt: input.SLA.BreachedAt, EscalationLevel: input.SLA.EscalationLevel,
		EscalatedAt: input.SLA.EscalatedAt, PickedUpAt: input.SLA.PickedUpAt, ExpiresAt: input.SLA.ExpiresAt,
		CreatedAt: input.CreatedAt, UpdatedAt: input.UpdatedAt,
	}
	if input.Contact != nil {
		output.ContactName = input.Contact.ProfileName
	}
	if input.Agent != nil {
		output.AgentName = input.Agent.FullName
	}
	if input.Team != nil {
		output.TeamName = input.Team.Name
	}
	if input.TransferredByUser != nil {
		output.TransferredByName = input.TransferredByUser.FullName
	}
	if input.ResumedByUser != nil {
		output.ResumedByName = input.ResumedByUser.FullName
	}
	return output
}

func mapChatbotSession(input models.ChatbotSession) firestorestore.ChatbotSession {
	return firestorestore.ChatbotSession{
		ID: input.ID.String(), OrganizationID: input.OrganizationID.String(), ContactID: input.ContactID.String(),
		WhatsAppAccount: input.WhatsAppAccount, PhoneNumber: input.PhoneNumber, Status: string(input.Status),
		CurrentFlowID: uuidString(input.CurrentFlowID), CurrentStep: input.CurrentStep, StepRetries: input.StepRetries,
		SessionData: mapOrEmpty(input.SessionData), StartedAt: input.StartedAt, LastActivityAt: input.LastActivityAt,
		CompletedAt: input.CompletedAt, CreatedAt: input.CreatedAt, UpdatedAt: input.UpdatedAt,
	}
}

func mapCannedResponse(input models.CannedResponse) firestorestore.CannedResponse {
	return firestorestore.CannedResponse{
		ID: input.ID.String(), OrganizationID: input.OrganizationID.String(), Name: input.Name, Shortcut: input.Shortcut,
		Content: input.Content, Category: input.Category, IsActive: input.IsActive, UsageCount: input.UsageCount,
		Buttons: append([]any(nil), input.Buttons...), CreatedByID: input.CreatedByID.String(), CreatedAt: input.CreatedAt, UpdatedAt: input.UpdatedAt,
	}
}

func countRows(db *gorm.DB, model any) int64 {
	var count int64
	check(db.Model(model).Count(&count).Error)
	return count
}

func countRowsWhere(db *gorm.DB, model any, condition string, args ...any) int64 {
	var count int64
	check(db.Model(model).Where(condition, args...).Count(&count).Error)
	return count
}

func countDistinctWhere(db *gorm.DB, model any, column, condition string, args ...any) int64 {
	var count int64
	check(db.Model(model).Where(condition, args...).Distinct(column).Count(&count).Error)
	return count
}

func appendGroupedCounts(db *gorm.DB, model any, column, prefix string, target map[string]int64) {
	var rows []struct {
		Value string `gorm:"column:value"`
		Count int64  `gorm:"column:count"`
	}
	check(db.Model(model).Select(column + " AS value, COUNT(*) AS count").Group(column).Scan(&rows).Error)
	for _, row := range rows {
		value := strings.ToLower(strings.TrimSpace(row.Value))
		if value == "" {
			value = "none"
		}
		target[prefix+value] = row.Count
	}
}

func mapConversationNote(input models.ConversationNote) firestorestore.ConversationNote {
	createdByName := ""
	if input.CreatedBy != nil {
		createdByName = input.CreatedBy.FullName
	}
	return firestorestore.ConversationNote{
		ID: input.ID.String(), OrganizationID: input.OrganizationID.String(), ContactID: input.ContactID.String(),
		CreatedByID: input.CreatedByID.String(), CreatedByName: createdByName, Content: input.Content,
		CreatedAt: input.CreatedAt, UpdatedAt: input.UpdatedAt,
	}
}

func mapTag(input models.Tag) firestorestore.Tag {
	return firestorestore.Tag{
		OrganizationID: input.OrganizationID.String(), Name: input.Name, Color: input.Color,
		CreatedAt: input.CreatedAt, UpdatedAt: input.UpdatedAt,
	}
}

func collectTags(tags []models.Tag, contacts []models.Contact) []firestorestore.Tag {
	byKey := make(map[string]firestorestore.Tag)
	for _, tag := range tags {
		document := mapTag(tag)
		byKey[document.OrganizationID+"\x00"+document.Name] = document
	}
	for _, contact := range contacts {
		for _, rawTag := range contact.Tags {
			name, ok := rawTag.(string)
			name = strings.TrimSpace(name)
			if !ok || name == "" {
				continue
			}
			key := contact.OrganizationID.String() + "\x00" + name
			if _, exists := byKey[key]; exists {
				continue
			}
			byKey[key] = firestorestore.Tag{
				OrganizationID: contact.OrganizationID.String(), Name: name, Color: "gray",
				CreatedAt: contact.CreatedAt, UpdatedAt: contact.UpdatedAt,
			}
		}
	}
	result := make([]firestorestore.Tag, 0, len(byKey))
	for _, tag := range byKey {
		result = append(result, tag)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].OrganizationID == result[j].OrganizationID {
			return result[i].Name < result[j].Name
		}
		return result[i].OrganizationID < result[j].OrganizationID
	})
	return result
}

func mapWhatsAppAccount(input models.WhatsAppAccount) firestorestore.WhatsAppAccount {
	output := firestorestore.WhatsAppAccount{
		ID: input.ID.String(), OrganizationID: input.OrganizationID.String(), Name: input.Name,
		AppID: input.AppID, PhoneID: input.PhoneID, BusinessID: input.BusinessID,
		CatalogBusinessID: input.CatalogBusinessID, AccessToken: input.AccessToken, AppSecret: input.AppSecret,
		WebhookVerifyToken: input.WebhookVerifyToken, APIVersion: input.APIVersion,
		IsDefaultIncoming: input.IsDefaultIncoming, IsDefaultOutgoing: input.IsDefaultOutgoing,
		AutoReadReceipt: input.AutoReadReceipt, BusinessCallingEnabled: input.BusinessCallingEnabled,
		IsSMB: input.IsSMB, Status: input.Status, Pin: input.Pin,
		CreatedByID: uuidString(input.CreatedByID), UpdatedByID: uuidString(input.UpdatedByID),
		CreatedAt: input.CreatedAt, UpdatedAt: input.UpdatedAt, IsDeleted: input.DeletedAt.Valid,
	}
	if input.DeletedAt.Valid {
		deletedAt := input.DeletedAt.Time
		output.DeletedAt = &deletedAt
	}
	return output
}

func mapTemplate(input models.Template) firestorestore.Template {
	output := firestorestore.Template{
		ID: input.ID.String(), OrganizationID: input.OrganizationID.String(), WhatsAppAccount: input.WhatsAppAccount,
		MetaTemplateID: input.MetaTemplateID, Name: input.Name, DisplayName: input.DisplayName,
		Language: input.Language, Category: input.Category, Status: input.Status, QualityRating: input.QualityRating,
		HeaderType: input.HeaderType, HeaderContent: input.HeaderContent, BodyContent: input.BodyContent,
		FooterContent: input.FooterContent, Buttons: append([]any(nil), input.Buttons...),
		SampleValues: append([]any(nil), input.SampleValues...), IsFirstMessage: input.IsFirstMessage,
		AddSecurityRecommendation: input.AddSecurityRecommendation, CodeExpirationMinutes: input.CodeExpirationMinutes,
		CreatedByID: uuidString(input.CreatedByID), UpdatedByID: uuidString(input.UpdatedByID),
		CreatedAt: input.CreatedAt, UpdatedAt: input.UpdatedAt, IsDeleted: input.DeletedAt.Valid,
		Metadata: map[string]any{},
	}
	if input.DeletedAt.Valid {
		deletedAt := input.DeletedAt.Time
		output.DeletedAt = &deletedAt
	}
	return output
}

func mapOrganization(input models.Organization) firestorestore.Organization {
	return firestorestore.Organization{
		ID: input.ID.String(), Name: input.Name, Slug: input.Slug,
		Settings: mapOrEmpty(input.Settings), CreatedAt: input.CreatedAt, UpdatedAt: input.UpdatedAt,
	}
}

func mapUser(input models.User) firestorestore.User {
	output := firestorestore.User{
		ID: input.ID.String(), OrganizationID: input.OrganizationID.String(), Email: input.Email,
		PasswordHash: input.PasswordHash, FullName: input.FullName, Settings: mapOrEmpty(input.Settings),
		IsActive: input.IsActive, IsAvailable: input.IsAvailable, IsSuperAdmin: input.IsSuperAdmin,
		CreatedAt: input.CreatedAt, UpdatedAt: input.UpdatedAt,
	}
	if input.RoleID != nil {
		output.RoleID = input.RoleID.String()
	}
	if input.Role != nil {
		role := &firestorestore.Role{
			ID: input.Role.ID.String(), Name: input.Role.Name, Description: input.Role.Description,
			IsSystem: input.Role.IsSystem, Permissions: make([]firestorestore.Permission, 0, len(input.Role.Permissions)),
		}
		for _, permission := range input.Role.Permissions {
			role.Permissions = append(role.Permissions, firestorestore.Permission{
				ID: permission.ID.String(), Resource: permission.Resource, Action: permission.Action, Description: permission.Description,
			})
		}
		output.Role = role
	}
	return output
}

func mapContact(input models.Contact) firestorestore.Contact {
	output := firestorestore.Contact{
		ID: input.ID.String(), OrganizationID: input.OrganizationID.String(), PhoneNumber: input.PhoneNumber,
		PhoneCountryCode: input.PhoneCountryCode, ProfileName: input.ProfileName, CustomerID: input.CustomerID,
		FirstName: input.FirstName, LastName: input.LastName, WhatsAppAccount: input.WhatsAppAccount,
		ChannelType: input.ChannelType, LastMessageAt: input.LastMessageAt, LastMessagePreview: input.LastMessagePreview,
		IsRead: input.IsRead, Tags: append([]any(nil), input.Tags...), Metadata: mapOrEmpty(input.Metadata),
		LastInboundAt: input.LastInboundAt, Email: input.Email, CompanyName: input.CompanyName, TaxOffice: input.TaxOffice,
		TaxNumber: input.TaxNumber, Address: input.Address, City: input.City, District: input.District,
		PostalCode: input.PostalCode, PurchaseScore: input.PurchaseScore, HasPurchased: input.HasPurchased,
		Note: input.Note, MarketingOptOut: input.MarketingOptOut, BSUID: input.BSUID,
		ChatbotLastMessageAt: input.ChatbotLastMessageAt, ChatbotReminderSent: input.ChatbotReminderSent,
		CreatedAt: input.CreatedAt, UpdatedAt: input.UpdatedAt, IsDeleted: input.DeletedAt.Valid,
	}
	if input.AssignedUserID != nil {
		output.AssignedUserID = input.AssignedUserID.String()
	}
	if input.DeletedAt.Valid {
		deletedAt := input.DeletedAt.Time
		output.DeletedAt = &deletedAt
	}
	output.SearchTokens = firestorestore.BuildContactSearchTokens(output)
	return output
}

func mapMessage(input models.Message) firestorestore.Message {
	output := firestorestore.Message{
		ID: input.ID.String(), OrganizationID: input.OrganizationID.String(), ContactID: input.ContactID.String(),
		WhatsAppAccount: input.WhatsAppAccount, ChannelType: input.ChannelType,
		ExternalID: input.WhatsAppMessageID, ConversationID: input.ConversationID,
		Direction: string(input.Direction), MessageType: string(input.MessageType), Content: input.Content,
		MediaURL: input.MediaURL, MediaMimeType: input.MediaMimeType, MediaFilename: input.MediaFilename,
		TemplateName: input.TemplateName, TemplateParams: mapOrEmpty(input.TemplateParams),
		InteractiveData: mapOrEmpty(input.InteractiveData), FlowResponse: mapOrEmpty(input.FlowResponse),
		Status: string(input.Status), ErrorMessage: input.ErrorMessage, IsReply: input.IsReply,
		Metadata: mapOrEmpty(input.Metadata), CreatedAt: input.CreatedAt, UpdatedAt: input.UpdatedAt,
	}
	output.ReplyToMessageID = uuidString(input.ReplyToMessageID)
	output.SentByUserID = uuidString(input.SentByUserID)
	return output
}

func uuidString(value *uuid.UUID) string {
	if value == nil {
		return ""
	}
	return value.String()
}

func mapOrEmpty(input models.JSONB) map[string]any {
	if input == nil {
		return map[string]any{}
	}
	return map[string]any(input)
}

func printReport(value report) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	check(encoder.Encode(value))
}

func check(err error) {
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
