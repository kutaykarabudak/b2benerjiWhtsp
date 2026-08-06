package models

import "github.com/google/uuid"

// Catalog represents a WhatsApp product catalog
type Catalog struct {
	BaseModel
	OrganizationID  uuid.UUID `gorm:"type:uuid;index;not null" json:"organization_id"`
	WhatsAppAccount string    `gorm:"size:100;index" json:"whatsapp_account"` // Links to WhatsAppAccount.Name
	MetaCatalogID   string    `gorm:"size:100;uniqueIndex" json:"meta_catalog_id"`
	Name            string    `gorm:"size:255;not null" json:"name"`
	IsActive        bool      `gorm:"default:true" json:"is_active"`

	// Relations
	Organization *Organization    `gorm:"foreignKey:OrganizationID" json:"organization,omitempty"`
	Products     []CatalogProduct `gorm:"foreignKey:CatalogID" json:"products,omitempty"`
}

func (Catalog) TableName() string {
	return "catalogs"
}

// CatalogProduct represents a product in a catalog
type CatalogProduct struct {
	BaseModel
	OrganizationID uuid.UUID `gorm:"type:uuid;index;not null" json:"organization_id"`
	CatalogID      uuid.UUID `gorm:"type:uuid;index;not null" json:"catalog_id"`
	MetaProductID  string    `gorm:"size:100;uniqueIndex" json:"meta_product_id"`
	Name           string    `gorm:"size:255;not null" json:"name"`
	Description    string    `gorm:"type:text" json:"description"`
	Price          int64     `gorm:"not null" json:"price"` // Price in cents
	Currency       string    `gorm:"size:3;default:'USD'" json:"currency"`
	// Meta may return signed/CDN URLs well beyond 500 characters. Keep these as
	// text so a valid Commerce product is not dropped during synchronization.
	URL          string `gorm:"type:text" json:"url"`
	ImageURL     string `gorm:"type:text" json:"image_url"`
	RetailerID   string `gorm:"size:100" json:"retailer_id"` // SKU
	Availability string `gorm:"size:30;default:'in stock'" json:"availability"`
	Condition    string `gorm:"size:30;default:'new'" json:"condition"`
	IsActive     bool   `gorm:"default:true" json:"is_active"`

	// Relations
	Organization *Organization `gorm:"foreignKey:OrganizationID" json:"organization,omitempty"`
	Catalog      *Catalog      `gorm:"foreignKey:CatalogID" json:"catalog,omitempty"`
}

func (CatalogProduct) TableName() string {
	return "catalog_products"
}

// CatalogImage stores product images uploaded from the panel. Meta requires an
// HTTPS image_link even when the user uploads a file, so the public token is
// used to expose a stable, non-guessable read-only URL to Meta's crawler.
type CatalogImage struct {
	BaseModel
	OrganizationID uuid.UUID `gorm:"type:uuid;index;not null" json:"organization_id"`
	PublicToken    uuid.UUID `gorm:"type:uuid;uniqueIndex;not null" json:"-"`
	Filename       string    `gorm:"size:255" json:"filename"`
	MimeType       string    `gorm:"size:100;not null" json:"mime_type"`
	Data           []byte    `gorm:"type:bytea;not null" json:"-"`
}

func (CatalogImage) TableName() string {
	return "catalog_images"
}
