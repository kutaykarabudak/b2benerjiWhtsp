package serverlessapp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	appcrypto "github.com/shridarpatil/whatomate/internal/crypto"
	"github.com/shridarpatil/whatomate/internal/firestorestore"
	"github.com/shridarpatil/whatomate/pkg/whatsapp"
	"github.com/valyala/fasthttp"
)

type accountAdminMessenger interface {
	ValidateCredentials(context.Context, string, string, string, string) (*whatsapp.CredentialsValidationResult, error)
	SubscribeApp(context.Context, *whatsapp.Account) error
	ExchangeCodeForToken(context.Context, string, string, string, string) (string, error)
	GetBusinessProfile(context.Context, *whatsapp.Account) (*whatsapp.BusinessProfile, error)
	UpdateBusinessProfile(context.Context, *whatsapp.Account, whatsapp.BusinessProfileInput) error
	UploadProfilePicture(context.Context, *whatsapp.Account, []byte, string) (string, error)
}

type templateAdminMessenger interface {
	FetchTemplates(context.Context, *whatsapp.Account) ([]whatsapp.MetaTemplate, error)
	SubmitTemplate(context.Context, *whatsapp.Account, *whatsapp.TemplateSubmission) (string, error)
	DeleteTemplate(context.Context, *whatsapp.Account, string) error
	ResumableUpload(context.Context, *whatsapp.Account, []byte, string, string) (string, error)
}

type administrationStore interface {
	WhatsAppAccount(context.Context, string, string) (*firestorestore.WhatsAppAccount, error)
	PutWhatsAppAccount(context.Context, firestorestore.WhatsAppAccount) error
	ReplaceWhatsAppAccount(context.Context, firestorestore.WhatsAppAccount, firestorestore.WhatsAppAccount) error
	DeleteWhatsAppAccount(context.Context, firestorestore.WhatsAppAccount, time.Time) error
	PutTemplate(context.Context, firestorestore.Template) error
	DeleteTemplate(context.Context, firestorestore.Template, time.Time) error
	PutOrganization(context.Context, firestorestore.Organization) error
}

func (a *App) administration() administrationStore { return a.store.(administrationStore) }

type accountRequest struct {
	Name                   string `json:"name"`
	AppID                  string `json:"app_id"`
	PhoneID                string `json:"phone_id"`
	BusinessID             string `json:"business_id"`
	CatalogBusinessID      string `json:"catalog_business_id"`
	AccessToken            string `json:"access_token"`
	AppSecret              string `json:"app_secret"`
	WebhookVerifyToken     string `json:"webhook_verify_token"`
	APIVersion             string `json:"api_version"`
	IsDefaultIncoming      bool   `json:"is_default_incoming"`
	IsDefaultOutgoing      bool   `json:"is_default_outgoing"`
	AutoReadReceipt        bool   `json:"auto_read_receipt"`
	BusinessCallingEnabled bool   `json:"business_calling_enabled"`
}

func accountResult(account firestorestore.WhatsAppAccount) accountResponse {
	return accountResponse{ID: account.ID, Name: account.Name, AppID: account.AppID, PhoneID: account.PhoneID,
		BusinessID: account.BusinessID, CatalogBusinessID: account.CatalogBusinessID,
		WebhookVerifyToken: account.WebhookVerifyToken, APIVersion: account.APIVersion,
		IsDefaultIncoming: account.IsDefaultIncoming, IsDefaultOutgoing: account.IsDefaultOutgoing,
		AutoReadReceipt: account.AutoReadReceipt, BusinessCallingEnabled: account.BusinessCallingEnabled,
		Status: account.Status, HasAccessToken: account.AccessToken != "", HasAppSecret: account.AppSecret != "",
		CreatedAt: account.CreatedAt, UpdatedAt: account.UpdatedAt}
}

func (a *App) GetAccount(ctx *fasthttp.RequestCtx) {
	claims, ok := a.authorize(ctx)
	if !ok {
		return
	}
	account, err := a.administration().WhatsAppAccount(context.Background(), claims.OrganizationID.String(), pathValue(ctx, "id"))
	if err != nil {
		writeError(ctx, http.StatusNotFound, "Account not found")
		return
	}
	writeData(ctx, http.StatusOK, accountResult(*account))
}

