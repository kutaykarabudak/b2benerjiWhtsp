package serverlessapp

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"mime/multipart"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/config"
	"github.com/shridarpatil/whatomate/internal/firestorestore"
	"github.com/shridarpatil/whatomate/internal/middleware"
	"github.com/shridarpatil/whatomate/pkg/whatsapp"
	"github.com/valyala/fasthttp"
	"golang.org/x/crypto/bcrypt"
)

type fakeStore struct {
	user         *firestorestore.User
	refresh      map[string]firestorestore.RefreshToken
	contacts     []firestorestore.Contact
	messages     []firestorestore.Message
	accounts     []firestorestore.WhatsAppAccount
	tags         []firestorestore.Tag
	notes        []firestorestore.ConversationNote
	transfers    []firestorestore.AgentTransfer
	sessions     []firestorestore.ChatbotSession
	canned       []firestorestore.CannedResponse
	templates    []firestorestore.Template
	organization *firestorestore.Organization
	healthErr    error
	lastUserMail string
}

func (s *fakeStore) ListAgentTransfers(_ context.Context, _ string, status string, limit, offset int) ([]firestorestore.AgentTransfer, int64, error) {
	filtered := make([]firestorestore.AgentTransfer, 0)
	for _, transfer := range s.transfers {
		if status == "" || transfer.Status == status {
			filtered = append(filtered, transfer)
		}
	}
	total := int64(len(filtered))
	if offset >= len(filtered) {
		return nil, total, nil
	}
	filtered = filtered[offset:]
	if limit > 0 && len(filtered) > limit {
		filtered = filtered[:limit]
	}
	return filtered, total, nil
}

func (s *fakeStore) CountGeneralActiveTransfers(_ context.Context, _ string) (int64, error) {
	var count int64
	for _, transfer := range s.transfers {
		if transfer.Status == "active" && transfer.AgentID == "" && transfer.TeamID == "" {
			count++
		}
	}
	return count, nil
}
func (s *fakeStore) ActiveTransfer(_ context.Context, _, contactID string) (*firestorestore.AgentTransfer, error) {
	for i := range s.transfers {
		if s.transfers[i].ContactID == contactID && s.transfers[i].Status == "active" {
			return &s.transfers[i], nil
		}
	}
	return nil, firestorestore.ErrNotFound
}
func (s *fakeStore) CreateAgentTransfer(_ context.Context, transfer firestorestore.AgentTransfer) error {
	if _, err := s.ActiveTransfer(context.Background(), transfer.OrganizationID, transfer.ContactID); err == nil {
		return firestorestore.ErrConflict
	}
	s.transfers = append(s.transfers, transfer)
	return nil
}
func (s *fakeStore) ResumeAgentTransfer(_ context.Context, _, id, userID, userName string, now time.Time) (*firestorestore.AgentTransfer, error) {
	for i := range s.transfers {
		if s.transfers[i].ID == id {
			if s.transfers[i].Status != "active" {
				return nil, firestorestore.ErrConflict
			}
			s.transfers[i].Status = "resumed"
			s.transfers[i].ResumedBy = userID
			s.transfers[i].ResumedByName = userName
			s.transfers[i].ResumedAt = &now
			return &s.transfers[i], nil
		}
	}
	return nil, firestorestore.ErrNotFound
}
func (s *fakeStore) AssignAgentTransfer(_ context.Context, _, id, agentID, agentName string, now time.Time) (*firestorestore.AgentTransfer, error) {
	for i := range s.transfers {
		if s.transfers[i].ID == id {
			if s.transfers[i].Status != "active" {
				return nil, firestorestore.ErrConflict
			}
			s.transfers[i].AgentID = agentID
			s.transfers[i].AgentName = agentName
			if agentID != "" && s.transfers[i].PickedUpAt == nil {
				s.transfers[i].PickedUpAt = &now
			}
			return &s.transfers[i], nil
		}
	}
	return nil, firestorestore.ErrNotFound
}
func (s *fakeStore) LatestChatbotSession(_ context.Context, _, contactID string) (*firestorestore.ChatbotSession, error) {
	for i := len(s.sessions) - 1; i >= 0; i-- {
		if s.sessions[i].ContactID == contactID {
			return &s.sessions[i], nil
		}
	}
	return nil, firestorestore.ErrNotFound
}
func (s *fakeStore) CancelActiveChatbotSessions(_ context.Context, _, contactID string, now time.Time) error {
	for i := range s.sessions {
		if s.sessions[i].ContactID == contactID && s.sessions[i].Status == "active" {
			s.sessions[i].Status = "cancelled"
			s.sessions[i].CompletedAt = &now
		}
	}
	return nil
}
func (s *fakeStore) ListCannedResponses(_ context.Context, _ string, activeOnly bool, limit int) ([]firestorestore.CannedResponse, error) {
	result := make([]firestorestore.CannedResponse, 0)
	for _, response := range s.canned {
		if !activeOnly || response.IsActive {
			result = append(result, response)
		}
	}
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}
func (s *fakeStore) IncrementCannedResponseUsage(_ context.Context, _ string, id string) error {
	for i := range s.canned {
		if s.canned[i].ID == id {
			s.canned[i].UsageCount++
			return nil
		}
	}
	return firestorestore.ErrNotFound
}

