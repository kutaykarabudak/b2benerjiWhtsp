package handlers

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/shridarpatil/whatomate/pkg/whatsapp"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

// mediaPresignExpiry is how long a redirected media URL stays valid. Chat
// clients fetch it once per render, so this only needs to outlive a page view.
const mediaPresignExpiry = 15 * time.Minute

// usesS3Media reports whether media should be stored/served via the S3-compatible
// backend rather than local disk. Local disk does not survive a Cloud Run
// redeploy, instance restart, or a request landing on a different instance —
// so this must be true in any multi-instance or ephemeral-filesystem deployment.
func (a *App) usesS3Media() bool {
	return a.Config.Storage.Type == "s3" && a.S3Client != nil
}

// saveMedia stores data under subdir/filename using the configured storage
// backend and returns a relative reference (S3 key or local relative path,
// same shape either way) to persist as the message/campaign's media_url.
func (a *App) saveMedia(ctx context.Context, data []byte, mimeType, subdir, filename string) (string, error) {
	key := subdir + "/" + filename

	if a.usesS3Media() {
		if err := a.S3Client.Upload(ctx, key, bytes.NewReader(data), mimeType); err != nil {
			return "", fmt.Errorf("failed to upload media to storage: %w", err)
		}
		return key, nil
	}

	if err := a.ensureMediaDir(subdir); err != nil {
		return "", fmt.Errorf("failed to create media directory: %w", err)
	}
	filePath := filepath.Join(a.getMediaStoragePath(), subdir, filename)
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return "", fmt.Errorf("failed to save media file: %w", err)
	}
	return key, nil
}

// writeMediaResponse serves a previously-saved media reference: a redirect to
// a presigned URL when stored in S3, or the file straight off local disk.
func (a *App) writeMediaResponse(r *fastglue.Request, storedRef string) error {
	if a.usesS3Media() {
		url, err := a.S3Client.GetPresignedURL(r.RequestCtx, storedRef, mediaPresignExpiry)
		if err != nil {
			a.Log.Error("Failed to presign media URL", "key", storedRef, "error", err)
			return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to load media", nil, "")
		}
		r.RequestCtx.Redirect(url, fasthttp.StatusFound)
		return nil
	}

	// Security: prevent directory traversal and symlink attacks
	filePath := filepath.Clean(storedRef)
	baseDir, err := filepath.Abs(a.getMediaStoragePath())
	if err != nil {
		a.Log.Error("Storage configuration error", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Storage configuration error", nil, "")
	}
	fullPath, err := filepath.Abs(filepath.Join(baseDir, filePath))
	if err != nil || !strings.HasPrefix(fullPath, baseDir+string(os.PathSeparator)) {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Invalid file path", nil, "")
	}

	// Reject symlinks
	info, err := os.Lstat(fullPath)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, "File not found", nil, "")
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Invalid file path", nil, "")
	}

	data, err := os.ReadFile(fullPath)
	if err != nil {
		a.Log.Error("Failed to read media file", "path", fullPath, "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to read file", nil, "")
	}

	r.RequestCtx.Response.Header.Set("Content-Type", contentTypeFromExtension(filePath))
	r.RequestCtx.Response.Header.Set("Cache-Control", "private, max-age=3600") // Cache for 1 hour, private
	r.RequestCtx.SetBody(data)
	return nil
}

// contentTypeFromExtension maps a stored file's extension to a MIME type for
// local-disk serving (S3 objects carry their own Content-Type from upload).
func contentTypeFromExtension(filePath string) string {
	switch strings.ToLower(filepath.Ext(filePath)) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".mp4":
		return "video/mp4"
	case ".3gp":
		return "video/3gpp"
	case ".mp3":
		return "audio/mpeg"
	case ".aac":
		return "audio/aac"
	case ".m4a":
		return "audio/mp4"
	case ".ogg":
		return "audio/ogg"
	case ".amr":
		return "audio/amr"
	case ".pdf":
		return "application/pdf"
	case ".doc":
		return "application/msword"
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case ".xls":
		return "application/vnd.ms-excel"
	case ".xlsx":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case ".txt":
		return "text/plain"
	default:
		return "application/octet-stream"
	}
}

// getMediaStoragePath returns the base path for media storage
func (a *App) getMediaStoragePath() string {
	basePath := a.Config.Storage.LocalPath
	if basePath == "" {
		basePath = "./media"
	}
	return basePath
}

// ensureMediaDir ensures the media directory exists
func (a *App) ensureMediaDir(subdir string) error {
	path := filepath.Join(a.getMediaStoragePath(), subdir)
	return os.MkdirAll(path, 0755)
}