func (a *App) CreateAccount(ctx *fasthttp.RequestCtx) {
	claims, ok := a.authorize(ctx)
	if !ok {
		return
	}
	var request accountRequest
	if !decodeJSON(ctx, &request) || strings.TrimSpace(request.Name) == "" || strings.TrimSpace(request.PhoneID) == "" || strings.TrimSpace(request.BusinessID) == "" || strings.TrimSpace(request.AccessToken) == "" {
		writeError(ctx, http.StatusBadRequest, "name, phone_id, business_id and access_token are required")
		return
	}
	now := a.now().UTC()
	account := firestorestore.WhatsAppAccount{ID: uuid.NewString(), OrganizationID: claims.OrganizationID.String(), Name: strings.TrimSpace(request.Name), AppID: strings.TrimSpace(request.AppID), PhoneID: strings.TrimSpace(request.PhoneID), BusinessID: strings.TrimSpace(request.BusinessID), CatalogBusinessID: strings.TrimSpace(request.CatalogBusinessID), AccessToken: strings.TrimSpace(request.AccessToken), AppSecret: strings.TrimSpace(request.AppSecret), WebhookVerifyToken: strings.TrimSpace(request.WebhookVerifyToken), APIVersion: strings.TrimSpace(request.APIVersion), IsDefaultIncoming: request.IsDefaultIncoming, IsDefaultOutgoing: request.IsDefaultOutgoing, AutoReadReceipt: request.AutoReadReceipt, BusinessCallingEnabled: request.BusinessCallingEnabled, Status: "active", CreatedByID: claims.UserID.String(), UpdatedByID: claims.UserID.String(), CreatedAt: now, UpdatedAt: now}
	if account.WebhookVerifyToken == "" {
		account.WebhookVerifyToken = randomID()
	}
	if account.APIVersion == "" {
		account.APIVersion = a.config.WhatsApp.APIVersion
	}
	if err := appcrypto.EncryptFields(a.config.App.EncryptionKey, &account.AccessToken, &account.AppSecret); err != nil {
		writeError(ctx, 500, "Failed to protect credentials")
		return
	}
	if err := a.clearOtherAccountDefaults(context.Background(), account); err != nil {
		writeError(ctx, 500, "Failed to update account defaults")
		return
	}
	if err := a.administration().PutWhatsAppAccount(context.Background(), account); err != nil {
		writeError(ctx, 500, "Failed to create account")
		return
	}
	writeData(ctx, http.StatusOK, accountResult(account))
}

func (a *App) UpdateAccount(ctx *fasthttp.RequestCtx) {
	claims, ok := a.authorize(ctx)
	if !ok {
		return
	}
	previous, err := a.administration().WhatsAppAccount(context.Background(), claims.OrganizationID.String(), pathValue(ctx, "id"))
	if err != nil {
		writeError(ctx, 404, "Account not found")
		return
	}
	var request accountRequest
	if !decodeJSON(ctx, &request) {
		writeError(ctx, 400, "Invalid request")
		return
	}
	account := *previous
	if strings.TrimSpace(request.Name) != "" {
		account.Name = strings.TrimSpace(request.Name)
	}
	if strings.TrimSpace(request.AppID) != "" {
		account.AppID = strings.TrimSpace(request.AppID)
	}
	if strings.TrimSpace(request.PhoneID) != "" {
		account.PhoneID = strings.TrimSpace(request.PhoneID)
	}
	if strings.TrimSpace(request.BusinessID) != "" {
		account.BusinessID = strings.TrimSpace(request.BusinessID)
	}
	account.CatalogBusinessID = strings.TrimSpace(request.CatalogBusinessID)
	if request.AccessToken != "" {
		account.AccessToken = request.AccessToken
	}
	if request.AppSecret != "" {
		account.AppSecret = request.AppSecret
	}
	if request.WebhookVerifyToken != "" {
		account.WebhookVerifyToken = request.WebhookVerifyToken
	}
	if request.APIVersion != "" {
		account.APIVersion = request.APIVersion
	}
	account.IsDefaultIncoming, account.IsDefaultOutgoing = request.IsDefaultIncoming, request.IsDefaultOutgoing
	account.AutoReadReceipt, account.BusinessCallingEnabled = request.AutoReadReceipt, request.BusinessCallingEnabled
	account.UpdatedByID, account.UpdatedAt = claims.UserID.String(), a.now().UTC()
	if err := appcrypto.EncryptFields(a.config.App.EncryptionKey, &account.AccessToken, &account.AppSecret); err != nil {
		writeError(ctx, 500, "Failed to protect credentials")
		return
	}
	if err := a.clearOtherAccountDefaults(context.Background(), account); err != nil {
		writeError(ctx, 500, "Failed to update account defaults")
		return
	}
	if err := a.administration().ReplaceWhatsAppAccount(context.Background(), *previous, account); err != nil {
		writeError(ctx, 500, "Failed to update account")
		return
	}
	writeData(ctx, 200, accountResult(account))
}

