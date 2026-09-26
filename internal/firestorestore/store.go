package firestorestore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"cloud.google.com/go/firestore"
	pb "cloud.google.com/go/firestore/apiv1/firestorepb"
	"github.com/google/uuid"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	ErrNotFound        = errors.New("document not found")
	ErrConflict        = errors.New("document conflict")
	ErrForbidden       = errors.New("operation forbidden")
	ErrDuplicateEvent  = errors.New("webhook event already processed")
	ErrInvalidArgument = errors.New("invalid argument")
)

type Store struct {
	client    *firestore.Client
	namespace string
}

type ContactCursor struct {
	LastMessageAt *time.Time
	ID            string
}

func New(ctx context.Context, projectID, databaseID, namespace string) (*Store, error) {
	if strings.TrimSpace(projectID) == "" {
		return nil, fmt.Errorf("firestore project id is required: %w", ErrInvalidArgument)
	}
	if databaseID == "" {
		databaseID = "(default)"
	}
	if namespace == "" {
		namespace = "whatomate-v1"
	}
	client, err := firestore.NewClientWithDatabase(ctx, projectID, databaseID)
	if err != nil {
		return nil, fmt.Errorf("create firestore client: %w", err)
	}
	return &Store{client: client, namespace: namespace}, nil
}

func NewWithClient(client *firestore.Client, namespace string) (*Store, error) {
	if client == nil {
		return nil, fmt.Errorf("firestore client is required: %w", ErrInvalidArgument)
	}
	if namespace == "" {
		namespace = "whatomate-v1"
	}
	return &Store{client: client, namespace: namespace}, nil
}

func (s *Store) Close() error { return s.client.Close() }

func (s *Store) root() *firestore.DocumentRef {
	return s.client.Collection("whatomateNamespaces").Doc(s.namespace)
}

func (s *Store) organization(id string) *firestore.DocumentRef {
	return s.root().Collection("organizations").Doc(id)
}

func (s *Store) contact(orgID, id string) *firestore.DocumentRef {
	return s.organization(orgID).Collection("contacts").Doc(id)
}

func (s *Store) account(orgID, id string) *firestore.DocumentRef {
	return s.organization(orgID).Collection("whatsAppAccounts").Doc(id)
}

func (s *Store) template(orgID, id string) *firestore.DocumentRef {
	return s.organization(orgID).Collection("templates").Doc(id)
}

func (s *Store) message(orgID, contactID, id string) *firestore.DocumentRef {
	return s.contact(orgID, contactID).Collection("messages").Doc(id)
}

func (s *Store) note(orgID, contactID, id string) *firestore.DocumentRef {
	return s.contact(orgID, contactID).Collection("notes").Doc(id)
}

func (s *Store) event(orgID, id string) *firestore.DocumentRef {
	return s.organization(orgID).Collection("events").Doc(id)
}

func (s *Store) transfer(orgID, id string) *firestore.DocumentRef {
	return s.organization(orgID).Collection("agentTransfers").Doc(id)
}

func (s *Store) session(orgID, id string) *firestore.DocumentRef {
	return s.organization(orgID).Collection("chatbotSessions").Doc(id)
}

func (s *Store) cannedResponse(orgID, id string) *firestore.DocumentRef {
	return s.organization(orgID).Collection("cannedResponses").Doc(id)
}

func (s *Store) Health(ctx context.Context) error {
	_, err := s.root().Get(ctx)
	if err == nil || firestoreIsNotFound(err) {
		return nil
	}
	return fmt.Errorf("firestore health check: %w", err)
}

func (s *Store) PutOrganization(ctx context.Context, organization Organization) error {
	if organization.ID == "" {
		return ErrInvalidArgument
	}
	_, err := s.organization(organization.ID).Set(ctx, organization)
	return err
}

func (s *Store) Organization(ctx context.Context, organizationID string) (*Organization, error) {
	snapshot, err := s.organization(organizationID).Get(ctx)
	if err != nil {
		if firestoreIsNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var organization Organization
	if err := snapshot.DataTo(&organization); err != nil {
		return nil, err
	}
	return &organization, nil
}

func (s *Store) ListOrganizations(ctx context.Context, limit int) ([]Organization, error) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	it := s.root().Collection("organizations").Limit(limit).Documents(ctx)
	defer it.Stop()
	organizations := make([]Organization, 0, limit)
	for {
		snapshot, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, err
		}
		var organization Organization
		if err := snapshot.DataTo(&organization); err != nil {
			return nil, err
		}
		organizations = append(organizations, organization)
	}
	sort.Slice(organizations, func(i, j int) bool {
		return strings.ToLower(organizations[i].Name) < strings.ToLower(organizations[j].Name)
	})
	return organizations, nil
}

func (s *Store) PutUser(ctx context.Context, user User) error {
	if user.ID == "" || user.OrganizationID == "" || strings.TrimSpace(user.Email) == "" {
		return ErrInvalidArgument
	}
	userRef := s.root().Collection("users").Doc(user.ID)
	emailRef := s.root().Collection("userEmails").Doc(hash(strings.ToLower(strings.TrimSpace(user.Email))))
	return s.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		if err := tx.Set(userRef, user); err != nil {
			return err
		}
		return tx.Set(emailRef, map[string]any{"userId": user.ID, "email": strings.ToLower(strings.TrimSpace(user.Email))})
	})
}

func (s *Store) UserByEmail(ctx context.Context, email string) (*User, error) {
	lookup, err := s.root().Collection("userEmails").Doc(hash(strings.ToLower(strings.TrimSpace(email)))).Get(ctx)
	if err != nil {
		if firestoreIsNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	userID, err := lookup.DataAt("userId")
	if err != nil {
		return nil, err
	}
	snapshot, err := s.root().Collection("users").Doc(fmt.Sprint(userID)).Get(ctx)
	if err != nil {
		if firestoreIsNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var user User
	if err := snapshot.DataTo(&user); err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *Store) User(ctx context.Context, userID string) (*User, error) {
	snapshot, err := s.root().Collection("users").Doc(userID).Get(ctx)
	if err != nil {
		if firestoreIsNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var user User
	if err := snapshot.DataTo(&user); err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *Store) ListUsers(ctx context.Context, orgID string, limit int) ([]User, error) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	it := s.root().Collection("users").Where("organizationId", "==", orgID).Limit(limit).Documents(ctx)
	defer it.Stop()
	users := make([]User, 0, limit)
	for {
		snapshot, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, err
		}
		var user User
		if err := snapshot.DataTo(&user); err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	sort.Slice(users, func(i, j int) bool { return strings.ToLower(users[i].FullName) < strings.ToLower(users[j].FullName) })
	return users, nil
}

func (s *Store) tag(orgID, name string) *firestore.DocumentRef {
	return s.organization(orgID).Collection("tags").Doc(hash(strings.ToLower(strings.TrimSpace(name))))
}

func (s *Store) PutTag(ctx context.Context, tag Tag) error {
	if tag.OrganizationID == "" || strings.TrimSpace(tag.Name) == "" {
		return ErrInvalidArgument
	}
	_, err := s.tag(tag.OrganizationID, tag.Name).Set(ctx, tag)
	return err
}

func (s *Store) ListTags(ctx context.Context, orgID string, limit int) ([]Tag, error) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	it := s.organization(orgID).Collection("tags").OrderBy("name", firestore.Asc).Limit(limit).Documents(ctx)
	defer it.Stop()
	tags := make([]Tag, 0, limit)
	for {
		snapshot, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, err
		}
		var tag Tag
		if err := snapshot.DataTo(&tag); err != nil {
			return nil, err
		}
		tags = append(tags, tag)
	}
	return tags, nil
}

