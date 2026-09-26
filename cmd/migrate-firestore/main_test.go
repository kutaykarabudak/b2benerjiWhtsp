package main

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/models"
	"gorm.io/gorm"
)

func TestMapContactPreservesDeletionAndConversationFields(t *testing.T) {
	now := time.Now().UTC()
	orgID := uuid.New()
	userID := uuid.New()
	input := models.Contact{
		BaseModel:      models.BaseModel{ID: uuid.New(), CreatedAt: now, UpdatedAt: now, DeletedAt: gorm.DeletedAt{Time: now, Valid: true}},
		OrganizationID: orgID, AssignedUserID: &userID, PhoneNumber: "+905550000000",
		ProfileName: "Test", PostalCode: "34000", HasPurchased: true, PurchaseScore: 9,
	}
	output := mapContact(input)
	if output.ID != input.ID.String() || output.OrganizationID != orgID.String() || output.AssignedUserID != userID.String() {
		t.Fatalf("identity fields were not preserved: %+v", output)
	}
	if !output.IsDeleted || output.DeletedAt == nil || output.PostalCode != "34000" || !output.HasPurchased {
		t.Fatalf("business fields were not preserved: %+v", output)
	}
}

func TestCollectTagsIncludesTagsEmbeddedInContacts(t *testing.T) {
	now := time.Now().UTC()
	orgID := uuid.New()
	tags := []models.Tag{{OrganizationID: orgID, Name: "vip", Color: "purple", CreatedAt: now, UpdatedAt: now}}
	contacts := []models.Contact{{
		BaseModel: models.BaseModel{ID: uuid.New(), CreatedAt: now, UpdatedAt: now}, OrganizationID: orgID,
		Tags: models.JSONBArray{"vip", "priority", "priority", 42},
	}}
	result := collectTags(tags, contacts)
	if len(result) != 2 || result[0].Name != "priority" || result[0].Color != "gray" || result[1].Name != "vip" || result[1].Color != "purple" {
		t.Fatalf("unexpected tags: %+v", result)
	}
}
