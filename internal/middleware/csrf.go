package middleware

import (
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

// CSRFProtection validates mutating cookie-authenticated requests. Standard
// deployments use double-submit cookies. Firebase Hosting only forwards the
// specially named __session cookie, so those requests use a strict Origin
// allowlist check instead.
func CSRFProtection(allowedOrigins map[string]bool) fastglue.FastMiddleware {
	return func(r *fastglue.Request) *fastglue.Request {
		method := string(r.RequestCtx.Method())

		// Only validate mutating methods.
		if method != "POST" && method != "PUT" && method != "DELETE" && method != "PATCH" {
			return r
		}

		// Skip if the request uses header-based auth (API key or Bearer token).
		// These are not automatically attached by the browser, so CSRF is not a concern.
		if len(r.RequestCtx.Request.Header.Peek("Authorization")) > 0 ||
			len(r.RequestCtx.Request.Header.Peek("X-API-Key")) > 0 {
			return r
		}

		// Firebase Hosting strips the separate CSRF cookie. Browser requests
		// carrying __session must therefore come from an explicitly allowed
		// origin. Production startup already requires a non-empty allowlist.
		if len(r.RequestCtx.Request.Header.Cookie(FirebaseSessionCookieName)) > 0 {
			origin := string(r.RequestCtx.Request.Header.Peek("Origin"))
			if origin == "" || !IsOriginAllowed(origin, allowedOrigins) {
				r.RequestCtx.SetStatusCode(fasthttp.StatusForbidden)
				r.RequestCtx.SetContentType("application/json")
				r.RequestCtx.SetBodyString(`{"status":"error","message":"Origin validation failed"}`)
				return nil
			}
			return r
		}

		// Skip if there is no access cookie — the auth middleware will reject
		// the request with 401 anyway.
		cookieVal := r.RequestCtx.Request.Header.Cookie("whm_access")
		if len(cookieVal) == 0 {
			return r
		}

		// Double-submit: compare whm_csrf cookie with X-CSRF-Token header.
		csrfCookie := string(r.RequestCtx.Request.Header.Cookie("whm_csrf"))
		csrfHeader := string(r.RequestCtx.Request.Header.Peek("X-CSRF-Token"))

		if csrfCookie == "" || csrfHeader == "" || csrfCookie != csrfHeader {
			r.RequestCtx.SetStatusCode(fasthttp.StatusForbidden)
			r.RequestCtx.SetContentType("application/json")
			r.RequestCtx.SetBodyString(`{"status":"error","message":"CSRF token mismatch"}`)
			return nil
		}

		return r
	}
}