type fakeMessenger struct {
	account    *whatsapp.Account
	body       string
	template   string
	components []map[string]any
	uploaded   []byte
}

func (s *fakeMessenger) SendTemplateMessage(_ context.Context, account *whatsapp.Account, _ whatsapp.Recipient, name, _ string, components []map[string]any) (string, error) {
	s.account = account
	s.template = name
	s.components = components
	return "wamid.template-1", nil
}

func (s *fakeMessenger) SendTextMessage(_ context.Context, account *whatsapp.Account, _ whatsapp.Recipient, body string, _ ...string) (string, error) {
	s.account = account
	s.body = body
	return "wamid.sent-1", nil
}

func (s *fakeMessenger) UploadMedia(_ context.Context, _ *whatsapp.Account, data []byte, _, _ string) (string, error) {
	s.uploaded = append([]byte(nil), data...)
	return "media-1", nil
}
func (s *fakeMessenger) SendImageMessage(context.Context, *whatsapp.Account, whatsapp.Recipient, string, string) (string, error) {
	return "wamid.image-1", nil
}
func (s *fakeMessenger) SendDocumentMessage(context.Context, *whatsapp.Account, whatsapp.Recipient, string, string, string) (string, error) {
	return "wamid.document-1", nil
}
func (s *fakeMessenger) SendVideoMessage(context.Context, *whatsapp.Account, whatsapp.Recipient, string, string) (string, error) {
	return "wamid.video-1", nil
}
func (s *fakeMessenger) SendAudioMessage(context.Context, *whatsapp.Account, whatsapp.Recipient, string) (string, error) {
	return "wamid.audio-1", nil
}
func (s *fakeMessenger) SendInteractiveButtons(context.Context, *whatsapp.Account, whatsapp.Recipient, string, []whatsapp.Button) (string, error) {
	return "wamid.interactive-1", nil
}
func (s *fakeMessenger) SendCTAURLButton(context.Context, *whatsapp.Account, whatsapp.Recipient, string, string, string) (string, error) {
	return "wamid.cta-1", nil
}
func (s *fakeMessenger) SendVoiceCallButton(context.Context, *whatsapp.Account, whatsapp.Recipient, string, string, int, string) (string, error) {
	return "wamid.voice-1", nil
}
func (s *fakeMessenger) SendFlowMessage(context.Context, *whatsapp.Account, whatsapp.Recipient, string, string, string, string, string, string) (string, error) {
	return "wamid.flow-1", nil
}
func (s *fakeMessenger) SendReaction(context.Context, *whatsapp.Account, whatsapp.Recipient, string, string) error {
	return nil
}
func (s *fakeMessenger) GetMediaURL(context.Context, string, *whatsapp.Account) (string, error) {
	return "https://example.invalid/media", nil
}
func (s *fakeMessenger) DownloadMedia(context.Context, string, string) ([]byte, error) {
	return []byte("media"), nil
}

