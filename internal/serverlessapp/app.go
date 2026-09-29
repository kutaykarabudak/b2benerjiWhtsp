package serverlessapp

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/config"
	appcrypto "github.com/shridarpatil/whatomate/internal/crypto"
	"github.com/shridarpatil/whatomate/internal/firestorestore"
	"github.com/shridarpatil/whatomate/internal/middleware"
	"github.com/shridarpatil/whatomate/internal/storage"
	"github.com/shridarpatil/whatomate/internal/templateutil"
	"github.com/shridarpatil/whatomate/pkg/whatsapp"
	"github.com/valyala/fasthttp"
	"golang.org/x/crypto/bcrypt"
)

type Store interface {
	Health(context.Context) error
	UserByEmail(context.Context, string) (*firestorestore.User, error)
	User(context.Context, string) (*firestorestore.User, error)
	PutUser(context.Context, firestorestore.User) error
	ListUsers(context.Context, string, int) ([]firestorestore.User, error)
	ListTags(context.Context, string, int) ([]firestorestore.Tag, error)
	Organization(context.Context, string) (*firestorestore.Organization, error)
	ListOrganizations(context.Context, int) ([]firestorestore.Organization, error)
	PutRefreshToken(context.Context, string, firestorestore.RefreshToken) error
	ConsumeRefreshToken(context.Context, string, string) error
	ListContacts(context.Context, string, int, *firestorestore.ContactCursor) ([]firestorestore.Contact, error)
	ListContactsByTags(context.Context, string, []string, int, *firestorestore.ContactCursor) ([]firestorestore.Contact, error)
	SearchContacts(context.Context, string, string, int) ([]firestorestore.Contact, int64, error)
	CountContacts(context.Context, string) (int64, error)
	CountContactsByTags(context.Context, string, []string) (int64, error)
	Contact(context.Context, string, string) (*firestorestore.Contact, error)
	CreateContact(context.Context, firestorestore.Contact) error
	UpdateContact(context.Context, firestorestore.Contact, firestorestore.Contact) error
	DeleteContact(context.Context, firestorestore.Contact, time.Time) error
	ListMessages(context.Context, string, string, int, *time.Time) ([]firestorestore.Message, error)
	ListMessagesForAccount(context.Context, string, string, string, int, *time.Time) ([]firestorestore.Message, error)
	Message(context.Context, string, string, string) (*firestorestore.Message, error)
	MessageByID(context.Context, string, string) (*firestorestore.Message, error)
	ListWhatsAppAccounts(context.Context, string) ([]firestorestore.WhatsAppAccount, error)
	ResolveWhatsAppAccount(context.Context, string, string) (*firestorestore.WhatsAppAccount, error)
	CreateOutgoingMessage(context.Context, firestorestore.Contact, firestorestore.Message) error
	AccountByPhoneID(context.Context, string) (*firestorestore.WhatsAppAccount, error)
	AccountByWebhookVerifyToken(context.Context, string) (*firestorestore.WhatsAppAccount, error)
	ContactByPhone(context.Context, string, string, string) (*firestorestore.Contact, error)
	CreateInboundMessage(context.Context, firestorestore.Contact, firestorestore.Message) error
	UpdateMessageStatus(context.Context, string, string, string, time.Time) error
	UpdateMessageReaction(context.Context, string, string, string, string, string, time.Time) (*firestorestore.Message, []map[string]any, error)
	ListTemplates(context.Context, string) ([]firestorestore.Template, error)
	Template(context.Context, string, string) (*firestorestore.Template, error)
	MarkContactRead(context.Context, string, string, time.Time) error
	ListEvents(context.Context, string, int, *firestorestore.EventCursor) ([]firestorestore.Event, error)
	ListConversationNotes(context.Context, string, string, int, string) ([]firestorestore.ConversationNote, int64, error)
	CreateConversationNote(context.Context, firestorestore.ConversationNote) error
	UpdateConversationNote(context.Context, string, string, string, string, string, time.Time) (*firestorestore.ConversationNote, error)
	DeleteConversationNote(context.Context, string, string, string, string, time.Time) error
	ListAgentTransfers(context.Context, string, string, int, int) ([]firestorestore.AgentTransfer, int64, error)
	CountGeneralActiveTransfers(context.Context, string) (int64, error)
	ActiveTransfer(context.Context, string, string) (*firestorestore.AgentTransfer, error)
	CreateAgentTransfer(context.Context, firestorestore.AgentTransfer) error
	ResumeAgentTransfer(context.Context, string, string, string, string, time.Time) (*firestorestore.AgentTransfer, error)
	AssignAgentTransfer(context.Context, string, string, string, string, time.Time) (*firestorestore.AgentTransfer, error)
	LatestChatbotSession(context.Context, string, string) (*firestorestore.ChatbotSession, error)
	CancelActiveChatbotSessions(context.Context, string, string, time.Time) error
	ListCannedResponses(context.Context, string, bool, int) ([]firestorestore.CannedResponse, error)
	IncrementCannedResponseUsage(context.Context, string, string) error
}

type Messenger interface {
	SendTextMessage(context.Context, *whatsapp.Account, whatsapp.Recipient, string, ...string) (string, error)
	SendTemplateMessage(context.Context, *whatsapp.Account, whatsapp.Recipient, string, string, []map[string]any) (string, error)
	UploadMedia(context.Context, *whatsapp.Account, []byte, string, string) (string, error)
	SendImageMessage(context.Context, *whatsapp.Account, whatsapp.Recipient, string, string) (string, error)
	SendDocumentMessage(context.Context, *whatsapp.Account, whatsapp.Recipient, string, string, string) (string, error)
	SendVideoMessage(context.Context, *whatsapp.Account, whatsapp.Recipient, string, string) (string, error)
	SendAudioMessage(context.Context, *whatsapp.Account, whatsapp.Recipient, string) (string, error)
	SendInteractiveButtons(context.Context, *whatsapp.Account, whatsapp.Recipient, string, []whatsapp.Button) (string, error)
	SendCTAURLButton(context.Context, *whatsapp.Account, whatsapp.Recipient, string, string, string) (string, error)
	SendVoiceCallButton(context.Context, *whatsapp.Account, whatsapp.Recipient, string, string, int, string) (string, error)
	SendFlowMessage(context.Context, *whatsapp.Account, whatsapp.Recipient, string, string, string, string, string, string) (string, error)
	SendReaction(context.Context, *whatsapp.Account, whatsapp.Recipient, string, string) error
	GetMediaURL(context.Context, string, *whatsapp.Account) (string, error)
	DownloadMedia(context.Context, string, string) ([]byte, error)
}

type App struct {
	store  Store
	config *config.Config
	sender Messenger
	media  *storage.S3Client
	now    func() time.Time
}

func (a *App) SetMessenger(sender Messenger)           { a.sender = sender }
func (a *App) SetMediaStorage(media *storage.S3Client) { a.media = media }

func New(store Store, cfg *config.Config) (*App, error) {
	if store == nil || cfg == nil {
		return nil, errors.New("store and config are required")
	}
	if len(cfg.JWT.Secret) < 32 {
		return nil, errors.New("JWT secret must be at least 32 characters")
	}
	return &App{store: store, config: cfg, now: time.Now}, nil
}

func (a *App) Health(ctx *fasthttp.RequestCtx) {
	writeData(ctx, http.StatusOK, map[string]string{"service": "whatomate-firestore", "status": "ok"})
}

func (a *App) Ready(ctx *fasthttp.RequestCtx) {
	requestCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := a.store.Health(requestCtx); err != nil {
		writeError(ctx, http.StatusServiceUnavailable, "Firestore is unavailable")
		return
	}
	writeData(ctx, http.StatusOK, map[string]string{"service": "whatomate-firestore", "status": "ready"})
}

func (a *App) Login(ctx *fasthttp.RequestCtx) {
	var request struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decodeJSON(ctx, &request) || strings.TrimSpace(request.Email) == "" || request.Password == "" {
		writeError(ctx, http.StatusBadRequest, "Email and password are required")
		return
	}
	requestCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	user, err := a.store.UserByEmail(requestCtx, request.Email)
	if err != nil || !user.IsActive || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(request.Password)) != nil {
		// Keep the timing profile similar when the account does not exist.
		if err != nil {
			_ = bcrypt.CompareHashAndPassword([]byte("$2a$10$xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"), []byte(request.Password))
		}
		writeError(ctx, http.StatusUnauthorized, "Invalid credentials")
		return
	}
	access, refresh, err := a.issueTokens(requestCtx, user)
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, "Failed to generate token")
		return
	}
	a.setCookies(ctx, access, refresh)
	writeData(ctx, http.StatusOK, map[string]any{"expires_in": a.config.JWT.AccessExpiryMins * 60, "user": user})
}

func (a *App) Refresh(ctx *fasthttp.RequestCtx) {
	_, refresh, err := middleware.DecodeFirebaseSession(string(ctx.Request.Header.Cookie(middleware.FirebaseSessionCookieName)))
	if err != nil {
		refresh = string(ctx.Request.Header.Cookie("whm_refresh"))
	}
	claims, err := a.parseToken(refresh)
	if err != nil || claims.ID == "" {
		writeError(ctx, http.StatusUnauthorized, "Invalid refresh token")
		return
	}
	requestCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := a.store.ConsumeRefreshToken(requestCtx, claims.ID, claims.UserID.String()); err != nil {
		writeError(ctx, http.StatusUnauthorized, "Refresh token has been revoked")
		return
	}
	user, err := a.store.User(requestCtx, claims.UserID.String())
	if err != nil || !user.IsActive {
		writeError(ctx, http.StatusUnauthorized, "User not found")
		return
	}
	access, newRefresh, err := a.issueTokens(requestCtx, user)
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, "Failed to generate token")
		return
	}
	a.setCookies(ctx, access, newRefresh)
	writeData(ctx, http.StatusOK, map[string]any{"expires_in": a.config.JWT.AccessExpiryMins * 60, "user": user})
}

func (a *App) Logout(ctx *fasthttp.RequestCtx) {
	_, refresh, _ := middleware.DecodeFirebaseSession(string(ctx.Request.Header.Cookie(middleware.FirebaseSessionCookieName)))
	if claims, err := a.parseToken(refresh); err == nil && claims.ID != "" {
		requestCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		_ = a.store.ConsumeRefreshToken(requestCtx, claims.ID, claims.UserID.String())
		cancel()
	}
	a.clearCookies(ctx)
	writeData(ctx, http.StatusOK, map[string]string{"status": "logged_out"})
}

func (a *App) Me(ctx *fasthttp.RequestCtx) {
	claims, ok := a.authorize(ctx)
	if !ok {
		return
	}
	requestCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	user, err := a.store.User(requestCtx, claims.UserID.String())
	if err != nil || !user.IsActive {
		writeError(ctx, http.StatusUnauthorized, "User not found")
		return
	}
	writeData(ctx, http.StatusOK, user)
}