func (s *Store) PutContact(ctx context.Context, contact Contact) error {
	if contact.ID == "" || contact.OrganizationID == "" {
		return ErrInvalidArgument
	}
	contactRef := s.contact(contact.OrganizationID, contact.ID)
	lookupRef := s.contactPhoneLookup(contact.OrganizationID, contact.WhatsAppAccount, contact.PhoneNumber)
	return s.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		if err := tx.Set(contactRef, contact); err != nil {
			return err
		}
		return tx.Set(lookupRef, map[string]any{"contactId": contact.ID, "phoneNumber": contact.PhoneNumber, "whatsAppAccount": contact.WhatsAppAccount})
	})
}

// CreateContact reserves the account/phone lookup before creating the contact.
// This keeps concurrent requests from creating duplicate conversation rows.
func (s *Store) CreateContact(ctx context.Context, contact Contact) error {
	if contact.ID == "" || contact.OrganizationID == "" || contact.PhoneNumber == "" {
		return ErrInvalidArgument
	}
	contactRef := s.contact(contact.OrganizationID, contact.ID)
	lookupRef := s.contactPhoneLookup(contact.OrganizationID, contact.WhatsAppAccount, contact.PhoneNumber)
	return s.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		if _, err := tx.Get(lookupRef); err == nil {
			return ErrConflict
		} else if !firestoreIsNotFound(err) {
			return err
		}
		if err := tx.Create(contactRef, contact); err != nil {
			return err
		}
		return tx.Create(lookupRef, map[string]any{"contactId": contact.ID, "phoneNumber": contact.PhoneNumber, "whatsAppAccount": contact.WhatsAppAccount})
	})
}

// UpdateContact writes the contact projection and moves its lookup atomically
// when the WhatsApp account changes.
func (s *Store) UpdateContact(ctx context.Context, previous, contact Contact) error {
	if contact.ID == "" || contact.OrganizationID == "" || contact.ID != previous.ID || contact.OrganizationID != previous.OrganizationID {
		return ErrInvalidArgument
	}
	oldLookup := s.contactPhoneLookup(previous.OrganizationID, previous.WhatsAppAccount, previous.PhoneNumber)
	newLookup := s.contactPhoneLookup(contact.OrganizationID, contact.WhatsAppAccount, contact.PhoneNumber)
	return s.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		if oldLookup.Path != newLookup.Path {
			if snapshot, err := tx.Get(newLookup); err == nil {
				owner, _ := snapshot.DataAt("contactId")
				if fmt.Sprint(owner) != contact.ID {
					return ErrConflict
				}
			} else if !firestoreIsNotFound(err) {
				return err
			}
			if err := tx.Delete(oldLookup); err != nil {
				return err
			}
		}
		if err := tx.Set(s.contact(contact.OrganizationID, contact.ID), contact); err != nil {
			return err
		}
		return tx.Set(newLookup, map[string]any{"contactId": contact.ID, "phoneNumber": contact.PhoneNumber, "whatsAppAccount": contact.WhatsAppAccount})
	})
}

func (s *Store) DeleteContact(ctx context.Context, contact Contact, deletedAt time.Time) error {
	if contact.ID == "" || contact.OrganizationID == "" {
		return ErrInvalidArgument
	}
	contact.IsDeleted = true
	contact.DeletedAt = &deletedAt
	contact.UpdatedAt = deletedAt
	contact.SearchTokens = []string{}
	return s.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		if err := tx.Set(s.contact(contact.OrganizationID, contact.ID), contact); err != nil {
			return err
		}
		return tx.Delete(s.contactPhoneLookup(contact.OrganizationID, contact.WhatsAppAccount, contact.PhoneNumber))
	})
}

func (s *Store) contactPhoneLookup(orgID, accountName, phone string) *firestore.DocumentRef {
	return s.organization(orgID).Collection("contactPhones").Doc(hash(accountName + "\x00" + phone))
}

func (s *Store) ContactByPhone(ctx context.Context, orgID, accountName, phone string) (*Contact, error) {
	lookup, err := s.contactPhoneLookup(orgID, accountName, phone).Get(ctx)
	if firestoreIsNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	contactID, err := lookup.DataAt("contactId")
	if err != nil {
		return nil, err
	}
	contact, err := s.Contact(ctx, orgID, fmt.Sprint(contactID))
	if err != nil {
		return nil, err
	}
	if contact.IsDeleted {
		return nil, ErrNotFound
	}
	return contact, nil
}

func (s *Store) PutWhatsAppAccount(ctx context.Context, account WhatsAppAccount) error {
	if account.ID == "" || account.OrganizationID == "" || account.Name == "" || account.PhoneID == "" {
		return ErrInvalidArgument
	}
	accountRef := s.account(account.OrganizationID, account.ID)
	lookupRef := s.root().Collection("accountPhones").Doc(hash(account.PhoneID))
	return s.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		if err := tx.Set(accountRef, account); err != nil {
			return err
		}
		if err := tx.Set(lookupRef, map[string]any{"accountId": account.ID, "organizationId": account.OrganizationID, "phoneId": account.PhoneID}); err != nil {
			return err
		}
		if account.WebhookVerifyToken != "" {
			tokenRef := s.root().Collection("webhookTokens").Doc(hash(account.WebhookVerifyToken))
			return tx.Set(tokenRef, map[string]any{"accountId": account.ID, "organizationId": account.OrganizationID})
		}
		return nil
	})
}

func (s *Store) ListWhatsAppAccounts(ctx context.Context, orgID string) ([]WhatsAppAccount, error) {
	it := s.organization(orgID).Collection("whatsAppAccounts").Where("isDeleted", "==", false).Documents(ctx)
	defer it.Stop()
	var accounts []WhatsAppAccount
	for {
		snapshot, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, err
		}
		var account WhatsAppAccount
		if err := snapshot.DataTo(&account); err != nil {
			return nil, err
		}
		accounts = append(accounts, account)
	}
	return accounts, nil
}

func (s *Store) ResolveWhatsAppAccount(ctx context.Context, orgID, name string) (*WhatsAppAccount, error) {
	accounts, err := s.ListWhatsAppAccounts(ctx, orgID)
	if err != nil {
		return nil, err
	}
	var fallback *WhatsAppAccount
	for i := range accounts {
		account := &accounts[i]
		if name != "" && account.Name == name {
			return account, nil
		}
		if fallback == nil || account.IsDefaultOutgoing {
			fallback = account
		}
	}
	if fallback == nil {
		return nil, ErrNotFound
	}
	return fallback, nil
}

