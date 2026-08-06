package whatsapp

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// buildCatalogsURL builds the catalogs endpoint URL for a business
func (c *Client) buildCatalogsURL(account *Account) string {
	return fmt.Sprintf("%s/%s/%s/owned_product_catalogs", c.getBaseURL(), account.APIVersion, account.BusinessID)
}

// buildCatalogProductsURL builds the products endpoint URL for a catalog
func (c *Client) buildCatalogProductsURL(account *Account, catalogID string) string {
	return fmt.Sprintf("%s/%s/%s/products", c.getBaseURL(), account.APIVersion, catalogID)
}

// buildProductURL builds the URL for a specific product
func (c *Client) buildProductURL(account *Account, productID string) string {
	return fmt.Sprintf("%s/%s/%s", c.getBaseURL(), account.APIVersion, productID)
}

// CreateCatalog creates a new product catalog
func (c *Client) CreateCatalog(ctx context.Context, account *Account, name string) (string, error) {
	apiURL := c.buildCatalogsURL(account)

	body := map[string]string{
		"name": name,
	}

	respBody, err := c.doRequest(ctx, http.MethodPost, apiURL, body, account.AccessToken)
	if err != nil {
		return "", err
	}

	var resp struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	return resp.ID, nil
}

// ListCatalogs lists all catalogs for a business
func (c *Client) ListCatalogs(ctx context.Context, account *Account) ([]CatalogInfo, error) {
	apiURL := c.buildCatalogsURL(account)

	resp, err := doJSON[CatalogListResponse](ctx, c, http.MethodGet, apiURL, nil, account.AccessToken)
	if err != nil {
		return nil, err
	}

	return resp.Data, nil
}

// DeleteCatalog deletes a catalog
func (c *Client) DeleteCatalog(ctx context.Context, account *Account, catalogID string) error {
	apiURL := fmt.Sprintf("%s/%s/%s", c.getBaseURL(), account.APIVersion, catalogID)

	_, err := c.doRequest(ctx, http.MethodDelete, apiURL, nil, account.AccessToken)
	return err
}

// ConnectCatalog associates a Commerce catalog with the WhatsApp Business
// Account. Creating a catalog under the same portfolio does not do this
// automatically.
func (c *Client) ConnectCatalog(ctx context.Context, account *Account, catalogID string) error {
	apiURL := fmt.Sprintf("%s/%s/%s/product_catalogs", c.getBaseURL(), account.APIVersion, account.BusinessID)
	_, err := c.doRequest(ctx, http.MethodPost, apiURL, map[string]string{
		"catalog_id": catalogID,
	}, account.AccessToken)
	return err
}

// IsCatalogConnected reports whether catalogID is attached to the WABA.
func (c *Client) IsCatalogConnected(ctx context.Context, account *Account, catalogID string) (bool, error) {
	apiURL := fmt.Sprintf("%s/%s/%s/product_catalogs", c.getBaseURL(), account.APIVersion, account.BusinessID)
	resp, err := doJSON[CatalogListResponse](ctx, c, http.MethodGet, apiURL, nil, account.AccessToken)
	if err != nil {
		return false, err
	}
	for _, catalog := range resp.Data {
		if catalog.ID == catalogID {
			return true, nil
		}
	}
	return false, nil
}

// EnableCommerceSettings makes the connected catalog visible from the
// WhatsApp business profile and enables the cart for this phone number.
func (c *Client) EnableCommerceSettings(ctx context.Context, account *Account) error {
	apiURL := fmt.Sprintf("%s/%s/%s/whatsapp_commerce_settings", c.getBaseURL(), account.APIVersion, account.PhoneID)
	// Meta documents these values as query parameters, not a JSON request body.
	params := url.Values{}
	params.Set("is_catalog_visible", "true")
	params.Set("is_cart_enabled", "true")
	apiURL += "?" + params.Encode()
	_, err := c.doRequest(ctx, http.MethodPost, apiURL, nil, account.AccessToken)
	return err
}

// GetCommerceSettings returns the effective commerce flags for a phone number.
func (c *Client) GetCommerceSettings(ctx context.Context, account *Account) (bool, bool, error) {
	apiURL := fmt.Sprintf("%s/%s/%s/whatsapp_commerce_settings", c.getBaseURL(), account.APIVersion, account.PhoneID)
	var resp struct {
		Data []struct {
			IsCatalogVisible bool `json:"is_catalog_visible"`
			IsCartEnabled    bool `json:"is_cart_enabled"`
		} `json:"data"`
	}
	var err error
	resp, err = doJSON[struct {
		Data []struct {
			IsCatalogVisible bool `json:"is_catalog_visible"`
			IsCartEnabled    bool `json:"is_cart_enabled"`
		} `json:"data"`
	}](ctx, c, http.MethodGet, apiURL, nil, account.AccessToken)
	if err != nil {
		return false, false, err
	}
	if len(resp.Data) == 0 {
		return false, false, fmt.Errorf("Meta returned no commerce settings for phone ID %s", account.PhoneID)
	}
	return resp.Data[0].IsCatalogVisible, resp.Data[0].IsCartEnabled, nil
}

