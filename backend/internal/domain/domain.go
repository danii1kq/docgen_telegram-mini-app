// Package domain holds the API contract: JSON shapes that match
// srunal_vite/src/lib/types.ts field for field (camelCase keys).
package domain

import "time"

// Product codes, languages, contact types.
const (
	ProductEssay        = "essay"
	ProductLab          = "lab"
	ProductPresentation = "presentation"
	ProductOther        = "other"
)

// Order statuses.
const (
	StatusAwaitingPayment = "awaiting_payment"
	StatusPaymentClaimed  = "payment_claimed"
	StatusPaymentRejected = "payment_rejected"
	StatusQueued          = "queued"
	StatusGenerating      = "generating"
	StatusCompleted       = "completed"
	StatusFailedFinal     = "failed_final"
	StatusExpired         = "expired"
	StatusCancelled       = "cancelled"
)

// Review statuses.
const (
	ReviewPending   = "pending"
	ReviewPublished = "published"
	ReviewRejected  = "rejected"
)

// Actor values for the order timeline.
const (
	ActorClient   = "client"
	ActorOperator = "operator"
	ActorSystem   = "system"
)

// FileMeta mirrors FileMeta from types.ts (attachments metadata).
type FileMeta struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	Type string `json:"type"`
}

// ResultFile is an extension over types.ts: the ready document plus a signed
// URL. The frontend can render order.resultFile.url instead of the hardcoded
// /demo/resultat.docx link.
type ResultFile struct {
	// ID identifies the row in order_files (per-file download links).
	ID        int64      `json:"id,omitempty"`
	Name      string     `json:"name"`
	Size      int64      `json:"size"`
	Type      string     `json:"type"`
	URL       string     `json:"url"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
}

// QuoteLine mirrors QuoteLine from types.ts.
type QuoteLine struct {
	Label    string `json:"label"`
	Value    string `json:"value"`
	Hint     string `json:"hint,omitempty"`
	Discount bool   `json:"discount,omitempty"`
}

// Quote mirrors Quote from types.ts and is returned by POST /api/quote.
type Quote struct {
	Product       string      `json:"product"`
	UnitPrice     int         `json:"unitPrice"`
	Units         int         `json:"units"`
	Works         int         `json:"works"`
	Subtotal      int         `json:"subtotal"`
	DiscountPct   int         `json:"discountPct"`
	Discount      int         `json:"discount"`
	PromoCode     *string     `json:"promoCode"`
	PromoPct      int         `json:"promoPct"`
	PromoDiscount int         `json:"promoDiscount"`
	Total         int         `json:"total"`
	Lines         []QuoteLine `json:"lines"`
}

// OrderEvent mirrors OrderEvent from types.ts (timeline entry, newest first).
type OrderEvent struct {
	At    string `json:"at"`
	Actor string `json:"actor"`
	Text  string `json:"text"`
}

// Order mirrors Order from types.ts.
type Order struct {
	Number     string    `json:"number"`
	Token      string    `json:"token,omitempty"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
	SenderName string    `json:"senderName"`
	Product    string    `json:"product"`
	Works      int       `json:"works"`
	Units      int       `json:"units"`
	Topics     string    `json:"topics"`
	Notes      string    `json:"notes"`
	// WorkKind is what exactly the work is for "Другой док." (СРС, СӨЖ, РГР...).
	WorkKind       string      `json:"workKind"`
	Language       string      `json:"language"`
	Institute      string      `json:"institute"`
	Department     string      `json:"department"`
	StudentName    string      `json:"studentName"`
	DisciplineCode string      `json:"disciplineCode"`
	TeacherName    string      `json:"teacherName"`
	Attachments    []FileMeta  `json:"attachments"`
	ContactName    string      `json:"contactName"`
	ContactType    string      `json:"contactType"`
	Contact        string      `json:"contact"`
	Amount         int         `json:"amount"`
	QuoteLines     []QuoteLine `json:"quoteLines"`

	PromoCode        *string `json:"promoCode,omitempty"`
	PromoDiscountPct *int    `json:"promoDiscountPct,omitempty"`

	ClientNote            string  `json:"clientNote"`
	PaymentRejectedReason *string `json:"paymentRejectedReason,omitempty"`
	OperatorNotes         *string `json:"operatorNotes,omitempty"`

	ClaimedAt   *time.Time `json:"claimedAt,omitempty"`
	ConfirmedAt *time.Time `json:"confirmedAt,omitempty"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`

	Events []OrderEvent `json:"events"`

	// Extensions over types.ts (safe for the frontend to ignore). ResultFile is
	// the first document, ResultFiles keeps the whole set (an order with several
	// works produces one document per work).
	ResultFile  *ResultFile   `json:"resultFile,omitempty"`
	ResultFiles []ResultFile  `json:"resultFiles,omitempty"`

	// RevisionNote holds the corrections to apply on a rework: written by the
	// client, edited by the operator before the order goes back to the agent.
	RevisionNote        string     `json:"revisionNote,omitempty"`
	RevisionRequestedAt *time.Time `json:"revisionRequestedAt,omitempty"`

	// AgentCostUSD is what the generation runs of this order cost in total;
	// AgentCostKZT is the same amount in tenge at the current rate (the panel
	// shows both).
	AgentCostUSD float64 `json:"agentCostUsd"`
	AgentCostKZT float64 `json:"agentCostKzt"`

	// TelegramChatID links the order to a chat for bot notifications. It is a
	// server-side detail and is never serialized.
	TelegramChatID int64 `json:"-"`

	// TokenHash is the stored hash of the access token, used to authorize
	// client actions. Never serialized.
	TokenHash string `json:"-"`
}

