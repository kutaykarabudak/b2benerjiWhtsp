package whatsapp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shridarpatil/whatomate/pkg/whatsapp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- CreateCatalog ---

func TestClient_CreateCatalog_Success(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Contains(t, r.URL.Path, "/owned_product_catalogs")

		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		assert.Equal(t, "My Catalog", body["name"])

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "catalog-123"})
	}))
	defer server.Close()

	client := newTestClient(t, server)
	account := testAccount(server.URL)

	id, err := client.CreateCatalog(context.Background(), account, "My Catalog")
	require.NoError(t, err)
	assert.Equal(t, "catalog-123", id)
}

func TestClient_CreateCatalog_APIError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(whatsapp.MetaAPIError{
			Error: struct {
				Message      string `json:"message"`
				Type         string `json:"type"`
				Code         int    `json:"code"`
				ErrorSubcode int    `json:"error_subcode"`
				ErrorUserMsg string `json:"error_user_msg"`
				ErrorData    struct {
					Details string `json:"details"`
				} `json:"error_data"`
				FBTraceID string `json:"fbtrace_id"`
			}{
				Message: "Catalog limit reached",
				Code:    100,
			},
		})
	}))
	defer server.Close()

	client := newTestClient(t, server)
	account := testAccount(server.URL)

	_, err := client.CreateCatalog(context.Background(), account, "Too Many")
	require.Error(t, err)
}

// --- ListCatalogs ---

func TestClient_ListCatalogs_Success(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]string{
				{"id": "cat-1", "name": "Catalog 1"},
				{"id": "cat-2", "name": "Catalog 2"},
			},
		})
	}))
	defer server.Close()

	client := newTestClient(t, server)
	account := testAccount(server.URL)

	catalogs, err := client.ListCatalogs(context.Background(), account)
	require.NoError(t, err)
	require.Len(t, catalogs, 2)
	assert.Equal(t, "cat-1", catalogs[0].ID)
	assert.Equal(t, "Catalog 1", catalogs[0].Name)
}

func TestClient_ListCatalogs_Empty(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{}})
	}))
	defer server.Close()

	client := newTestClient(t, server)
	account := testAccount(server.URL)

	catalogs, err := client.ListCatalogs(context.Background(), account)
	require.NoError(t, err)
	assert.Empty(t, catalogs)
}

// --- DeleteCatalog ---

func TestClient_DeleteCatalog_Success(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
	}))
	defer server.Close()

	client := newTestClient(t, server)
	account := testAccount(server.URL)

	err := client.DeleteCatalog(context.Background(), account, "catalog-123")
	require.NoError(t, err)
}

// --- ListCatalogProducts ---

func TestClient_ListCatalogProducts_Success(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Contains(t, r.URL.Path, "/products")
		assert.Contains(t, r.URL.Query().Get("fields"), "visibility")
		assert.Contains(t, r.URL.Query().Get("fields"), "status")

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "prod-1", "name": "Product 1", "price": "1000", "currency": "USD", "visibility": "published", "status": "PUBLISHED"},
				{"id": "prod-2", "name": "Product 2", "price": "2000", "currency": "USD"},
			},
		})
	}))
	defer server.Close()

	client := newTestClient(t, server)
	account := testAccount(server.URL)

	products, err := client.ListCatalogProducts(context.Background(), account, "catalog-123")
	require.NoError(t, err)
	require.Len(t, products, 2)
	assert.Equal(t, "prod-1", products[0].ID)
	assert.Equal(t, "Product 1", products[0].Name)
	assert.Equal(t, "published", products[0].Visibility)
	assert.Equal(t, "PUBLISHED", products[0].Status)
}

func TestClient_ListCatalogProducts_Empty(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{}})
	}))
	defer server.Close()

	client := newTestClient(t, server)
	account := testAccount(server.URL)

	products, err := client.ListCatalogProducts(context.Background(), account, "catalog-123")
	require.NoError(t, err)
	assert.Empty(t, products)
}

// --- CreateProduct ---

