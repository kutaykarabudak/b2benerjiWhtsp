package firestorestore

import "time"

// Documents intentionally use strings for IDs. Existing UUIDs survive the
// migration byte-for-byte while Firestore document paths stay easy to inspect.

type Organization struct {
	ID        string         `firestore:"id" json:"id"`
	Name      string         `firestore:"name" json:"name"`
	Slug      string         `firestore:"slug" json:"slug"`
	Settings  map[string]any `firestore:"settings" json:"settings"`
	CreatedAt time.Time      `firestore:"createdAt" json:"created_at"`
	UpdatedAt time.Time      `firestore:"updatedAt" json:"updated_at"`
}

type User struct {
	ID             string         `firestore:"id" json:"id"`
	OrganizationID string         `firestore:"organizationId" json:"organization_id"`
	Email          string         `firestore:"email" json:"email"`
	PasswordHash   string         `firestore:"passwordHash" json:"-"`
	FullName       string         `firestore:"fullName" json:"full_name"`
	RoleID         string         `firestore:"roleId,omitempty" json:"role_id,omitempty"`
	Role           *Role          `firestore:"role,omitempty" json:"role,omitempty"`
	Settings       map[string]any `firestore:"settings" json:"settings"`
	IsActive       bool           `firestore:"isActive" json:"is_active"`
	IsAvailable    bool           `firestore:"isAvailable" json:"is_available"`
	IsSuperAdmin   bool           `firestore:"isSuperAdmin" json:"is_super_admin"`
	CreatedAt      time.Time      `firestore:"createdAt" json:"created_at"`
	UpdatedAt      time.Time      `firestore:"updatedAt" json:"updated_at"`
}

type Permission struct {
	ID          string `firestore:"id" json:"id"`
	Resource    string `firestore:"resource" json:"resource"`
	Action      string `firestore:"action" json:"action"`
	Description string `firestore:"description,omitempty" json:"description,omitempty"`
}

type Role struct {
	ID          string       `firestore:"id" json:"id"`
	Name        string       `firestore:"name" json:"name"`
	Description string       `firestore:"description,omitempty" json:"description,omitempty"`
	IsSystem    bool         `firestore:"isSystem" json:"is_system"`
	Permissions []Permission `firestore:"permissions" json:"permissions"`
}

type Tag struct {
	OrganizationID string    `firestore:"organizationId" json:"organization_id"`
	Name           string    `firestore:"name" json:"name"`
	Color          string    `firestore:"color" json:"color"`
	CreatedAt      time.Time `firestore:"createdAt" json:"created_at"`
	UpdatedAt      time.Time `firestore:"updatedAt" json:"updated_at"`
}

// WhatsAppAccount keeps credentials encrypted at rest. Decryption only happens
// in the server process immediately before a request to Meta.
type WhatsAppAccount struct {
	ID                     string     `firestore:"id" json:"id"`
	OrganizationID         string     `firestore:"organizationId" json:"organization_id"`
	Name                   string     `firestore:"name" json:"name"`
	AppID                  string     `firestore:"appId,omitempty" json:"app_id"`
	PhoneID                string     `firestore:"phoneId" json:"phone_id"`
	BusinessID             string     `firestore:"businessId" json:"business_id"`
	CatalogBusinessID      string     `firestore:"catalogBusinessId,omitempty" json:"catalog_business_id"`
	AccessToken            string     `firestore:"accessToken" json:"-"`
	AppSecret              string     `firestore:"appSecret,omitempty" json:"-"`
	WebhookVerifyToken     string     `firestore:"webhookVerifyToken" json:"webhook_verify_token"`
	APIVersion             string     `firestore:"apiVersion" json:"api_version"`
	IsDefaultIncoming      bool       `firestore:"isDefaultIncoming" json:"is_default_incoming"`
	IsDefaultOutgoing      bool       `firestore:"isDefaultOutgoing" json:"is_default_outgoing"`
	AutoReadReceipt        bool       `firestore:"autoReadReceipt" json:"auto_read_receipt"`
	BusinessCallingEnabled bool       `firestore:"businessCallingEnabled" json:"business_calling_enabled"`
	IsSMB                  bool       `firestore:"isSMB" json:"is_smb"`
	Status                 string     `firestore:"status" json:"status"`
	Pin                    string     `firestore:"pin,omitempty" json:"-"`
	CreatedByID            string     `firestore:"createdById,omitempty" json:"created_by_id,omitempty"`
	UpdatedByID            string     `firestore:"updatedById,omitempty" json:"updated_by_id,omitempty"`
	CreatedAt              time.Time  `firestore:"createdAt" json:"created_at"`
	UpdatedAt              time.Time  `firestore:"updatedAt" json:"updated_at"`
	DeletedAt              *time.Time `firestore:"deletedAt,omitempty" json:"deleted_at,omitempty"`
	IsDeleted              bool       `firestore:"isDeleted" json:"-"`
}