func (a *App) clearOtherAccountDefaults(requestCtx context.Context, selected firestorestore.WhatsAppAccount) error {
	if !selected.IsDefaultIncoming && !selected.IsDefaultOutgoing {
		return nil
	}
	accounts, err := a.store.ListWhatsAppAccounts(requestCtx, selected.OrganizationID)
	if err != nil {
		return err
	}
	for _, other := range accounts {
		if other.ID == selected.ID {
			continue
		}
		before := other
		changed := false
		if selected.IsDefaultIncoming && other.IsDefaultIncoming {
			other.IsDefaultIncoming = false
			changed = true
		}
		if selected.IsDefaultOutgoing && other.IsDefaultOutgoing {
			other.IsDefaultOutgoing = false
			changed = true
		}
		if changed {
			other.UpdatedAt = a.now().UTC()
			if err := a.administration().ReplaceWhatsAppAccount(requestCtx, before, other); err != nil {
				return err
			}
		}
	}
	return nil
}

func (a *App) DeleteAccount(ctx *fasthttp.RequestCtx) {
	claims, ok := a.authorize(ctx)
	if !ok {
		return
	}
	account, err := a.administration().WhatsAppAccount(context.Background(), claims.OrganizationID.String(), pathValue(ctx, "id"))
	if err != nil {
		writeError(ctx, 404, "Account not found")
		return
	}
	if err := a.administration().DeleteWhatsAppAccount(context.Background(), *account, a.now().UTC()); err != nil {
		writeError(ctx, 500, "Failed to delete account")
		return
	}
	writeData(ctx, 200, map[string]string{"message": "Account deleted successfully"})
}

func (a *App) accountForAdmin(ctx *fasthttp.RequestCtx) (*firestorestore.WhatsAppAccount, *whatsapp.Account, accountAdminMessenger, bool) {
	claims, ok := a.authorize(ctx)
	if !ok {
		return nil, nil, nil, false
	}
	account, err := a.administration().WhatsAppAccount(context.Background(), claims.OrganizationID.String(), pathValue(ctx, "id"))
	if err != nil {
		writeError(ctx, 404, "Account not found")
		return nil, nil, nil, false
	}
	token, err := appcrypto.Decrypt(account.AccessToken, a.config.App.EncryptionKey)
	if err != nil {
		writeError(ctx, 500, "WhatsApp credentials could not be opened")
		return nil, nil, nil, false
	}
	admin, ok := a.sender.(accountAdminMessenger)
	if !ok {
		writeError(ctx, 501, "WhatsApp account administration is unavailable")
		return nil, nil, nil, false
	}
	return account, &whatsapp.Account{PhoneID: account.PhoneID, BusinessID: account.BusinessID, AppID: account.AppID, APIVersion: account.APIVersion, AccessToken: token}, admin, true
}

func (a *App) TestAccountConnection(ctx *fasthttp.RequestCtx) {
	account, wa, admin, ok := a.accountForAdmin(ctx)
	if !ok {
		return
	}
	requestCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := admin.ValidateCredentials(requestCtx, wa.PhoneID, wa.BusinessID, wa.AccessToken, wa.APIVersion)
	if err != nil {
		writeData(ctx, 200, map[string]any{"success": false, "error": err.Error()})
		return
	}
	if err := admin.SubscribeApp(requestCtx, wa); err != nil {
		writeData(ctx, 200, map[string]any{"success": false, "error": err.Error()})
		return
	}
	account.Status = "active"
	account.UpdatedAt = a.now().UTC()
	_ = a.administration().PutWhatsAppAccount(context.Background(), *account)
	writeData(ctx, 200, map[string]any{"success": true, "display_phone_number": result.PhoneNumber, "verified_name": result.VerifiedName, "quality_rating": result.QualityRating, "code_verification_status": result.CodeVerificationStatus, "account_mode": result.AccountMode, "is_test_number": result.IsTestNumber, "warning": result.Warning})
}

func (a *App) SubscribeAccount(ctx *fasthttp.RequestCtx) {
	_, wa, admin, ok := a.accountForAdmin(ctx)
	if !ok {
		return
	}
	requestCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := admin.SubscribeApp(requestCtx, wa); err != nil {
		writeError(ctx, 502, err.Error())
		return
	}
	writeData(ctx, 200, map[string]any{"success": true, "message": "Subscribed successfully"})
}

