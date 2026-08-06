package handlers

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/shridarpatil/whatomate/pkg/whatsapp"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

const catalogPermissionError = "Meta erişim token'ında katalog yetkisi bulunmuyor. Seçili hesap için catalog_management ve business_management izinlerini içeren tokenı Yönetim > Kanallar bölümünde güncelleyin."

func catalogAPIErrorMessage(err error, fallback string) (int, string) {
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "missing permission") {
		return fasthttp.StatusForbidden, catalogPermissionError
	}
	if err != nil {
		return fasthttp.StatusBadGateway, fmt.Sprintf("%s: %s", fallback, err.Error())
	}
	return fasthttp.StatusBadGateway, fallback
}

// CatalogRequest represents the request body for creating a catalog
type CatalogRequest struct {
	WhatsAppAccount string `json:"whatsapp_account"`
	Name            string `json:"name"`
}

// CatalogResponse represents the API response for a catalog
type CatalogResponse struct {
	ID              uuid.UUID                `json:"id"`
	MetaCatalogID   string                   `json:"meta_catalog_id"`
	WhatsAppAccount string                   `json:"whatsapp_account"`
	Name            string                   `json:"name"`
	IsActive        bool                     `json:"is_active"`
	ProductCount    int                      `json:"product_count"`
	Products        []CatalogProductResponse `json:"products,omitempty"`
	CreatedAt       string                   `json:"created_at"`
	UpdatedAt       string                   `json:"updated_at"`
}

// CatalogProductRequest represents the request body for creating/updating a product
type CatalogProductRequest struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	Price        int64  `json:"price"` // Price in cents
	Currency     string `json:"currency"`
	URL          string `json:"url"`
	ImageURL     string `json:"image_url"`
	RetailerID   string `json:"retailer_id"` // SKU
	Availability string `json:"availability"`
	Condition    string `json:"condition"`
}

// CatalogProductResponse represents the API response for a product
type CatalogProductResponse struct {
	ID            uuid.UUID `json:"id"`
	MetaProductID string    `json:"meta_product_id"`
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	Price         int64     `json:"price"`
	Currency      string    `json:"currency"`
	URL           string    `json:"url"`
	ImageURL      string    `json:"image_url"`
	RetailerID    string    `json:"retailer_id"`
	Availability  string    `json:"availability"`
	Condition     string    `json:"condition"`
	IsActive      bool      `json:"is_active"`
	CreatedAt     string    `json:"created_at"`
	UpdatedAt     string    `json:"updated_at"`
}

// SyncCatalogsRequest represents the request body for syncing catalogs
type SyncCatalogsRequest struct {
	WhatsAppAccount string `json:"whatsapp_account"`
}

func parseMetaCatalogPrice(value string) int64 {
	value = strings.TrimSpace(value)
	if price, err := strconv.ParseInt(value, 10, 64); err == nil {
		return price
	}

	lastSeparator := strings.LastIndexAny(value, ".,")
	digitsAfterSeparator := 0
	if lastSeparator >= 0 {
		for _, char := range value[lastSeparator+1:] {
			if char >= '0' && char <= '9' {
				digitsAfterSeparator++
			}
		}
	}

	var digits strings.Builder
	for _, char := range value {
		if char >= '0' && char <= '9' {
			digits.WriteRune(char)
		}
	}
	if digits.Len() == 0 {
		return 0
	}
	price, err := strconv.ParseInt(digits.String(), 10, 64)
	if err != nil {
		return 0
	}
	// Graph may return localized prices such as "₺1,00" or "$1.00".
	// Their digit-only representation is already in minor currency units.
	if digitsAfterSeparator == 2 {
		return price
	}
	return price
}