type Template struct {
	ID                        string         `firestore:"id" json:"id"`
	OrganizationID            string         `firestore:"organizationId" json:"organization_id"`
	WhatsAppAccount           string         `firestore:"whatsAppAccount" json:"whatsapp_account"`
	MetaTemplateID            string         `firestore:"metaTemplateId,omitempty" json:"meta_template_id"`
	Name                      string         `firestore:"name" json:"name"`
	DisplayName               string         `firestore:"displayName,omitempty" json:"display_name"`
	Language                  string         `firestore:"language" json:"language"`
	Category                  string         `firestore:"category" json:"category"`
	Status                    string         `firestore:"status" json:"status"`
	QualityRating             string         `firestore:"qualityRating" json:"quality_rating"`
	HeaderType                string         `firestore:"headerType,omitempty" json:"header_type"`
	HeaderContent             string         `firestore:"headerContent,omitempty" json:"header_content"`
	BodyContent               string         `firestore:"bodyContent" json:"body_content"`
	FooterContent             string         `firestore:"footerContent,omitempty" json:"footer_content"`
	Buttons                   []any          `firestore:"buttons" json:"buttons"`
	SampleValues              []any          `firestore:"sampleValues" json:"sample_values"`
	IsFirstMessage            bool           `firestore:"isFirstMessage" json:"is_first_message"`
	AddSecurityRecommendation bool           `firestore:"addSecurityRecommendation" json:"add_security_recommendation"`
	CodeExpirationMinutes     int            `firestore:"codeExpirationMinutes" json:"code_expiration_minutes"`
	CreatedByID               string         `firestore:"createdById,omitempty" json:"created_by_id,omitempty"`
	UpdatedByID               string         `firestore:"updatedById,omitempty" json:"updated_by_id,omitempty"`
	CreatedAt                 time.Time      `firestore:"createdAt" json:"created_at"`
	UpdatedAt                 time.Time      `firestore:"updatedAt" json:"updated_at"`
	DeletedAt                 *time.Time     `firestore:"deletedAt,omitempty" json:"deleted_at,omitempty"`
	IsDeleted                 bool           `firestore:"isDeleted" json:"-"`
	Metadata                  map[string]any `firestore:"metadata" json:"metadata,omitempty"`
}

type Contact struct {
	ID                   string         `firestore:"id" json:"id"`
	OrganizationID       string         `firestore:"organizationId" json:"organization_id"`
	PhoneNumber          string         `firestore:"phoneNumber" json:"phone_number"`
	PhoneCountryCode     string         `firestore:"phoneCountryCode,omitempty" json:"phone_country_code"`
	ProfileName          string         `firestore:"profileName" json:"profile_name"`
	CustomerID           string         `firestore:"customerId,omitempty" json:"customer_id"`
	FirstName            string         `firestore:"firstName,omitempty" json:"first_name"`
	LastName             string         `firestore:"lastName,omitempty" json:"last_name"`
	WhatsAppAccount      string         `firestore:"whatsAppAccount" json:"whatsapp_account"`
	ChannelType          string         `firestore:"channelType" json:"channel_type"`
	AssignedUserID       string         `firestore:"assignedUserId,omitempty" json:"assigned_user_id,omitempty"`
	LastMessageAt        *time.Time     `firestore:"lastMessageAt,omitempty" json:"last_message_at,omitempty"`
	LastMessagePreview   string         `firestore:"lastMessagePreview" json:"last_message_preview"`
	IsRead               bool           `firestore:"isRead" json:"is_read"`
	Tags                 []any          `firestore:"tags" json:"tags"`
	Metadata             map[string]any `firestore:"metadata" json:"metadata"`
	SearchTokens         []string       `firestore:"searchTokens" json:"-"`
	LastInboundAt        *time.Time     `firestore:"lastInboundAt,omitempty" json:"last_inbound_at,omitempty"`
	Email                string         `firestore:"email,omitempty" json:"email"`
	CompanyName          string         `firestore:"companyName,omitempty" json:"company_name"`
	TaxOffice            string         `firestore:"taxOffice,omitempty" json:"tax_office"`
	TaxNumber            string         `firestore:"taxNumber,omitempty" json:"tax_number"`
	Address              string         `firestore:"address,omitempty" json:"address"`
	City                 string         `firestore:"city,omitempty" json:"city"`
	District             string         `firestore:"district,omitempty" json:"district"`
	PostalCode           string         `firestore:"postalCode,omitempty" json:"postal_code"`
	PurchaseScore        int            `firestore:"purchaseScore" json:"purchase_score"`
	HasPurchased         bool           `firestore:"hasPurchased" json:"has_purchased"`
	Note                 string         `firestore:"note,omitempty" json:"note"`
	MarketingOptOut      bool           `firestore:"marketingOptOut" json:"marketing_opt_out"`
	BSUID                string         `firestore:"bsuid,omitempty" json:"bsuid,omitempty"`
	ChatbotLastMessageAt *time.Time     `firestore:"chatbotLastMessageAt,omitempty" json:"chatbot_last_message_at,omitempty"`
	ChatbotReminderSent  bool           `firestore:"chatbotReminderSent" json:"chatbot_reminder_sent"`
	CreatedAt            time.Time      `firestore:"createdAt" json:"created_at"`
	UpdatedAt            time.Time      `firestore:"updatedAt" json:"updated_at"`
	DeletedAt            *time.Time     `firestore:"deletedAt,omitempty" json:"deleted_at,omitempty"`
	IsDeleted            bool           `firestore:"isDeleted" json:"-"`
}