func (a *App) GetEmbeddedSignupConfig(ctx *fasthttp.RequestCtx) {
	claims, ok := a.authorize(ctx)
	if !ok {
		return
	}
	appID, configID, apiVersion := a.config.WhatsApp.AppID, a.config.WhatsApp.ConfigID, a.config.WhatsApp.APIVersion
	if org, err := a.store.Organization(context.Background(), claims.OrganizationID.String()); err == nil {
		if value, ok := org.Settings["meta_app_id"].(string); ok && value != "" {
			appID = value
		}
		if value, ok := org.Settings["meta_config_id"].(string); ok && value != "" {
			configID = value
		}
	}
	writeData(ctx, 200, map[string]string{"whatsapp_app_id": appID, "whatsapp_config_id": configID, "whatsapp_api_version": apiVersion})
}

func (a *App) ExchangeAccountToken(ctx *fasthttp.RequestCtx) {
	claims, ok := a.authorize(ctx)
	if !ok {
		return
	}
	var request struct {
		Code    string `json:"code"`
		PhoneID string `json:"phone_id"`
		WABAID  string `json:"waba_id"`
	}
	if !decodeJSON(ctx, &request) || request.Code == "" {
		writeError(ctx, 400, "code is required")
		return
	}
	org, err := a.store.Organization(context.Background(), claims.OrganizationID.String())
	if err != nil {
		writeError(ctx, 404, "Organization not found")
		return
	}
	appID, apiVersion := a.config.WhatsApp.AppID, a.config.WhatsApp.APIVersion
	appSecret := a.config.WhatsApp.AppSecret
	if value, ok := org.Settings["meta_app_id"].(string); ok && value != "" {
		appID = value
	}
	if value, ok := org.Settings["meta_app_secret_encrypted"].(string); ok && value != "" {
		if plain, decErr := appcrypto.Decrypt(value, a.config.App.EncryptionKey); decErr == nil {
			appSecret = plain
		}
	}
	if appID == "" || appSecret == "" {
		writeError(ctx, 400, "Meta App ID and App Secret must be configured first")
		return
	}
	admin, ok := a.sender.(accountAdminMessenger)
	if !ok {
		writeError(ctx, 501, "Embedded signup is unavailable")
		return
	}
	requestCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	token, err := admin.ExchangeCodeForToken(requestCtx, request.Code, appID, appSecret, apiVersion)
	if err != nil {
		writeError(ctx, 502, "Failed to exchange Meta token: "+err.Error())
		return
	}
	if request.PhoneID == "" || request.WABAID == "" {
		writeError(ctx, 400, "Meta did not return phone_id and waba_id; add the account manually")
		return
	}
	now := a.now().UTC()
	encrypted, err := appcrypto.Encrypt(token, a.config.App.EncryptionKey)
	if err != nil {
		writeError(ctx, 500, "Failed to protect credentials")
		return
	}
	account := firestorestore.WhatsAppAccount{ID: uuid.NewString(), OrganizationID: claims.OrganizationID.String(), Name: "WhatsApp " + request.PhoneID, AppID: appID, PhoneID: request.PhoneID, BusinessID: request.WABAID, AccessToken: encrypted, WebhookVerifyToken: randomID(), APIVersion: apiVersion, Status: "active", CreatedByID: claims.UserID.String(), UpdatedByID: claims.UserID.String(), CreatedAt: now, UpdatedAt: now}
	if err := a.administration().PutWhatsAppAccount(requestCtx, account); err != nil {
		writeError(ctx, 500, "Failed to save connected account")
		return
	}
	wa := &whatsapp.Account{PhoneID: account.PhoneID, BusinessID: account.BusinessID, AppID: account.AppID, APIVersion: account.APIVersion, AccessToken: token}
	if err := admin.SubscribeApp(requestCtx, wa); err != nil {
		account.Status = "subscription_failed"
		_ = a.administration().PutWhatsAppAccount(requestCtx, account)
	}
	writeData(ctx, 200, map[string]any{"account": accountResult(account)})
}