func (a *App) syncCatalogProducts(ctx context.Context, orgID uuid.UUID, catalog *models.Catalog, waAccount *whatsapp.Account) (int, error) {
	metaProducts, err := a.WhatsApp.ListCatalogProducts(ctx, waAccount, catalog.MetaCatalogID)
	if err != nil {
		return 0, err
	}

	synced := 0
	for _, item := range metaProducts {
		price := parseMetaCatalogPrice(item.Price)
		isActive := !strings.EqualFold(item.Visibility, "hidden") && !strings.EqualFold(item.Status, "hidden")
		product := models.CatalogProduct{
			OrganizationID: orgID,
			CatalogID:      catalog.ID,
			MetaProductID:  item.ID,
			Name:           item.Name,
			Description:    item.Description,
			Price:          price,
			Currency:       item.Currency,
			URL:            item.URL,
			ImageURL:       item.ImageURL,
			RetailerID:     item.RetailerID,
			Availability:   item.Availability,
			Condition:      item.Condition,
			IsActive:       isActive,
		}

		var existing models.CatalogProduct
		findExisting := a.DB.Unscoped().
			Where("organization_id = ?", orgID).
			Where("meta_product_id = ? OR (catalog_id = ? AND retailer_id = ?)", item.ID, catalog.ID, item.RetailerID).
			First(&existing).Error
		if findExisting == nil {
			product.ID = existing.ID
			product.CreatedAt = existing.CreatedAt
			if err := a.DB.Unscoped().Save(&product).Error; err != nil {
				a.Log.Error("Failed to update synced product", "error", err, "meta_id", item.ID)
				continue
			}
		} else if err := a.DB.Create(&product).Error; err != nil {
			a.Log.Error("Failed to create synced product", "error", err, "meta_id", item.ID)
			continue
		}
		synced++
	}

	return synced, nil
}

// applyCatalogBusinessID switches only this WhatsApp account to its owning Meta
// Business Portfolio ID. WABA IDs and Business Portfolio IDs are different
// identifiers and accounts in the same panel may belong to different businesses.
func applyCatalogBusinessID(account *models.WhatsAppAccount, waAccount *whatsapp.Account) bool {
	if account.CatalogBusinessID == "" {
		return false
	}
	waAccount.BusinessID = account.CatalogBusinessID
	return true
}

func (a *App) activateCatalogForWhatsApp(ctx context.Context, account *models.WhatsAppAccount, catalogID string) error {
	waAccount := a.toWhatsAppAccount(account)
	// product_catalogs is a WABA edge, so keep the account's WABA BusinessID
	// here rather than replacing it with the catalog owner's portfolio ID.
	connected, err := a.WhatsApp.IsCatalogConnected(ctx, waAccount, catalogID)
	if err != nil {
		return fmt.Errorf("katalog bağlantısı doğrulanamadı: %w", err)
	}
	// Connecting an already-attached catalog again makes Meta return error 100
	// ("a catalog can only be connected to one WABA"), even when that WABA is
	// this same account. In that case skip directly to commerce visibility.
	if !connected {
		if err := a.WhatsApp.ConnectCatalog(ctx, waAccount, catalogID); err != nil {
			return err
		}
		connected, err = a.WhatsApp.IsCatalogConnected(ctx, waAccount, catalogID)
		if err != nil {
			return fmt.Errorf("katalog bağlantısı doğrulanamadı: %w", err)
		}
	}
	if !connected {
		return fmt.Errorf("Meta işlemi kabul etti ancak katalog WABA hesabına bağlanmadı; WABA ID ve katalog sahipliğini kontrol edin")
	}
	if err := a.WhatsApp.EnableCommerceSettings(ctx, waAccount); err != nil {
		return err
	}
	visible, cartEnabled, err := a.WhatsApp.GetCommerceSettings(ctx, waAccount)
	if err != nil {
		return fmt.Errorf("commerce ayarları doğrulanamadı: %w", err)
	}
	if !visible || !cartEnabled {
		return fmt.Errorf("Meta commerce ayarlarını etkinleştirmedi (catalog_visible=%t, cart_enabled=%t)", visible, cartEnabled)
	}
	return nil
}

// refreshCatalogAfterProductMutation reasserts the WABA/catalog connection and
// phone-level commerce visibility after Meta accepts a product mutation. Meta
// processes catalog items asynchronously, so this is deliberately best-effort:
// the product operation must not be reported as failed after it already
// succeeded.
func (a *App) refreshCatalogAfterProductMutation(ctx context.Context, account *models.WhatsAppAccount, catalogID string) {
	if err := a.activateCatalogForWhatsApp(ctx, account, catalogID); err != nil {
		a.Log.Warn("Product saved but WhatsApp catalog visibility could not be refreshed",
			"error", err, "catalog_id", catalogID, "phone_id", account.PhoneID)
	}
}