func (s *fakeStore) Health(context.Context) error { return s.healthErr }
func (s *fakeStore) UserByEmail(_ context.Context, email string) (*firestorestore.User, error) {
	s.lastUserMail = email
	if s.user == nil {
		return nil, firestorestore.ErrNotFound
	}
	return s.user, nil
}
func (s *fakeStore) User(_ context.Context, id string) (*firestorestore.User, error) {
	if s.user == nil || s.user.ID != id {
		return nil, firestorestore.ErrNotFound
	}
	return s.user, nil
}
func (s *fakeStore) PutUser(_ context.Context, user firestorestore.User) error {
	s.user = &user
	return nil
}
func (s *fakeStore) ListUsers(context.Context, string, int) ([]firestorestore.User, error) {
	if s.user == nil {
		return nil, nil
	}
	return []firestorestore.User{*s.user}, nil
}
func (s *fakeStore) ListTags(context.Context, string, int) ([]firestorestore.Tag, error) {
	return s.tags, nil
}
func (s *fakeStore) Organization(_ context.Context, id string) (*firestorestore.Organization, error) {
	if s.organization != nil && s.organization.ID == id {
		return s.organization, nil
	}
	return nil, firestorestore.ErrNotFound
}
func (s *fakeStore) ListOrganizations(context.Context, int) ([]firestorestore.Organization, error) {
	if s.organization == nil {
		return nil, nil
	}
	return []firestorestore.Organization{*s.organization}, nil
}
func (s *fakeStore) PutRefreshToken(_ context.Context, jti string, token firestorestore.RefreshToken) error {
	if s.refresh == nil {
		s.refresh = make(map[string]firestorestore.RefreshToken)
	}
	s.refresh[jti] = token
	return nil
}
func (s *fakeStore) ConsumeRefreshToken(_ context.Context, jti, userID string) error {
	token, ok := s.refresh[jti]
	if !ok || token.UserID != userID {
		return firestorestore.ErrNotFound
	}
	delete(s.refresh, jti)
	return nil
}
func (s *fakeStore) ListContacts(context.Context, string, int, *firestorestore.ContactCursor) ([]firestorestore.Contact, error) {
	return s.contacts, nil
}
func (s *fakeStore) ListContactsByTags(_ context.Context, _ string, tags []string, _ int, _ *firestorestore.ContactCursor) ([]firestorestore.Contact, error) {
	result := make([]firestorestore.Contact, 0)
	for _, contact := range s.contacts {
		for _, rawTag := range contact.Tags {
			for _, tag := range tags {
				if rawTag == tag {
					result = append(result, contact)
					goto nextContact
				}
			}
		}
	nextContact:
	}
	return result, nil
}
func (s *fakeStore) SearchContacts(context.Context, string, string, int) ([]firestorestore.Contact, int64, error) {
	return s.contacts, int64(len(s.contacts)), nil
}
func (s *fakeStore) CountContacts(context.Context, string) (int64, error) {
	return int64(len(s.contacts)), nil
}
func (s *fakeStore) CountContactsByTags(ctx context.Context, orgID string, tags []string) (int64, error) {
	contacts, err := s.ListContactsByTags(ctx, orgID, tags, 100, nil)
	return int64(len(contacts)), err
}
func (s *fakeStore) Contact(_ context.Context, _, id string) (*firestorestore.Contact, error) {
	for i := range s.contacts {
		if s.contacts[i].ID == id {
			return &s.contacts[i], nil
		}
	}
	return nil, firestorestore.ErrNotFound
}
func (s *fakeStore) CreateContact(_ context.Context, contact firestorestore.Contact) error {
	for _, existing := range s.contacts {
		if !existing.IsDeleted && existing.OrganizationID == contact.OrganizationID && existing.WhatsAppAccount == contact.WhatsAppAccount && existing.PhoneNumber == contact.PhoneNumber {
			return firestorestore.ErrConflict
		}
	}
	s.contacts = append(s.contacts, contact)
	return nil
}
func (s *fakeStore) UpdateContact(_ context.Context, _, contact firestorestore.Contact) error {
	for i := range s.contacts {
		if s.contacts[i].ID == contact.ID {
			s.contacts[i] = contact
			return nil
		}
	}
	return firestorestore.ErrNotFound
}
func (s *fakeStore) DeleteContact(_ context.Context, contact firestorestore.Contact, deletedAt time.Time) error {
	for i := range s.contacts {
		if s.contacts[i].ID == contact.ID {
			s.contacts[i].IsDeleted = true
			s.contacts[i].DeletedAt = &deletedAt
			return nil
		}
	}
	return firestorestore.ErrNotFound
}
func (s *fakeStore) ListMessages(context.Context, string, string, int, *time.Time) ([]firestorestore.Message, error) {
	return s.messages, nil
}
func (s *fakeStore) Message(_ context.Context, _, _, messageID string) (*firestorestore.Message, error) {
	for i := range s.messages {
		if s.messages[i].ID == messageID {
			return &s.messages[i], nil
		}
	}
	return nil, firestorestore.ErrNotFound
}
func (s *fakeStore) MessageByID(_ context.Context, orgID, messageID string) (*firestorestore.Message, error) {
	for i := range s.messages {
		if s.messages[i].ID == messageID && s.messages[i].OrganizationID == orgID {
			return &s.messages[i], nil
		}
	}
	return nil, firestorestore.ErrNotFound
}
func (s *fakeStore) ListWhatsAppAccounts(context.Context, string) ([]firestorestore.WhatsAppAccount, error) {
	return s.accounts, nil
}
func (s *fakeStore) ResolveWhatsAppAccount(context.Context, string, string) (*firestorestore.WhatsAppAccount, error) {
	if len(s.accounts) == 0 {
		return nil, firestorestore.ErrNotFound
	}
	return &s.accounts[0], nil
}
func (s *fakeStore) CreateOutgoingMessage(_ context.Context, contact firestorestore.Contact, message firestorestore.Message) error {
	s.messages = append(s.messages, message)
	for i := range s.contacts {
		if s.contacts[i].ID == contact.ID {
			s.contacts[i] = contact
		}
	}
	return nil
}
func (s *fakeStore) AccountByPhoneID(_ context.Context, phoneID string) (*firestorestore.WhatsAppAccount, error) {
	for i := range s.accounts {
		if s.accounts[i].PhoneID == phoneID {
			return &s.accounts[i], nil
		}
	}
	return nil, firestorestore.ErrNotFound
}
func (s *fakeStore) AccountByWebhookVerifyToken(_ context.Context, token string) (*firestorestore.WhatsAppAccount, error) {
	for i := range s.accounts {
		if s.accounts[i].WebhookVerifyToken == token {
			return &s.accounts[i], nil
		}
	}
	return nil, firestorestore.ErrNotFound
}
func (s *fakeStore) ContactByPhone(_ context.Context, orgID, accountName, phone string) (*firestorestore.Contact, error) {
	for i := range s.contacts {
		if s.contacts[i].OrganizationID == orgID && s.contacts[i].WhatsAppAccount == accountName && s.contacts[i].PhoneNumber == phone {
			return &s.contacts[i], nil
		}
	}
	return nil, firestorestore.ErrNotFound
}
func (s *fakeStore) CreateInboundMessage(_ context.Context, contact firestorestore.Contact, message firestorestore.Message) error {
	for _, existing := range s.messages {
		if existing.ExternalID == message.ExternalID {
			return firestorestore.ErrDuplicateEvent
		}
	}
	s.contacts = append(s.contacts, contact)
	s.messages = append(s.messages, message)
	return nil
}
func (s *fakeStore) UpdateMessageStatus(_ context.Context, externalID, status, errorMessage string, updatedAt time.Time) error {
	for i := range s.messages {
		if s.messages[i].ExternalID == externalID {
			s.messages[i].Status = status
			s.messages[i].ErrorMessage = errorMessage
			s.messages[i].UpdatedAt = updatedAt
			return nil
		}
	}
	return firestorestore.ErrNotFound
}
func (s *fakeStore) UpdateMessageReaction(_ context.Context, _, _, messageID, userID, emoji string, updatedAt time.Time) (*firestorestore.Message, []map[string]any, error) {
	for i := range s.messages {
		if s.messages[i].ID != messageID {
			continue
		}
		reactions := make([]map[string]any, 0)
		if emoji != "" {
			reactions = append(reactions, map[string]any{"emoji": emoji, "from_user": userID})
		}
		if s.messages[i].Metadata == nil {
			s.messages[i].Metadata = map[string]any{}
		}
		s.messages[i].Metadata["reactions"] = reactions
		s.messages[i].UpdatedAt = updatedAt
		return &s.messages[i], reactions, nil
	}
	return nil, nil, firestorestore.ErrNotFound
}
func (s *fakeStore) ListTemplates(context.Context, string) ([]firestorestore.Template, error) {
	return s.templates, nil
}
func (s *fakeStore) Template(_ context.Context, _ string, idOrName string) (*firestorestore.Template, error) {
	for i := range s.templates {
		if s.templates[i].ID == idOrName || s.templates[i].Name == idOrName {
			return &s.templates[i], nil
		}
	}
	return nil, firestorestore.ErrNotFound
}
func (s *fakeStore) MarkContactRead(_ context.Context, _, contactID string, updatedAt time.Time) error {
	for i := range s.contacts {
		if s.contacts[i].ID == contactID {
			s.contacts[i].IsRead = true
			s.contacts[i].UpdatedAt = updatedAt
			return nil
		}
	}
	return firestorestore.ErrNotFound
}
func (s *fakeStore) ListEvents(context.Context, string, int, *firestorestore.EventCursor) ([]firestorestore.Event, error) {
	return nil, nil
}
func (s *fakeStore) ListConversationNotes(_ context.Context, _, contactID string, _ int, _ string) ([]firestorestore.ConversationNote, int64, error) {
	result := make([]firestorestore.ConversationNote, 0)
	for _, note := range s.notes {
		if note.ContactID == contactID {
			result = append(result, note)
		}
	}
	return result, int64(len(result)), nil
}
func (s *fakeStore) CreateConversationNote(_ context.Context, note firestorestore.ConversationNote) error {
	s.notes = append(s.notes, note)
	return nil
}
func (s *fakeStore) UpdateConversationNote(_ context.Context, _, contactID, noteID, userID, content string, updatedAt time.Time) (*firestorestore.ConversationNote, error) {
	for i := range s.notes {
		if s.notes[i].ID == noteID && s.notes[i].ContactID == contactID {
			if s.notes[i].CreatedByID != userID {
				return nil, firestorestore.ErrForbidden
			}
			s.notes[i].Content = content
			s.notes[i].UpdatedAt = updatedAt
			return &s.notes[i], nil
		}
	}
	return nil, firestorestore.ErrNotFound
}
func (s *fakeStore) DeleteConversationNote(_ context.Context, _, contactID, noteID, userID string, _ time.Time) error {
	for i := range s.notes {
		if s.notes[i].ID == noteID && s.notes[i].ContactID == contactID {
			if s.notes[i].CreatedByID != userID {
				return firestorestore.ErrForbidden
			}
			s.notes = append(s.notes[:i], s.notes[i+1:]...)
			return nil
		}
	}
	return firestorestore.ErrNotFound
}