func (a *App) GetBusinessProfile(ctx *fasthttp.RequestCtx) {
	_, wa, admin, ok := a.accountForAdmin(ctx)
	if !ok {
		return
	}
	requestCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	profile, err := admin.GetBusinessProfile(requestCtx, wa)
	if err != nil {
		writeError(ctx, 502, err.Error())
		return
	}
	writeData(ctx, 200, profile)
}
func (a *App) UpdateBusinessProfile(ctx *fasthttp.RequestCtx) {
	_, wa, admin, ok := a.accountForAdmin(ctx)
	if !ok {
		return
	}
	var request whatsapp.BusinessProfileInput
	if !decodeJSON(ctx, &request) {
		writeError(ctx, 400, "Invalid request")
		return
	}
	requestCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := admin.UpdateBusinessProfile(requestCtx, wa, request); err != nil {
		writeError(ctx, 502, err.Error())
		return
	}
	writeData(ctx, 200, map[string]string{"message": "Business profile updated successfully"})
}
func (a *App) UpdateBusinessProfilePhoto(ctx *fasthttp.RequestCtx) {
	_, wa, admin, ok := a.accountForAdmin(ctx)
	if !ok {
		return
	}
	form, err := ctx.MultipartForm()
	if err != nil || len(form.File["file"]) == 0 {
		writeError(ctx, 400, "Profile image is required")
		return
	}
	fileHeader := form.File["file"][0]
	if fileHeader.Size > 5<<20 {
		writeError(ctx, 400, "Profile image must be smaller than 5 MB")
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		writeError(ctx, 400, "Failed to open profile image")
		return
	}
	defer file.Close()
	data := make([]byte, fileHeader.Size)
	if _, err := io.ReadFull(file, data); err != nil {
		writeError(ctx, 400, "Failed to read profile image")
		return
	}
	mimeType := fileHeader.Header.Get("Content-Type")
	requestCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	handle, err := admin.UploadProfilePicture(requestCtx, wa, data, mimeType)
	if err != nil {
		writeError(ctx, 502, err.Error())
		return
	}
	if err := admin.UpdateBusinessProfile(requestCtx, wa, whatsapp.BusinessProfileInput{ProfilePictureHandle: handle}); err != nil {
		writeError(ctx, 502, err.Error())
		return
	}
	writeData(ctx, 200, map[string]string{"message": "Profile picture updated successfully"})
}

type templateRequest struct {
	WhatsAppAccount           string `json:"whatsapp_account"`
	Name                      string `json:"name"`
	DisplayName               string `json:"display_name"`
	Language                  string `json:"language"`
	Category                  string `json:"category"`
	HeaderType                string `json:"header_type"`
	HeaderContent             string `json:"header_content"`
	BodyContent               string `json:"body_content"`
	FooterContent             string `json:"footer_content"`
	Buttons                   []any  `json:"buttons"`
	SampleValues              []any  `json:"sample_values"`
	AddSecurityRecommendation bool   `json:"add_security_recommendation"`
	CodeExpirationMinutes     int    `json:"code_expiration_minutes"`
}

var invalidTemplateChars = regexp.MustCompile(`[^a-z0-9_]+`)

func normalizedTemplateName(value string) string {
	return strings.Trim(invalidTemplateChars.ReplaceAllString(strings.ReplaceAll(strings.ToLower(strings.TrimSpace(value)), " ", "_"), "_"), "_")
}

func (a *App) CreateTemplate(ctx *fasthttp.RequestCtx) {
	claims, ok := a.authorize(ctx)
	if !ok {
		return
	}
	var request templateRequest
	if !decodeJSON(ctx, &request) {
		writeError(ctx, 400, "Invalid request")
		return
	}
	if request.WhatsAppAccount == "" || request.Name == "" || request.Language == "" || request.Category == "" || (request.BodyContent == "" && !strings.EqualFold(request.Category, "AUTHENTICATION")) {
		writeError(ctx, 400, "whatsapp_account, name, language, category and body are required")
		return
	}
	if _, err := a.store.ResolveWhatsAppAccount(context.Background(), claims.OrganizationID.String(), request.WhatsAppAccount); err != nil {
		writeError(ctx, 400, "WhatsApp account not found")
		return
	}
	now := a.now().UTC()
	template := firestorestore.Template{ID: uuid.NewString(), OrganizationID: claims.OrganizationID.String(), WhatsAppAccount: request.WhatsAppAccount, Name: normalizedTemplateName(request.Name), DisplayName: firstNonEmpty(request.DisplayName, request.Name), Language: request.Language, Category: strings.ToUpper(request.Category), Status: "DRAFT", QualityRating: "UNKNOWN", HeaderType: strings.ToUpper(request.HeaderType), HeaderContent: request.HeaderContent, BodyContent: request.BodyContent, FooterContent: request.FooterContent, Buttons: request.Buttons, SampleValues: request.SampleValues, AddSecurityRecommendation: request.AddSecurityRecommendation, CodeExpirationMinutes: request.CodeExpirationMinutes, CreatedByID: claims.UserID.String(), UpdatedByID: claims.UserID.String(), CreatedAt: now, UpdatedAt: now}
	if err := a.administration().PutTemplate(context.Background(), template); err != nil {
		writeError(ctx, 500, "Failed to create template")
		return
	}
	writeData(ctx, 200, template)
}