// getExtensionFromMimeType returns file extension based on mime type
func getExtensionFromMimeType(mimeType string) string {
	switch {
	case strings.HasPrefix(mimeType, "image/jpeg"):
		return ".jpg"
	case strings.HasPrefix(mimeType, "image/png"):
		return ".png"
	case strings.HasPrefix(mimeType, "image/gif"):
		return ".gif"
	case strings.HasPrefix(mimeType, "image/webp"):
		return ".webp"
	case strings.HasPrefix(mimeType, "video/mp4"):
		return ".mp4"
	case strings.HasPrefix(mimeType, "video/3gpp"):
		return ".3gp"
	case strings.HasPrefix(mimeType, "audio/aac"):
		return ".aac"
	case strings.HasPrefix(mimeType, "audio/mp4"):
		return ".m4a"
	case strings.HasPrefix(mimeType, "audio/mpeg"):
		return ".mp3"
	case strings.HasPrefix(mimeType, "audio/amr"):
		return ".amr"
	case strings.HasPrefix(mimeType, "audio/ogg"):
		return ".ogg"
	case strings.HasPrefix(mimeType, "application/pdf"):
		return ".pdf"
	case strings.HasPrefix(mimeType, "application/vnd.ms-powerpoint"):
		return ".ppt"
	case strings.HasPrefix(mimeType, "application/msword"):
		return ".doc"
	case strings.HasPrefix(mimeType, "application/vnd.ms-excel"):
		return ".xls"
	case strings.HasPrefix(mimeType, "application/vnd.openxmlformats-officedocument.wordprocessingml"):
		return ".docx"
	case strings.HasPrefix(mimeType, "application/vnd.openxmlformats-officedocument.spreadsheetml"):
		return ".xlsx"
	case strings.HasPrefix(mimeType, "application/vnd.openxmlformats-officedocument.presentationml"):
		return ".pptx"
	case strings.HasPrefix(mimeType, "text/plain"):
		return ".txt"
	default:
		return ""
	}
}

// DownloadAndSaveMedia downloads media from Meta and saves it locally
// Returns the local file path (relative to media storage) or error
func (a *App) DownloadAndSaveMedia(ctx context.Context, mediaID string, mimeType string, account *whatsapp.Account) (string, error) {
	// Get the media URL from Meta
	mediaURL, err := a.WhatsApp.GetMediaURL(ctx, mediaID, account)
	if err != nil {
		return "", fmt.Errorf("failed to get media URL: %w", err)
	}

	// Download the media content
	data, err := a.WhatsApp.DownloadMedia(ctx, mediaURL, account.AccessToken)
	if err != nil {
		return "", fmt.Errorf("failed to download media: %w", err)
	}

	// Determine file extension
	ext := getExtensionFromMimeType(mimeType)
	if ext == "" {
		ext = ".bin"
	}

	// Generate unique filename
	filename := uuid.New().String() + ext

	// Determine subdirectory based on media type
	var subdir string
	switch {
	case strings.HasPrefix(mimeType, "image/"):
		subdir = "images"
	case strings.HasPrefix(mimeType, "video/"):
		subdir = "videos"
	case strings.HasPrefix(mimeType, "audio/"):
		subdir = "audio"
	default:
		subdir = "documents"
	}

	relativePath, err := a.saveMedia(ctx, data, mimeType, subdir, filename)
	if err != nil {
		return "", err
	}
	a.Log.Info("Media saved", "path", relativePath, "size", len(data))

	return relativePath, nil
}

// ServeMedia serves media files from local storage
// Only authorized users who have access to the message can view the media
func (a *App) ServeMedia(r *fastglue.Request) error {
	// Get auth context
	orgID, userID, err := a.getOrgAndUserID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}

	// Get the message ID from URL parameter
	messageIDStr := r.RequestCtx.UserValue("message_id").(string)
	messageID, err := uuid.Parse(messageIDStr)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Invalid message ID", nil, "")
	}

	// Find the message and verify access
	message, err := findByIDAndOrg[models.Message](a.DB, r, messageID, orgID, "Message")
	if err != nil {
		return nil
	}

	// Users without contacts:read permission can only access media from contacts
	// assigned to them — the persistent owner or an active transfer assigned
	// directly to them (via scopeAssignedContact) — or from contacts with an
	// active team transfer where the user is a team member.
	if !a.HasPermission(userID, models.ResourceContacts, models.ActionRead, orgID) {
		var contact models.Contact
		q := a.scopeAssignedContact(a.DB.Where("id = ? AND organization_id = ?", message.ContactID, orgID), userID, orgID)
		if err := q.First(&contact).Error; err != nil {
			// Not owner / not directly assigned — check team membership via active transfer
			var transfer models.AgentTransfer
			if err := a.DB.Where("contact_id = ? AND organization_id = ? AND status = ? AND team_id IS NOT NULL",
				message.ContactID, orgID, models.TransferStatusActive).First(&transfer).Error; err != nil {
				return r.SendErrorEnvelope(fasthttp.StatusForbidden, "Access denied", nil, "")
			}
			var count int64
			a.DB.Model(&models.TeamMember{}).Where("team_id = ? AND user_id = ?", transfer.TeamID, userID).Count(&count)
			if count == 0 {
				return r.SendErrorEnvelope(fasthttp.StatusForbidden, "Access denied", nil, "")
			}
		}
	}

	// Check if message has media
	if message.MediaURL == "" {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, "No media found", nil, "")
	}

	return a.writeMediaResponse(r, message.MediaURL)
}
