package frontend

import (
	"net/http/httptest"
	"testing"
)

func TestSPAHTMLIsNotCached(t *testing.T) {
	response := httptest.NewRecorder()
	setSPAHTMLCacheHeaders(response)

	if got := response.Header().Get("Cache-Control"); got != "no-store, no-cache, must-revalidate" {
		t.Fatalf("Cache-Control = %q", got)
	}
}

func TestVersionedAssetsAreCachedAndVaryOnEncoding(t *testing.T) {
	response := httptest.NewRecorder()
	setAssetCacheHeaders(response, "assets/index-abc123.js")

	if got := response.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("Cache-Control = %q", got)
	}
	if got := response.Header().Get("Vary"); got != "Accept-Encoding" {
		t.Fatalf("Vary = %q", got)
	}
}