func (a *App) GetTemplate(ctx *fasthttp.RequestCtx) {
	claims, ok := a.authorize(ctx)
	if !ok {
		return
	}
	template, err := a.store.Template(context.Background(), claims.OrganizationID.String(), pathValue(ctx, "id"))
	if err != nil {
		writeError(ctx, 404, "Template not found")
		return
	}
	writeData(ctx, 200, template)
}

func applyTemplateRequest(template *firestorestore.Template, request templateRequest) {
	if request.WhatsAppAccount != "" {
		template.WhatsAppAccount = request.WhatsAppAccount
	}
	if request.DisplayName != "" {
		template.DisplayName = request.DisplayName
	}
	if request.Language != "" {
		template.Language = request.Language
	}
	if request.Category != "" {
		template.Category = strings.ToUpper(request.Category)
	}
	if request.HeaderType != "" {
		template.HeaderType = strings.ToUpper(request.HeaderType)
	}
	template.HeaderContent = request.HeaderContent
	if request.BodyContent != "" {
		template.BodyContent = request.BodyContent
	}
	template.FooterContent = request.FooterContent
	if request.Buttons != nil {
		template.Buttons = request.Buttons
	}
	if request.SampleValues != nil {
		template.SampleValues = request.SampleValues
	}
	template.AddSecurityRecommendation = request.AddSecurityRecommendation
	template.CodeExpirationMinutes = request.CodeExpirationMinutes
}

func (a *App) UpdateTemplate(ctx *fasthttp.RequestCtx) {
	claims, ok := a.authorize(ctx)
	if !ok {
		return
	}
	template, err := a.store.Template(context.Background(), claims.OrganizationID.String(), pathValue(ctx, "id"))
	if err != nil {
		writeError(ctx, 404, "Template not found")
		return
	}
	var request templateRequest
	if !decodeJSON(ctx, &request) {
		writeError(ctx, 400, "Invalid request")
		return
	}
	applyTemplateRequest(template, request)
	if template.Status == "APPROVED" || template.Status == "REJECTED" {
		template.Status = "DRAFT"
	}
	template.UpdatedByID = claims.UserID.String()
	template.UpdatedAt = a.now().UTC()
	if err := a.administration().PutTemplate(context.Background(), *template); err != nil {
		writeError(ctx, 500, "Failed to update template")
		return
	}
	writeData(ctx, 200, template)
}

func (a *App) DeleteTemplate(ctx *fasthttp.RequestCtx) {
	claims, ok := a.authorize(ctx)
	if !ok {
		return
	}
	template, err := a.store.Template(context.Background(), claims.OrganizationID.String(), pathValue(ctx, "id"))
	if err != nil {
		writeError(ctx, 404, "Template not found")
		return
	}
	if template.MetaTemplateID != "" {
		if account, wa, adminErr := a.templateAccount(context.Background(), claims.OrganizationID.String(), template.WhatsAppAccount); adminErr == nil {
			_ = account
			_ = a.templateDelete(context.Background(), wa, template.Name)
		}
	}
	if err := a.administration().DeleteTemplate(context.Background(), *template, a.now().UTC()); err != nil {
		writeError(ctx, 500, "Failed to delete template")
		return
	}
	writeData(ctx, 200, map[string]string{"message": "Template deleted successfully"})
}

func (a *App) templateAccount(ctx context.Context, orgID, name string) (*firestorestore.WhatsAppAccount, *whatsapp.Account, error) {
	account, err := a.store.ResolveWhatsAppAccount(ctx, orgID, name)
	if err != nil {
		return nil, nil, err
	}
	token, err := appcrypto.Decrypt(account.AccessToken, a.config.App.EncryptionKey)
	if err != nil {
		return nil, nil, err
	}
	return account, &whatsapp.Account{PhoneID: account.PhoneID, BusinessID: account.BusinessID, AppID: account.AppID, APIVersion: account.APIVersion, AccessToken: token}, nil
}
func (a *App) templateDelete(ctx context.Context, account *whatsapp.Account, name string) error {
	admin, ok := a.sender.(templateAdminMessenger)
	if !ok {
		return errors.New("template administration unavailable")
	}
	return admin.DeleteTemplate(ctx, account, name)
}