type Message struct {
	ID               string         `firestore:"id" json:"id"`
	OrganizationID   string         `firestore:"organizationId" json:"organization_id"`
	ContactID        string         `firestore:"contactId" json:"contact_id"`
	WhatsAppAccount  string         `firestore:"whatsAppAccount" json:"whatsapp_account"`
	ChannelType      string         `firestore:"channelType" json:"channel_type"`
	ExternalID       string         `firestore:"externalId,omitempty" json:"external_id"`
	ConversationID   string         `firestore:"conversationId,omitempty" json:"conversation_id"`
	Direction        string         `firestore:"direction" json:"direction"`
	MessageType      string         `firestore:"messageType" json:"message_type"`
	Content          string         `firestore:"content" json:"content"`
	MediaURL         string         `firestore:"mediaUrl,omitempty" json:"media_url"`
	MediaMimeType    string         `firestore:"mediaMimeType,omitempty" json:"media_mime_type"`
	MediaFilename    string         `firestore:"mediaFilename,omitempty" json:"media_filename"`
	TemplateName     string         `firestore:"templateName,omitempty" json:"template_name"`
	TemplateParams   map[string]any `firestore:"templateParams,omitempty" json:"template_params"`
	InteractiveData  map[string]any `firestore:"interactiveData,omitempty" json:"interactive_data"`
	FlowResponse     map[string]any `firestore:"flowResponse,omitempty" json:"flow_response"`
	Status           string         `firestore:"status" json:"status"`
	ErrorMessage     string         `firestore:"errorMessage,omitempty" json:"error_message"`
	IsReply          bool           `firestore:"isReply" json:"is_reply"`
	ReplyToMessageID string         `firestore:"replyToMessageId,omitempty" json:"reply_to_message_id,omitempty"`
	SentByUserID     string         `firestore:"sentByUserId,omitempty" json:"sent_by_user_id,omitempty"`
	Metadata         map[string]any `firestore:"metadata" json:"metadata"`
	CreatedAt        time.Time      `firestore:"createdAt" json:"created_at"`
	UpdatedAt        time.Time      `firestore:"updatedAt" json:"updated_at"`
}

type ConversationNote struct {
	ID             string    `firestore:"id" json:"id"`
	OrganizationID string    `firestore:"organizationId" json:"organization_id"`
	ContactID      string    `firestore:"contactId" json:"contact_id"`
	CreatedByID    string    `firestore:"createdById" json:"created_by_id"`
	CreatedByName  string    `firestore:"createdByName" json:"created_by_name"`
	Content        string    `firestore:"content" json:"content"`
	CreatedAt      time.Time `firestore:"createdAt" json:"created_at"`
	UpdatedAt      time.Time `firestore:"updatedAt" json:"updated_at"`
}