func (s *Store) AccountByPhoneID(ctx context.Context, phoneID string) (*WhatsAppAccount, error) {
	lookup, err := s.root().Collection("accountPhones").Doc(hash(phoneID)).Get(ctx)
	if firestoreIsNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	accountID, err := lookup.DataAt("accountId")
	if err != nil {
		return nil, err
	}
	orgID, err := lookup.DataAt("organizationId")
	if err != nil {
		return nil, err
	}
	snapshot, err := s.account(fmt.Sprint(orgID), fmt.Sprint(accountID)).Get(ctx)
	if firestoreIsNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var account WhatsAppAccount
	if err := snapshot.DataTo(&account); err != nil {
		return nil, err
	}
	if account.IsDeleted {
		return nil, ErrNotFound
	}
	return &account, nil
}

func (s *Store) AccountByWebhookVerifyToken(ctx context.Context, token string) (*WhatsAppAccount, error) {
	if token == "" {
		return nil, ErrNotFound
	}
	lookup, err := s.root().Collection("webhookTokens").Doc(hash(token)).Get(ctx)
	if firestoreIsNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	accountID, err := lookup.DataAt("accountId")
	if err != nil {
		return nil, err
	}
	orgID, err := lookup.DataAt("organizationId")
	if err != nil {
		return nil, err
	}
	snapshot, err := s.account(fmt.Sprint(orgID), fmt.Sprint(accountID)).Get(ctx)
	if firestoreIsNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var account WhatsAppAccount
	if err := snapshot.DataTo(&account); err != nil {
		return nil, err
	}
	if account.IsDeleted {
		return nil, ErrNotFound
	}
	return &account, nil
}

func (s *Store) ImportWhatsAppAccounts(ctx context.Context, accounts []WhatsAppAccount) error {
	writer := s.client.BulkWriter(ctx)
	defer writer.End()
	jobs := make([]*firestore.BulkWriterJob, 0, len(accounts)*3)
	for i := range accounts {
		account := accounts[i]
		if account.ID == "" || account.OrganizationID == "" {
			return ErrInvalidArgument
		}
		job, err := writer.Set(s.account(account.OrganizationID, account.ID), account)
		if err != nil {
			return err
		}
		jobs = append(jobs, job)
		if account.IsDeleted {
			continue
		}
		lookup := map[string]any{"accountId": account.ID, "organizationId": account.OrganizationID, "phoneId": account.PhoneID}
		job, err = writer.Set(s.root().Collection("accountPhones").Doc(hash(account.PhoneID)), lookup)
		if err != nil {
			return err
		}
		jobs = append(jobs, job)
		if account.WebhookVerifyToken != "" {
			lookup = map[string]any{"accountId": account.ID, "organizationId": account.OrganizationID}
			job, err = writer.Set(s.root().Collection("webhookTokens").Doc(hash(account.WebhookVerifyToken)), lookup)
			if err != nil {
				return err
			}
			jobs = append(jobs, job)
		}
	}
	writer.Flush()
	for _, job := range jobs {
		if _, err := job.Results(); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ImportTemplates(ctx context.Context, templates []Template) error {
	return bulkSet(ctx, s.client, len(templates), func(writer *firestore.BulkWriter, i int) (*firestore.BulkWriterJob, error) {
		template := templates[i]
		if template.ID == "" || template.OrganizationID == "" {
			return nil, ErrInvalidArgument
		}
		return writer.Set(s.template(template.OrganizationID, template.ID), template)
	})
}

func (s *Store) ListTemplates(ctx context.Context, orgID string) ([]Template, error) {
	it := s.organization(orgID).Collection("templates").Where("isDeleted", "==", false).Documents(ctx)
	defer it.Stop()
	var templates []Template
	for {
		snapshot, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, err
		}
		var template Template
		if err := snapshot.DataTo(&template); err != nil {
			return nil, err
		}
		templates = append(templates, template)
	}
	return templates, nil
}

func (s *Store) Template(ctx context.Context, orgID, idOrName string) (*Template, error) {
	if snapshot, err := s.template(orgID, idOrName).Get(ctx); err == nil {
		var template Template
		if err := snapshot.DataTo(&template); err != nil {
			return nil, err
		}
		if template.IsDeleted {
			return nil, ErrNotFound
		}
		return &template, nil
	} else if !firestoreIsNotFound(err) {
		return nil, err
	}
	templates, err := s.ListTemplates(ctx, orgID)
	if err != nil {
		return nil, err
	}
	for i := range templates {
		if templates[i].Name == idOrName {
			return &templates[i], nil
		}
	}
	return nil, ErrNotFound
}

func (s *Store) Contact(ctx context.Context, orgID, contactID string) (*Contact, error) {
	snapshot, err := s.contact(orgID, contactID).Get(ctx)
	if err != nil {
		if firestoreIsNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var contact Contact
	if err := snapshot.DataTo(&contact); err != nil {
		return nil, err
	}
	if contact.IsDeleted {
		return nil, ErrNotFound
	}
	return &contact, nil
}

func (s *Store) ListContacts(ctx context.Context, orgID string, limit int, startAfter *ContactCursor) ([]Contact, error) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	query := s.organization(orgID).Collection("contacts").Where("isDeleted", "==", false).
		OrderBy("lastMessageAt", firestore.Desc).
		OrderBy(firestore.DocumentID, firestore.Desc).
		Limit(limit)
	if startAfter != nil {
		var lastMessageAt any
		if startAfter.LastMessageAt != nil {
			lastMessageAt = *startAfter.LastMessageAt
		}
		query = query.StartAfter(lastMessageAt, startAfter.ID)
	}
	it := query.Documents(ctx)
	defer it.Stop()
	contacts := make([]Contact, 0, limit)
	for {
		snapshot, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, err
		}
		var contact Contact
		if err := snapshot.DataTo(&contact); err != nil {
			return nil, err
		}
		contacts = append(contacts, contact)
	}
	return contacts, nil
}

func (s *Store) ListContactsByTags(ctx context.Context, orgID string, tags []string, limit int, startAfter *ContactCursor) ([]Contact, error) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	values := uniqueStrings(tags, 30)
	if len(values) == 0 {
		return s.ListContacts(ctx, orgID, limit, startAfter)
	}
	query := s.organization(orgID).Collection("contacts").Where("isDeleted", "==", false).
		Where("tags", "array-contains-any", values).
		OrderBy("lastMessageAt", firestore.Desc).
		OrderBy(firestore.DocumentID, firestore.Desc).
		Limit(limit)
	if startAfter != nil {
		var lastMessageAt any
		if startAfter.LastMessageAt != nil {
			lastMessageAt = *startAfter.LastMessageAt
		}
		query = query.StartAfter(lastMessageAt, startAfter.ID)
	}
	return readContacts(ctx, query, limit)
}

func (s *Store) SearchContacts(ctx context.Context, orgID, search string, limit int) ([]Contact, int64, error) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	token := normalizeSearchTerm(search)
	if token == "" {
		return nil, 0, ErrInvalidArgument
	}
	query := s.organization(orgID).Collection("contacts").Where("searchTokens", "array-contains", token)
	result, err := query.NewAggregationQuery().WithCount("total").Get(ctx)
	if err != nil {
		return nil, 0, err
	}
	value, ok := result["total"].(*pb.Value)
	if !ok {
		return nil, 0, errors.New("firestore search count result is invalid")
	}
	it := query.Limit(limit).Documents(ctx)
	defer it.Stop()
	contacts := make([]Contact, 0, limit)
	for {
		snapshot, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, 0, err
		}
		var contact Contact
		if err := snapshot.DataTo(&contact); err != nil {
			return nil, 0, err
		}
		if !contact.IsDeleted {
			contacts = append(contacts, contact)
		}
	}
	return contacts, value.GetIntegerValue(), nil
}

