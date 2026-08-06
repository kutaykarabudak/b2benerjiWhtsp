package handlers

import (
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

const maxCatalogImageBytes = 8 << 20

var allowedCatalogImageTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/webp": true,
}

func catalogImageFile(form *multipart.Form) (*multipart.FileHeader, error) {
	files := form.File["file"]
	if len(files) == 0 {
		return nil, fmt.Errorf("Ürün görseli zorunludur.")
	}
	file := files[0]
	if file.Size <= 0 || file.Size > maxCatalogImageBytes {
		return nil, fmt.Errorf("Ürün görseli en fazla 8 MB olabilir.")
	}
	return file, nil
}

// UploadCatalogImage accepts a product image from the authenticated panel and
// returns the stable public URL Meta needs in the catalog item image_link.
func (a *App) UploadCatalogImage(r *fastglue.Request) error {
	orgID, err := a.getOrgID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}
	form, err := r.RequestCtx.MultipartForm()
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Geçersiz dosya yükleme isteği.", nil, "")
	}
	fileHeader, err := catalogImageFile(form)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, err.Error(), nil, "")
	}
	file, err := fileHeader.Open()
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Ürün görseli okunamadı.", nil, "")
	}
	defer file.Close() //nolint:errcheck

	data, err := io.ReadAll(io.LimitReader(file, maxCatalogImageBytes+1))
	if err != nil || len(data) == 0 || len(data) > maxCatalogImageBytes {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Ürün görseli okunamadı veya 8 MB sınırını aşıyor.", nil, "")
	}
	mimeType := http.DetectContentType(data)
	if !allowedCatalogImageTypes[mimeType] {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Yalnızca JPG, PNG veya WEBP ürün görseli yükleyin.", nil, "")
	}

	publicToken := uuid.New()
	image := models.CatalogImage{
		OrganizationID: orgID,
		PublicToken:    publicToken,
		Filename:       fileHeader.Filename,
		MimeType:       mimeType,
		Data:           data,
	}
	if err := a.DB.Create(&image).Error; err != nil {
		a.Log.Error("Failed to store catalog image", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Ürün görseli kaydedilemedi.", nil, "")
	}

	host := string(r.RequestCtx.Request.Header.Peek("X-Forwarded-Host"))
	if host == "" {
		host = string(r.RequestCtx.Host())
	}
	if comma := strings.IndexByte(host, ','); comma >= 0 {
		host = strings.TrimSpace(host[:comma])
	}
	scheme := string(r.RequestCtx.Request.Header.Peek("X-Forwarded-Proto"))
	if comma := strings.IndexByte(scheme, ','); comma >= 0 {
		scheme = strings.TrimSpace(scheme[:comma])
	}
	if scheme == "" {
		scheme = "https"
		if strings.HasPrefix(host, "localhost") || strings.HasPrefix(host, "127.0.0.1") {
			scheme = "http"
		}
	}
	path := "/catalog-images/" + publicToken.String()
	return r.SendEnvelope(map[string]string{
		"url":  scheme + "://" + host + path,
		"path": path,
	})
}

// ServeCatalogImage is intentionally public: Meta's catalog crawler must be
// able to fetch image_link without panel authentication. UUID tokens are
// unguessable and the response exposes only immutable image bytes.
func (a *App) ServeCatalogImage(r *fastglue.Request) error {
	tokenValue, _ := r.RequestCtx.UserValue("token").(string)
	token, err := uuid.Parse(tokenValue)
	if err != nil {
		r.RequestCtx.SetStatusCode(fasthttp.StatusNotFound)
		return nil
	}
	var image models.CatalogImage
	if err := a.DB.Where("public_token = ?", token).First(&image).Error; err != nil {
		r.RequestCtx.SetStatusCode(fasthttp.StatusNotFound)
		return nil
	}
	r.RequestCtx.Response.Header.SetContentType(image.MimeType)
	r.RequestCtx.Response.Header.Set("Cache-Control", "public, max-age=31536000, immutable")
	r.RequestCtx.Response.Header.Set("X-Content-Type-Options", "nosniff")
	r.RequestCtx.SetBody(image.Data)
	return nil
}