func TestClient_CreateProduct_Success(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			assert.Contains(t, r.URL.Path, "/items_batch")
			require.NoError(t, r.ParseForm())
			assert.Equal(t, "PRODUCT_ITEM", r.Form.Get("item_type"))
			var requests []struct {
				Method string            `json:"method"`
				Data   map[string]string `json:"data"`
			}
			require.NoError(t, json.Unmarshal([]byte(r.Form.Get("requests")), &requests))
			require.Len(t, requests, 1)
			assert.Equal(t, "CREATE", requests[0].Method)
			assert.Equal(t, "SKU-001", requests[0].Data["id"])
			assert.Equal(t, "Test Product", requests[0].Data["title"])
			assert.Equal(t, "19.99 USD", requests[0].Data["price"])
			assert.Equal(t, "published", requests[0].Data["visibility"])
			_ = json.NewEncoder(w).Encode(map[string]any{"handles": []string{"batch-handle"}})
		case http.MethodGet:
			assert.Contains(t, r.URL.Path, "/products")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]string{{"id": "prod-new", "retailer_id": "SKU-001"}},
			})
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	client := newTestClient(t, server)
	account := testAccount(server.URL)

	product := &whatsapp.ProductInput{
		Name:        "Test Product",
		Price:       1999,
		Currency:    "USD",
		URL:         "https://example.com/product",
		ImageURL:    "https://example.com/image.jpg",
		RetailerID:  "SKU-001",
		Description: "A test product",
	}

	id, err := client.CreateProduct(context.Background(), account, "catalog-123", product)
	require.NoError(t, err)
	assert.Equal(t, "prod-new", id)
}

// --- UpdateProduct ---

func TestClient_UpdateProduct_Success(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Contains(t, r.URL.Path, "/items_batch")
		require.NoError(t, r.ParseForm())
		var requests []struct {
			Method string            `json:"method"`
			Data   map[string]string `json:"data"`
		}
		require.NoError(t, json.Unmarshal([]byte(r.Form.Get("requests")), &requests))
		require.Len(t, requests, 1)
		assert.Equal(t, "UPDATE", requests[0].Method)
		assert.Equal(t, "SKU-001", requests[0].Data["id"])
		assert.Equal(t, "Updated Product", requests[0].Data["title"])
		_ = json.NewEncoder(w).Encode(map[string]any{"handles": []string{"batch-handle"}})
	}))
	defer server.Close()

	client := newTestClient(t, server)
	account := testAccount(server.URL)

	product := &whatsapp.ProductInput{
		Name:       "Updated Product",
		Price:      2999,
		Currency:   "USD",
		RetailerID: "SKU-001",
	}

	err := client.UpdateProduct(context.Background(), account, "catalog-123", "SKU-001", product)
	require.NoError(t, err)
}

// --- DeleteProduct ---

func TestClient_DeleteProduct_Success(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Contains(t, r.URL.Path, "/items_batch")
		require.NoError(t, r.ParseForm())
		var requests []struct {
			Method string            `json:"method"`
			Data   map[string]string `json:"data"`
		}
		require.NoError(t, json.Unmarshal([]byte(r.Form.Get("requests")), &requests))
		require.Len(t, requests, 1)
		assert.Equal(t, "DELETE", requests[0].Method)
		assert.Equal(t, "SKU-001", requests[0].Data["id"])
		_ = json.NewEncoder(w).Encode(map[string]any{"handles": []string{"batch-handle"}})
	}))
	defer server.Close()

	client := newTestClient(t, server)
	account := testAccount(server.URL)

	err := client.DeleteProduct(context.Background(), account, "catalog-123", "SKU-001")
	require.NoError(t, err)
}

func TestClient_DeleteProduct_APIError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(whatsapp.MetaAPIError{
			Error: struct {
				Message      string `json:"message"`
				Type         string `json:"type"`
				Code         int    `json:"code"`
				ErrorSubcode int    `json:"error_subcode"`
				ErrorUserMsg string `json:"error_user_msg"`
				ErrorData    struct {
					Details string `json:"details"`
				} `json:"error_data"`
				FBTraceID string `json:"fbtrace_id"`
			}{
				Message: "Product not found",
				Code:    100,
			},
		})
	}))
	defer server.Close()

	client := newTestClient(t, server)
	account := testAccount(server.URL)

	err := client.DeleteProduct(context.Background(), account, "catalog-123", "nonexistent")
	require.Error(t, err)
}