func BuildContactSearchTokens(contact Contact) []string {
	if contact.IsDeleted {
		return []string{}
	}
	values := []string{
		contact.ProfileName, contact.FirstName, contact.LastName,
		strings.TrimSpace(contact.FirstName + " " + contact.LastName), contact.CompanyName,
		contact.CustomerID, contact.Email,
	}
	tokens := make(map[string]struct{})
	addPrefixes(tokens, digitsOnly(contact.PhoneNumber))
	for _, value := range values {
		normalized := normalizeSearch(value)
		if normalized == "" {
			continue
		}
		addPrefixes(tokens, normalized)
		for _, word := range strings.Fields(normalized) {
			addPrefixes(tokens, word)
		}
	}
	result := make([]string, 0, len(tokens))
	for token := range tokens {
		result = append(result, token)
	}
	sort.Strings(result)
	return result
}

func normalizeSearchTerm(value string) string {
	for _, char := range value {
		if unicode.IsLetter(char) {
			return normalizeSearch(value)
		}
	}
	return digitsOnly(value)
}

func digitsOnly(value string) string {
	var output strings.Builder
	for _, char := range value {
		if unicode.IsDigit(char) {
			output.WriteRune(char)
		}
	}
	return output.String()
}

func normalizeSearch(value string) string {
	var output strings.Builder
	spacePending := false
	for _, char := range strings.ToLower(strings.TrimSpace(value)) {
		if unicode.IsLetter(char) || unicode.IsDigit(char) {
			if spacePending && output.Len() > 0 {
				output.WriteByte(' ')
			}
			spacePending = false
			output.WriteRune(char)
		} else if output.Len() > 0 {
			spacePending = true
		}
	}
	return strings.TrimSpace(output.String())
}

func addPrefixes(target map[string]struct{}, value string) {
	runes := []rune(value)
	if len(runes) > 64 {
		runes = runes[:64]
	}
	for length := 1; length <= len(runes); length++ {
		target[string(runes[:length])] = struct{}{}
	}
}

func (s *Store) CountContacts(ctx context.Context, orgID string) (int64, error) {
	query := s.organization(orgID).Collection("contacts").Where("isDeleted", "==", false)
	result, err := query.NewAggregationQuery().WithCount("total").Get(ctx)
	if err != nil {
		return 0, err
	}
	value, ok := result["total"]
	if !ok {
		return 0, errors.New("firestore count result is missing")
	}
	switch count := value.(type) {
	case *pb.Value:
		return count.GetIntegerValue(), nil
	case int64:
		return count, nil
	case int:
		return int64(count), nil
	default:
		return 0, fmt.Errorf("unexpected firestore count type %T", value)
	}
}

func (s *Store) CountContactsByTags(ctx context.Context, orgID string, tags []string) (int64, error) {
	values := uniqueStrings(tags, 30)
	if len(values) == 0 {
		return s.CountContacts(ctx, orgID)
	}
	query := s.organization(orgID).Collection("contacts").Where("isDeleted", "==", false).Where("tags", "array-contains-any", values)
	result, err := query.NewAggregationQuery().WithCount("total").Get(ctx)
	if err != nil {
		return 0, err
	}
	return aggregationCount(result, "total")
}

func readContacts(ctx context.Context, query firestore.Query, capacity int) ([]Contact, error) {
	it := query.Documents(ctx)
	defer it.Stop()
	contacts := make([]Contact, 0, capacity)
	for {
		snapshot, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, err
		}
		var contact Contact
		if err := snapshot.DataTo(&contact); err != nil {
			return nil, err
		}
		contacts = append(contacts, contact)
	}
	return contacts, nil
}

func aggregationCount(result firestore.AggregationResult, key string) (int64, error) {
	value, ok := result[key]
	if !ok {
		return 0, errors.New("firestore count result is missing")
	}
	switch count := value.(type) {
	case *pb.Value:
		return count.GetIntegerValue(), nil
	case int64:
		return count, nil
	case int:
		return int64(count), nil
	default:
		return 0, fmt.Errorf("unexpected firestore count type %T", value)
	}
}

func uniqueStrings(values []string, max int) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
		if len(result) == max {
			break
		}
	}
	return result
}

func (s *Store) ImportContacts(ctx context.Context, contacts []Contact) error {
	writer := s.client.BulkWriter(ctx)
	defer writer.End()
	jobs := make([]*firestore.BulkWriterJob, 0, len(contacts)*2)
	for _, contact := range contacts {
		if contact.ID == "" || contact.OrganizationID == "" {
			return ErrInvalidArgument
		}
		job, err := writer.Set(s.contact(contact.OrganizationID, contact.ID), contact)
		if err != nil {
			return err
		}
		jobs = append(jobs, job)
		if !contact.IsDeleted {
			lookup := map[string]any{"contactId": contact.ID, "phoneNumber": contact.PhoneNumber, "whatsAppAccount": contact.WhatsAppAccount}
			job, err = writer.Set(s.contactPhoneLookup(contact.OrganizationID, contact.WhatsAppAccount, contact.PhoneNumber), lookup)
			if err != nil {
				return err
			}
			jobs = append(jobs, job)
		}
	}
	writer.Flush()
	for _, job := range jobs {
		if _, err := job.Results(); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) MergeContactIfNewer(ctx context.Context, contact Contact) error {
	ref := s.contact(contact.OrganizationID, contact.ID)
	lookup := s.contactPhoneLookup(contact.OrganizationID, contact.WhatsAppAccount, contact.PhoneNumber)
	return s.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		snapshot, err := tx.Get(ref)
		if err == nil {
			var current Contact
			if err := snapshot.DataTo(&current); err != nil {
				return err
			}
			if current.UpdatedAt.After(contact.UpdatedAt) {
				return nil
			}
		} else if !firestoreIsNotFound(err) {
			return err
		}
		if err := tx.Set(ref, contact); err != nil {
			return err
		}
		if contact.IsDeleted {
			return tx.Delete(lookup)
		}
		return tx.Set(lookup, map[string]any{"contactId": contact.ID})
	})
}

func (s *Store) ImportMessages(ctx context.Context, messages []Message) error {
	writer := s.client.BulkWriter(ctx)
	defer writer.End()
	jobs := make([]*firestore.BulkWriterJob, 0, len(messages)*2)
	for _, message := range messages {
		if message.ID == "" || message.OrganizationID == "" || message.ContactID == "" {
			return ErrInvalidArgument
		}
		job, err := writer.Set(s.message(message.OrganizationID, message.ContactID, message.ID), message)
		if err != nil {
			return err
		}
		jobs = append(jobs, job)
		job, err = writer.Set(s.root().Collection("messageIds").Doc(message.ID), map[string]any{
			"organizationId": message.OrganizationID,
			"contactId":      message.ContactID,
		})
		if err != nil {
			return err
		}
		jobs = append(jobs, job)
	}
	writer.Flush()
	for _, job := range jobs {
		if _, err := job.Results(); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) MergeMessageIfNewer(ctx context.Context, message Message) error {
	ref := s.message(message.OrganizationID, message.ContactID, message.ID)
	return s.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		snapshot, err := tx.Get(ref)
		if err == nil {
			var current Message
			if err := snapshot.DataTo(&current); err != nil {
				return err
			}
			if current.UpdatedAt.After(message.UpdatedAt) {
				return nil
			}
		} else if !firestoreIsNotFound(err) {
			return err
		}
		if err := tx.Set(ref, message); err != nil {
			return err
		}
		if err := tx.Set(s.root().Collection("messageIds").Doc(message.ID), map[string]any{"organizationId": message.OrganizationID, "contactId": message.ContactID}); err != nil {
			return err
		}
		if message.ExternalID != "" {
			return tx.Set(s.root().Collection("externalMessages").Doc(hash(message.ExternalID)), map[string]any{"organizationId": message.OrganizationID, "contactId": message.ContactID, "messageId": message.ID, "externalId": message.ExternalID})
		}
		return nil
	})
}