type AgentTransfer struct {
	ID                    string     `firestore:"id" json:"id"`
	OrganizationID        string     `firestore:"organizationId" json:"organization_id"`
	ContactID             string     `firestore:"contactId" json:"contact_id"`
	ContactName           string     `firestore:"contactName" json:"contact_name"`
	PhoneNumber           string     `firestore:"phoneNumber" json:"phone_number"`
	WhatsAppAccount       string     `firestore:"whatsAppAccount" json:"whatsapp_account"`
	Status                string     `firestore:"status" json:"status"`
	Source                string     `firestore:"source" json:"source"`
	AgentID               string     `firestore:"agentId" json:"agent_id,omitempty"`
	AgentName             string     `firestore:"agentName,omitempty" json:"agent_name,omitempty"`
	TeamID                string     `firestore:"teamId" json:"team_id,omitempty"`
	TeamName              string     `firestore:"teamName,omitempty" json:"team_name,omitempty"`
	TransferredBy         string     `firestore:"transferredBy,omitempty" json:"transferred_by,omitempty"`
	TransferredByName     string     `firestore:"transferredByName,omitempty" json:"transferred_by_name,omitempty"`
	Notes                 string     `firestore:"notes" json:"notes"`
	TransferredAt         time.Time  `firestore:"transferredAt" json:"transferred_at"`
	ResumedAt             *time.Time `firestore:"resumedAt,omitempty" json:"resumed_at,omitempty"`
	ResumedBy             string     `firestore:"resumedBy,omitempty" json:"resumed_by,omitempty"`
	ResumedByName         string     `firestore:"resumedByName,omitempty" json:"resumed_by_name,omitempty"`
	SLAResponseDeadline   *time.Time `firestore:"slaResponseDeadline,omitempty" json:"sla_response_deadline,omitempty"`
	SLAResolutionDeadline *time.Time `firestore:"slaResolutionDeadline,omitempty" json:"sla_resolution_deadline,omitempty"`
	SLABreached           bool       `firestore:"slaBreached" json:"sla_breached"`
	SLABreachedAt         *time.Time `firestore:"slaBreachedAt,omitempty" json:"sla_breached_at,omitempty"`
	EscalationLevel       int        `firestore:"escalationLevel" json:"escalation_level"`
	EscalatedAt           *time.Time `firestore:"escalatedAt,omitempty" json:"escalated_at,omitempty"`
	PickedUpAt            *time.Time `firestore:"pickedUpAt,omitempty" json:"picked_up_at,omitempty"`
	ExpiresAt             *time.Time `firestore:"expiresAt,omitempty" json:"expires_at,omitempty"`
	CreatedAt             time.Time  `firestore:"createdAt" json:"created_at"`
	UpdatedAt             time.Time  `firestore:"updatedAt" json:"updated_at"`
}

type ChatbotSession struct {
	ID              string         `firestore:"id" json:"id"`
	OrganizationID  string         `firestore:"organizationId" json:"organization_id"`
	ContactID       string         `firestore:"contactId" json:"contact_id"`
	WhatsAppAccount string         `firestore:"whatsAppAccount" json:"whatsapp_account"`
	PhoneNumber     string         `firestore:"phoneNumber" json:"phone_number"`
	Status          string         `firestore:"status" json:"status"`
	CurrentFlowID   string         `firestore:"currentFlowId,omitempty" json:"current_flow_id,omitempty"`
	CurrentStep     string         `firestore:"currentStep" json:"current_step"`
	StepRetries     int            `firestore:"stepRetries" json:"step_retries"`
	SessionData     map[string]any `firestore:"sessionData" json:"session_data"`
	StartedAt       time.Time      `firestore:"startedAt" json:"started_at"`
	LastActivityAt  time.Time      `firestore:"lastActivityAt" json:"last_activity_at"`
	CompletedAt     *time.Time     `firestore:"completedAt,omitempty" json:"completed_at,omitempty"`
	CreatedAt       time.Time      `firestore:"createdAt" json:"created_at"`
	UpdatedAt       time.Time      `firestore:"updatedAt" json:"updated_at"`
}

type CannedResponse struct {
	ID             string    `firestore:"id" json:"id"`
	OrganizationID string    `firestore:"organizationId" json:"organization_id"`
	Name           string    `firestore:"name" json:"name"`
	Shortcut       string    `firestore:"shortcut" json:"shortcut"`
	Content        string    `firestore:"content" json:"content"`
	Category       string    `firestore:"category" json:"category"`
	IsActive       bool      `firestore:"isActive" json:"is_active"`
	UsageCount     int       `firestore:"usageCount" json:"usage_count"`
	Buttons        []any     `firestore:"buttons" json:"buttons"`
	CreatedByID    string    `firestore:"createdById" json:"created_by_id"`
	CreatedAt      time.Time `firestore:"createdAt" json:"created_at"`
	UpdatedAt      time.Time `firestore:"updatedAt" json:"updated_at"`
}

type RefreshToken struct {
	UserID    string    `firestore:"userId"`
	ExpiresAt time.Time `firestore:"expiresAt"`
	CreatedAt time.Time `firestore:"createdAt"`
}

type WebhookReceipt struct {
	ExternalID string    `firestore:"externalId"`
	CreatedAt  time.Time `firestore:"createdAt"`
	ExpiresAt  time.Time `firestore:"expiresAt"`
}

type Event struct {
	ID             string         `firestore:"id" json:"id"`
	OrganizationID string         `firestore:"organizationId" json:"organization_id"`
	Type           string         `firestore:"type" json:"type"`
	Payload        map[string]any `firestore:"payload" json:"payload"`
	CreatedAt      time.Time      `firestore:"createdAt" json:"created_at"`
	ExpiresAt      time.Time      `firestore:"expiresAt" json:"-"`
}

type EventCursor struct {
	CreatedAt time.Time
	ID        string
}