// ListCatalogProducts lists all products in a catalog
func (c *Client) ListCatalogProducts(ctx context.Context, account *Account, catalogID string) ([]ProductInfo, error) {
	apiURL := c.buildCatalogProductsURL(account, catalogID)

	// Add fields parameter to get all product details
	params := url.Values{}
	params.Add("fields", "id,name,price,currency,url,image_url,retailer_id,description,availability,condition,visibility,status")
	apiURL = apiURL + "?" + params.Encode()

	resp, err := doJSON[ProductListResponse](ctx, c, http.MethodGet, apiURL, nil, account.AccessToken)
	if err != nil {
		return nil, err
	}

	return resp.Data, nil
}

type catalogBatchRequest struct {
	Method string            `json:"method"`
	Data   map[string]string `json:"data"`
}

type catalogBatchResponse struct {
	Handles          []string `json:"handles"`
	ValidationStatus []struct {
		RetailerID string `json:"retailer_id"`
		Errors     []struct {
			Message string `json:"message"`
		} `json:"errors"`
	} `json:"validation_status"`
}

func catalogBatchProductData(product *ProductInput) map[string]string {
	availability := product.Availability
	if availability == "" {
		availability = "in stock"
	}
	condition := product.Condition
	if condition == "" {
		condition = "new"
	}
	data := map[string]string{
		"id":           product.RetailerID,
		"title":        product.Name,
		"description":  product.Description,
		"link":         product.URL,
		"image_link":   product.ImageURL,
		"availability": availability,
		"condition":    condition,
		// Without this field batch-created items may land in Commerce Manager
		// as hidden. Panel-managed products should be visible immediately.
		"visibility": "published",
	}
	// The current PRODUCT_ITEM batch API accepts price-less WhatsApp catalog
	// items. Omit price entirely when the user leaves it blank; sending "0.00"
	// would incorrectly advertise the product as free.
	if product.Price > 0 {
		currency := strings.ToUpper(product.Currency)
		if currency == "" {
			currency = "USD"
		}
		data["price"] = fmt.Sprintf("%d.%02d %s", product.Price/100, product.Price%100, currency)
	}
	// Empty optional values should be absent rather than blank feed fields.
	if data["description"] == "" {
		delete(data, "description")
	}
	if data["link"] == "" {
		delete(data, "link")
	}
	return data
}

func (c *Client) mutateCatalogProduct(ctx context.Context, account *Account, catalogID, method string, data map[string]string) error {
	requestsJSON, err := json.Marshal([]catalogBatchRequest{{Method: method, Data: data}})
	if err != nil {
		return fmt.Errorf("failed to marshal catalog batch request: %w", err)
	}

	form := url.Values{}
	// Meta's current /products edge rejects catalogs containing batch-managed
	// items. The supported replacement for commerce catalogs is items_batch
	// with PRODUCT_ITEM and the retailer SKU in the data.id field.
	form.Set("item_type", "PRODUCT_ITEM")
	form.Set("requests", string(requestsJSON))

	apiURL := fmt.Sprintf("%s/%s/%s/items_batch", c.getBaseURL(), account.APIVersion, catalogID)
	respBody, err := c.doFormRequest(ctx, http.MethodPost, apiURL, form, account.AccessToken)
	if err != nil {
		return err
	}

	var resp catalogBatchResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return fmt.Errorf("failed to parse catalog batch response: %w", err)
	}
	var validationErrors []string
	for _, status := range resp.ValidationStatus {
		for _, validationErr := range status.Errors {
			validationErrors = append(validationErrors, validationErr.Message)
		}
	}
	if len(validationErrors) > 0 {
		return fmt.Errorf("catalog batch validation failed: %s", strings.Join(validationErrors, "; "))
	}
	if len(resp.Handles) == 0 {
		return fmt.Errorf("Meta accepted no catalog item operations")
	}
	return nil
}

// CreateProduct adds a product through Meta's current catalog batch API.
func (c *Client) CreateProduct(ctx context.Context, account *Account, catalogID string, product *ProductInput) (string, error) {
	if err := c.mutateCatalogProduct(ctx, account, catalogID, "CREATE", catalogBatchProductData(product)); err != nil {
		return "", err
	}

	// items_batch is asynchronous, but Meta documents read-after-write support.
	// Prefer the real Graph item ID when it is already visible.
	if products, err := c.ListCatalogProducts(ctx, account, catalogID); err == nil {
		for _, item := range products {
			if item.RetailerID == product.RetailerID {
				return item.ID, nil
			}
		}
	}

	// Keep a stable, short local placeholder until the next Meta sync replaces
	// it with the Graph item ID.
	sum := sha256.Sum256([]byte(catalogID + ":" + product.RetailerID))
	return fmt.Sprintf("batch:%x", sum), nil
}

// UpdateProduct updates a product by its immutable retailer SKU.
func (c *Client) UpdateProduct(ctx context.Context, account *Account, catalogID, retailerID string, product *ProductInput) error {
	data := catalogBatchProductData(product)
	data["id"] = retailerID
	return c.mutateCatalogProduct(ctx, account, catalogID, "UPDATE", data)
}

// DeleteProduct deletes a product by its retailer SKU.
func (c *Client) DeleteProduct(ctx context.Context, account *Account, catalogID, retailerID string) error {
	return c.mutateCatalogProduct(ctx, account, catalogID, "DELETE", map[string]string{"id": retailerID})
}