func (s *Store) ImportConversationNotes(ctx context.Context, notes []ConversationNote) error {
	return bulkSet(ctx, s.client, len(notes), func(writer *firestore.BulkWriter, index int) (*firestore.BulkWriterJob, error) {
		note := notes[index]
		if note.ID == "" || note.OrganizationID == "" || note.ContactID == "" {
			return nil, ErrInvalidArgument
		}
		return writer.Set(s.note(note.OrganizationID, note.ContactID, note.ID), note)
	})
}

func (s *Store) ListConversationNotes(ctx context.Context, orgID, contactID string, limit int, beforeID string) ([]ConversationNote, int64, error) {
	if limit < 1 || limit > 100 {
		limit = 30
	}
	collection := s.contact(orgID, contactID).Collection("notes")
	query := collection.OrderBy("createdAt", firestore.Desc).OrderBy(firestore.DocumentID, firestore.Desc)
	if beforeID != "" {
		before, err := s.note(orgID, contactID, beforeID).Get(ctx)
		if err != nil {
			if firestoreIsNotFound(err) {
				return nil, 0, ErrNotFound
			}
			return nil, 0, err
		}
		var cursor ConversationNote
		if err := before.DataTo(&cursor); err != nil {
			return nil, 0, err
		}
		query = query.StartAfter(cursor.CreatedAt, cursor.ID)
	}
	countResult, err := collection.NewAggregationQuery().WithCount("total").Get(ctx)
	if err != nil {
		return nil, 0, err
	}
	total, err := aggregationCount(countResult, "total")
	if err != nil {
		return nil, 0, err
	}
	it := query.Limit(limit).Documents(ctx)
	defer it.Stop()
	notes := make([]ConversationNote, 0, limit)
	for {
		snapshot, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, 0, err
		}
		var note ConversationNote
		if err := snapshot.DataTo(&note); err != nil {
			return nil, 0, err
		}
		notes = append(notes, note)
	}
	for left, right := 0, len(notes)-1; left < right; left, right = left+1, right-1 {
		notes[left], notes[right] = notes[right], notes[left]
	}
	return notes, total, nil
}

func (s *Store) CreateConversationNote(ctx context.Context, note ConversationNote) error {
	if note.ID == "" || note.OrganizationID == "" || note.ContactID == "" || note.CreatedByID == "" || strings.TrimSpace(note.Content) == "" {
		return ErrInvalidArgument
	}
	event := Event{ID: uuid.NewString(), OrganizationID: note.OrganizationID, Type: "conversation_note_created", Payload: structToMap(note), CreatedAt: note.CreatedAt, ExpiresAt: note.CreatedAt.Add(24 * time.Hour)}
	return s.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		if err := tx.Create(s.note(note.OrganizationID, note.ContactID, note.ID), note); err != nil {
			return err
		}
		return tx.Create(s.event(note.OrganizationID, event.ID), event)
	})
}

func (s *Store) UpdateConversationNote(ctx context.Context, orgID, contactID, noteID, userID, content string, updatedAt time.Time) (*ConversationNote, error) {
	ref := s.note(orgID, contactID, noteID)
	var updated ConversationNote
	err := s.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		snapshot, err := tx.Get(ref)
		if err != nil {
			if firestoreIsNotFound(err) {
				return ErrNotFound
			}
			return err
		}
		if err := snapshot.DataTo(&updated); err != nil {
			return err
		}
		if updated.CreatedByID != userID {
			return ErrForbidden
		}
		updated.Content = content
		updated.UpdatedAt = updatedAt
		event := Event{ID: uuid.NewString(), OrganizationID: orgID, Type: "conversation_note_updated", Payload: structToMap(updated), CreatedAt: updatedAt, ExpiresAt: updatedAt.Add(24 * time.Hour)}
		if err := tx.Set(ref, updated); err != nil {
			return err
		}
		return tx.Create(s.event(orgID, event.ID), event)
	})
	return &updated, err
}

func (s *Store) DeleteConversationNote(ctx context.Context, orgID, contactID, noteID, userID string, deletedAt time.Time) error {
	ref := s.note(orgID, contactID, noteID)
	return s.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		snapshot, err := tx.Get(ref)
		if err != nil {
			if firestoreIsNotFound(err) {
				return ErrNotFound
			}
			return err
		}
		var note ConversationNote
		if err := snapshot.DataTo(&note); err != nil {
			return err
		}
		if note.CreatedByID != userID {
			return ErrForbidden
		}
		payload := map[string]any{"id": note.ID, "contact_id": note.ContactID}
		event := Event{ID: uuid.NewString(), OrganizationID: orgID, Type: "conversation_note_deleted", Payload: payload, CreatedAt: deletedAt, ExpiresAt: deletedAt.Add(24 * time.Hour)}
		if err := tx.Delete(ref); err != nil {
			return err
		}
		return tx.Create(s.event(orgID, event.ID), event)
	})
}

func structToMap(value any) map[string]any {
	payload, _ := json.Marshal(value)
	result := make(map[string]any)
	_ = json.Unmarshal(payload, &result)
	return result
}

func (s *Store) CreateOutgoingMessage(ctx context.Context, contact Contact, message Message) error {
	if contact.ID == "" || contact.OrganizationID == "" || message.ID == "" || message.ExternalID == "" {
		return ErrInvalidArgument
	}
	contactRef := s.contact(contact.OrganizationID, contact.ID)
	messageRef := s.message(contact.OrganizationID, contact.ID, message.ID)
	messageIDRef := s.root().Collection("messageIds").Doc(message.ID)
	lookupRef := s.root().Collection("externalMessages").Doc(hash(message.ExternalID))
	event := messageEvent(contact, message)
	return s.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		if err := tx.Set(contactRef, contact); err != nil {
			return err
		}
		if err := tx.Create(messageRef, message); err != nil {
			return err
		}
		if err := tx.Set(messageIDRef, map[string]any{"organizationId": contact.OrganizationID, "contactId": contact.ID}); err != nil {
			return err
		}
		if err := tx.Set(lookupRef, map[string]any{
			"organizationId": contact.OrganizationID,
			"contactId":      contact.ID,
			"messageId":      message.ID,
			"externalId":     message.ExternalID,
		}); err != nil {
			return err
		}
		return tx.Create(s.event(contact.OrganizationID, event.ID), event)
	})
}

