// Package catalog is the Go mirror of src/lib/catalog.ts plus src/lib/validate.ts
// rules: product definitions, institutes, languages, status metadata.
package catalog

// Product mirrors ProductDef from catalog.ts. Name and unit words are used to
// build the quote lines and promo messages exactly as the frontend renders them.
type Product struct {
	Code         string
	Name         string
	Short        string
	UnitOne      string // 1 страница
	UnitFew      string // 2 страницы
	UnitMany     string // 5 страниц (genitive plural, also used in "от 4 до 40 ...")
	UnitTitle    string // Nominative plural for headings: "Страницы"
	Min          int
	Max          int
	Fixed        bool
	PricePerUnit int
}

// Products is ordered exactly like PRODUCTS in catalog.ts.
var Products = []Product{
	{
		Code: "essay", Name: "Реферат", Short: "Реферат",
		UnitOne: "страница", UnitFew: "страницы", UnitMany: "страниц", UnitTitle: "страницы",
		Min: 4, Max: 40, Fixed: false, PricePerUnit: 80,
	},
	{
		Code: "lab", Name: "Лабораторная", Short: "Лабораторная",
		UnitOne: "работа", UnitFew: "работы", UnitMany: "работ", UnitTitle: "работы",
		Min: 1, Max: 1, Fixed: true, PricePerUnit: 1000,
	},
	{
		Code: "presentation", Name: "Презентация", Short: "Презентация",
		UnitOne: "слайд", UnitFew: "слайда", UnitMany: "слайдов", UnitTitle: "слайды",
		Min: 5, Max: 30, Fixed: false, PricePerUnit: 240,
	},
	{
		Code: "other", Name: "Другой док.", Short: "СРС / СРСП / другое",
		UnitOne: "работа", UnitFew: "работы", UnitMany: "работ", UnitTitle: "работы",
		Min: 1, Max: 1, Fixed: true, PricePerUnit: 1000,
	},
}

// Codes lists every product code, handy for validation.
func Codes() []string {
	out := make([]string, 0, len(Products))
	for _, product := range Products {
		out = append(out, product.Code)
	}
	return out
}

// Get returns the product by code. Unknown codes fall back to the first
// product, mirroring getProduct() in catalog.ts.
func Get(code string) Product {
	for _, product := range Products {
		if product.Code == code {
			return product
		}
	}
	return Products[0]
}

// Known reports whether the code is a real product.
func Known(code string) bool {
	for _, product := range Products {
		if product.Code == code {
			return true
		}
	}
	return false
}

// UnitLabel is the short unit used in the price line: "стр." / "слайд" / "раб.".
func (p Product) UnitLabel() string {
	switch p.UnitOne {
	case "страница":
		return "стр."
	case "слайд":
		return "слайд"
	default:
		return "раб."
	}
}

// Institutes are the short names accepted by the order form (catalog.ts).
// The seeds also contain "ИМА", so the server only requires a non-empty value.
var Institutes = []string{"ИЦИТ", "ГМИ", "ИФТ", "ИАС", "Другая кафедра Satbayev Univ"}

// Languages accepted by the order form.
var Languages = []string{"ru", "kk", "en"}

// Status metadata for the UI, mirroring STATUS_META in catalog.ts.
type StatusMeta struct {
	Label string
	Badge string
}

var Statuses = map[string]StatusMeta{
	"awaiting_payment": {"Ожидает оплату", "warning"},
	"payment_claimed":  {"Оплата проверяется", "warning"},
	"payment_rejected": {"Оплата отклонена", "error"},
	"queued":           {"В очереди", "primary"},
	"generating":       {"Генерируется", "primary"},
	"completed":        {"Готово", "success"},
	"failed_final":     {"Проблема", "error"},
	"expired":          {"Истёк", "neutral"},
	"cancelled":        {"Отменён", "neutral"},
}

// AllStatuses is the canonical status order (UI chips, KPI cards).
var AllStatuses = []string{
	"awaiting_payment", "payment_claimed", "payment_rejected",
	"queued", "generating", "completed",
	"failed_final", "expired", "cancelled",
}

// ReviewStatuses.
var ReviewStatuses = []string{"pending", "published", "rejected"}

// File limits from catalog.ts.
const (
	MaxFiles          = 3
	MaxFileSize       = 5 * 1024 * 1024
	MinTopicsLen      = 3
	MaxTopicsLen      = 5000
	MaxTextLen        = 200 // department/name/discipline/teacher
	MaxNotesLen       = 5000
	MaxContactNameLen = 120
	MaxSenderWords    = 5 // sanity limit, not in the frontend
)