func (a *App) ListUsers(ctx *fasthttp.RequestCtx) {
	claims, ok := a.authorize(ctx)
	if !ok {
		return
	}
	limit := parseLimit(ctx, 50)
	requestCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	users, err := a.store.ListUsers(requestCtx, claims.OrganizationID.String(), limit)
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, "Failed to list users")
		return
	}
	search := strings.ToLower(strings.TrimSpace(string(ctx.QueryArgs().Peek("search"))))
	filtered := make([]firestorestore.User, 0, len(users))
	online := 0
	for _, user := range users {
		if search != "" && !strings.Contains(strings.ToLower(user.FullName), search) && !strings.Contains(strings.ToLower(user.Email), search) {
			continue
		}
		if user.IsAvailable {
			online++
		}
		filtered = append(filtered, user)
	}
	writeData(ctx, http.StatusOK, map[string]any{"users": filtered, "total": len(filtered), "page": 1, "limit": limit, "online_count": online})
}

func (a *App) ListTags(ctx *fasthttp.RequestCtx) {
	claims, ok := a.authorize(ctx)
	if !ok {
		return
	}
	limit := parseLimit(ctx, 50)
	requestCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tags, err := a.store.ListTags(requestCtx, claims.OrganizationID.String(), limit)
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, "Failed to list tags")
		return
	}
	search := strings.ToLower(strings.TrimSpace(string(ctx.QueryArgs().Peek("search"))))
	if search != "" {
		filtered := tags[:0]
		for _, tag := range tags {
			if strings.Contains(strings.ToLower(tag.Name), search) {
				filtered = append(filtered, tag)
			}
		}
		tags = filtered
	}
	writeData(ctx, http.StatusOK, map[string]any{"tags": tags, "total": len(tags), "page": 1, "limit": limit})
}

func (a *App) ListOrganizations(ctx *fasthttp.RequestCtx) {
	claims, ok := a.authorize(ctx)
	if !ok {
		return
	}
	requestCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if !claims.IsSuperAdmin {
		organization, err := a.store.Organization(requestCtx, claims.OrganizationID.String())
		if err != nil {
			writeError(ctx, http.StatusNotFound, "Organization not found")
			return
		}
		writeData(ctx, http.StatusOK, map[string]any{"organizations": []firestorestore.Organization{*organization}, "total": 1})
		return
	}
	organizations, err := a.store.ListOrganizations(requestCtx, parseLimit(ctx, 50))
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, "Failed to list organizations")
		return
	}
	writeData(ctx, http.StatusOK, map[string]any{"organizations": organizations, "total": len(organizations)})
}

func (a *App) ListMyOrganizations(ctx *fasthttp.RequestCtx) {
	claims, ok := a.authorize(ctx)
	if !ok {
		return
	}
	requestCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	organization, err := a.store.Organization(requestCtx, claims.OrganizationID.String())
	if err != nil {
		writeError(ctx, http.StatusNotFound, "Organization not found")
		return
	}
	roleName := ""
	if user, userErr := a.store.User(requestCtx, claims.UserID.String()); userErr == nil && user.Role != nil {
		roleName = user.Role.Name
	}
	writeData(ctx, http.StatusOK, map[string]any{"organizations": []map[string]any{{"organization_id": organization.ID, "name": organization.Name, "slug": organization.Slug, "role_name": roleName, "is_default": true}}})
}

func (a *App) UpdateAvailability(ctx *fasthttp.RequestCtx) {
	claims, ok := a.authorize(ctx)
	if !ok {
		return
	}
	var request struct {
		IsAvailable bool `json:"is_available"`
	}
	if !decodeJSON(ctx, &request) {
		return
	}
	requestCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	user, err := a.store.User(requestCtx, claims.UserID.String())
	if err != nil {
		writeError(ctx, http.StatusNotFound, "User not found")
		return
	}
	user.IsAvailable = request.IsAvailable
	user.UpdatedAt = a.now().UTC()
	if err := a.store.PutUser(requestCtx, *user); err != nil {
		writeError(ctx, http.StatusInternalServerError, "Failed to update availability")
		return
	}
	var breakStartedAt any
	if !request.IsAvailable {
		breakStartedAt = a.now().UTC()
	}
	writeData(ctx, http.StatusOK, map[string]any{"is_available": request.IsAvailable, "break_started_at": breakStartedAt, "transfers_to_queue": 0})
}

func (a *App) ListContacts(ctx *fasthttp.RequestCtx) {
	claims, ok := a.authorize(ctx)
	if !ok {
		return
	}
	limit := parseLimit(ctx, 50)
	requestCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var cursor *firestorestore.ContactCursor
	if rawCursor := string(ctx.QueryArgs().Peek("cursor")); rawCursor != "" {
		cursor, _ = decodeContactCursor(rawCursor)
		if cursor == nil {
			writeError(ctx, http.StatusBadRequest, "Invalid contact cursor")
			return
		}
	}
	search := string(ctx.QueryArgs().Peek("search"))
	tags := splitCSV(string(ctx.QueryArgs().Peek("tags")))
	var contacts []firestorestore.Contact
	var total int64
	var err error
	if strings.TrimSpace(search) != "" {
		contacts, total, err = a.store.SearchContacts(requestCtx, claims.OrganizationID.String(), search, limit)
	} else if len(tags) > 0 {
		contacts, err = a.store.ListContactsByTags(requestCtx, claims.OrganizationID.String(), tags, limit, cursor)
	} else {
		contacts, err = a.store.ListContacts(requestCtx, claims.OrganizationID.String(), limit, cursor)
	}
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, "Failed to list contacts")
		return
	}
	if strings.TrimSpace(search) == "" {
		if len(tags) > 0 {
			total, err = a.store.CountContactsByTags(requestCtx, claims.OrganizationID.String(), tags)
		} else {
			total, err = a.store.CountContacts(requestCtx, claims.OrganizationID.String())
		}
		if err != nil {
			writeError(ctx, http.StatusInternalServerError, "Failed to count contacts")
			return
		}
	}
	sort.Slice(contacts, func(i, j int) bool {
		if contacts[i].LastMessageAt == nil {
			return false
		}
		if contacts[j].LastMessageAt == nil {
			return true
		}
		return contacts[i].LastMessageAt.After(*contacts[j].LastMessageAt)
	})
	response := make([]map[string]any, 0, len(contacts))
	for _, contact := range contacts {
		response = append(response, contactResponse(contact, a.now().UTC()))
	}
	nextCursor := ""
	if len(contacts) == limit {
		last := contacts[len(contacts)-1]
		nextCursor = encodeContactCursor(firestorestore.ContactCursor{LastMessageAt: last.LastMessageAt, ID: last.ID})
	}
	writeData(ctx, http.StatusOK, map[string]any{"contacts": response, "total": total, "page": 1, "limit": limit, "next_cursor": nextCursor})
}

func (a *App) GetContact(ctx *fasthttp.RequestCtx) {
	claims, ok := a.authorize(ctx)
	if !ok {
		return
	}
	contactID, _ := ctx.UserValue("id").(string)
	requestCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	contact, err := a.store.Contact(requestCtx, claims.OrganizationID.String(), contactID)
	if err != nil {
		writeError(ctx, http.StatusNotFound, "Contact not found")
		return
	}
	writeData(ctx, http.StatusOK, contactResponse(*contact, a.now().UTC()))
}

type createContactRequest struct {
	PhoneNumber     string         `json:"phone_number"`
	ProfileName     string         `json:"profile_name"`
	WhatsAppAccount string         `json:"whatsapp_account"`
	Tags            []string       `json:"tags"`
	Metadata        map[string]any `json:"metadata"`
	Email           string         `json:"email"`
	CompanyName     string         `json:"company_name"`
	TaxOffice       string         `json:"tax_office"`
	TaxNumber       string         `json:"tax_number"`
	Address         string         `json:"address"`
	City            string         `json:"city"`
	District        string         `json:"district"`
	PostalCode      string         `json:"postal_code"`
	PurchaseScore   int            `json:"purchase_score"`
	HasPurchased    bool           `json:"has_purchased"`
}

func (a *App) CreateContact(ctx *fasthttp.RequestCtx) {
	claims, ok := a.authorize(ctx)
	if !ok {
		return
	}
	var request createContactRequest
	if !decodeJSON(ctx, &request) {
		return
	}
	request.PhoneNumber = normalizePhone(request.PhoneNumber)
	if request.PhoneNumber == "" {
		writeError(ctx, http.StatusBadRequest, "phone_number is required")
		return
	}
	if request.PurchaseScore < 0 || request.PurchaseScore > 100 {
		writeError(ctx, http.StatusBadRequest, "purchase_score must be between 0 and 100")
		return
	}
	if request.Metadata == nil {
		request.Metadata = map[string]any{}
	}
	now := a.now().UTC()
	contact := firestorestore.Contact{
		ID: uuid.NewString(), OrganizationID: claims.OrganizationID.String(), PhoneNumber: request.PhoneNumber,
		ProfileName: strings.TrimSpace(request.ProfileName), WhatsAppAccount: strings.TrimSpace(request.WhatsAppAccount),
		ChannelType: "whatsapp", IsRead: true, Tags: stringsToAny(request.Tags), Metadata: request.Metadata,
		Email: strings.TrimSpace(request.Email), CompanyName: strings.TrimSpace(request.CompanyName),
		TaxOffice: strings.TrimSpace(request.TaxOffice), TaxNumber: strings.TrimSpace(request.TaxNumber),
		Address: strings.TrimSpace(request.Address), City: strings.TrimSpace(request.City), District: strings.TrimSpace(request.District),
		PostalCode: strings.TrimSpace(request.PostalCode), PurchaseScore: request.PurchaseScore, HasPurchased: request.HasPurchased,
		CreatedAt: now, UpdatedAt: now,
	}
	contact.SearchTokens = firestorestore.BuildContactSearchTokens(contact)
	requestCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := a.store.CreateContact(requestCtx, contact); err != nil {
		if errors.Is(err, firestorestore.ErrConflict) {
			writeError(ctx, http.StatusConflict, "Contact with this phone number already exists")
			return
		}
		writeError(ctx, http.StatusInternalServerError, "Failed to create contact")
		return
	}
	writeData(ctx, http.StatusOK, contactResponse(contact, now))
}

type updateContactRequest struct {
	ProfileName        *string         `json:"profile_name"`
	WhatsAppAccount    *string         `json:"whatsapp_account"`
	Tags               []string        `json:"tags"`
	Metadata           *map[string]any `json:"metadata"`
	AssignedUserID     *string         `json:"assigned_user_id"`
	ClearAssignedAgent *bool           `json:"clear_assigned_agent"`
	Email              *string         `json:"email"`
	CompanyName        *string         `json:"company_name"`
	TaxOffice          *string         `json:"tax_office"`
	TaxNumber          *string         `json:"tax_number"`
	Address            *string         `json:"address"`
	City               *string         `json:"city"`
	District           *string         `json:"district"`
	PostalCode         *string         `json:"postal_code"`
	PurchaseScore      *int            `json:"purchase_score"`
	HasPurchased       *bool           `json:"has_purchased"`
}

