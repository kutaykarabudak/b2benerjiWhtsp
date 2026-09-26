package firestorestore

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"cloud.google.com/go/firestore"
)

func TestEmulatorCoreLifecycle(t *testing.T) {
	if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
		t.Skip("FIRESTORE_EMULATOR_HOST is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	client, err := firestore.NewClient(ctx, "demo-whatomate-firestore")
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewWithClient(client, "test-"+time.Now().UTC().Format("20060102150405.000000000"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	now := time.Now().UTC().Truncate(time.Millisecond)
	org := Organization{ID: "org-1", Name: "B2B Enerji", Slug: "b2b-enerji", Settings: map[string]any{}, CreatedAt: now, UpdatedAt: now}
	if err := store.PutOrganization(ctx, org); err != nil {
		t.Fatal(err)
	}
	user := User{ID: "user-1", OrganizationID: org.ID, Email: "Owner@Example.com", PasswordHash: "hash", FullName: "Owner", IsActive: true, CreatedAt: now, UpdatedAt: now}
	if err := store.PutUser(ctx, user); err != nil {
		t.Fatal(err)
	}
	users, err := store.ListUsers(ctx, org.ID, 20)
	if err != nil || len(users) != 1 || users[0].ID != user.ID {
		t.Fatalf("users=%+v err=%v", users, err)
	}
	found, err := store.UserByEmail(ctx, " owner@example.com ")
	if err != nil || found.ID != user.ID {
		t.Fatalf("user lookup failed: user=%+v err=%v", found, err)
	}
	account := WhatsAppAccount{ID: "account-1", OrganizationID: org.ID, Name: "main", PhoneID: "phone-1", BusinessID: "waba-1", AccessToken: "enc:test", WebhookVerifyToken: "verify-1", APIVersion: "v23.0", IsDefaultOutgoing: true, Status: "active", CreatedAt: now, UpdatedAt: now}
	if err := store.ImportWhatsAppAccounts(ctx, []WhatsAppAccount{account}); err != nil {
		t.Fatal(err)
	}
	resolved, err := store.ResolveWhatsAppAccount(ctx, org.ID, "")
	if err != nil || resolved.ID != account.ID {
		t.Fatalf("account=%+v err=%v", resolved, err)
	}
	byPhone, err := store.AccountByPhoneID(ctx, account.PhoneID)
	if err != nil || byPhone.ID != account.ID {
		t.Fatalf("phone account=%+v err=%v", byPhone, err)
	}
	byToken, err := store.AccountByWebhookVerifyToken(ctx, account.WebhookVerifyToken)
	if err != nil || byToken.ID != account.ID {
		t.Fatalf("token account=%+v err=%v", byToken, err)
	}
	template := Template{ID: "template-1", OrganizationID: org.ID, WhatsAppAccount: account.Name, Name: "hello", Language: "tr", Category: "UTILITY", Status: "APPROVED", BodyContent: "Merhaba", Buttons: []any{}, SampleValues: []any{}, CreatedAt: now, UpdatedAt: now}
	if err := store.ImportTemplates(ctx, []Template{template}); err != nil {
		t.Fatal(err)
	}
	tag := Tag{OrganizationID: org.ID, Name: "vip", Color: "purple", CreatedAt: now, UpdatedAt: now}
	if err := store.PutTag(ctx, tag); err != nil {
		t.Fatal(err)
	}
	tags, err := store.ListTags(ctx, org.ID, 20)
	if err != nil || len(tags) != 1 || tags[0].Name != tag.Name {
		t.Fatalf("tags=%+v err=%v", tags, err)
	}

	contact := Contact{ID: "contact-1", OrganizationID: org.ID, PhoneNumber: "+905550000000", ProfileName: "Test", WhatsAppAccount: account.Name, ChannelType: "whatsapp", IsRead: false, Tags: []any{}, Metadata: map[string]any{}, LastMessageAt: &now, CreatedAt: now, UpdatedAt: now}
	contact.SearchTokens = BuildContactSearchTokens(contact)
	message := Message{ID: "message-1", OrganizationID: org.ID, ContactID: contact.ID, ExternalID: "wamid.1", Direction: "incoming", MessageType: "text", Content: "merhaba", Status: "delivered", Metadata: map[string]any{}, CreatedAt: now, UpdatedAt: now}
	if err := store.CreateInboundMessage(ctx, contact, message); err != nil {
		t.Fatal(err)
	}
	byContactPhone, err := store.ContactByPhone(ctx, org.ID, contact.WhatsAppAccount, contact.PhoneNumber)
	if err != nil || byContactPhone.ID != contact.ID {
		t.Fatalf("phone contact=%+v err=%v", byContactPhone, err)
	}
	if err := store.CreateInboundMessage(ctx, contact, message); !errors.Is(err, ErrDuplicateEvent) {
		t.Fatalf("expected duplicate event, got %v", err)
	}
	contacts, err := store.ListContacts(ctx, org.ID, 20, nil)
	if err != nil || len(contacts) != 1 {
		t.Fatalf("contacts=%d err=%v", len(contacts), err)
	}
	count, err := store.CountContacts(ctx, org.ID)
	if err != nil || count != 1 {
		t.Fatalf("contact count=%d err=%v", count, err)
	}
	searchResults, searchTotal, err := store.SearchContacts(ctx, org.ID, "90555", 20)
	if err != nil || searchTotal != 1 || len(searchResults) != 1 {
		t.Fatalf("search total=%d results=%+v err=%v", searchTotal, searchResults, err)
	}
	managed := Contact{ID: "contact-2", OrganizationID: org.ID, PhoneNumber: "905551111111", ProfileName: "Managed", WhatsAppAccount: account.Name, ChannelType: "whatsapp", IsRead: true, Tags: []any{"vip"}, Metadata: map[string]any{}, LastMessageAt: &now, CreatedAt: now, UpdatedAt: now}
	managed.SearchTokens = BuildContactSearchTokens(managed)
	if err := store.CreateContact(ctx, managed); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateContact(ctx, managed); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected contact conflict, got %v", err)
	}
	tagged, err := store.ListContactsByTags(ctx, org.ID, []string{"vip"}, 20, nil)
	if err != nil || len(tagged) != 1 || tagged[0].ID != managed.ID {
		t.Fatalf("tagged=%+v err=%v", tagged, err)
	}
	taggedCount, err := store.CountContactsByTags(ctx, org.ID, []string{"vip"})
	if err != nil || taggedCount != 1 {
		t.Fatalf("tagged count=%d err=%v", taggedCount, err)
	}
	previous := managed
	managed.ProfileName = "Managed Updated"
	managed.Tags = []any{"priority"}
	managed.SearchTokens = BuildContactSearchTokens(managed)
	if err := store.UpdateContact(ctx, previous, managed); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteContact(ctx, managed, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Contact(ctx, org.ID, managed.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected deleted contact to be hidden, got %v", err)
	}
	if _, err := store.ContactByPhone(ctx, org.ID, managed.WhatsAppAccount, managed.PhoneNumber); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected deleted contact lookup to be removed, got %v", err)
	}
	messages, err := store.ListMessages(ctx, org.ID, contact.ID, 20, nil)
	if err != nil || len(messages) != 1 || messages[0].ExternalID != message.ExternalID {
		t.Fatalf("messages=%+v err=%v", messages, err)
	}
	messageByID, err := store.MessageByID(ctx, org.ID, message.ID)
	if err != nil || messageByID.ExternalID != message.ExternalID {
		t.Fatalf("message by id=%+v err=%v", messageByID, err)
	}
	outgoing := Message{ID: "message-2", OrganizationID: org.ID, ContactID: contact.ID, ExternalID: "wamid.2", Direction: "outgoing", MessageType: "text", Content: "selam", Status: "sent", Metadata: map[string]any{}, CreatedAt: now, UpdatedAt: now}
	if err := store.CreateOutgoingMessage(ctx, contact, outgoing); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateMessageStatus(ctx, outgoing.ExternalID, "delivered", "", now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	messages, err = store.ListMessages(ctx, org.ID, contact.ID, 20, nil)
	if err != nil || len(messages) != 2 || messages[1].Status != "delivered" {
		t.Fatalf("updated messages=%+v err=%v", messages, err)
	}
	_, reactions, err := store.UpdateMessageReaction(ctx, org.ID, contact.ID, outgoing.ID, user.ID, "ðŸ‘", now.Add(2*time.Second))
	if err != nil || len(reactions) != 1 || reactions[0]["from_user"] != user.ID {
		t.Fatalf("reactions=%+v err=%v", reactions, err)
	}
	events, err := store.ListEvents(ctx, org.ID, 20, nil)
	if err != nil || len(events) != 3 || events[0].Type != "new_message" || events[2].Type != "status_update" {
		t.Fatalf("events=%+v err=%v", events, err)
	}
	note := ConversationNote{ID: "note-1", OrganizationID: org.ID, ContactID: contact.ID, CreatedByID: user.ID, CreatedByName: user.FullName, Content: "İlk not", CreatedAt: now, UpdatedAt: now}
	if err := store.CreateConversationNote(ctx, note); err != nil {
		t.Fatal(err)
	}
	notes, notesTotal, err := store.ListConversationNotes(ctx, org.ID, contact.ID, 30, "")
	if err != nil || notesTotal != 1 || len(notes) != 1 || notes[0].Content != note.Content {
		t.Fatalf("notes=%+v total=%d err=%v", notes, notesTotal, err)
	}
	updatedNote, err := store.UpdateConversationNote(ctx, org.ID, contact.ID, note.ID, user.ID, "Güncel not", now.Add(time.Second))
	if err != nil || updatedNote.Content != "Güncel not" {
		t.Fatalf("updated note=%+v err=%v", updatedNote, err)
	}
	if _, err := store.UpdateConversationNote(ctx, org.ID, contact.ID, note.ID, "another-user", "yasak", now); !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected forbidden note update, got %v", err)
	}
	if err := store.DeleteConversationNote(ctx, org.ID, contact.ID, note.ID, user.ID, now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	notes, notesTotal, err = store.ListConversationNotes(ctx, org.ID, contact.ID, 30, "")
	if err != nil || notesTotal != 0 || len(notes) != 0 {
		t.Fatalf("deleted notes=%+v total=%d err=%v", notes, notesTotal, err)
	}

	session := ChatbotSession{ID: "session-1", OrganizationID: org.ID, ContactID: contact.ID, WhatsAppAccount: account.Name, PhoneNumber: contact.PhoneNumber, Status: "active", SessionData: map[string]any{"customer_no": "42"}, StartedAt: now, LastActivityAt: now, CreatedAt: now, UpdatedAt: now}
	if err := store.ImportChatbotSessions(ctx, []ChatbotSession{session}); err != nil {
		t.Fatal(err)
	}
	latestSession, err := store.LatestChatbotSession(ctx, org.ID, contact.ID)
	if err != nil || latestSession.SessionData["customer_no"] != "42" {
		t.Fatalf("session=%+v err=%v", latestSession, err)
	}
	canned := CannedResponse{ID: "canned-1", OrganizationID: org.ID, Name: "Merhaba", Shortcut: "mrb", Content: "Merhaba", IsActive: true, Buttons: []any{}, CreatedByID: user.ID, CreatedAt: now, UpdatedAt: now}
	if err := store.ImportCannedResponses(ctx, []CannedResponse{canned}); err != nil {
		t.Fatal(err)
	}
	cannedList, err := store.ListCannedResponses(ctx, org.ID, true, 20)
	if err != nil || len(cannedList) != 1 {
		t.Fatalf("canned=%+v err=%v", cannedList, err)
	}
	if err := store.IncrementCannedResponseUsage(ctx, org.ID, canned.ID); err != nil {
		t.Fatal(err)
	}
	transfer := AgentTransfer{ID: "transfer-1", OrganizationID: org.ID, ContactID: contact.ID, ContactName: contact.ProfileName, PhoneNumber: contact.PhoneNumber, WhatsAppAccount: account.Name, Status: "active", Source: "manual", TransferredAt: now, CreatedAt: now, UpdatedAt: now}
	if err := store.CreateAgentTransfer(ctx, transfer); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateAgentTransfer(ctx, transfer); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected transfer conflict, got %v", err)
	}
	active, err := store.ActiveTransfer(ctx, org.ID, contact.ID)
	if err != nil || active.ID != transfer.ID {
		t.Fatalf("active transfer=%+v err=%v", active, err)
	}
	assigned, err := store.AssignAgentTransfer(ctx, org.ID, transfer.ID, user.ID, user.FullName, now.Add(time.Second))
	if err != nil || assigned.AgentID != user.ID || assigned.PickedUpAt == nil {
		t.Fatalf("assigned=%+v err=%v", assigned, err)
	}
	if _, err := store.ResumeAgentTransfer(ctx, org.ID, transfer.ID, user.ID, user.FullName, now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ActiveTransfer(ctx, org.ID, contact.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected active transfer lookup removal, got %v", err)
	}
	if err := store.CancelActiveChatbotSessions(ctx, org.ID, contact.ID, now.Add(3*time.Second)); err != nil {
		t.Fatal(err)
	}

	token := RefreshToken{UserID: user.ID, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
	if err := store.PutRefreshToken(ctx, "jti-1", token); err != nil {
		t.Fatal(err)
	}
	if err := store.ConsumeRefreshToken(ctx, "jti-1", user.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.ConsumeRefreshToken(ctx, "jti-1", user.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected consumed token to be missing, got %v", err)
	}
}