// Redacted returns a copy safe for the public /order/{number} page: the
// operator's private fields and the client's contact details are removed.
func (o *Order) Redacted() *Order {
	if o == nil {
		return nil
	}
	copy := *o
	copy.Contact = ""
	copy.ClientNote = ""
	copy.OperatorNotes = nil
	copy.SenderName = ""
	copy.Token = ""
	return &copy
}

// Review mirrors Review from types.ts.
type Review struct {
	ID          string    `json:"id"`
	OrderNumber string    `json:"orderNumber"`
	Author      string    `json:"author"`
	Institute   string    `json:"institute"`
	Department  string    `json:"department"`
	Discipline  string    `json:"discipline"`
	Product     string    `json:"product"`
	Rating      float64   `json:"rating"`
	Text        string    `json:"text"`
	CreatedAt   time.Time `json:"createdAt"`
	Status      string    `json:"status"`
}

// ReviewInput is the POST /api/reviews body (ReviewInput in api.ts).
type ReviewInput struct {
	OrderNumber string  `json:"orderNumber"`
	Author      string  `json:"author"`
	Institute   string  `json:"institute"`
	Department  string  `json:"department"`
	Discipline  string  `json:"discipline"`
	Product     string  `json:"product"`
	Rating      float64 `json:"rating"`
	Text        string  `json:"text"`
}

// PromoCode mirrors PromoCode from types.ts.
type PromoCode struct {
	ID          string     `json:"id"`
	Code        string     `json:"code"`
	DiscountPct int        `json:"discountPct"`
	AppliesTo   string     `json:"appliesTo"` // first | any
	Products    []string   `json:"products"`  // empty = all products
	MinAmount   *int       `json:"minAmount"`
	MinUnits    *int       `json:"minUnits"`
	UsageLimit  *int       `json:"usageLimit"`
	UsedCount   int        `json:"usedCount"`
	ExpiresAt   *time.Time `json:"expiresAt"`
	Active      bool       `json:"active"`
	CreatedAt   time.Time  `json:"createdAt"`
}

// PromoInput is the admin create/update payload (PromoInput in api.ts).
type PromoInput struct {
	Code        string     `json:"code"`
	DiscountPct int        `json:"discountPct"`
	AppliesTo   string     `json:"appliesTo"`
	Products    []string   `json:"products"`
	MinAmount   *int       `json:"minAmount"`
	MinUnits    *int       `json:"minUnits"`
	UsageLimit  *int       `json:"usageLimit"`
	ExpiresAt   *time.Time `json:"expiresAt"`
	Active      *bool      `json:"active"`
}

// Requisites is the legacy bank-transfer block of Settings.
type Requisites struct {
	Recipient string `json:"recipient"`
	Bank      string `json:"bank"`
	Card      string `json:"card"`
	IBAN      string `json:"iban"`
	Note      string `json:"note"`
}

