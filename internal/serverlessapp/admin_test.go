package serverlessapp

import (
	"context"
	"testing"
	"time"

	"github.com/shridarpatil/whatomate/internal/config"
	appcrypto "github.com/shridarpatil/whatomate/internal/crypto"
	"github.com/shridarpatil/whatomate/internal/firestorestore"
	"github.com/shridarpatil/whatomate/pkg/whatsapp"
)

func TestTemplateAccountFallsBackToOrganizationMetaAppID(t *testing.T) {
	const encryptionKey = "0123456789abcdef0123456789abcdef"
	encryptedToken, err := appcrypto.Encrypt("access-token", encryptionKey)
	if err != nil {
		t.Fatal(err)
	}
	store := &fakeStore{
		accounts: []firestorestore.WhatsAppAccount{{Name: "main", AccessToken: encryptedToken}},
		organization: &firestorestore.Organization{
			ID:       "org",
			Settings: map[string]any{"meta_app_id": "org-app-id"},
		},
	}
	app := &App{store: store, config: &config.Config{App: config.AppConfig{EncryptionKey: encryptionKey}}}

	_, account, err := app.templateAccount(context.Background(), "org", "main")
	if err != nil {
		t.Fatal(err)
	}
	if account.AppID != "org-app-id" {
		t.Fatalf("app id=%q, want organization Meta App ID", account.AppID)
	}
	if account.AccessToken != "access-token" {
		t.Fatalf("access token was not decrypted")
	}
}

func TestTemplateAccountPrefersAccountAppID(t *testing.T) {
	const encryptionKey = "0123456789abcdef0123456789abcdef"
	encryptedToken, err := appcrypto.Encrypt("access-token", encryptionKey)
	if err != nil {
		t.Fatal(err)
	}
	store := &fakeStore{
		accounts: []firestorestore.WhatsAppAccount{{Name: "main", AppID: "account-app-id", AccessToken: encryptedToken}},
		organization: &firestorestore.Organization{
			ID:       "org",
			Settings: map[string]any{"meta_app_id": "org-app-id"},
		},
	}
	app := &App{store: store, config: &config.Config{App: config.AppConfig{EncryptionKey: encryptionKey}, WhatsApp: config.WhatsAppConfig{AppID: "config-app-id"}}}

	_, account, err := app.templateAccount(context.Background(), "org", "main")
	if err != nil {
		t.Fatal(err)
	}
	if account.AppID != "account-app-id" {
		t.Fatalf("app id=%q, want account App ID", account.AppID)
	}
}

func TestNormalizedTemplateNameTransliteratesTurkishCharacters(t *testing.T) {
	if got := normalizedTemplateName("B2B Tanıtım Şablonu"); got != "b2b_tanitim_sablonu" {
		t.Fatalf("normalized name=%q", got)
	}
}

func TestReconcileTemplateSyncMarksRemoteMissingTemplateUnavailable(t *testing.T) {
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	existing := []firestorestore.Template{
		{ID: "stale", OrganizationID: "org", WhatsAppAccount: "main", Name: "b2b_tan_t_m", Language: "tr", Status: "APPROVED", MetaTemplateID: "old-meta"},
		{ID: "current", OrganizationID: "org", WhatsAppAccount: "main", Name: "b2b_enerji_hatirlatma", Language: "tr", Status: "APPROVED", MetaTemplateID: "current-meta"},
		{ID: "draft", OrganizationID: "org", WhatsAppAccount: "main", Name: "local_draft", Language: "tr", Status: "DRAFT"},
		{ID: "other-account", OrganizationID: "org", WhatsAppAccount: "other", Name: "other", Language: "tr", Status: "APPROVED", MetaTemplateID: "other-meta"},
	}
	remote := []whatsapp.MetaTemplate{{ID: "current-meta", Name: "b2b_enerji_hatirlatma", Language: "tr", Status: "APPROVED", Category: "MARKETING"}}

	synced, unavailable := reconcileTemplateSync(existing, remote, "org", "main", now)
	if len(synced) != 1 || synced[0].ID != "current" || synced[0].MetaTemplateID != "current-meta" {
		t.Fatalf("synced=%+v", synced)
	}
	if len(unavailable) != 1 || unavailable[0].ID != "stale" {
		t.Fatalf("unavailable=%+v", unavailable)
	}
	if unavailable[0].Status != "NOT_AVAILABLE" || unavailable[0].MetaTemplateID != "" || !unavailable[0].UpdatedAt.Equal(now) {
		t.Fatalf("stale template=%+v", unavailable[0])
	}
}