func (a *App) SyncTemplates(ctx *fasthttp.RequestCtx) {
	claims, ok := a.authorize(ctx)
	if !ok {
		return
	}
	var request struct {
		WhatsAppAccount string `json:"whatsapp_account"`
	}
	_ = json.Unmarshal(ctx.PostBody(), &request)
	if request.WhatsAppAccount == "" {
		request.WhatsAppAccount = string(ctx.QueryArgs().Peek("account"))
	}
	if request.WhatsAppAccount == "" {
		writeError(ctx, 400, "whatsapp_account is required")
		return
	}
	_, wa, err := a.templateAccount(context.Background(), claims.OrganizationID.String(), request.WhatsAppAccount)
	if err != nil {
		writeError(ctx, 404, "WhatsApp account not found")
		return
	}
	admin, ok := a.sender.(templateAdminMessenger)
	if !ok {
		writeError(ctx, 501, "Template synchronization is unavailable")
		return
	}
	requestCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	remote, err := admin.FetchTemplates(requestCtx, wa)
	if err != nil {
		writeError(ctx, 502, "Failed to fetch templates from Meta: "+err.Error())
		return
	}
	existing, _ := a.store.ListTemplates(requestCtx, claims.OrganizationID.String())
	now := a.now().UTC()
	synced := 0
	for _, meta := range remote {
		var template *firestorestore.Template
		for i := range existing {
			if existing[i].WhatsAppAccount == request.WhatsAppAccount && existing[i].Name == meta.Name && existing[i].Language == meta.Language {
				copy := existing[i]
				template = &copy
				break
			}
		}
		if template == nil {
			template = &firestorestore.Template{ID: uuid.NewString(), OrganizationID: claims.OrganizationID.String(), WhatsAppAccount: request.WhatsAppAccount, Name: meta.Name, DisplayName: meta.Name, CreatedAt: now}
		}
		template.MetaTemplateID = meta.ID
		template.Language = meta.Language
		template.Category = meta.Category
		template.Status = meta.Status
		template.QualityRating = meta.QualityRating
		if meta.QualityScore != nil && meta.QualityScore.Score != "" {
			template.QualityRating = meta.QualityScore.Score
		}
		for _, component := range meta.Components {
			switch component.Type {
			case "HEADER":
				template.HeaderType = component.Format
				template.HeaderContent = component.Text
			case "BODY":
				template.BodyContent = component.Text
			case "FOOTER":
				template.FooterContent = component.Text
			case "BUTTONS":
				raw, _ := json.Marshal(component.Buttons)
				_ = json.Unmarshal(raw, &template.Buttons)
			}
		}
		template.IsDeleted = false
		template.DeletedAt = nil
		template.UpdatedAt = now
		if err := a.administration().PutTemplate(requestCtx, *template); err == nil {
			synced++
		}
	}
	writeData(ctx, 200, map[string]any{"message": "Templates synchronized successfully", "synced": synced})
}

func (a *App) PublishTemplate(ctx *fasthttp.RequestCtx) {
	claims, ok := a.authorize(ctx)
	if !ok {
		return
	}
	template, err := a.store.Template(context.Background(), claims.OrganizationID.String(), pathValue(ctx, "id"))
	if err != nil {
		writeError(ctx, 404, "Template not found")
		return
	}
	_, wa, err := a.templateAccount(context.Background(), claims.OrganizationID.String(), template.WhatsAppAccount)
	if err != nil {
		writeError(ctx, 400, "WhatsApp account not found")
		return
	}
	admin, ok := a.sender.(templateAdminMessenger)
	if !ok {
		writeError(ctx, 501, "Template publishing is unavailable")
		return
	}
	submission := &whatsapp.TemplateSubmission{MetaTemplateID: template.MetaTemplateID, Name: template.Name, Language: template.Language, Category: template.Category, HeaderType: template.HeaderType, HeaderContent: template.HeaderContent, BodyContent: template.BodyContent, FooterContent: template.FooterContent, Buttons: template.Buttons, SampleValues: template.SampleValues, AddSecurityRecommendation: template.AddSecurityRecommendation, CodeExpirationMinutes: template.CodeExpirationMinutes}
	requestCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	metaID, err := admin.SubmitTemplate(requestCtx, wa, submission)
	if err != nil {
		writeError(ctx, 502, "Failed to submit template to Meta: "+err.Error())
		return
	}
	template.MetaTemplateID = metaID
	template.Status = "PENDING"
	template.UpdatedAt = a.now().UTC()
	if err := a.administration().PutTemplate(requestCtx, *template); err != nil {
		writeError(ctx, 500, "Template submitted but local state could not be saved")
		return
	}
	writeData(ctx, 200, map[string]any{"message": "Template submitted to Meta for approval", "meta_template_id": metaID, "status": template.Status, "template": template})
}