func messageEvent(contact Contact, message Message) Event {
	createdAt := message.CreatedAt
	return Event{
		ID: uuid.NewString(), OrganizationID: contact.OrganizationID, Type: "new_message",
		Payload: map[string]any{
			"id": message.ID, "contact_id": message.ContactID, "direction": message.Direction,
			"message_type": message.MessageType, "content": map[string]string{"body": message.Content},
			"media_url": message.MediaURL, "media_mime_type": message.MediaMimeType,
			"media_filename": message.MediaFilename, "interactive_data": message.InteractiveData,
			"status": message.Status, "wamid": message.ExternalID, "error_message": message.ErrorMessage,
			"is_reply": message.IsReply, "reply_to_message_id": message.ReplyToMessageID,
			"whatsapp_account": message.WhatsAppAccount, "profile_name": contact.ProfileName,
			"assigned_user_id": contact.AssignedUserID, "created_at": createdAt, "updated_at": message.UpdatedAt,
		},
		CreatedAt: createdAt, ExpiresAt: createdAt.Add(48 * time.Hour),
	}
}

func (s *Store) ListEvents(ctx context.Context, orgID string, limit int, after *EventCursor) ([]Event, error) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	query := s.organization(orgID).Collection("events").OrderBy("createdAt", firestore.Asc).OrderBy(firestore.DocumentID, firestore.Asc).Limit(limit)
	if after != nil {
		query = query.StartAfter(after.CreatedAt, after.ID)
	}
	it := query.Documents(ctx)
	defer it.Stop()
	events := make([]Event, 0, limit)
	for {
		snapshot, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, err
		}
		var event Event
		if err := snapshot.DataTo(&event); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, nil
}

func bulkSet(ctx context.Context, client *firestore.Client, size int, add func(*firestore.BulkWriter, int) (*firestore.BulkWriterJob, error)) error {
	writer := client.BulkWriter(ctx)
	defer writer.End()
	jobs := make([]*firestore.BulkWriterJob, 0, size)
	for i := 0; i < size; i++ {
		job, err := add(writer, i)
		if err != nil {
			return err
		}
		jobs = append(jobs, job)
	}
	writer.Flush()
	for _, job := range jobs {
		if _, err := job.Results(); err != nil {
			return err
		}
	}
	return nil
}

// CreateInboundMessage atomically deduplicates a Meta event, writes the
// message, and updates the conversation projection.
func (s *Store) CreateInboundMessage(ctx context.Context, contact Contact, message Message) error {
	if contact.ID == "" || contact.OrganizationID == "" || message.ID == "" || message.ExternalID == "" {
		return ErrInvalidArgument
	}
	receiptRef := s.organization(contact.OrganizationID).Collection("webhookReceipts").Doc(hash(message.ExternalID))
	contactRef := s.contact(contact.OrganizationID, contact.ID)
	messageRef := s.message(contact.OrganizationID, contact.ID, message.ID)
	messageIDRef := s.root().Collection("messageIds").Doc(message.ID)
	phoneRef := s.contactPhoneLookup(contact.OrganizationID, contact.WhatsAppAccount, contact.PhoneNumber)
	externalRef := s.root().Collection("externalMessages").Doc(hash(message.ExternalID))
	event := messageEvent(contact, message)
	return s.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		if _, err := tx.Get(receiptRef); err == nil {
			return ErrDuplicateEvent
		} else if !firestoreIsNotFound(err) {
			return err
		}
		now := time.Now().UTC()
		if err := tx.Create(receiptRef, WebhookReceipt{ExternalID: message.ExternalID, CreatedAt: now, ExpiresAt: now.Add(7 * 24 * time.Hour)}); err != nil {
			return err
		}
		if err := tx.Set(contactRef, contact); err != nil {
			return err
		}
		if err := tx.Set(phoneRef, map[string]any{"contactId": contact.ID, "phoneNumber": contact.PhoneNumber, "whatsAppAccount": contact.WhatsAppAccount}); err != nil {
			return err
		}
		if err := tx.Create(messageRef, message); err != nil {
			return err
		}
		if err := tx.Set(messageIDRef, map[string]any{"organizationId": contact.OrganizationID, "contactId": contact.ID}); err != nil {
			return err
		}
		if err := tx.Set(externalRef, map[string]any{"organizationId": contact.OrganizationID, "contactId": contact.ID, "messageId": message.ID, "externalId": message.ExternalID}); err != nil {
			return err
		}
		return tx.Create(s.event(contact.OrganizationID, event.ID), event)
	})
}

func (s *Store) UpdateMessageStatus(ctx context.Context, externalID, messageStatus, errorMessage string, updatedAt time.Time) error {
	lookup, err := s.root().Collection("externalMessages").Doc(hash(externalID)).Get(ctx)
	if firestoreIsNotFound(err) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	orgID, err := lookup.DataAt("organizationId")
	if err != nil {
		return err
	}
	contactID, err := lookup.DataAt("contactId")
	if err != nil {
		return err
	}
	messageID, err := lookup.DataAt("messageId")
	if err != nil {
		return err
	}
	org := fmt.Sprint(orgID)
	contact := fmt.Sprint(contactID)
	message := fmt.Sprint(messageID)
	eventID := uuid.NewString()
	event := Event{
		ID: eventID, OrganizationID: org, Type: "status_update",
		Payload:   map[string]any{"message_id": message, "status": messageStatus, "error_message": errorMessage},
		CreatedAt: updatedAt, ExpiresAt: updatedAt.Add(48 * time.Hour),
	}
	return s.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		if err := tx.Update(s.message(org, contact, message), []firestore.Update{
			{Path: "status", Value: messageStatus},
			{Path: "errorMessage", Value: errorMessage},
			{Path: "updatedAt", Value: updatedAt},
		}); err != nil {
			return err
		}
		return tx.Create(s.event(org, eventID), event)
	})
}