// Settings mirrors Settings from types.ts.
type Settings struct {
	Requisites       Requisites `json:"requisites"`
	PaymentQr        *string    `json:"paymentQr"`
	PaymentRecipient string     `json:"paymentRecipient"`
	SupportTelegram  string     `json:"supportTelegram"`
	Maintenance      bool       `json:"maintenance"`
	QueuePaused      bool       `json:"queuePaused"`
	// PaymentLink is the Kaspi QR deep link behind the «Оплатить через Kaspi»
	// button (empty = the Mini App shows the QR image instead).
	PaymentLink string `json:"paymentLink"`
}

// PricingProduct mirrors PricingProduct from types.ts.
type PricingProduct struct {
	Code      string `json:"code"`
	Min       int    `json:"min"`
	Max       int    `json:"max"`
	UnitPrice int    `json:"unitPrice"`
	Active    bool   `json:"active"`
}

// VolumeDiscount mirrors the entries of PricingConfig.discounts.
type VolumeDiscount struct {
	Min int `json:"min"`
	Pct int `json:"pct"`
}

// PricingConfig mirrors PricingConfig from types.ts.
type PricingConfig struct {
	Products  []PricingProduct `json:"products"`
	Discounts []VolumeDiscount `json:"discounts"`
}

// UnitPriceFor returns the configured unit price for a product code.
func (p PricingConfig) UnitPriceFor(code string) (int, bool) {
	for _, product := range p.Products {
		if product.Code == code {
			return product.UnitPrice, true
		}
	}
	return 0, false
}

// TierFor returns the volume discount percentage for a works count: only the
// highest reached tier applies (mirrors the tier lookup in pricing.ts).
func (p PricingConfig) TierFor(works int) int {
	pct := 0
	bestMin := -1
	for _, discount := range p.Discounts {
		if works >= discount.Min && discount.Min > bestMin {
			bestMin = discount.Min
			pct = discount.Pct
		}
	}
	return pct
}

// AdminSession mirrors AdminSession from types.ts.
type AdminSession struct {
	OperatorID string    `json:"operatorId"`
	Name       string    `json:"name"`
	Since      time.Time `json:"since"`
}

// ClientSession is the Telegram identity of a logged-in client. Orders created
// with this session are attached to the chat, so status messages arrive in the
// bot automatically.
type ClientSession struct {
	ChatID   int64     `json:"chatId"`
	Username string    `json:"username"`
	Since    time.Time `json:"since"`
	// OperatorID/OperatorName are set when the chat is bound to an operator
	// (the Mini App then shows the operator panel).
	OperatorID   string `json:"operatorId,omitempty"`
	OperatorName string `json:"operatorName,omitempty"`
}

// Stats is the GET /api/admin/stats payload. The names match the KPI blocks
// of AdminOrdersPage / AdminStatsPage (see README section 7.2).
type Stats struct {
	OrdersByStatus           map[string]int `json:"ordersByStatus"`
	OrdersTotal              int            `json:"ordersTotal"`
	ConfirmedRevenue         int            `json:"confirmedRevenue"` // queued|generating|completed
	RevenueToday             int            `json:"revenueToday"`     // confirmed today
	CompletedToday           int            `json:"completedToday"`
	QueueDepth               int            `json:"queueDepth"`               // queued + generating
	OldestQueuedMinutes      float64        `json:"oldestQueuedMinutes"`      // -1 when the queue is empty
	QueueWarning             bool           `json:"queueWarning"`             // oldest queued order > 30 min
	AverageGenerationSeconds float64        `json:"averageGenerationSeconds"` // -1 when unknown
	FailureRatePct           float64        `json:"failureRatePct"`
	DiskUsedBytes            int64          `json:"diskUsedBytes"`
	PendingReviews           int            `json:"pendingReviews"`
	ActivePromos             int            `json:"activePromos"`
	// Generation spend (the model provider bills in USD) and the net revenue:
	// the operator panel shows both currencies and what is left after the spend.
	AgentCostUSD float64   `json:"agentCostUsd"`
	AgentCostKZT float64   `json:"agentCostKzt"`
	USDRate      float64   `json:"usdRate"`
	NetRevenue   int       `json:"netRevenue"`
	GeneratedAt  time.Time `json:"generatedAt"`
}