// ListCatalogs returns all catalogs for the organization
func (a *App) ListCatalogs(r *fastglue.Request) error {
	orgID, err := a.getOrgID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}

	whatsAppAccount := string(r.RequestCtx.QueryArgs().Peek("whatsapp_account"))

	query := a.DB.Where("organization_id = ?", orgID)
	if whatsAppAccount != "" {
		query = query.Where("whats_app_account = ?", whatsAppAccount)
	}

	var catalogs []models.Catalog
	if err := query.Order("name ASC").Find(&catalogs).Error; err != nil {
		a.Log.Error("Failed to list catalogs", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to list catalogs", nil, "")
	}

	result := make([]CatalogResponse, len(catalogs))
	for i, c := range catalogs {
		// Get product count
		var productCount int64
		a.DB.Model(&models.CatalogProduct{}).Where("catalog_id = ?", c.ID).Count(&productCount)
		result[i] = catalogToResponse(c, int(productCount))
	}

	return r.SendEnvelope(map[string]any{
		"catalogs": result,
	})
}

// CreateCatalog creates a new catalog in Meta and stores it locally
func (a *App) CreateCatalog(r *fastglue.Request) error {
	orgID, err := a.getOrgID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}

	var req CatalogRequest
	if err := a.decodeRequest(r, &req); err != nil {
		return nil
	}

	if req.Name == "" || req.WhatsAppAccount == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "name and whatsapp_account are required", nil, "")
	}

	// Get WhatsApp account
	account, err := a.resolveWhatsAppAccount(orgID, req.WhatsAppAccount)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, "WhatsApp account not found", nil, "")
	}

	// Create catalog in Meta
	ctx := context.Background()
	waAccount := a.toWhatsAppAccount(account)
	if !applyCatalogBusinessID(account, waAccount) {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Bu hesap için Katalog Business Portfolio ID girilmemiş", nil, "")
	}

	metaCatalogID, err := a.WhatsApp.CreateCatalog(ctx, waAccount, req.Name)
	if err != nil {
		a.Log.Error("Failed to create catalog in Meta", "error", err)
		status, message := catalogAPIErrorMessage(err, "Meta üzerinde katalog oluşturulamadı")
		return r.SendErrorEnvelope(status, message, nil, "")
	}

	// Store catalog locally
	catalog := models.Catalog{
		OrganizationID:  orgID,
		WhatsAppAccount: req.WhatsAppAccount,
		MetaCatalogID:   metaCatalogID,
		Name:            req.Name,
		IsActive:        true,
	}

	if err := a.DB.Create(&catalog).Error; err != nil {
		a.Log.Error("Failed to save catalog", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to save catalog", nil, "")
	}

	if err := a.activateCatalogForWhatsApp(ctx, account, metaCatalogID); err != nil {
		a.Log.Error("Catalog created but could not be connected to WhatsApp", "error", err, "catalog_id", metaCatalogID)
		status, message := catalogAPIErrorMessage(err, "Katalog oluşturuldu ancak WhatsApp profiline bağlanamadı; katalog listesinden “WhatsApp’ta göster” ile tekrar deneyin")
		return r.SendErrorEnvelope(status, message, nil, "")
	}

	return r.SendEnvelope(catalogToResponse(catalog, 0))
}