func (a *App) UpdateContact(ctx *fasthttp.RequestCtx) {
	claims, ok := a.authorize(ctx)
	if !ok {
		return
	}
	contactID, _ := ctx.UserValue("id").(string)
	var request updateContactRequest
	if !decodeJSON(ctx, &request) {
		return
	}
	requestCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	contact, err := a.store.Contact(requestCtx, claims.OrganizationID.String(), contactID)
	if err != nil {
		writeError(ctx, http.StatusNotFound, "Contact not found")
		return
	}
	previous := *contact
	if request.ProfileName != nil {
		contact.ProfileName = strings.TrimSpace(*request.ProfileName)
	}
	if request.WhatsAppAccount != nil {
		contact.WhatsAppAccount = strings.TrimSpace(*request.WhatsAppAccount)
	}
	if request.Tags != nil {
		contact.Tags = stringsToAny(request.Tags)
	}
	if request.Metadata != nil {
		contact.Metadata = *request.Metadata
	}
	if request.Email != nil {
		contact.Email = strings.TrimSpace(*request.Email)
	}
	if request.CompanyName != nil {
		contact.CompanyName = strings.TrimSpace(*request.CompanyName)
	}
	if request.TaxOffice != nil {
		contact.TaxOffice = strings.TrimSpace(*request.TaxOffice)
	}
	if request.TaxNumber != nil {
		contact.TaxNumber = strings.TrimSpace(*request.TaxNumber)
	}
	if request.Address != nil {
		contact.Address = strings.TrimSpace(*request.Address)
	}
	if request.City != nil {
		contact.City = strings.TrimSpace(*request.City)
	}
	if request.District != nil {
		contact.District = strings.TrimSpace(*request.District)
	}
	if request.PostalCode != nil {
		contact.PostalCode = strings.TrimSpace(*request.PostalCode)
	}
	if request.PurchaseScore != nil {
		if *request.PurchaseScore < 0 || *request.PurchaseScore > 100 {
			writeError(ctx, http.StatusBadRequest, "purchase_score must be between 0 and 100")
			return
		}
		contact.PurchaseScore = *request.PurchaseScore
	}
	if request.HasPurchased != nil {
		contact.HasPurchased = *request.HasPurchased
	}
	if request.ClearAssignedAgent != nil && *request.ClearAssignedAgent {
		contact.AssignedUserID = ""
	} else if request.AssignedUserID != nil {
		if *request.AssignedUserID != "" {
			user, userErr := a.store.User(requestCtx, *request.AssignedUserID)
			if userErr != nil || user.OrganizationID != claims.OrganizationID.String() {
				writeError(ctx, http.StatusBadRequest, "Assigned user not found")
				return
			}
		}
		contact.AssignedUserID = *request.AssignedUserID
	}
	contact.UpdatedAt = a.now().UTC()
	contact.SearchTokens = firestorestore.BuildContactSearchTokens(*contact)
	if err := a.store.UpdateContact(requestCtx, previous, *contact); err != nil {
		if errors.Is(err, firestorestore.ErrConflict) {
			writeError(ctx, http.StatusConflict, "Contact already exists for that account and phone")
			return
		}
		writeError(ctx, http.StatusInternalServerError, "Failed to update contact")
		return
	}
	writeData(ctx, http.StatusOK, contactResponse(*contact, a.now().UTC()))
}

func (a *App) DeleteContact(ctx *fasthttp.RequestCtx) {
	claims, ok := a.authorize(ctx)
	if !ok {
		return
	}
	contactID, _ := ctx.UserValue("id").(string)
	requestCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	contact, err := a.store.Contact(requestCtx, claims.OrganizationID.String(), contactID)
	if err != nil {
		writeError(ctx, http.StatusNotFound, "Contact not found")
		return
	}
	if err := a.store.DeleteContact(requestCtx, *contact, a.now().UTC()); err != nil {
		writeError(ctx, http.StatusInternalServerError, "Failed to delete contact")
		return
	}
	writeData(ctx, http.StatusOK, map[string]string{"message": "Contact deleted successfully"})
}

func (a *App) AssignContact(ctx *fasthttp.RequestCtx) {
	var request struct {
		UserID *string `json:"user_id"`
	}
	if !decodeJSON(ctx, &request) {
		return
	}
	update := updateContactRequest{ClearAssignedAgent: boolPointer(request.UserID == nil)}
	if request.UserID != nil {
		update.AssignedUserID = request.UserID
	}
	a.updateContactWithRequest(ctx, update)
}

func (a *App) UpdateContactTags(ctx *fasthttp.RequestCtx) {
	var request struct {
		Tags []string `json:"tags"`
	}
	if !decodeJSON(ctx, &request) {
		return
	}
	a.updateContactWithRequest(ctx, updateContactRequest{Tags: request.Tags})
}

func (a *App) updateContactWithRequest(ctx *fasthttp.RequestCtx, request updateContactRequest) {
	body, _ := json.Marshal(request)
	ctx.Request.SetBody(body)
	a.UpdateContact(ctx)
}

func (a *App) GetContactSessionData(ctx *fasthttp.RequestCtx) {
	claims, ok := a.authorize(ctx)
	if !ok {
		return
	}
	contactID, _ := ctx.UserValue("id").(string)
	requestCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := a.store.Contact(requestCtx, claims.OrganizationID.String(), contactID); err != nil {
		writeError(ctx, http.StatusNotFound, "Contact not found")
		return
	}
	session, err := a.store.LatestChatbotSession(requestCtx, claims.OrganizationID.String(), contactID)
	if err != nil && !errors.Is(err, firestorestore.ErrNotFound) {
		writeError(ctx, http.StatusInternalServerError, "Failed to load chatbot session")
		return
	}
	response := map[string]any{"session_data": map[string]any{}, "panel_config": map[string]any{"sections": []any{}}}
	if session != nil {
		response["session_id"] = session.ID
		response["session_data"] = session.SessionData
		if session.CurrentFlowID != "" {
			response["flow_id"] = session.CurrentFlowID
		}
	}
	if transfer, transferErr := a.store.ActiveTransfer(requestCtx, claims.OrganizationID.String(), contactID); transferErr == nil {
		response["active_transfer"] = transfer
	} else if !errors.Is(transferErr, firestorestore.ErrNotFound) {
		writeError(ctx, http.StatusInternalServerError, "Failed to load active transfer")
		return
	}
	writeData(ctx, http.StatusOK, response)
}

func (a *App) ListAgentTransfers(ctx *fasthttp.RequestCtx) {
	claims, ok := a.authorize(ctx)
	if !ok {
		return
	}
	statusFilter := strings.TrimSpace(string(ctx.QueryArgs().Peek("status")))
	limit := parseLimit(ctx, 100)
	offset, _ := strconv.Atoi(string(ctx.QueryArgs().Peek("offset")))
	requestCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	transfers, total, err := a.store.ListAgentTransfers(requestCtx, claims.OrganizationID.String(), statusFilter, limit, offset)
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, "Failed to list transfers")
		return
	}
	general, err := a.store.CountGeneralActiveTransfers(requestCtx, claims.OrganizationID.String())
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, "Failed to count transfer queue")
		return
	}
	teamCounts := map[string]int{}
	for _, transfer := range transfers {
		if transfer.Status != "active" || transfer.AgentID != "" {
			continue
		}
		if transfer.TeamID != "" {
			teamCounts[transfer.TeamID]++
		}
	}
	writeData(ctx, http.StatusOK, map[string]any{"transfers": transfers, "general_queue_count": general, "team_queue_counts": teamCounts, "total_count": total, "limit": limit, "offset": offset})
}

func (a *App) CreateAgentTransfer(ctx *fasthttp.RequestCtx) {
	claims, ok := a.authorize(ctx)
	if !ok {
		return
	}
	var request struct {
		ContactID       string `json:"contact_id"`
		WhatsAppAccount string `json:"whatsapp_account"`
		AgentID         string `json:"agent_id"`
		Notes           string `json:"notes"`
		Source          string `json:"source"`
	}
	if !decodeJSON(ctx, &request) || request.ContactID == "" {
		writeError(ctx, http.StatusBadRequest, "contact_id is required")
		return
	}
	requestCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	contact, err := a.store.Contact(requestCtx, claims.OrganizationID.String(), request.ContactID)
	if err != nil {
		writeError(ctx, http.StatusNotFound, "Contact not found")
		return
	}
	account := strings.TrimSpace(request.WhatsAppAccount)
	if account == "" {
		account = contact.WhatsAppAccount
	}
	source := strings.TrimSpace(request.Source)
	if source == "" {
		source = "manual"
	}
	now := a.now().UTC()
	transfer := firestorestore.AgentTransfer{ID: uuid.NewString(), OrganizationID: claims.OrganizationID.String(), ContactID: contact.ID, ContactName: contact.ProfileName, PhoneNumber: contact.PhoneNumber, WhatsAppAccount: account, Status: "active", Source: source, AgentID: request.AgentID, TransferredBy: claims.UserID.String(), Notes: request.Notes, TransferredAt: now, CreatedAt: now, UpdatedAt: now}
	if request.AgentID != "" {
		agent, err := a.store.User(requestCtx, request.AgentID)
		if err != nil || agent.OrganizationID != claims.OrganizationID.String() || !agent.IsAvailable {
			writeError(ctx, http.StatusBadRequest, "Agent not found or unavailable")
			return
		}
		transfer.AgentName = agent.FullName
		transfer.PickedUpAt = &now
	}
	if err := a.store.CreateAgentTransfer(requestCtx, transfer); errors.Is(err, firestorestore.ErrConflict) {
		writeError(ctx, http.StatusConflict, "Contact already has an active transfer")
		return
	} else if err != nil {
		writeError(ctx, http.StatusInternalServerError, "Failed to create transfer")
		return
	}
	if err := a.store.CancelActiveChatbotSessions(requestCtx, transfer.OrganizationID, transfer.ContactID, now); err != nil {
		writeError(ctx, http.StatusInternalServerError, "Transfer created but chatbot session could not be stopped")
		return
	}
	writeData(ctx, http.StatusOK, map[string]any{"transfer": transfer, "message": "Transfer created successfully"})
}

func (a *App) ResumeAgentTransfer(ctx *fasthttp.RequestCtx) {
	claims, ok := a.authorize(ctx)
	if !ok {
		return
	}
	id, _ := ctx.UserValue("id").(string)
	requestCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	user, _ := a.store.User(requestCtx, claims.UserID.String())
	name := ""
	if user != nil {
		name = user.FullName
	}
	_, err := a.store.ResumeAgentTransfer(requestCtx, claims.OrganizationID.String(), id, claims.UserID.String(), name, a.now().UTC())
	if errors.Is(err, firestorestore.ErrNotFound) {
		writeError(ctx, http.StatusNotFound, "Transfer not found")
		return
	}
	if errors.Is(err, firestorestore.ErrConflict) {
		writeError(ctx, http.StatusBadRequest, "Transfer is not active")
		return
	}
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, "Failed to resume transfer")
		return
	}
	writeData(ctx, http.StatusOK, map[string]string{"message": "Transfer resumed, chatbot is now active for this contact"})
}

