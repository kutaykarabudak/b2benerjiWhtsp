package serverlessapp

import (
	"errors"
	"testing"

	"github.com/shridarpatil/whatomate/internal/firestorestore"
)

func TestSingleActiveCampaignAccount(t *testing.T) {
	accounts := []firestorestore.WhatsAppAccount{
		{Name: "deleted", Status: "active", IsDeleted: true},
		{Name: "paused", Status: "paused"},
		{Name: "main", Status: "ACTIVE"},
	}

	account, err := singleActiveCampaignAccount(accounts)
	if err != nil {
		t.Fatal(err)
	}
	if account.Name != "main" {
		t.Fatalf("account = %q", account.Name)
	}
}

func TestSingleActiveCampaignAccountRejectsAmbiguousConfiguration(t *testing.T) {
	_, err := singleActiveCampaignAccount([]firestorestore.WhatsAppAccount{
		{Name: "first", Status: "active"},
		{Name: "second", Status: "active"},
	})
	if !errors.Is(err, firestorestore.ErrConflict) {
		t.Fatalf("error = %v", err)
	}
}

func TestSingleActiveCampaignAccountRequiresAnActiveAccount(t *testing.T) {
	_, err := singleActiveCampaignAccount([]firestorestore.WhatsAppAccount{{Name: "paused", Status: "paused"}})
	if !errors.Is(err, firestorestore.ErrNotFound) {
		t.Fatalf("error = %v", err)
	}
}