// ActivateCatalog connects an existing catalog to its WABA and turns on its
// profile/catalog commerce flags.
func (a *App) ActivateCatalog(r *fastglue.Request) error {
	orgID, err := a.getOrgID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}

	id, err := parsePathUUID(r, "id", "catalog")
	if err != nil {
		return nil
	}
	catalog, err := findByIDAndOrg[models.Catalog](a.DB, r, id, orgID, "Catalog")
	if err != nil {
		return nil
	}
	account, err := a.resolveWhatsAppAccount(orgID, catalog.WhatsAppAccount)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, "WhatsApp account not found", nil, "")
	}

	if err := a.activateCatalogForWhatsApp(context.Background(), account, catalog.MetaCatalogID); err != nil {
		a.Log.Error("Failed to activate catalog for WhatsApp", "error", err, "catalog_id", catalog.MetaCatalogID)
		status, message := catalogAPIErrorMessage(err, "Katalog WhatsApp profilinde etkinleştirilemedi")
		return r.SendErrorEnvelope(status, message, nil, "")
	}

	return r.SendEnvelope(map[string]string{"message": "Katalog WhatsApp profilinde etkinleştirildi"})
}

// GetCatalog returns a single catalog with its products
func (a *App) GetCatalog(r *fastglue.Request) error {
	orgID, err := a.getOrgID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}

	id, err := parsePathUUID(r, "id", "catalog")
	if err != nil {
		return nil
	}

	var catalog models.Catalog
	if err := a.DB.Where("id = ? AND organization_id = ?", id, orgID).
		Preload("Products").First(&catalog).Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, "Catalog not found", nil, "")
	}

	resp := catalogToResponse(catalog, len(catalog.Products))
	resp.Products = make([]CatalogProductResponse, len(catalog.Products))
	for i, p := range catalog.Products {
		resp.Products[i] = productToResponse(p)
	}

	return r.SendEnvelope(resp)
}

// DeleteCatalog deletes a catalog from Meta and locally
func (a *App) DeleteCatalog(r *fastglue.Request) error {
	orgID, err := a.getOrgID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}

	id, err := parsePathUUID(r, "id", "catalog")
	if err != nil {
		return nil
	}

	catalog, err := findByIDAndOrg[models.Catalog](a.DB, r, id, orgID, "Catalog")
	if err != nil {
		return nil
	}

	// Get WhatsApp account. A catalog can outlive a deleted channel because the
	// legacy relation is the account name, not an account foreign key.
	account, err := a.resolveWhatsAppAccount(orgID, catalog.WhatsAppAccount)
	if err == nil {
		ctx := context.Background()
		waAccount := a.toWhatsAppAccount(account)
		applyCatalogBusinessID(account, waAccount)
		if err := a.WhatsApp.DeleteCatalog(ctx, waAccount, catalog.MetaCatalogID); err != nil {
			a.Log.Error("Failed to delete catalog from Meta", "error", err)
			status, message := catalogAPIErrorMessage(err, "Katalog Meta’dan silinemedi")
			return r.SendErrorEnvelope(status, message, nil, "")
		}
	} else {
		a.Log.Warn("Deleting orphaned local catalog whose WhatsApp account no longer exists",
			"catalog_id", catalog.ID, "whatsapp_account", catalog.WhatsAppAccount)
	}

	tx := a.DB.Begin()
	if err := tx.Unscoped().Where("catalog_id = ?", id).Delete(&models.CatalogProduct{}).Error; err != nil {
		tx.Rollback()
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to delete catalog products", nil, "")
	}
	if err := tx.Unscoped().Delete(catalog).Error; err != nil {
		tx.Rollback()
		a.Log.Error("Failed to delete catalog", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to delete catalog", nil, "")
	}
	if err := tx.Commit().Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to commit catalog deletion", nil, "")
	}

	return r.SendEnvelope(map[string]string{"message": "Catalog deleted"})
}