func (a *App) AssignAgentTransfer(ctx *fasthttp.RequestCtx) {
	claims, ok := a.authorize(ctx)
	if !ok {
		return
	}
	id, _ := ctx.UserValue("id").(string)
	var request struct {
		AgentID *string `json:"agent_id"`
	}
	if !decodeJSON(ctx, &request) {
		return
	}
	agentID := ""
	if request.AgentID != nil {
		agentID = strings.TrimSpace(*request.AgentID)
	}
	requestCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	agentName := ""
	if agentID != "" {
		agent, err := a.store.User(requestCtx, agentID)
		if err != nil || agent.OrganizationID != claims.OrganizationID.String() || !agent.IsAvailable {
			writeError(ctx, http.StatusBadRequest, "Agent not found or unavailable")
			return
		}
		agentName = agent.FullName
	}
	_, err := a.store.AssignAgentTransfer(requestCtx, claims.OrganizationID.String(), id, agentID, agentName, a.now().UTC())
	if errors.Is(err, firestorestore.ErrNotFound) {
		writeError(ctx, http.StatusNotFound, "Transfer not found")
		return
	}
	if errors.Is(err, firestorestore.ErrConflict) {
		writeError(ctx, http.StatusBadRequest, "Transfer is not active")
		return
	}
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, "Failed to assign transfer")
		return
	}
	writeData(ctx, http.StatusOK, map[string]any{"message": "Transfer assigned successfully", "agent_id": agentID})
}

func (a *App) ListCannedResponses(ctx *fasthttp.RequestCtx) {
	claims, ok := a.authorize(ctx)
	if !ok {
		return
	}
	requestCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	responses, err := a.store.ListCannedResponses(requestCtx, claims.OrganizationID.String(), string(ctx.QueryArgs().Peek("active_only")) == "true", parseLimit(ctx, 100))
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, "Failed to list canned responses")
		return
	}
	search := strings.ToLower(strings.TrimSpace(string(ctx.QueryArgs().Peek("search"))))
	category := strings.TrimSpace(string(ctx.QueryArgs().Peek("category")))
	filtered := responses[:0]
	for _, response := range responses {
		if category != "" && response.Category != category {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(response.Name+" "+response.Content+" "+response.Shortcut), search) {
			continue
		}
		filtered = append(filtered, response)
	}
	writeData(ctx, http.StatusOK, map[string]any{"canned_responses": filtered, "total": len(filtered)})
}

func (a *App) IncrementCannedResponseUsage(ctx *fasthttp.RequestCtx) {
	claims, ok := a.authorize(ctx)
	if !ok {
		return
	}
	id, _ := ctx.UserValue("id").(string)
	requestCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := a.store.IncrementCannedResponseUsage(requestCtx, claims.OrganizationID.String(), id); errors.Is(err, firestorestore.ErrNotFound) {
		writeError(ctx, http.StatusNotFound, "Canned response not found")
		return
	} else if err != nil {
		writeError(ctx, http.StatusInternalServerError, "Failed to update usage")
		return
	}
	writeData(ctx, http.StatusOK, map[string]string{"message": "Usage incremented"})
}

func (a *App) ListConversationNotes(ctx *fasthttp.RequestCtx) {
	claims, ok := a.authorize(ctx)
	if !ok {
		return
	}
	contactID, _ := ctx.UserValue("id").(string)
	requestCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if _, err := a.store.Contact(requestCtx, claims.OrganizationID.String(), contactID); err != nil {
		writeError(ctx, http.StatusNotFound, "Contact not found")
		return
	}
	notes, total, err := a.store.ListConversationNotes(requestCtx, claims.OrganizationID.String(), contactID, parseLimit(ctx, 30), string(ctx.QueryArgs().Peek("before")))
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, "Failed to list notes")
		return
	}
	writeData(ctx, http.StatusOK, map[string]any{"notes": notes, "total": total, "has_more": len(notes) == parseLimit(ctx, 30)})
}

func (a *App) CreateConversationNote(ctx *fasthttp.RequestCtx) {
	claims, ok := a.authorize(ctx)
	if !ok {
		return
	}
	contactID, _ := ctx.UserValue("id").(string)
	var request struct {
		Content string `json:"content"`
	}
	if !decodeJSON(ctx, &request) {
		return
	}
	request.Content = strings.TrimSpace(request.Content)
	if request.Content == "" {
		writeError(ctx, http.StatusBadRequest, "content is required")
		return
	}
	requestCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if _, err := a.store.Contact(requestCtx, claims.OrganizationID.String(), contactID); err != nil {
		writeError(ctx, http.StatusNotFound, "Contact not found")
		return
	}
	user, err := a.store.User(requestCtx, claims.UserID.String())
	if err != nil || user.OrganizationID != claims.OrganizationID.String() {
		writeError(ctx, http.StatusUnauthorized, "User not found")
		return
	}
	now := a.now().UTC()
	note := firestorestore.ConversationNote{ID: uuid.NewString(), OrganizationID: claims.OrganizationID.String(), ContactID: contactID, CreatedByID: claims.UserID.String(), CreatedByName: user.FullName, Content: request.Content, CreatedAt: now, UpdatedAt: now}
	if err := a.store.CreateConversationNote(requestCtx, note); err != nil {
		writeError(ctx, http.StatusInternalServerError, "Failed to create note")
		return
	}
	writeData(ctx, http.StatusOK, note)
}

func (a *App) UpdateConversationNote(ctx *fasthttp.RequestCtx) {
	claims, ok := a.authorize(ctx)
	if !ok {
		return
	}
	contactID, _ := ctx.UserValue("id").(string)
	noteID, _ := ctx.UserValue("note_id").(string)
	var request struct {
		Content string `json:"content"`
	}
	if !decodeJSON(ctx, &request) {
		return
	}
	request.Content = strings.TrimSpace(request.Content)
	if request.Content == "" {
		writeError(ctx, http.StatusBadRequest, "content is required")
		return
	}
	requestCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	note, err := a.store.UpdateConversationNote(requestCtx, claims.OrganizationID.String(), contactID, noteID, claims.UserID.String(), request.Content, a.now().UTC())
	if errors.Is(err, firestorestore.ErrForbidden) {
		writeError(ctx, http.StatusForbidden, "You can only edit your own notes")
		return
	}
	if errors.Is(err, firestorestore.ErrNotFound) {
		writeError(ctx, http.StatusNotFound, "Note not found")
		return
	}
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, "Failed to update note")
		return
	}
	writeData(ctx, http.StatusOK, note)
}

func (a *App) DeleteConversationNote(ctx *fasthttp.RequestCtx) {
	claims, ok := a.authorize(ctx)
	if !ok {
		return
	}
	contactID, _ := ctx.UserValue("id").(string)
	noteID, _ := ctx.UserValue("note_id").(string)
	requestCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	err := a.store.DeleteConversationNote(requestCtx, claims.OrganizationID.String(), contactID, noteID, claims.UserID.String(), a.now().UTC())
	if errors.Is(err, firestorestore.ErrForbidden) {
		writeError(ctx, http.StatusForbidden, "You can only delete your own notes")
		return
	}
	if errors.Is(err, firestorestore.ErrNotFound) {
		writeError(ctx, http.StatusNotFound, "Note not found")
		return
	}
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, "Failed to delete note")
		return
	}
	writeData(ctx, http.StatusOK, map[string]string{"message": "Note deleted"})
}

func (a *App) ListMessages(ctx *fasthttp.RequestCtx) {
	claims, ok := a.authorize(ctx)
	if !ok {
		return
	}
	contactID, _ := ctx.UserValue("id").(string)
	limit := parseLimit(ctx, 50)
	requestCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var before *time.Time
	if beforeID := string(ctx.QueryArgs().Peek("before_id")); beforeID != "" {
		if message, findErr := a.store.Message(requestCtx, claims.OrganizationID.String(), contactID, beforeID); findErr == nil {
			value := message.CreatedAt
			before = &value
		}
	}
	account := strings.TrimSpace(string(ctx.QueryArgs().Peek("account")))
	var messages []firestorestore.Message
	var err error
	if account == "" {
		messages, err = a.store.ListMessages(requestCtx, claims.OrganizationID.String(), contactID, limit, before)
	} else {
		messages, err = a.store.ListMessagesForAccount(requestCtx, claims.OrganizationID.String(), contactID, account, limit, before)
	}
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, "Failed to list messages")
		return
	}
	response := make([]map[string]any, 0, len(messages))
	for _, message := range messages {
		response = append(response, messageResponse(message))
	}
	writeData(ctx, http.StatusOK, map[string]any{"messages": response, "limit": limit, "has_more": len(messages) == limit})
}

func contactResponse(contact firestorestore.Contact, now time.Time) map[string]any {
	name := strings.TrimSpace(contact.FirstName + " " + contact.LastName)
	if name == "" {
		name = contact.ProfileName
	}
	tags := make([]string, 0, len(contact.Tags))
	for _, tag := range contact.Tags {
		if value, ok := tag.(string); ok {
			tags = append(tags, value)
		}
	}
	serviceWindowOpen := contact.LastInboundAt != nil && now.Sub(contact.LastInboundAt.UTC()) <= 24*time.Hour
	unreadCount := 0
	if !contact.IsRead {
		unreadCount = 1
	}
	return map[string]any{
		"id": contact.ID, "customer_id": contact.CustomerID, "phone_number": contact.PhoneNumber,
		"name": name, "profile_name": contact.ProfileName, "avatar_url": "", "status": "active",
		"tags": tags, "metadata": contact.Metadata, "last_message_at": contact.LastMessageAt,
		"last_message_preview": contact.LastMessagePreview, "unread_count": unreadCount,
		"assigned_user_id": contact.AssignedUserID, "whatsapp_account": contact.WhatsAppAccount,
		"channel_type": firstNonEmpty(contact.ChannelType, "whatsapp"), "last_inbound_at": contact.LastInboundAt,
		"service_window_open": serviceWindowOpen, "marketing_opt_out": contact.MarketingOptOut,
		"email": contact.Email, "company_name": contact.CompanyName, "tax_office": contact.TaxOffice,
		"tax_number": contact.TaxNumber, "address": contact.Address, "city": contact.City,
		"district": contact.District, "postal_code": contact.PostalCode,
		"purchase_score": contact.PurchaseScore, "has_purchased": contact.HasPurchased,
		"created_at": contact.CreatedAt, "updated_at": contact.UpdatedAt,
	}
}

func messageResponse(message firestorestore.Message) map[string]any {
	response := map[string]any{
		"id": message.ID, "contact_id": message.ContactID, "direction": message.Direction,
		"message_type": message.MessageType, "content": map[string]string{"body": message.Content},
		"media_url": message.MediaURL, "media_mime_type": message.MediaMimeType,
		"media_filename": message.MediaFilename, "interactive_data": message.InteractiveData,
		"status": message.Status, "wamid": message.ExternalID, "error_message": message.ErrorMessage,
		"is_reply": message.IsReply, "whatsapp_account": message.WhatsAppAccount,
		"created_at": message.CreatedAt, "updated_at": message.UpdatedAt,
	}
	if message.ReplyToMessageID != "" {
		response["reply_to_message_id"] = message.ReplyToMessageID
	}
	if reactions, ok := message.Metadata["reactions"]; ok {
		response["reactions"] = reactions
	}
	return response
}