func testApp(t *testing.T) (*App, *fakeStore) {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("correct horse battery staple"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	store := &fakeStore{user: &firestorestore.User{
		ID: uuid.NewString(), OrganizationID: uuid.NewString(), Email: "owner@example.com",
		PasswordHash: string(hash), FullName: "Owner", IsActive: true, IsSuperAdmin: true,
	}}
	store.organization = &firestorestore.Organization{ID: store.user.OrganizationID, Name: "B2B Enerji", Slug: "b2b-enerji"}
	cfg := &config.Config{}
	cfg.JWT.Secret = "01234567890123456789012345678901"
	cfg.JWT.AccessExpiryMins = 15
	cfg.JWT.RefreshExpiryDays = 30
	app, err := New(store, cfg)
	if err != nil {
		t.Fatal(err)
	}
	fixedNow := time.Now().UTC().Truncate(time.Second)
	app.now = func() time.Time { return fixedNow }
	return app, store
}

func TestLoginAndAuthenticatedContactList(t *testing.T) {
	app, store := testApp(t)
	store.contacts = []firestorestore.Contact{{ID: uuid.NewString(), OrganizationID: store.user.OrganizationID, ProfileName: "Test"}}

	var loginCtx fasthttp.RequestCtx
	loginCtx.Request.Header.SetMethod(fasthttp.MethodPost)
	loginCtx.Request.SetBodyString(`{"email":"owner@example.com","password":"correct horse battery staple"}`)
	app.Login(&loginCtx)
	if loginCtx.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("login status=%d body=%s", loginCtx.Response.StatusCode(), loginCtx.Response.Body())
	}
	rawSession := string(loginCtx.Response.Header.PeekCookie(middleware.FirebaseSessionCookieName))
	if rawSession == "" {
		t.Fatal("missing Firebase session cookie")
	}
	session := strings.SplitN(rawSession, "=", 2)[1]
	session = strings.SplitN(session, ";", 2)[0]
	if access, _, err := middleware.DecodeFirebaseSession(session); err != nil {
		t.Fatalf("invalid Firebase session cookie: %v", err)
	} else if _, err := app.parseToken(access); err != nil {
		t.Fatalf("invalid access token in session: %v", err)
	}

	var listCtx fasthttp.RequestCtx
	listCtx.Request.Header.SetCookie(middleware.FirebaseSessionCookieName, session)
	app.ListContacts(&listCtx)
	if listCtx.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("list status=%d body=%s", listCtx.Response.StatusCode(), listCtx.Response.Body())
	}
	var envelope struct {
		Data struct {
			Contacts []firestorestore.Contact `json:"contacts"`
			Total    int64                    `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(listCtx.Response.Body(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.Total != 1 || len(envelope.Data.Contacts) != 1 {
		t.Fatalf("unexpected envelope: %+v", envelope)
	}
}

func TestContactManagementCompatibilityEndpoints(t *testing.T) {
	app, store := testApp(t)
	store.tags = []firestorestore.Tag{{OrganizationID: store.user.OrganizationID, Name: "vip", Color: "purple"}}
	access, _, err := app.issueTokens(context.Background(), store.user)
	if err != nil {
		t.Fatal(err)
	}

	var createCtx fasthttp.RequestCtx
	createCtx.Request.Header.Set("Authorization", "Bearer "+access)
	createCtx.Request.SetBodyString(`{"phone_number":"+905551234567","profile_name":"Yeni Kişi","tags":["vip"],"purchase_score":25}`)
	app.CreateContact(&createCtx)
	if createCtx.Response.StatusCode() != fasthttp.StatusOK || len(store.contacts) != 1 {
		t.Fatalf("create status=%d body=%s contacts=%+v", createCtx.Response.StatusCode(), createCtx.Response.Body(), store.contacts)
	}
	contactID := store.contacts[0].ID
	if store.contacts[0].PhoneNumber != "905551234567" || len(store.contacts[0].SearchTokens) == 0 {
		t.Fatalf("created contact=%+v", store.contacts[0])
	}

	var assignCtx fasthttp.RequestCtx
	assignCtx.SetUserValue("id", contactID)
	assignCtx.Request.Header.Set("Authorization", "Bearer "+access)
	assignCtx.Request.SetBodyString(`{"user_id":"` + store.user.ID + `"}`)
	app.AssignContact(&assignCtx)
	if assignCtx.Response.StatusCode() != fasthttp.StatusOK || store.contacts[0].AssignedUserID != store.user.ID {
		t.Fatalf("assign status=%d body=%s contact=%+v", assignCtx.Response.StatusCode(), assignCtx.Response.Body(), store.contacts[0])
	}

	var tagsCtx fasthttp.RequestCtx
	tagsCtx.SetUserValue("id", contactID)
	tagsCtx.Request.Header.Set("Authorization", "Bearer "+access)
	tagsCtx.Request.SetBodyString(`{"tags":["priority"]}`)
	app.UpdateContactTags(&tagsCtx)
	if tagsCtx.Response.StatusCode() != fasthttp.StatusOK || len(store.contacts[0].Tags) != 1 || store.contacts[0].Tags[0] != "priority" {
		t.Fatalf("tags status=%d body=%s contact=%+v", tagsCtx.Response.StatusCode(), tagsCtx.Response.Body(), store.contacts[0])
	}

	var sessionCtx fasthttp.RequestCtx
	sessionCtx.SetUserValue("id", contactID)
	sessionCtx.Request.Header.Set("Authorization", "Bearer "+access)
	app.GetContactSessionData(&sessionCtx)
	if sessionCtx.Response.StatusCode() != fasthttp.StatusOK || !strings.Contains(string(sessionCtx.Response.Body()), "panel_config") {
		t.Fatalf("session status=%d body=%s", sessionCtx.Response.StatusCode(), sessionCtx.Response.Body())
	}

	var usersCtx fasthttp.RequestCtx
	usersCtx.Request.Header.Set("Authorization", "Bearer "+access)
	app.ListUsers(&usersCtx)
	if usersCtx.Response.StatusCode() != fasthttp.StatusOK || strings.Contains(string(usersCtx.Response.Body()), store.user.PasswordHash) {
		t.Fatalf("users status=%d body=%s", usersCtx.Response.StatusCode(), usersCtx.Response.Body())
	}

	var listTagsCtx fasthttp.RequestCtx
	listTagsCtx.Request.Header.Set("Authorization", "Bearer "+access)
	app.ListTags(&listTagsCtx)
	if listTagsCtx.Response.StatusCode() != fasthttp.StatusOK || !strings.Contains(string(listTagsCtx.Response.Body()), `"name":"vip"`) {
		t.Fatalf("tags list status=%d body=%s", listTagsCtx.Response.StatusCode(), listTagsCtx.Response.Body())
	}

	var deleteCtx fasthttp.RequestCtx
	deleteCtx.SetUserValue("id", contactID)
	deleteCtx.Request.Header.Set("Authorization", "Bearer "+access)
	app.DeleteContact(&deleteCtx)
	if deleteCtx.Response.StatusCode() != fasthttp.StatusOK || !store.contacts[0].IsDeleted {
		t.Fatalf("delete status=%d body=%s contact=%+v", deleteCtx.Response.StatusCode(), deleteCtx.Response.Body(), store.contacts[0])
	}
}

func TestListAccountsHidesSecretsAndSendTextPersists(t *testing.T) {
	app, store := testApp(t)
	now := app.now().UTC()
	contactID := uuid.NewString()
	store.contacts = []firestorestore.Contact{{
		ID: contactID, OrganizationID: store.user.OrganizationID, PhoneNumber: "905550000000",
		ProfileName: "Test", WhatsAppAccount: "main", ChannelType: "whatsapp",
		LastInboundAt: &now, CreatedAt: now, UpdatedAt: now,
	}}
	store.accounts = []firestorestore.WhatsAppAccount{{
		ID: uuid.NewString(), OrganizationID: store.user.OrganizationID, Name: "main",
		PhoneID: "phone-1", BusinessID: "waba-1", AccessToken: "secret-token",
		APIVersion: "v23.0", IsDefaultOutgoing: true, Status: "active", CreatedAt: now, UpdatedAt: now,
	}}
	sender := &fakeMessenger{}
	app.SetMessenger(sender)
	access, _, err := app.issueTokens(context.Background(), store.user)
	if err != nil {
		t.Fatal(err)
	}

	var accountsCtx fasthttp.RequestCtx
	accountsCtx.Request.Header.Set("Authorization", "Bearer "+access)
	app.ListAccounts(&accountsCtx)
	if accountsCtx.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("accounts status=%d body=%s", accountsCtx.Response.StatusCode(), accountsCtx.Response.Body())
	}
	if strings.Contains(string(accountsCtx.Response.Body()), "secret-token") {
		t.Fatal("account response leaked the access token")
	}

	var sendCtx fasthttp.RequestCtx
	sendCtx.SetUserValue("id", contactID)
	sendCtx.Request.Header.Set("Authorization", "Bearer "+access)
	sendCtx.Request.SetBodyString(`{"type":"text","content":{"body":"Merhaba"}}`)
	app.SendMessage(&sendCtx)
	if sendCtx.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("send status=%d body=%s", sendCtx.Response.StatusCode(), sendCtx.Response.Body())
	}
	if sender.account == nil || sender.account.AccessToken != "secret-token" || sender.body != "Merhaba" {
		t.Fatalf("sender account=%+v body=%q", sender.account, sender.body)
	}
	if len(store.messages) != 1 || store.messages[0].ExternalID != "wamid.sent-1" {
		t.Fatalf("messages=%+v", store.messages)
	}

	var interactiveCtx fasthttp.RequestCtx
	interactiveCtx.SetUserValue("id", contactID)
	interactiveCtx.Request.Header.Set("Authorization", "Bearer "+access)
	interactiveCtx.Request.SetBodyString(`{"type":"interactive","content":{"body":"SeÃ§iniz"},"interactive":{"type":"button","body":"SeÃ§iniz","buttons":[{"id":"one","title":"Bir"}]}}`)
	app.SendMessage(&interactiveCtx)
	if interactiveCtx.Response.StatusCode() != fasthttp.StatusOK || len(store.messages) != 2 || store.messages[1].MessageType != "interactive" || store.messages[1].InteractiveData["type"] != "button" {
		t.Fatalf("interactive status=%d body=%s messages=%+v", interactiveCtx.Response.StatusCode(), interactiveCtx.Response.Body(), store.messages)
	}
}

func TestSendTemplateWithMediaHeader(t *testing.T) {
	app, store := testApp(t)
	now := app.now().UTC()
	contactID := uuid.NewString()
	store.contacts = []firestorestore.Contact{{ID: contactID, OrganizationID: store.user.OrganizationID, PhoneNumber: "905550000000", ProfileName: "Test", WhatsAppAccount: "main", ChannelType: "whatsapp", LastInboundAt: &now, CreatedAt: now, UpdatedAt: now}}
	store.accounts = []firestorestore.WhatsAppAccount{{ID: uuid.NewString(), OrganizationID: store.user.OrganizationID, Name: "main", PhoneID: "phone-1", BusinessID: "waba-1", AccessToken: "secret-token", APIVersion: "v23.0", IsDefaultOutgoing: true, Status: "active", CreatedAt: now, UpdatedAt: now}}
	store.templates = []firestorestore.Template{{ID: uuid.NewString(), OrganizationID: store.user.OrganizationID, WhatsAppAccount: "main", Name: "image_template", Language: "tr", Status: "APPROVED", HeaderType: "IMAGE", BodyContent: "Merhaba", Buttons: []any{}, SampleValues: []any{}, CreatedAt: now, UpdatedAt: now}}
	sender := &fakeMessenger{}
	app.SetMessenger(sender)
	access, _, err := app.issueTokens(context.Background(), store.user)
	if err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("contact_id", contactID)
	_ = writer.WriteField("template_name", "image_template")
	file, err := writer.CreateFormFile("header_file", "header.png")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = file.Write([]byte("fake-png"))
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	var requestCtx fasthttp.RequestCtx
	requestCtx.Request.Header.Set("Authorization", "Bearer "+access)
	requestCtx.Request.Header.SetContentType(writer.FormDataContentType())
	requestCtx.Request.SetBody(body.Bytes())
	app.SendTemplate(&requestCtx)
	if requestCtx.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("status=%d body=%s", requestCtx.Response.StatusCode(), requestCtx.Response.Body())
	}
	if string(sender.uploaded) != "fake-png" || sender.template != "image_template" || len(sender.components) == 0 {
		t.Fatalf("uploaded=%q template=%q components=%+v", sender.uploaded, sender.template, sender.components)
	}
	if len(store.messages) != 1 || store.messages[0].MediaURL != "media-1" || store.messages[0].MediaFilename != "header.png" {
		t.Fatalf("messages=%+v", store.messages)
	}
}

func TestWebhookVerificationSignatureAndInboundDeduplication(t *testing.T) {
	app, store := testApp(t)
	store.accounts = []firestorestore.WhatsAppAccount{{
		ID: uuid.NewString(), OrganizationID: store.user.OrganizationID, Name: "main",
		PhoneID: "phone-1", BusinessID: "waba-1", AppSecret: "app-secret",
		WebhookVerifyToken: "verify-me", Status: "active",
	}}
	var verifyCtx fasthttp.RequestCtx
	verifyCtx.QueryArgs().Set("hub.mode", "subscribe")
	verifyCtx.QueryArgs().Set("hub.verify_token", "verify-me")
	verifyCtx.QueryArgs().Set("hub.challenge", "challenge-123")
	app.WebhookVerify(&verifyCtx)
	if verifyCtx.Response.StatusCode() != fasthttp.StatusOK || string(verifyCtx.Response.Body()) != "challenge-123" {
		t.Fatalf("verify status=%d body=%s", verifyCtx.Response.StatusCode(), verifyCtx.Response.Body())
	}

	body := []byte(`{"object":"whatsapp_business_account","entry":[{"id":"waba-1","changes":[{"field":"messages","value":{"messaging_product":"whatsapp","metadata":{"display_phone_number":"+90555","phone_number_id":"phone-1"},"contacts":[{"profile":{"name":"Musteri"},"wa_id":"905550000000"}],"messages":[{"from":"905550000000","id":"wamid.in-1","timestamp":"1700000000","type":"text","text":{"body":"Merhaba"}}]}}]}]}`)
	mac := hmac.New(sha256.New, []byte("app-secret"))
	_, _ = mac.Write(body)
	var webhookCtx fasthttp.RequestCtx
	webhookCtx.Request.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	webhookCtx.Request.SetBody(body)
	app.Webhook(&webhookCtx)
	if webhookCtx.Response.StatusCode() != fasthttp.StatusOK || len(store.messages) != 1 {
		t.Fatalf("webhook status=%d body=%s messages=%+v", webhookCtx.Response.StatusCode(), webhookCtx.Response.Body(), store.messages)
	}
	var replayCtx fasthttp.RequestCtx
	replayCtx.Request.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	replayCtx.Request.SetBody(body)
	app.Webhook(&replayCtx)
	if replayCtx.Response.StatusCode() != fasthttp.StatusOK || len(store.messages) != 1 {
		t.Fatalf("replay status=%d messages=%d", replayCtx.Response.StatusCode(), len(store.messages))
	}
}

func TestReadyReportsStoreFailure(t *testing.T) {
	app, store := testApp(t)
	store.healthErr = errors.New("offline")
	var ctx fasthttp.RequestCtx
	app.Ready(&ctx)
	if ctx.Response.StatusCode() != fasthttp.StatusServiceUnavailable {
		t.Fatalf("status=%d", ctx.Response.StatusCode())
	}
}

func TestNewRejectsShortJWTSecret(t *testing.T) {
	_, err := New(&fakeStore{}, &config.Config{JWT: config.JWTConfig{Secret: "short"}})
	if err == nil {
		t.Fatal("expected short secret to fail")
	}
}