// SyncCatalogs syncs catalogs from Meta API
func (a *App) SyncCatalogs(r *fastglue.Request) error {
	orgID, err := a.getOrgID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}

	var req SyncCatalogsRequest
	if err := a.decodeRequest(r, &req); err != nil {
		return nil
	}

	if req.WhatsAppAccount == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "whatsapp_account is required", nil, "")
	}

	// Get WhatsApp account
	account, err := a.resolveWhatsAppAccount(orgID, req.WhatsAppAccount)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, "WhatsApp account not found", nil, "")
	}

	// Fetch catalogs from Meta
	ctx := context.Background()
	waAccount := a.toWhatsAppAccount(account)
	if !applyCatalogBusinessID(account, waAccount) {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Bu hesap için Katalog Business Portfolio ID girilmemiş", nil, "")
	}

	metaCatalogs, err := a.WhatsApp.ListCatalogs(ctx, waAccount)
	if err != nil {
		a.Log.Error("Failed to fetch catalogs from Meta", "error", err)
		status, message := catalogAPIErrorMessage(err, "Meta katalogları alınamadı")
		return r.SendErrorEnvelope(status, message, nil, "")
	}

	// Sync each catalog
	synced := 0
	productsSynced := 0
	for _, mc := range metaCatalogs {
		var existing models.Catalog
		err := a.DB.Unscoped().Where("organization_id = ? AND meta_catalog_id = ?", orgID, mc.ID).First(&existing).Error
		if err != nil {
			// Create new catalog
			catalog := models.Catalog{
				OrganizationID:  orgID,
				WhatsAppAccount: req.WhatsAppAccount,
				MetaCatalogID:   mc.ID,
				Name:            mc.Name,
				IsActive:        true,
			}
			if err := a.DB.Create(&catalog).Error; err != nil {
				a.Log.Error("Failed to create synced catalog", "error", err, "meta_id", mc.ID)
				continue
			}
			existing = catalog
			synced++
		} else {
			// Restore soft-deleted records and move catalogs away from channel
			// names that no longer exist.
			existing.Name = mc.Name
			existing.WhatsAppAccount = req.WhatsAppAccount
			existing.DeletedAt.Valid = false
			if err := a.DB.Unscoped().Save(&existing).Error; err != nil {
				a.Log.Error("Failed to update synced catalog", "error", err, "meta_id", mc.ID)
				continue
			}
			synced++
		}

		count, err := a.syncCatalogProducts(ctx, orgID, &existing, waAccount)
		if err != nil {
			a.Log.Error("Failed to sync catalog products from Meta", "error", err, "catalog_id", mc.ID)
			status, message := catalogAPIErrorMessage(err, "Meta kataloğundaki ürünler alınamadı")
			return r.SendErrorEnvelope(status, message, nil, "")
		}
		productsSynced += count
	}

	return r.SendEnvelope(map[string]any{
		"message":         "Catalogs synced",
		"synced":          synced,
		"total":           len(metaCatalogs),
		"products_synced": productsSynced,
	})
}

// ListCatalogProducts returns all products in a catalog
func (a *App) ListCatalogProducts(r *fastglue.Request) error {
	orgID, err := a.getOrgID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}

	catalogID, err := parsePathUUID(r, "id", "catalog")
	if err != nil {
		return nil
	}

	// Verify catalog belongs to org
	catalog, err := findByIDAndOrg[models.Catalog](a.DB, r, catalogID, orgID, "Catalog")
	if err != nil {
		return nil
	}
	_ = catalog

	var products []models.CatalogProduct
	if err := a.DB.Where("catalog_id = ?", catalogID).Order("name ASC").Find(&products).Error; err != nil {
		a.Log.Error("Failed to list products", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to list products", nil, "")
	}

	result := make([]CatalogProductResponse, len(products))
	for i, p := range products {
		result[i] = productToResponse(p)
	}

	return r.SendEnvelope(map[string]any{
		"products": result,
	})
}