func (a *App) UploadTemplateMedia(ctx *fasthttp.RequestCtx) {
	claims, ok := a.authorize(ctx)
	if !ok {
		return
	}
	accountName := string(ctx.FormValue("account"))
	if accountName == "" {
		accountName = string(ctx.QueryArgs().Peek("account"))
	}
	if accountName == "" {
		writeError(ctx, 400, "account is required")
		return
	}
	_, wa, err := a.templateAccount(context.Background(), claims.OrganizationID.String(), accountName)
	if err != nil {
		writeError(ctx, 400, "WhatsApp account not found")
		return
	}
	admin, ok := a.sender.(templateAdminMessenger)
	if !ok {
		writeError(ctx, 501, "Template media upload is unavailable")
		return
	}
	fileHeader, err := ctx.FormFile("file")
	if err != nil {
		writeError(ctx, 400, "No file provided")
		return
	}
	if fileHeader.Size > 32<<20 {
		writeError(ctx, 400, "File is too large")
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
	requestCtx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	handle, err := admin.ResumableUpload(requestCtx, wa, data, mimeType, fileHeader.Filename)
	if err != nil {
		writeError(ctx, 502, "Failed to upload media to Meta: "+err.Error())
		return
	}
	writeData(ctx, 200, map[string]any{"handle": handle, "filename": fileHeader.Filename, "mime_type": mimeType, "size": fileHeader.Size})
}

func (a *App) GetOrganizationSettings(ctx *fasthttp.RequestCtx) {
	claims, ok := a.authorize(ctx)
	if !ok {
		return
	}
	org, err := a.store.Organization(context.Background(), claims.OrganizationID.String())
	if err != nil {
		writeError(ctx, 404, "Organization not found")
		return
	}
	settings := org.Settings
	if settings == nil {
		settings = map[string]any{}
	}
	safe := map[string]any{}
	for key, value := range settings {
		if key != "meta_app_secret_encrypted" {
			safe[key] = value
		}
	}
	safe["has_meta_app_secret"] = settings["meta_app_secret_encrypted"] != ""
	writeData(ctx, 200, map[string]any{"settings": safe, "name": org.Name})
}

func (a *App) UpdateOrganizationSettings(ctx *fasthttp.RequestCtx) {
	claims, ok := a.authorize(ctx)
	if !ok {
		return
	}
	org, err := a.store.Organization(context.Background(), claims.OrganizationID.String())
	if err != nil {
		writeError(ctx, 404, "Organization not found")
		return
	}
	var request map[string]any
	if !decodeJSON(ctx, &request) {
		writeError(ctx, 400, "Invalid request")
		return
	}
	if org.Settings == nil {
		org.Settings = map[string]any{}
	}
	for key, value := range request {
		switch key {
		case "name":
			if name, ok := value.(string); ok && strings.TrimSpace(name) != "" {
				org.Name = strings.TrimSpace(name)
			}
		case "meta_app_secret":
			if secret, ok := value.(string); ok && secret != "" {
				encrypted, encErr := appcrypto.Encrypt(secret, a.config.App.EncryptionKey)
				if encErr != nil {
					writeError(ctx, 500, "Failed to protect Meta secret")
					return
				}
				org.Settings["meta_app_secret_encrypted"] = encrypted
			}
		default:
			org.Settings[key] = value
		}
	}
	org.UpdatedAt = a.now().UTC()
	if err := a.administration().PutOrganization(context.Background(), *org); err != nil {
		writeError(ctx, 500, "Failed to update settings")
		return
	}
	a.GetOrganizationSettings(ctx)
}

func (a *App) UpdateCurrentUserSettings(ctx *fasthttp.RequestCtx) {
	claims, ok := a.authorize(ctx)
	if !ok {
		return
	}
	user, err := a.store.User(context.Background(), claims.UserID.String())
	if err != nil {
		writeError(ctx, 404, "User not found")
		return
	}
	var request map[string]any
	if !decodeJSON(ctx, &request) {
		writeError(ctx, 400, "Invalid request")
		return
	}
	if user.Settings == nil {
		user.Settings = map[string]any{}
	}
	for key, value := range request {
		user.Settings[key] = value
	}
	user.UpdatedAt = a.now().UTC()
	if err := a.store.PutUser(context.Background(), *user); err != nil {
		writeError(ctx, 500, "Failed to update user settings")
		return
	}
	writeData(ctx, 200, user.Settings)
}

func (a *App) ListAuditLogs(ctx *fasthttp.RequestCtx) {
	if _, ok := a.authorize(ctx); !ok {
		return
	}
	writeData(ctx, 200, map[string]any{"audit_logs": []any{}, "total": 0, "page": 1, "limit": 10})
}

func pathValue(ctx *fasthttp.RequestCtx, key string) string {
	value, _ := ctx.UserValue(key).(string)
	return value
}