type accountResponse struct {
	ID                     string    `json:"id"`
	Name                   string    `json:"name"`
	AppID                  string    `json:"app_id"`
	PhoneID                string    `json:"phone_id"`
	BusinessID             string    `json:"business_id"`
	CatalogBusinessID      string    `json:"catalog_business_id"`
	WebhookVerifyToken     string    `json:"webhook_verify_token"`
	APIVersion             string    `json:"api_version"`
	IsDefaultIncoming      bool      `json:"is_default_incoming"`
	IsDefaultOutgoing      bool      `json:"is_default_outgoing"`
	AutoReadReceipt        bool      `json:"auto_read_receipt"`
	BusinessCallingEnabled bool      `json:"business_calling_enabled"`
	Status                 string    `json:"status"`
	HasAccessToken         bool      `json:"has_access_token"`
	HasAppSecret           bool      `json:"has_app_secret"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

func (a *App) ListAccounts(ctx *fasthttp.RequestCtx) {
	claims, ok := a.authorize(ctx)
	if !ok {
		return
	}
	requestCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	accounts, err := a.store.ListWhatsAppAccounts(requestCtx, claims.OrganizationID.String())
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, "Failed to list accounts")
		return
	}
	response := make([]accountResponse, 0, len(accounts))
	for _, account := range accounts {
		response = append(response, accountResponse{
			ID: account.ID, Name: account.Name, AppID: account.AppID, PhoneID: account.PhoneID,
			BusinessID: account.BusinessID, CatalogBusinessID: account.CatalogBusinessID,
			WebhookVerifyToken: account.WebhookVerifyToken, APIVersion: account.APIVersion,
			IsDefaultIncoming: account.IsDefaultIncoming, IsDefaultOutgoing: account.IsDefaultOutgoing,
			AutoReadReceipt: account.AutoReadReceipt, BusinessCallingEnabled: account.BusinessCallingEnabled,
			Status: account.Status, HasAccessToken: account.AccessToken != "", HasAppSecret: account.AppSecret != "",
			CreatedAt: account.CreatedAt, UpdatedAt: account.UpdatedAt,
		})
	}
	writeData(ctx, http.StatusOK, map[string]any{"accounts": response})
}

func (a *App) ListTemplates(ctx *fasthttp.RequestCtx) {
	claims, ok := a.authorize(ctx)
	if !ok {
		return
	}
	requestCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	templates, err := a.store.ListTemplates(requestCtx, claims.OrganizationID.String())
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, "Failed to list templates")
		return
	}
	account := string(ctx.QueryArgs().Peek("account"))
	statusFilter := string(ctx.QueryArgs().Peek("status"))
	category := string(ctx.QueryArgs().Peek("category"))
	search := strings.ToLower(strings.TrimSpace(string(ctx.QueryArgs().Peek("search"))))
	filtered := make([]firestorestore.Template, 0, len(templates))
	for _, template := range templates {
		if account != "" && template.WhatsAppAccount != account {
			continue
		}
		if statusFilter != "" && !strings.EqualFold(template.Status, statusFilter) {
			continue
		}
		if category != "" && !strings.EqualFold(template.Category, category) {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(template.Name+" "+template.DisplayName), search) {
			continue
		}
		filtered = append(filtered, template)
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].CreatedAt.After(filtered[j].CreatedAt) })
	total := len(filtered)
	limit := parseLimit(ctx, 50)
	page, err := strconv.Atoi(string(ctx.QueryArgs().Peek("page")))
	if err != nil || page < 1 {
		page = 1
	}
	start := (page - 1) * limit
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}
	writeData(ctx, http.StatusOK, map[string]any{"templates": filtered[start:end], "total": total, "page": page, "limit": limit})
}

func (a *App) MarkContactRead(ctx *fasthttp.RequestCtx) {
	claims, ok := a.authorize(ctx)
	if !ok {
		return
	}
	contactID, _ := ctx.UserValue("id").(string)
	requestCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := a.store.MarkContactRead(requestCtx, claims.OrganizationID.String(), contactID, a.now().UTC()); err != nil {
		if errors.Is(err, firestorestore.ErrNotFound) {
			writeError(ctx, http.StatusNotFound, "Contact not found")
		} else {
			writeError(ctx, http.StatusInternalServerError, "Failed to mark contact read")
		}
		return
	}
	writeData(ctx, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *App) ListEvents(ctx *fasthttp.RequestCtx) {
	claims, ok := a.authorize(ctx)
	if !ok {
		return
	}
	var cursor *firestorestore.EventCursor
	if raw := string(ctx.QueryArgs().Peek("cursor")); raw != "" {
		payload, err := base64.RawURLEncoding.DecodeString(raw)
		if err != nil || json.Unmarshal(payload, &cursor) != nil || cursor == nil || cursor.ID == "" {
			writeError(ctx, http.StatusBadRequest, "Invalid event cursor")
			return
		}
	}
	requestCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	events, err := a.store.ListEvents(requestCtx, claims.OrganizationID.String(), 100, cursor)
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, "Failed to list events")
		return
	}
	nextCursor := string(ctx.QueryArgs().Peek("cursor"))
	if len(events) > 0 {
		last := events[len(events)-1]
		payload, _ := json.Marshal(firestorestore.EventCursor{CreatedAt: last.CreatedAt, ID: last.ID})
		nextCursor = base64.RawURLEncoding.EncodeToString(payload)
	}
	writeData(ctx, http.StatusOK, map[string]any{"events": events, "next_cursor": nextCursor})
}

func (a *App) SendTemplate(ctx *fasthttp.RequestCtx) {
	claims, ok := a.authorize(ctx)
	if !ok {
		return
	}
	if a.sender == nil {
		writeError(ctx, http.StatusServiceUnavailable, "Messaging is not configured")
		return
	}
	var request struct {
		ContactID      string            `json:"contact_id"`
		TemplateName   string            `json:"template_name"`
		TemplateID     string            `json:"template_id"`
		TemplateParams map[string]string `json:"template_params"`
		HeaderParams   map[string]string `json:"header_params"`
		ButtonParams   map[string]string `json:"button_params"`
		AccountName    string            `json:"account_name"`
		HeaderMediaID  string            `json:"header_media_id"`
	}
	var headerData []byte
	var headerMimeType, headerFilename string
	if strings.HasPrefix(string(ctx.Request.Header.ContentType()), "multipart/form-data") {
		form, err := ctx.MultipartForm()
		if err != nil {
			writeError(ctx, http.StatusBadRequest, "Invalid multipart form")
			return
		}
		request.ContactID = firstFormValue(form.Value["contact_id"])
		request.TemplateName = firstFormValue(form.Value["template_name"])
		request.TemplateID = firstFormValue(form.Value["template_id"])
		request.AccountName = firstFormValue(form.Value["account_name"])
		for key, target := range map[string]*map[string]string{"template_params": &request.TemplateParams, "header_params": &request.HeaderParams, "button_params": &request.ButtonParams} {
			if raw := firstFormValue(form.Value[key]); raw != "" {
				if err := json.Unmarshal([]byte(raw), target); err != nil {
					writeError(ctx, http.StatusBadRequest, "Invalid "+key+" JSON")
					return
				}
			}
		}
		if files := form.File["header_file"]; len(files) > 0 {
			file, err := files[0].Open()
			if err != nil {
				writeError(ctx, http.StatusBadRequest, "Failed to read header file")
				return
			}
			headerData, err = io.ReadAll(io.LimitReader(file, 32<<20+1))
			_ = file.Close()
			if err != nil || len(headerData) > 32<<20 {
				writeError(ctx, http.StatusBadRequest, "Header media exceeds the 32 MiB limit")
				return
			}
			headerMimeType = files[0].Header.Get("Content-Type")
			if headerMimeType == "" {
				headerMimeType = http.DetectContentType(headerData)
			}
			headerFilename = filepath.Base(files[0].Filename)
		}
	} else if !decodeJSON(ctx, &request) {
		return
	}
	if request.ContactID == "" || (request.TemplateName == "" && request.TemplateID == "") {
		writeError(ctx, http.StatusBadRequest, "contact_id and template are required")
		return
	}
	requestCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	orgID := claims.OrganizationID.String()
	contact, err := a.store.Contact(requestCtx, orgID, request.ContactID)
	if err != nil || contact.IsDeleted {
		writeError(ctx, http.StatusNotFound, "Contact not found")
		return
	}
	templateKey := firstNonEmpty(request.TemplateID, request.TemplateName)
	template, err := a.store.Template(requestCtx, orgID, templateKey)
	if err != nil {
		writeError(ctx, http.StatusNotFound, "Template not found")
		return
	}
	if !strings.EqualFold(template.Status, "APPROVED") {
		writeError(ctx, http.StatusBadRequest, "Template is not approved")
		return
	}
	if contact.MarketingOptOut && strings.EqualFold(template.Category, "MARKETING") {
		writeError(ctx, http.StatusBadRequest, "Contact has opted out of marketing messages")
		return
	}
	accountName := firstNonEmpty(request.AccountName, template.WhatsAppAccount, contact.WhatsAppAccount)
	account, err := a.store.ResolveWhatsAppAccount(requestCtx, orgID, accountName)
	if err != nil {
		writeError(ctx, http.StatusBadRequest, "No WhatsApp account is configured")
		return
	}
	accessToken, err := appcrypto.Decrypt(account.AccessToken, a.config.App.EncryptionKey)
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, "WhatsApp credentials could not be opened")
		return
	}
	waAccount := &whatsapp.Account{PhoneID: account.PhoneID, BusinessID: account.BusinessID, AppID: account.AppID, APIVersion: account.APIVersion, AccessToken: accessToken}
	headerMediaID := request.HeaderMediaID
	if len(headerData) > 0 {
		headerMediaID, err = a.sender.UploadMedia(requestCtx, waAccount, headerData, headerMimeType, headerFilename)
		if err != nil {
			writeError(ctx, http.StatusBadGateway, "Meta rejected the template header media")
			return
		}
	}
	mediaHeader := strings.EqualFold(template.HeaderType, "IMAGE") || strings.EqualFold(template.HeaderType, "VIDEO") || strings.EqualFold(template.HeaderType, "DOCUMENT")
	if mediaHeader && headerMediaID == "" {
		writeError(ctx, http.StatusBadRequest, "Template header media is required")
		return
	}
	components, err := whatsapp.BuildTemplateComponents(request.TemplateParams, template.HeaderType, template.HeaderContent, request.HeaderParams, headerMediaID, headerFilename)
	if err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	components = append(components, whatsapp.AutoButtonComponents(template.Buttons)...)
	components = append(components, whatsapp.ButtonURLParamsToComponents(request.ButtonParams, template.Buttons)...)
	externalID, err := a.sender.SendTemplateMessage(requestCtx, waAccount, whatsapp.Recipient{Phone: contact.PhoneNumber, BSUID: contact.BSUID}, template.Name, template.Language, components)
	if err != nil {
		writeError(ctx, http.StatusBadGateway, "Meta rejected the template message")
		return
	}
	now := a.now().UTC()
	params := make(map[string]any, len(request.TemplateParams))
	for key, value := range request.TemplateParams {
		params[key] = value
	}
	renderedContent := templateutil.ReplaceWithStringParams(template.BodyContent, request.TemplateParams)
	if strings.TrimSpace(renderedContent) == "" {
		renderedContent = firstNonEmpty(template.DisplayName, template.Name)
	}
	message := firestorestore.Message{
		ID: uuid.NewString(), OrganizationID: orgID, ContactID: contact.ID,
		WhatsAppAccount: account.Name, ChannelType: contact.ChannelType, ExternalID: externalID,
		Direction: "outgoing", MessageType: "template", Content: renderedContent,
		TemplateName: template.Name, TemplateParams: params, Status: "sent", SentByUserID: claims.UserID.String(),
		Metadata: map[string]any{}, CreatedAt: now, UpdatedAt: now,
	}
	if mediaHeader {
		message.MediaURL, message.MediaMimeType, message.MediaFilename = headerMediaID, headerMimeType, headerFilename
		if len(headerData) > 0 && a.media != nil {
			extension := strings.ToLower(filepath.Ext(headerFilename))
			if len(extension) > 12 {
				extension = ""
			}
			storageKey := "template-headers/" + uuid.NewString() + extension
			if uploadErr := a.media.Upload(requestCtx, storageKey, bytes.NewReader(headerData), headerMimeType); uploadErr == nil {
				message.MediaURL = storageKey
			}
		}
	}
	contact.WhatsAppAccount = account.Name
	contact.LastMessageAt = &now
	contact.LastMessagePreview = message.Content
	contact.IsRead = true
	contact.UpdatedAt = now
	if err := a.store.CreateOutgoingMessage(requestCtx, *contact, message); err != nil {
		writeError(ctx, http.StatusInternalServerError, "Template was sent but could not be saved")
		return
	}
	writeData(ctx, http.StatusOK, map[string]any{
		"id": message.ID, "contact_id": message.ContactID, "direction": message.Direction,
		"message_type": message.MessageType, "content": map[string]string{"body": message.Content},
		"status": message.Status, "is_reply": false, "whatsapp_account": message.WhatsAppAccount,
		"media_url": message.MediaURL, "media_mime_type": message.MediaMimeType, "media_filename": message.MediaFilename,
		"created_at": message.CreatedAt, "updated_at": message.UpdatedAt,
	})
}

func (a *App) SendMessage(ctx *fasthttp.RequestCtx) {
	claims, ok := a.authorize(ctx)
	if !ok {
		return
	}
	if a.sender == nil {
		writeError(ctx, http.StatusServiceUnavailable, "Messaging is not configured")
		return
	}
	contactID, _ := ctx.UserValue("id").(string)
	var request struct {
		Type    string `json:"type"`
		Content struct {
			Body string `json:"body"`
		} `json:"content"`
		ReplyToMessageID string `json:"reply_to_message_id"`
		WhatsAppAccount  string `json:"whatsapp_account"`
		Interactive      struct {
			Type    string `json:"type"`
			Body    string `json:"body"`
			Buttons []struct {
				ID    string `json:"id"`
				Title string `json:"title"`
			} `json:"buttons"`
			ButtonText  string `json:"button_text"`
			URL         string `json:"url"`
			DisplayText string `json:"display_text"`
			TTLMinutes  int    `json:"ttl_minutes"`
			FlowID      string `json:"flow_id"`
			FirstScreen string `json:"first_screen"`
			Header      string `json:"header"`
		} `json:"interactive"`
	}
	if !decodeJSON(ctx, &request) || (request.Type != "" && request.Type != "text" && request.Type != "interactive") || strings.TrimSpace(request.Content.Body) == "" {
		writeError(ctx, http.StatusBadRequest, "A message body is required")
		return
	}
	requestCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	orgID := claims.OrganizationID.String()
	contact, err := a.store.Contact(requestCtx, orgID, contactID)
	if err != nil || contact.IsDeleted {
		writeError(ctx, http.StatusNotFound, "Contact not found")
		return
	}
	if contact.LastInboundAt == nil || a.now().UTC().Sub(contact.LastInboundAt.UTC()) > 24*time.Hour {
		writeError(ctx, http.StatusBadRequest, "The 24-hour customer service window is closed; send an approved template")
		return
	}
	accountName := request.WhatsAppAccount
	if accountName == "" {
		accountName = contact.WhatsAppAccount
	}
	account, err := a.store.ResolveWhatsAppAccount(requestCtx, orgID, accountName)
	if err != nil {
		writeError(ctx, http.StatusBadRequest, "No WhatsApp account is configured")
		return
	}
	accessToken, err := appcrypto.Decrypt(account.AccessToken, a.config.App.EncryptionKey)
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, "WhatsApp credentials could not be opened")
		return
	}
	waAccount := &whatsapp.Account{PhoneID: account.PhoneID, BusinessID: account.BusinessID, AppID: account.AppID, APIVersion: account.APIVersion, AccessToken: accessToken}
	replyExternalID := ""
	if request.ReplyToMessageID != "" {
		if messages, listErr := a.store.ListMessages(requestCtx, orgID, contact.ID, 100, nil); listErr == nil {
			for _, candidate := range messages {
				if candidate.ID == request.ReplyToMessageID {
					replyExternalID = candidate.ExternalID
					break
				}
			}
		}
	}
	recipient := whatsapp.Recipient{Phone: contact.PhoneNumber, BSUID: contact.BSUID}
	var externalID string
	if request.Type == "interactive" {
		switch request.Interactive.Type {
		case "button", "list":
			buttons := make([]whatsapp.Button, 0, len(request.Interactive.Buttons))
			for _, button := range request.Interactive.Buttons {
				buttons = append(buttons, whatsapp.Button{ID: button.ID, Title: button.Title})
			}
			externalID, err = a.sender.SendInteractiveButtons(requestCtx, waAccount, recipient, request.Content.Body, buttons)
		case "cta_url":
			externalID, err = a.sender.SendCTAURLButton(requestCtx, waAccount, recipient, request.Content.Body, request.Interactive.ButtonText, request.Interactive.URL)
		case "voice_call":
			externalID, err = a.sender.SendVoiceCallButton(requestCtx, waAccount, recipient, request.Content.Body, request.Interactive.DisplayText, request.Interactive.TTLMinutes, "agent:"+claims.UserID.String())
		case "flow":
			externalID, err = a.sender.SendFlowMessage(requestCtx, waAccount, recipient, request.Interactive.FlowID, request.Interactive.Header, request.Content.Body, request.Interactive.ButtonText, uuid.NewString(), request.Interactive.FirstScreen)
		default:
			writeError(ctx, http.StatusBadRequest, "Unsupported interactive message type")
			return
		}
	} else {
		externalID, err = a.sender.SendTextMessage(requestCtx, waAccount, recipient, request.Content.Body, replyExternalID)
	}
	if err != nil {
		writeError(ctx, http.StatusBadGateway, "Meta rejected the message")
		return
	}
	now := a.now().UTC()
	message := firestorestore.Message{
		ID: uuid.NewString(), OrganizationID: orgID, ContactID: contact.ID,
		WhatsAppAccount: account.Name, ChannelType: contact.ChannelType, ExternalID: externalID,
		Direction: "outgoing", MessageType: firstNonEmpty(request.Type, "text"), Content: request.Content.Body, Status: "sent",
		IsReply: replyExternalID != "", ReplyToMessageID: request.ReplyToMessageID,
		SentByUserID: claims.UserID.String(), Metadata: map[string]any{}, CreatedAt: now, UpdatedAt: now,
	}
	if request.Type == "interactive" {
		encoded, _ := json.Marshal(request.Interactive)
		_ = json.Unmarshal(encoded, &message.InteractiveData)
	}
	contact.WhatsAppAccount = account.Name
	contact.LastMessageAt = &now
	contact.LastMessagePreview = request.Content.Body
	contact.IsRead = true
	contact.UpdatedAt = now
	if err := a.store.CreateOutgoingMessage(requestCtx, *contact, message); err != nil {
		writeError(ctx, http.StatusInternalServerError, "Message was sent but could not be saved")
		return
	}
	writeData(ctx, http.StatusOK, map[string]any{
		"id": message.ID, "contact_id": message.ContactID, "direction": message.Direction,
		"message_type": message.MessageType, "content": map[string]string{"body": message.Content},
		"status": message.Status, "is_reply": message.IsReply, "reply_to_message_id": message.ReplyToMessageID,
		"interactive_data": message.InteractiveData, "whatsapp_account": message.WhatsAppAccount,
		"created_at": message.CreatedAt, "updated_at": message.UpdatedAt,
	})
}

func (a *App) SendMediaMessage(ctx *fasthttp.RequestCtx) {
	claims, ok := a.authorize(ctx)
	if !ok {
		return
	}
	if a.media == nil {
		writeError(ctx, http.StatusServiceUnavailable, "Media storage is not configured")
		return
	}
	form, err := ctx.MultipartForm()
	if err != nil {
		writeError(ctx, http.StatusBadRequest, "Invalid multipart form")
		return
	}
	contactID := firstFormValue(form.Value["contact_id"])
	mediaType := firstNonEmpty(firstFormValue(form.Value["type"]), "image")
	caption := firstFormValue(form.Value["caption"])
	accountName := firstFormValue(form.Value["whatsapp_account"])
	if contactID == "" || !isMediaType(mediaType) {
		writeError(ctx, http.StatusBadRequest, "A valid contact_id and media type are required")
		return
	}
	files := form.File["file"]
	if len(files) == 0 {
		writeError(ctx, http.StatusBadRequest, "file is required")
		return
	}
	file, err := files[0].Open()
	if err != nil {
		writeError(ctx, http.StatusBadRequest, "Failed to read file")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 32<<20+1))
	if err != nil || len(data) > 32<<20 {
		writeError(ctx, http.StatusBadRequest, "Media file exceeds the 32 MiB limit")
		return
	}
	mimeType := files[0].Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = http.DetectContentType(data)
	}
	requestCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	orgID := claims.OrganizationID.String()
	contact, err := a.store.Contact(requestCtx, orgID, contactID)
	if err != nil {
		writeError(ctx, http.StatusNotFound, "Contact not found")
		return
	}
	if contact.LastInboundAt == nil || a.now().UTC().Sub(contact.LastInboundAt.UTC()) > 24*time.Hour {
		writeError(ctx, http.StatusBadRequest, "The 24-hour customer service window is closed; send an approved template")
		return
	}
	if accountName == "" {
		accountName = contact.WhatsAppAccount
	}
	account, err := a.store.ResolveWhatsAppAccount(requestCtx, orgID, accountName)
	if err != nil {
		writeError(ctx, http.StatusBadRequest, "No WhatsApp account is configured")
		return
	}
	accessToken, err := appcrypto.Decrypt(account.AccessToken, a.config.App.EncryptionKey)
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, "WhatsApp credentials could not be opened")
		return
	}
	waAccount := &whatsapp.Account{PhoneID: account.PhoneID, BusinessID: account.BusinessID, AppID: account.AppID, APIVersion: account.APIVersion, AccessToken: accessToken}
	filename := filepath.Base(files[0].Filename)
	extension := strings.ToLower(filepath.Ext(filename))
	if len(extension) > 12 {
		extension = ""
	}
	storageKey := mediaType + "s/" + uuid.NewString() + extension
	if err := a.media.Upload(requestCtx, storageKey, bytes.NewReader(data), mimeType); err != nil {
		writeError(ctx, http.StatusBadGateway, "Failed to save media")
		return
	}
	mediaID, err := a.sender.UploadMedia(requestCtx, waAccount, data, mimeType, filename)
	if err != nil {
		writeError(ctx, http.StatusBadGateway, "Meta rejected the media upload")
		return
	}
	recipient := whatsapp.Recipient{Phone: contact.PhoneNumber, BSUID: contact.BSUID}
	var externalID string
	switch mediaType {
	case "image":
		externalID, err = a.sender.SendImageMessage(requestCtx, waAccount, recipient, mediaID, caption)
	case "document":
		externalID, err = a.sender.SendDocumentMessage(requestCtx, waAccount, recipient, mediaID, filename, caption)
	case "video":
		externalID, err = a.sender.SendVideoMessage(requestCtx, waAccount, recipient, mediaID, caption)
	case "audio":
		externalID, err = a.sender.SendAudioMessage(requestCtx, waAccount, recipient, mediaID)
	}
	if err != nil {
		writeError(ctx, http.StatusBadGateway, "Meta rejected the media message")
		return
	}
	now := a.now().UTC()
	message := firestorestore.Message{ID: uuid.NewString(), OrganizationID: orgID, ContactID: contact.ID, WhatsAppAccount: account.Name, ChannelType: firstNonEmpty(contact.ChannelType, "whatsapp"), ExternalID: externalID, Direction: "outgoing", MessageType: mediaType, Content: caption, MediaURL: storageKey, MediaMimeType: mimeType, MediaFilename: filename, Status: "sent", SentByUserID: claims.UserID.String(), Metadata: map[string]any{}, CreatedAt: now, UpdatedAt: now}
	contact.WhatsAppAccount = account.Name
	contact.LastMessageAt = &now
	contact.LastMessagePreview = firstNonEmpty(caption, "["+mediaType+"]")
	contact.IsRead = true
	contact.UpdatedAt = now
	if err := a.store.CreateOutgoingMessage(requestCtx, *contact, message); err != nil {
		writeError(ctx, http.StatusInternalServerError, "Message was sent but could not be saved")
		return
	}
	writeData(ctx, http.StatusOK, messageResponse(message))
}

func (a *App) SendReaction(ctx *fasthttp.RequestCtx) {
	claims, ok := a.authorize(ctx)
	if !ok {
		return
	}
	if a.sender == nil {
		writeError(ctx, http.StatusServiceUnavailable, "Messaging is not configured")
		return
	}
	contactID, _ := ctx.UserValue("id").(string)
	messageID, _ := ctx.UserValue("message_id").(string)
	var request struct {
		Emoji string `json:"emoji"`
	}
	if !decodeJSON(ctx, &request) {
		return
	}
	requestCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	orgID := claims.OrganizationID.String()
	contact, err := a.store.Contact(requestCtx, orgID, contactID)
	if err != nil {
		writeError(ctx, http.StatusNotFound, "Contact not found")
		return
	}
	message, err := a.store.Message(requestCtx, orgID, contactID, messageID)
	if err != nil || message.ExternalID == "" {
		writeError(ctx, http.StatusNotFound, "Message not found")
		return
	}
	account, err := a.store.ResolveWhatsAppAccount(requestCtx, orgID, firstNonEmpty(message.WhatsAppAccount, contact.WhatsAppAccount))
	if err != nil {
		writeError(ctx, http.StatusBadRequest, "No WhatsApp account is configured")
		return
	}
	accessToken, err := appcrypto.Decrypt(account.AccessToken, a.config.App.EncryptionKey)
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, "WhatsApp credentials could not be opened")
		return
	}
	waAccount := &whatsapp.Account{PhoneID: account.PhoneID, BusinessID: account.BusinessID, AppID: account.AppID, APIVersion: account.APIVersion, AccessToken: accessToken}
	if err := a.sender.SendReaction(requestCtx, waAccount, whatsapp.Recipient{Phone: contact.PhoneNumber, BSUID: contact.BSUID}, message.ExternalID, request.Emoji); err != nil {
		writeError(ctx, http.StatusBadGateway, "Meta rejected the reaction")
		return
	}
	_, reactions, err := a.store.UpdateMessageReaction(requestCtx, orgID, contactID, messageID, claims.UserID.String(), request.Emoji, a.now().UTC())
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, "Reaction was sent but could not be saved")
		return
	}
	writeData(ctx, http.StatusOK, map[string]any{"message_id": messageID, "reactions": reactions})
}

func (a *App) ServeMedia(ctx *fasthttp.RequestCtx) {
	claims, ok := a.authorize(ctx)
	if !ok {
		return
	}
	messageID, _ := ctx.UserValue("message_id").(string)
	requestCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	message, err := a.store.MessageByID(requestCtx, claims.OrganizationID.String(), messageID)
	if err != nil {
		writeError(ctx, http.StatusNotFound, "Message not found")
		return
	}
	if message.MediaURL == "" {
		writeError(ctx, http.StatusNotFound, "No media found")
		return
	}
	if strings.Contains(message.MediaURL, "/") {
		if a.media == nil {
			writeError(ctx, http.StatusServiceUnavailable, "Media storage is not configured")
			return
		}
		url, err := a.media.GetPresignedURL(requestCtx, message.MediaURL, 15*time.Minute)
		if err != nil {
			writeError(ctx, http.StatusBadGateway, "Failed to load media")
			return
		}
		ctx.Redirect(url, http.StatusFound)
		return
	}
	account, err := a.store.ResolveWhatsAppAccount(requestCtx, claims.OrganizationID.String(), message.WhatsAppAccount)
	if err != nil {
		writeError(ctx, http.StatusNotFound, "WhatsApp account not found")
		return
	}
	accessToken, err := appcrypto.Decrypt(account.AccessToken, a.config.App.EncryptionKey)
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, "WhatsApp credentials could not be opened")
		return
	}
	waAccount := &whatsapp.Account{PhoneID: account.PhoneID, BusinessID: account.BusinessID, AppID: account.AppID, APIVersion: account.APIVersion, AccessToken: accessToken}
	mediaURL, err := a.sender.GetMediaURL(requestCtx, message.MediaURL, waAccount)
	if err != nil {
		writeError(ctx, http.StatusBadGateway, "Failed to resolve media")
		return
	}
	data, err := a.sender.DownloadMedia(requestCtx, mediaURL, accessToken)
	if err != nil {
		writeError(ctx, http.StatusBadGateway, "Failed to download media")
		return
	}
	ctx.SetContentType(firstNonEmpty(message.MediaMimeType, http.DetectContentType(data)))
	ctx.Response.Header.Set("Cache-Control", "private, max-age=300")
	ctx.SetBody(data)
}

func firstFormValue(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return strings.TrimSpace(values[0])
}

func isMediaType(value string) bool {
	return value == "image" || value == "document" || value == "video" || value == "audio"
}

func (a *App) WebhookVerify(ctx *fasthttp.RequestCtx) {
	mode := string(ctx.QueryArgs().Peek("hub.mode"))
	token := string(ctx.QueryArgs().Peek("hub.verify_token"))
	challenge := string(ctx.QueryArgs().Peek("hub.challenge"))
	if mode != "subscribe" || token == "" {
		writeError(ctx, http.StatusForbidden, "Verification failed")
		return
	}
	if token != a.config.WhatsApp.WebhookVerifyToken {
		requestCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, err := a.store.AccountByWebhookVerifyToken(requestCtx, token)
		cancel()
		if err != nil {
			writeError(ctx, http.StatusForbidden, "Verification failed")
			return
		}
	}
	ctx.SetStatusCode(http.StatusOK)
	ctx.SetContentType("text/plain; charset=utf-8")
	ctx.SetBodyString(challenge)
}

func (a *App) Webhook(ctx *fasthttp.RequestCtx) {
	body := append([]byte(nil), ctx.PostBody()...)
	payload, err := whatsapp.ParseWebhook(body)
	if err != nil {
		writeError(ctx, http.StatusBadRequest, "Invalid payload")
		return
	}
	phoneID := payload.GetPhoneNumberID()
	requestCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var account *firestorestore.WhatsAppAccount
	if phoneID != "" {
		account, err = a.store.AccountByPhoneID(requestCtx, phoneID)
	}
	if err != nil || account == nil {
		writeError(ctx, http.StatusNotFound, "WhatsApp account not found")
		return
	}
	if !a.verifyWebhookSignature(body, string(ctx.Request.Header.Peek("X-Hub-Signature-256")), account) {
		writeError(ctx, http.StatusForbidden, "Invalid signature")
		return
	}

	for _, incoming := range payload.ExtractMessages() {
		messageAccount, accountErr := a.store.AccountByPhoneID(requestCtx, incoming.PhoneNumberID)
		if accountErr != nil {
			continue
		}
		contact, contactErr := a.store.ContactByPhone(requestCtx, messageAccount.OrganizationID, messageAccount.Name, incoming.From)
		if contactErr != nil && !errors.Is(contactErr, firestorestore.ErrNotFound) {
			continue
		}
		if contact == nil {
			contact = &firestorestore.Contact{
				ID: uuid.NewString(), OrganizationID: messageAccount.OrganizationID,
				PhoneNumber: incoming.From, ProfileName: incoming.ContactName,
				WhatsAppAccount: messageAccount.Name, ChannelType: "whatsapp",
				Tags: []any{}, Metadata: map[string]any{}, CreatedAt: a.now().UTC(),
			}
		}
		occurredAt := incoming.Timestamp.UTC()
		if occurredAt.IsZero() {
			occurredAt = a.now().UTC()
		}
		content := incoming.Text
		if content == "" {
			content = incoming.Caption
		}
		contact.ProfileName = firstNonEmpty(incoming.ContactName, contact.ProfileName)
		contact.LastMessageAt = &occurredAt
		contact.LastInboundAt = &occurredAt
		contact.LastMessagePreview = content
		contact.IsRead = false
		contact.UpdatedAt = occurredAt
		contact.SearchTokens = firestorestore.BuildContactSearchTokens(*contact)
		message := firestorestore.Message{
			ID: uuid.NewString(), OrganizationID: contact.OrganizationID, ContactID: contact.ID,
			WhatsAppAccount: messageAccount.Name, ChannelType: "whatsapp", ExternalID: incoming.ID,
			Direction: "incoming", MessageType: incoming.Type, Content: content,
			MediaURL: incoming.MediaID, MediaMimeType: incoming.MediaMimeType,
			Status: "delivered", Metadata: map[string]any{}, CreatedAt: occurredAt, UpdatedAt: occurredAt,
		}
		if createErr := a.store.CreateInboundMessage(requestCtx, *contact, message); createErr != nil && !errors.Is(createErr, firestorestore.ErrDuplicateEvent) {
			writeError(ctx, http.StatusInternalServerError, "Webhook processing failed")
			return
		}
		// The production account has chatbot automation disabled. Preserve its
		// existing behavior: every new conversation is placed in the human queue,
		// while the active-transfer lookup makes this idempotent per contact.
		if _, transferErr := a.store.ActiveTransfer(requestCtx, contact.OrganizationID, contact.ID); errors.Is(transferErr, firestorestore.ErrNotFound) {
			transfer := firestorestore.AgentTransfer{
				ID: uuid.NewString(), OrganizationID: contact.OrganizationID, ContactID: contact.ID,
				ContactName: contact.ProfileName, PhoneNumber: contact.PhoneNumber, WhatsAppAccount: messageAccount.Name,
				Status: "active", Source: "chatbot_disabled", TransferredAt: occurredAt, CreatedAt: occurredAt, UpdatedAt: occurredAt,
			}
			if createTransferErr := a.store.CreateAgentTransfer(requestCtx, transfer); createTransferErr != nil && !errors.Is(createTransferErr, firestorestore.ErrConflict) {
				writeError(ctx, http.StatusInternalServerError, "Message saved but transfer queue update failed")
				return
			}
		} else if transferErr != nil {
			writeError(ctx, http.StatusInternalServerError, "Message saved but transfer queue lookup failed")
			return
		}
	}
	for _, update := range payload.ExtractStatuses() {
		errorMessage := update.ErrorMsg
		if errorMessage == "" {
			errorMessage = update.ErrorTitle
		}
		updatedAt := update.Timestamp.UTC()
		if updatedAt.IsZero() {
			updatedAt = a.now().UTC()
		}
		messageErr := a.store.UpdateMessageStatus(requestCtx, update.MessageID, update.Status, errorMessage, updatedAt)
		campaignErr := error(firestorestore.ErrNotFound)
		if tracker, supported := a.store.(interface {
			UpdateCampaignMessageStatus(context.Context, string, string, string, time.Time) error
		}); supported {
			campaignErr = tracker.UpdateCampaignMessageStatus(requestCtx, update.MessageID, update.Status, errorMessage, updatedAt)
		}
		for _, updateErr := range []error{messageErr, campaignErr} {
			if updateErr != nil && !errors.Is(updateErr, firestorestore.ErrNotFound) {
				writeError(ctx, http.StatusInternalServerError, "Webhook processing failed")
				return
			}
		}
	}
	ctx.SetStatusCode(http.StatusOK)
	ctx.SetContentType("text/plain; charset=utf-8")
	ctx.SetBodyString("EVENT_RECEIVED")
}

func (a *App) verifyWebhookSignature(body []byte, signature string, account *firestorestore.WhatsAppAccount) bool {
	if signature == "" || !strings.HasPrefix(signature, "sha256=") {
		return false
	}
	provided, err := hex.DecodeString(strings.TrimPrefix(signature, "sha256="))
	if err != nil {
		return false
	}
	secrets := []string{a.config.WhatsApp.AppSecret, account.AppSecret}
	for _, secret := range secrets {
		plain, decryptErr := appcrypto.Decrypt(secret, a.config.App.EncryptionKey)
		if decryptErr != nil || plain == "" {
			continue
		}
		mac := hmac.New(sha256.New, []byte(plain))
		_, _ = mac.Write(body)
		if hmac.Equal(provided, mac.Sum(nil)) {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func encodeContactCursor(cursor firestorestore.ContactCursor) string {
	payload, _ := json.Marshal(cursor)
	return base64.RawURLEncoding.EncodeToString(payload)
}

func decodeContactCursor(raw string) (*firestorestore.ContactCursor, error) {
	payload, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, err
	}
	var cursor firestorestore.ContactCursor
	if err := json.Unmarshal(payload, &cursor); err != nil || cursor.ID == "" {
		return nil, errors.New("invalid cursor")
	}
	return &cursor, nil
}

func (a *App) issueTokens(ctx context.Context, user *firestorestore.User) (string, string, error) {
	userID, err := uuid.Parse(user.ID)
	if err != nil {
		return "", "", err
	}
	orgID, err := uuid.Parse(user.OrganizationID)
	if err != nil {
		return "", "", err
	}
	var roleID *uuid.UUID
	if user.RoleID != "" {
		parsed, parseErr := uuid.Parse(user.RoleID)
		if parseErr != nil {
			return "", "", parseErr
		}
		roleID = &parsed
	}
	now := a.now().UTC()
	base := middleware.JWTClaims{UserID: userID, OrganizationID: orgID, Email: user.Email, RoleID: roleID, IsSuperAdmin: user.IsSuperAdmin}
	accessClaims := base
	accessClaims.RegisteredClaims = jwt.RegisteredClaims{IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(time.Duration(a.config.JWT.AccessExpiryMins) * time.Minute)), Issuer: "whatomate"}
	access, err := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims).SignedString([]byte(a.config.JWT.Secret))
	if err != nil {
		return "", "", err
	}
	jti := randomID()
	expiresAt := now.Add(time.Duration(a.config.JWT.RefreshExpiryDays) * 24 * time.Hour)
	refreshClaims := base
	refreshClaims.RegisteredClaims = jwt.RegisteredClaims{ID: jti, IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(expiresAt), Issuer: "whatomate"}
	refresh, err := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims).SignedString([]byte(a.config.JWT.Secret))
	if err != nil {
		return "", "", err
	}
	if err := a.store.PutRefreshToken(ctx, jti, firestorestore.RefreshToken{UserID: user.ID, CreatedAt: now, ExpiresAt: expiresAt}); err != nil {
		return "", "", err
	}
	return access, refresh, nil
}

func (a *App) authorize(ctx *fasthttp.RequestCtx) (*middleware.JWTClaims, bool) {
	access, _, _ := middleware.DecodeFirebaseSession(string(ctx.Request.Header.Cookie(middleware.FirebaseSessionCookieName)))
	if access == "" {
		access = string(ctx.Request.Header.Cookie("whm_access"))
	}
	if header := string(ctx.Request.Header.Peek("Authorization")); strings.HasPrefix(header, "Bearer ") {
		access = strings.TrimPrefix(header, "Bearer ")
	}
	claims, err := a.parseToken(access)
	if err != nil {
		writeError(ctx, http.StatusUnauthorized, "Invalid or expired token")
		return nil, false
	}
	return claims, true
}

func (a *App) parseToken(raw string) (*middleware.JWTClaims, error) {
	parsed, err := jwt.ParseWithClaims(raw, &middleware.JWTClaims{}, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(a.config.JWT.Secret), nil
	})
	if err != nil || !parsed.Valid {
		return nil, errors.New("invalid token")
	}
	claims, ok := parsed.Claims.(*middleware.JWTClaims)
	if !ok {
		return nil, errors.New("invalid claims")
	}
	return claims, nil
}

func (a *App) setCookies(ctx *fasthttp.RequestCtx, access, refresh string) {
	secure := a.config.Cookie.Secure || a.config.App.Environment == "production"
	setCookie(ctx, middleware.FirebaseSessionCookieName, middleware.EncodeFirebaseSession(access, refresh), secure, true, a.config.JWT.RefreshExpiryDays*86400)
	if a.config.Cookie.FirebaseHosting {
		// Firebase Hosting forwards only __session. Expire legacy duplicates so
		// Facebook SDK cookies cannot push the request headers over proxy limits.
		expireCookie(ctx, "whm_access")
		expireCookie(ctx, "whm_refresh")
	} else {
		setCookie(ctx, "whm_access", access, secure, true, a.config.JWT.AccessExpiryMins*60)
		setCookie(ctx, "whm_refresh", refresh, secure, true, a.config.JWT.RefreshExpiryDays*86400)
	}
	setCookie(ctx, "whm_csrf", randomID(), secure, false, a.config.JWT.RefreshExpiryDays*86400)
}

func (a *App) clearCookies(ctx *fasthttp.RequestCtx) {
	for _, name := range []string{middleware.FirebaseSessionCookieName, "whm_access", "whm_refresh", "whm_csrf"} {
		expireCookie(ctx, name)
	}
}

func expireCookie(ctx *fasthttp.RequestCtx, name string) {
	cookie := fasthttp.AcquireCookie()
	defer fasthttp.ReleaseCookie(cookie)
	cookie.SetKey(name)
	cookie.SetPath("/")
	cookie.SetExpire(time.Unix(1, 0))
	cookie.SetMaxAge(-1)
	ctx.Response.Header.SetCookie(cookie)
}

func setCookie(ctx *fasthttp.RequestCtx, name, value string, secure, httpOnly bool, maxAge int) {
	cookie := fasthttp.AcquireCookie()
	defer fasthttp.ReleaseCookie(cookie)
	cookie.SetKey(name)
	cookie.SetValue(value)
	cookie.SetPath("/")
	cookie.SetSecure(secure)
	cookie.SetHTTPOnly(httpOnly)
	cookie.SetSameSite(fasthttp.CookieSameSiteLaxMode)
	cookie.SetMaxAge(maxAge)
	ctx.Response.Header.SetCookie(cookie)
}

func decodeJSON(ctx *fasthttp.RequestCtx, target any) bool {
	if len(ctx.PostBody()) > 1<<20 {
		return false
	}
	return json.Unmarshal(ctx.PostBody(), target) == nil
}

func parseLimit(ctx *fasthttp.RequestCtx, fallback int) int {
	limit, err := strconv.Atoi(string(ctx.QueryArgs().Peek("limit")))
	if err != nil || limit < 1 || limit > 100 {
		return fallback
	}
	return limit
}

func normalizePhone(value string) string {
	value = strings.TrimSpace(value)
	return strings.TrimPrefix(value, "+")
}

func stringsToAny(values []string) []any {
	result := make([]any, 0, len(values))
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
	}
	return result
}

func splitCSV(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	result := make([]string, 0)
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			result = append(result, item)
		}
	}
	return result
}

func boolPointer(value bool) *bool { return &value }

func randomID() string {
	buffer := make([]byte, 24)
	if _, err := rand.Read(buffer); err != nil {
		return uuid.NewString()
	}
	return hex.EncodeToString(buffer)
}

func writeData(ctx *fasthttp.RequestCtx, status int, data any) {
	ctx.SetStatusCode(status)
	ctx.SetContentType("application/json; charset=utf-8")
	_ = json.NewEncoder(ctx).Encode(map[string]any{"status": "success", "data": data})
}

func writeError(ctx *fasthttp.RequestCtx, status int, message string) {
	ctx.SetStatusCode(status)
	ctx.SetContentType("application/json; charset=utf-8")
	_ = json.NewEncoder(ctx).Encode(map[string]any{"status": "error", "message": message})
}