// CreateCatalogProduct creates a new product in a catalog
func (a *App) CreateCatalogProduct(r *fastglue.Request) error {
	orgID, err := a.getOrgID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}

	catalogID, err := parsePathUUID(r, "id", "catalog")
	if err != nil {
		return nil
	}

	var req CatalogProductRequest
	if err := a.decodeRequest(r, &req); err != nil {
		return nil
	}

	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.RetailerID) == "" || strings.TrimSpace(req.ImageURL) == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Ürün adı, SKU ve yüklenmiş ürün görseli zorunludur", nil, "")
	}
	if req.Price < 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Ürün fiyatı negatif olamaz", nil, "")
	}

	// Get catalog and verify ownership
	catalog, err := findByIDAndOrg[models.Catalog](a.DB, r, catalogID, orgID, "Catalog")
	if err != nil {
		return nil
	}

	// Get WhatsApp account
	account, err := a.resolveWhatsAppAccount(orgID, catalog.WhatsAppAccount)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, "WhatsApp account not found", nil, "")
	}

	// Set defaults
	if req.Currency == "" {
		req.Currency = "USD"
	}
	if req.Availability == "" {
		req.Availability = "in stock"
	}
	if req.Condition == "" {
		req.Condition = "new"
	}

	// Create product in Meta
	ctx := context.Background()
	waAccount := a.toWhatsAppAccount(account)
	applyCatalogBusinessID(account, waAccount)

	productInput := &whatsapp.ProductInput{
		Name:         req.Name,
		Price:        req.Price,
		Currency:     req.Currency,
		URL:          req.URL,
		ImageURL:     req.ImageURL,
		RetailerID:   req.RetailerID,
		Description:  req.Description,
		Availability: req.Availability,
		Condition:    req.Condition,
	}

	metaProductID, err := a.WhatsApp.CreateProduct(ctx, waAccount, catalog.MetaCatalogID, productInput)
	if err != nil {
		a.Log.Error("Failed to create product in Meta", "error", err)
		status, message := catalogAPIErrorMessage(err, "Ürün Meta kataloğunda oluşturulamadı")
		return r.SendErrorEnvelope(status, message, nil, "")
	}

	// Store product locally
	product := models.CatalogProduct{
		OrganizationID: orgID,
		CatalogID:      catalogID,
		MetaProductID:  metaProductID,
		Name:           req.Name,
		Description:    req.Description,
		Price:          req.Price,
		Currency:       req.Currency,
		URL:            req.URL,
		ImageURL:       req.ImageURL,
		RetailerID:     req.RetailerID,
		Availability:   req.Availability,
		Condition:      req.Condition,
		IsActive:       true,
	}

	if err := a.DB.Create(&product).Error; err != nil {
		a.Log.Error("Failed to save product", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to save product", nil, "")
	}
	a.refreshCatalogAfterProductMutation(ctx, account, catalog.MetaCatalogID)

	return r.SendEnvelope(productToResponse(product))
}

// GetCatalogProduct returns a single product
func (a *App) GetCatalogProduct(r *fastglue.Request) error {
	orgID, err := a.getOrgID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}

	id, err := parsePathUUID(r, "id", "product")
	if err != nil {
		return nil
	}

	product, err := findByIDAndOrg[models.CatalogProduct](a.DB, r, id, orgID, "Product")
	if err != nil {
		return nil
	}

	return r.SendEnvelope(productToResponse(*product))
}

