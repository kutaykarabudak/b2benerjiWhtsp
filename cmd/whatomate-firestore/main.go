package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/fasthttp/router"
	"github.com/shridarpatil/whatomate/internal/config"
	"github.com/shridarpatil/whatomate/internal/firestorestore"
	"github.com/shridarpatil/whatomate/internal/frontend"
	"github.com/shridarpatil/whatomate/internal/serverlessapp"
	"github.com/shridarpatil/whatomate/internal/storage"
	"github.com/shridarpatil/whatomate/pkg/whatsapp"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/logf"
)

func main() {
	configPath := flag.String("config", "config.toml", "configuration file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		fatal(err)
	}
	projectID := cfg.Firestore.ProjectID
	if projectID == "" {
		projectID = os.Getenv("GOOGLE_CLOUD_PROJECT")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	store, err := firestorestore.New(ctx, projectID, cfg.Firestore.DatabaseID, cfg.Firestore.Namespace)
	cancel()
	if err != nil {
		fatal(err)
	}
	defer store.Close()

	app, err := serverlessapp.New(store, cfg)
	if err != nil {
		fatal(err)
	}
	app.SetMessenger(whatsapp.New(logf.New(logf.Opts{Level: logf.InfoLevel})))
	if cfg.Storage.Type == "s3" {
		mediaStorage, storageErr := storage.NewS3Client(&cfg.Storage)
		if storageErr != nil {
			fatal(storageErr)
		}
		app.SetMediaStorage(mediaStorage)
	}

	r := router.New()
	r.GET("/health", app.Health)
	r.GET("/ready", app.Ready)
	r.POST("/api/auth/login", app.Login)
	r.POST("/api/auth/refresh", app.Refresh)
	r.POST("/api/auth/logout", app.Logout)
	r.GET("/api/me", app.Me)
	r.GET("/api/users", app.ListUsers)
	r.GET("/api/tags", app.ListTags)
	r.GET("/api/organizations", app.ListOrganizations)
	r.GET("/api/me/organizations", app.ListMyOrganizations)
	r.PUT("/api/me/availability", app.UpdateAvailability)
	r.GET("/api/contacts", app.ListContacts)
	r.POST("/api/contacts", app.CreateContact)
	r.GET("/api/contacts/{id}", app.GetContact)
	r.PUT("/api/contacts/{id}", app.UpdateContact)
	r.DELETE("/api/contacts/{id}", app.DeleteContact)
	r.PUT("/api/contacts/{id}/assign", app.AssignContact)
	r.PUT("/api/contacts/{id}/tags", app.UpdateContactTags)
	r.GET("/api/contacts/{id}/session-data", app.GetContactSessionData)
	r.GET("/api/chatbot/transfers", app.ListAgentTransfers)
	r.POST("/api/chatbot/transfers", app.CreateAgentTransfer)
	r.PUT("/api/chatbot/transfers/{id}/resume", app.ResumeAgentTransfer)
	r.PUT("/api/chatbot/transfers/{id}/assign", app.AssignAgentTransfer)
	r.GET("/api/canned-responses", app.ListCannedResponses)
	r.POST("/api/canned-responses/{id}/use", app.IncrementCannedResponseUsage)
	r.GET("/api/contacts/{id}/notes", app.ListConversationNotes)
	r.POST("/api/contacts/{id}/notes", app.CreateConversationNote)
	r.PUT("/api/contacts/{id}/notes/{note_id}", app.UpdateConversationNote)
	r.DELETE("/api/contacts/{id}/notes/{note_id}", app.DeleteConversationNote)
	r.GET("/api/contacts/{id}/messages", app.ListMessages)
	r.POST("/api/contacts/{id}/messages", app.SendMessage)
	r.POST("/api/contacts/{id}/messages/{message_id}/reaction", app.SendReaction)
	r.GET("/api/accounts", app.ListAccounts)
	r.POST("/api/accounts", app.CreateAccount)
	r.GET("/api/accounts/{id}", app.GetAccount)
	r.PUT("/api/accounts/{id}", app.UpdateAccount)
	r.DELETE("/api/accounts/{id}", app.DeleteAccount)
	r.POST("/api/accounts/{id}/test", app.TestAccountConnection)
	r.POST("/api/accounts/{id}/subscribe", app.SubscribeAccount)
	r.GET("/api/accounts/{id}/business_profile", app.GetBusinessProfile)
	r.PUT("/api/accounts/{id}/business_profile", app.UpdateBusinessProfile)
	r.POST("/api/accounts/{id}/business_profile/photo", app.UpdateBusinessProfilePhoto)
	r.GET("/api/embedded-signup/config", app.GetEmbeddedSignupConfig)
	r.POST("/api/accounts/exchange-token", app.ExchangeAccountToken)
	r.GET("/api/templates", app.ListTemplates)
	r.POST("/api/templates", app.CreateTemplate)
	r.GET("/api/templates/{id}", app.GetTemplate)
	r.PUT("/api/templates/{id}", app.UpdateTemplate)
	r.DELETE("/api/templates/{id}", app.DeleteTemplate)
	r.POST("/api/templates/sync", app.SyncTemplates)
	r.POST("/api/templates/{id}/publish", app.PublishTemplate)
	r.POST("/api/templates/upload-media", app.UploadTemplateMedia)
	r.GET("/api/org/settings", app.GetOrganizationSettings)
	r.PUT("/api/org/settings", app.UpdateOrganizationSettings)
	r.PUT("/api/me/settings", app.UpdateCurrentUserSettings)
	r.GET("/api/audit-logs", app.ListAuditLogs)
	r.POST("/api/messages/template", app.SendTemplate)
	r.POST("/api/messages/media", app.SendMediaMessage)
	r.GET("/api/media/{message_id}", app.ServeMedia)
	r.POST("/api/contacts/{id}/mark-read", app.MarkContactRead)
	r.GET("/api/events", app.ListEvents)
	r.GET("/api/webhook", app.WebhookVerify)
	r.POST("/api/webhook", app.Webhook)
	r.NotFound = frontend.Handler(cfg.Server.BasePath)

	allowedOrigins := parseOrigins(cfg.Server.AllowedOrigins)
	handler := securityAndCORS(allowedOrigins, r.Handler)
	server := &fasthttp.Server{
		Handler:            handler,
		Name:               "whatomate-firestore",
		ReadTimeout:        time.Duration(cfg.Server.ReadTimeout) * time.Second,
		WriteTimeout:       time.Duration(cfg.Server.WriteTimeout) * time.Second,
		MaxRequestBodySize: 32 << 20,
		// Meta/Facebook SDK cookies plus session JWTs exceed fasthttp's 4 KiB
		// default header buffer and otherwise surface as HTTP 431.
		ReadBufferSize: 64 << 10,
	}

	listenAddress := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	errCh := make(chan error, 1)
	go func() { errCh <- server.ListenAndServe(listenAddress) }()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	select {
	case err := <-errCh:
		fatal(err)
	case <-signals:
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if err := server.ShutdownWithContext(shutdownCtx); err != nil {
			fatal(err)
		}
	}
}

func securityAndCORS(origins map[string]struct{}, next fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		ctx.Response.Header.Set("X-Content-Type-Options", "nosniff")
		ctx.Response.Header.Set("X-Frame-Options", "DENY")
		ctx.Response.Header.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		ctx.Response.Header.Set("Permissions-Policy", "camera=(self), microphone=(self), geolocation=(self)")
		origin := string(ctx.Request.Header.Peek("Origin"))
		if _, ok := origins[origin]; ok && origin != "" {
			ctx.Response.Header.Set("Access-Control-Allow-Origin", origin)
			ctx.Response.Header.Set("Access-Control-Allow-Credentials", "true")
		}
		ctx.Response.Header.Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH")
		ctx.Response.Header.Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key, X-Organization-ID, X-CSRF-Token")
		if ctx.IsOptions() {
			ctx.SetStatusCode(fasthttp.StatusNoContent)
			return
		}
		next(ctx)
	}
}

func parseOrigins(raw string) map[string]struct{} {
	result := make(map[string]struct{})
	for _, origin := range strings.Split(raw, ",") {
		if trimmed := strings.TrimSpace(origin); trimmed != "" {
			result[trimmed] = struct{}{}
		}
	}
	return result
}

func fatal(err error) {
	_, _ = fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