func (s *Store) MarkContactRead(ctx context.Context, orgID, contactID string, updatedAt time.Time) error {
	contact, err := s.Contact(ctx, orgID, contactID)
	if err != nil {
		return err
	}
	contact.IsRead = true
	contact.UpdatedAt = updatedAt
	writer := s.client.BulkWriter(ctx)
	defer writer.End()
	jobs := make([]*firestore.BulkWriterJob, 0, 101)
	job, err := writer.Set(s.contact(orgID, contactID), contact)
	if err != nil {
		return err
	}
	jobs = append(jobs, job)
	messages, err := s.ListMessages(ctx, orgID, contactID, 100, nil)
	if err != nil {
		return err
	}
	for _, message := range messages {
		if message.Direction != "incoming" || message.Status == "read" {
			continue
		}
		job, err = writer.Update(s.message(orgID, contactID, message.ID), []firestore.Update{
			{Path: "status", Value: "read"}, {Path: "updatedAt", Value: updatedAt},
		})
		if err != nil {
			return err
		}
		jobs = append(jobs, job)
	}
	writer.Flush()
	for _, pending := range jobs {
		if _, err := pending.Results(); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ListMessages(ctx context.Context, orgID, contactID string, limit int, before *time.Time) ([]Message, error) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	query := s.contact(orgID, contactID).Collection("messages").OrderBy("createdAt", firestore.Desc).Limit(limit)
	if before != nil {
		query = query.StartAfter(*before)
	}
	it := query.Documents(ctx)
	defer it.Stop()
	messages := make([]Message, 0, limit)
	for {
		snapshot, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, err
		}
		var message Message
		if err := snapshot.DataTo(&message); err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	for left, right := 0, len(messages)-1; left < right; left, right = left+1, right-1 {
		messages[left], messages[right] = messages[right], messages[left]
	}
	return messages, nil
}

func (s *Store) Message(ctx context.Context, orgID, contactID, messageID string) (*Message, error) {
	snapshot, err := s.message(orgID, contactID, messageID).Get(ctx)
	if firestoreIsNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var message Message
	if err := snapshot.DataTo(&message); err != nil {
		return nil, err
	}
	return &message, nil
}

func (s *Store) MessageByID(ctx context.Context, orgID, messageID string) (*Message, error) {
	lookup, err := s.root().Collection("messageIds").Doc(messageID).Get(ctx)
	if err != nil {
		if firestoreIsNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	lookupOrg, err := lookup.DataAt("organizationId")
	if err != nil || fmt.Sprint(lookupOrg) != orgID {
		return nil, ErrNotFound
	}
	contactID, err := lookup.DataAt("contactId")
	if err != nil {
		return nil, err
	}
	return s.Message(ctx, orgID, fmt.Sprint(contactID), messageID)
}

func (s *Store) UpdateMessageReaction(ctx context.Context, orgID, contactID, messageID, userID, emoji string, updatedAt time.Time) (*Message, []map[string]any, error) {
	ref := s.message(orgID, contactID, messageID)
	var output Message
	var reactions []map[string]any
	err := s.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		snapshot, err := tx.Get(ref)
		if firestoreIsNotFound(err) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if err := snapshot.DataTo(&output); err != nil {
			return err
		}
		if output.Metadata == nil {
			output.Metadata = map[string]any{}
		}
		if raw, ok := output.Metadata["reactions"].([]any); ok {
			for _, item := range raw {
				if reaction, ok := item.(map[string]any); ok && fmt.Sprint(reaction["from_user"]) != userID {
					reactions = append(reactions, reaction)
				}
			}
		} else if raw, ok := output.Metadata["reactions"].([]map[string]any); ok {
			for _, reaction := range raw {
				if fmt.Sprint(reaction["from_user"]) != userID {
					reactions = append(reactions, reaction)
				}
			}
		}
		if emoji != "" {
			reactions = append(reactions, map[string]any{"emoji": emoji, "from_user": userID})
		}
		output.Metadata["reactions"] = reactions
		output.UpdatedAt = updatedAt
		return tx.Set(ref, output)
	})
	return &output, reactions, err
}

func (s *Store) PutRefreshToken(ctx context.Context, jti string, token RefreshToken) error {
	if jti == "" || token.UserID == "" {
		return ErrInvalidArgument
	}
	_, err := s.root().Collection("refreshTokens").Doc(hash(jti)).Create(ctx, token)
	return err
}

func (s *Store) ConsumeRefreshToken(ctx context.Context, jti, userID string) error {
	ref := s.root().Collection("refreshTokens").Doc(hash(jti))
	return s.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		snapshot, err := tx.Get(ref)
		if err != nil {
			if firestoreIsNotFound(err) {
				return ErrNotFound
			}
			return err
		}
		var token RefreshToken
		if err := snapshot.DataTo(&token); err != nil {
			return err
		}
		if token.UserID != userID || time.Now().After(token.ExpiresAt) {
			return ErrNotFound
		}
		return tx.Delete(ref)
	})
}

func (s *Store) ImportAgentTransfers(ctx context.Context, transfers []AgentTransfer) error {
	writer := s.client.BulkWriter(ctx)
	defer writer.End()
	jobs := make([]*firestore.BulkWriterJob, 0, len(transfers)*2)
	oldestActive := make(map[string]AgentTransfer)
	for _, transfer := range transfers {
		job, err := writer.Set(s.transfer(transfer.OrganizationID, transfer.ID), transfer)
		if err != nil {
			return err
		}
		jobs = append(jobs, job)
		if transfer.Status == "active" {
			key := transfer.OrganizationID + "\x00" + transfer.ContactID
			current, exists := oldestActive[key]
			if !exists || transfer.TransferredAt.Before(current.TransferredAt) {
				oldestActive[key] = transfer
			}
		}
	}
	for _, transfer := range oldestActive {
		job, err := writer.Set(s.organization(transfer.OrganizationID).Collection("activeTransfers").Doc(transfer.ContactID), map[string]any{"transferId": transfer.ID})
		if err != nil {
			return err
		}
		jobs = append(jobs, job)
	}
	writer.Flush()
	for _, job := range jobs {
		if _, err := job.Results(); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) MergeAgentTransferIfNewer(ctx context.Context, transfer AgentTransfer) error {
	ref := s.transfer(transfer.OrganizationID, transfer.ID)
	activeRef := s.organization(transfer.OrganizationID).Collection("activeTransfers").Doc(transfer.ContactID)
	return s.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		snapshot, err := tx.Get(ref)
		if err == nil {
			var current AgentTransfer
			if err := snapshot.DataTo(&current); err != nil {
				return err
			}
			if current.UpdatedAt.After(transfer.UpdatedAt) {
				return nil
			}
		} else if !firestoreIsNotFound(err) {
			return err
		}
		if err := tx.Set(ref, transfer); err != nil {
			return err
		}
		if transfer.Status == "active" {
			return tx.Set(activeRef, map[string]any{"transferId": transfer.ID})
		}
		return tx.Delete(activeRef)
	})
}

func (s *Store) ListAgentTransfers(ctx context.Context, orgID, transferStatus string, limit, offset int) ([]AgentTransfer, int64, error) {
	if limit < 1 || limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	query := s.organization(orgID).Collection("agentTransfers").Query
	if transferStatus != "" {
		query = query.Where("status", "==", transferStatus)
	}
	if transferStatus == "resumed" {
		query = query.OrderBy("resumedAt", firestore.Desc)
	} else {
		query = query.OrderBy("transferredAt", firestore.Asc)
	}
	countResult, err := query.NewAggregationQuery().WithCount("total").Get(ctx)
	if err != nil {
		return nil, 0, err
	}
	total, err := aggregationCount(countResult, "total")
	if err != nil {
		return nil, 0, err
	}
	it := query.Offset(offset).Limit(limit).Documents(ctx)
	defer it.Stop()
	result := make([]AgentTransfer, 0, limit)
	for {
		snapshot, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, 0, err
		}
		var transfer AgentTransfer
		if err := snapshot.DataTo(&transfer); err != nil {
			return nil, 0, err
		}
		result = append(result, transfer)
	}
	return result, total, nil
}

func (s *Store) CountGeneralActiveTransfers(ctx context.Context, orgID string) (int64, error) {
	query := s.organization(orgID).Collection("agentTransfers").
		Where("status", "==", "active").
		Where("agentId", "==", "").
		Where("teamId", "==", "")
	result, err := query.NewAggregationQuery().WithCount("total").Get(ctx)
	if err != nil {
		return 0, err
	}
	return aggregationCount(result, "total")
}