// UpdateCatalogProduct updates a product
func (a *App) UpdateCatalogProduct(r *fastglue.Request) error {
	orgID, err := a.getOrgID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}

	id, err := parsePathUUID(r, "id", "product")
	if err != nil {
		return nil
	}

	product, err := findByIDAndOrg[models.CatalogProduct](a.DB, r, id, orgID, "Product")
	if err != nil {
		return nil
	}

	var req CatalogProductRequest
	if err := a.decodeRequest(r, &req); err != nil {
		return nil
	}
	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.RetailerID) == "" || strings.TrimSpace(req.ImageURL) == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Ürün adı, SKU ve yüklenmiş ürün görseli zorunludur", nil, "")
	}
	if req.Price < 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Ürün fiyatı negatif olamaz", nil, "")
	}
	if req.Currency == "" {
		req.Currency = "TRY"
	}
	if req.Availability == "" {
		req.Availability = "in stock"
	}
	if req.Condition == "" {
		req.Condition = "new"
	}
	if req.RetailerID != product.RetailerID {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Stok kodu mevcut bir üründe değiştirilemez; yeni stok kodu için yeni ürün oluşturun", nil, "")
	}

	// Get catalog to get WhatsApp account
	var catalog models.Catalog
	if err := a.DB.Where("id = ?", product.CatalogID).First(&catalog).Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, "Catalog not found", nil, "")
	}

	// Get WhatsApp account
	account, err := a.resolveWhatsAppAccount(orgID, catalog.WhatsAppAccount)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, "WhatsApp account not found", nil, "")
	}

	// Update product in Meta
	ctx := context.Background()
	waAccount := a.toWhatsAppAccount(account)
	applyCatalogBusinessID(account, waAccount)

	productInput := &whatsapp.ProductInput{
		Name:         req.Name,
		Price:        req.Price,
		Currency:     req.Currency,
		URL:          req.URL,
		ImageURL:     req.ImageURL,
		RetailerID:   req.RetailerID,
		Description:  req.Description,
		Availability: req.Availability,
		Condition:    req.Condition,
	}

	if err := a.WhatsApp.UpdateProduct(ctx, waAccount, catalog.MetaCatalogID, product.RetailerID, productInput); err != nil {
		a.Log.Error("Failed to update product in Meta", "error", err)
		status, message := catalogAPIErrorMessage(err, "Ürün Meta kataloğunda güncellenemedi")
		return r.SendErrorEnvelope(status, message, nil, "")
	}

	// Update locally
	product.Name = req.Name
	product.Description = req.Description
	product.Price = req.Price
	product.Currency = req.Currency
	product.URL = req.URL
	product.ImageURL = req.ImageURL
	product.RetailerID = req.RetailerID
	product.Availability = req.Availability
	product.Condition = req.Condition

	if err := a.DB.Save(product).Error; err != nil {
		a.Log.Error("Failed to save product", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to save product", nil, "")
	}
	a.refreshCatalogAfterProductMutation(ctx, account, catalog.MetaCatalogID)

	return r.SendEnvelope(productToResponse(*product))
}

// DeleteCatalogProduct deletes a product
func (a *App) DeleteCatalogProduct(r *fastglue.Request) error {
	orgID, err := a.getOrgID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}

	id, err := parsePathUUID(r, "id", "product")
	if err != nil {
		return nil
	}

	product, err := findByIDAndOrg[models.CatalogProduct](a.DB, r, id, orgID, "Product")
	if err != nil {
		return nil
	}

	// Get catalog to get WhatsApp account
	var catalog models.Catalog
	if err := a.DB.Where("id = ?", product.CatalogID).First(&catalog).Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, "Catalog not found", nil, "")
	}

	// Get WhatsApp account
	account, err := a.resolveWhatsAppAccount(orgID, catalog.WhatsAppAccount)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, "WhatsApp account not found", nil, "")
	}

	// Delete from Meta
	ctx := context.Background()
	waAccount := a.toWhatsAppAccount(account)
	applyCatalogBusinessID(account, waAccount)

	if err := a.WhatsApp.DeleteProduct(ctx, waAccount, catalog.MetaCatalogID, product.RetailerID); err != nil {
		a.Log.Error("Failed to delete product from Meta", "error", err)
		status, message := catalogAPIErrorMessage(err, "Ürün Meta kataloğundan silinemedi")
		return r.SendErrorEnvelope(status, message, nil, "")
	}

	if err := a.DB.Unscoped().Delete(product).Error; err != nil {
		a.Log.Error("Failed to delete product", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to delete product", nil, "")
	}

	return r.SendEnvelope(map[string]string{"message": "Product deleted"})
}

// Helper functions

func catalogToResponse(c models.Catalog, productCount int) CatalogResponse {
	return CatalogResponse{
		ID:              c.ID,
		MetaCatalogID:   c.MetaCatalogID,
		WhatsAppAccount: c.WhatsAppAccount,
		Name:            c.Name,
		IsActive:        c.IsActive,
		ProductCount:    productCount,
		CreatedAt:       c.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:       c.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

func productToResponse(p models.CatalogProduct) CatalogProductResponse {
	return CatalogProductResponse{
		ID:            p.ID,
		MetaProductID: p.MetaProductID,
		Name:          p.Name,
		Description:   p.Description,
		Price:         p.Price,
		Currency:      p.Currency,
		URL:           p.URL,
		ImageURL:      p.ImageURL,
		RetailerID:    p.RetailerID,
		Availability:  p.Availability,
		Condition:     p.Condition,
		IsActive:      p.IsActive,
		CreatedAt:     p.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:     p.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}