func (s *Store) ActiveTransfer(ctx context.Context, orgID, contactID string) (*AgentTransfer, error) {
	lookup, err := s.organization(orgID).Collection("activeTransfers").Doc(contactID).Get(ctx)
	if firestoreIsNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	id, err := lookup.DataAt("transferId")
	if err != nil {
		return nil, err
	}
	snapshot, err := s.transfer(orgID, fmt.Sprint(id)).Get(ctx)
	if firestoreIsNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var transfer AgentTransfer
	if err := snapshot.DataTo(&transfer); err != nil {
		return nil, err
	}
	return &transfer, nil
}

func (s *Store) CreateAgentTransfer(ctx context.Context, transfer AgentTransfer) error {
	active := s.organization(transfer.OrganizationID).Collection("activeTransfers").Doc(transfer.ContactID)
	return s.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		if _, err := tx.Get(active); err == nil {
			return ErrConflict
		} else if !firestoreIsNotFound(err) {
			return err
		}
		if err := tx.Create(s.transfer(transfer.OrganizationID, transfer.ID), transfer); err != nil {
			return err
		}
		return tx.Create(active, map[string]any{"transferId": transfer.ID})
	})
}

func (s *Store) ResumeAgentTransfer(ctx context.Context, orgID, transferID, userID, userName string, now time.Time) (*AgentTransfer, error) {
	ref := s.transfer(orgID, transferID)
	var output AgentTransfer
	err := s.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		snapshot, err := tx.Get(ref)
		if firestoreIsNotFound(err) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if err := snapshot.DataTo(&output); err != nil {
			return err
		}
		if output.Status != "active" {
			return ErrConflict
		}
		output.Status, output.ResumedBy, output.ResumedByName = "resumed", userID, userName
		output.ResumedAt, output.UpdatedAt = &now, now
		if err := tx.Set(ref, output); err != nil {
			return err
		}
		return tx.Delete(s.organization(orgID).Collection("activeTransfers").Doc(output.ContactID))
	})
	if err == nil {
		query := s.organization(orgID).Collection("agentTransfers").Where("contactId", "==", output.ContactID).Where("status", "==", "active").OrderBy("transferredAt", firestore.Asc).Limit(1)
		it := query.Documents(ctx)
		snapshot, nextErr := it.Next()
		it.Stop()
		if nextErr == nil {
			var next AgentTransfer
			if decodeErr := snapshot.DataTo(&next); decodeErr != nil {
				return &output, decodeErr
			}
			_, err = s.organization(orgID).Collection("activeTransfers").Doc(output.ContactID).Set(ctx, map[string]any{"transferId": next.ID})
		} else if !errors.Is(nextErr, iterator.Done) {
			err = nextErr
		}
	}
	return &output, err
}

func (s *Store) AssignAgentTransfer(ctx context.Context, orgID, transferID, agentID, agentName string, now time.Time) (*AgentTransfer, error) {
	ref := s.transfer(orgID, transferID)
	var output AgentTransfer
	err := s.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		snapshot, err := tx.Get(ref)
		if firestoreIsNotFound(err) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if err := snapshot.DataTo(&output); err != nil {
			return err
		}
		if output.Status != "active" {
			return ErrConflict
		}
		output.AgentID, output.AgentName, output.UpdatedAt = agentID, agentName, now
		if agentID != "" && output.PickedUpAt == nil {
			output.PickedUpAt = &now
		}
		return tx.Set(ref, output)
	})
	return &output, err
}

func (s *Store) ImportChatbotSessions(ctx context.Context, sessions []ChatbotSession) error {
	return bulkSet(ctx, s.client, len(sessions), func(writer *firestore.BulkWriter, i int) (*firestore.BulkWriterJob, error) {
		return writer.Set(s.session(sessions[i].OrganizationID, sessions[i].ID), sessions[i])
	})
}

func (s *Store) MergeChatbotSessionIfNewer(ctx context.Context, session ChatbotSession) error {
	ref := s.session(session.OrganizationID, session.ID)
	return s.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		snapshot, err := tx.Get(ref)
		if err == nil {
			var current ChatbotSession
			if err := snapshot.DataTo(&current); err != nil {
				return err
			}
			if current.UpdatedAt.After(session.UpdatedAt) {
				return nil
			}
		} else if !firestoreIsNotFound(err) {
			return err
		}
		return tx.Set(ref, session)
	})
}

func (s *Store) LatestChatbotSession(ctx context.Context, orgID, contactID string) (*ChatbotSession, error) {
	it := s.organization(orgID).Collection("chatbotSessions").Where("contactId", "==", contactID).OrderBy("createdAt", firestore.Desc).Limit(20).Documents(ctx)
	defer it.Stop()
	for {
		snapshot, err := it.Next()
		if errors.Is(err, iterator.Done) {
			return nil, ErrNotFound
		}
		if err != nil {
			return nil, err
		}
		var session ChatbotSession
		if err := snapshot.DataTo(&session); err != nil {
			return nil, err
		}
		if session.Status == "active" || session.Status == "completed" {
			return &session, nil
		}
	}
}

func (s *Store) CancelActiveChatbotSessions(ctx context.Context, orgID, contactID string, now time.Time) error {
	it := s.organization(orgID).Collection("chatbotSessions").Where("contactId", "==", contactID).Where("status", "==", "active").Documents(ctx)
	defer it.Stop()
	writer := s.client.BulkWriter(ctx)
	defer writer.End()
	jobs := make([]*firestore.BulkWriterJob, 0)
	for {
		snapshot, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return err
		}
		job, err := writer.Update(snapshot.Ref, []firestore.Update{{Path: "status", Value: "cancelled"}, {Path: "completedAt", Value: now}, {Path: "updatedAt", Value: now}})
		if err != nil {
			return err
		}
		jobs = append(jobs, job)
	}
	writer.Flush()
	for _, job := range jobs {
		if _, err := job.Results(); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ImportCannedResponses(ctx context.Context, responses []CannedResponse) error {
	return bulkSet(ctx, s.client, len(responses), func(writer *firestore.BulkWriter, i int) (*firestore.BulkWriterJob, error) {
		return writer.Set(s.cannedResponse(responses[i].OrganizationID, responses[i].ID), responses[i])
	})
}

func (s *Store) ListCannedResponses(ctx context.Context, orgID string, activeOnly bool, limit int) ([]CannedResponse, error) {
	if limit < 1 || limit > 100 {
		limit = 100
	}
	query := s.organization(orgID).Collection("cannedResponses").Query
	if activeOnly {
		query = query.Where("isActive", "==", true)
	}
	it := query.Documents(ctx)
	defer it.Stop()
	result := make([]CannedResponse, 0, limit)
	for len(result) < limit {
		snapshot, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, err
		}
		var response CannedResponse
		if err := snapshot.DataTo(&response); err != nil {
			return nil, err
		}
		result = append(result, response)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].UsageCount == result[j].UsageCount {
			return result[i].Name < result[j].Name
		}
		return result[i].UsageCount > result[j].UsageCount
	})
	return result, nil
}

func (s *Store) IncrementCannedResponseUsage(ctx context.Context, orgID, id string) error {
	_, err := s.cannedResponse(orgID, id).Update(ctx, []firestore.Update{{Path: "usageCount", Value: firestore.Increment(1)}})
	if firestoreIsNotFound(err) {
		return ErrNotFound
	}
	return err
}

func hash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func firestoreIsNotFound(err error) bool {
	return status.Code(err) == codes.NotFound
}
