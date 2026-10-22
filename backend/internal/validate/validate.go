// Package validate mirrors src/lib/validate.ts. Field keys match the DOM ids
// and the JSON body of POST /api/orders; every message is the exact Russian
// string the client already renders.
package validate

import (
	"strings"
	"unicode/utf8"

	"github.com/srunal/srunal-go/internal/catalog"
	"github.com/srunal/srunal-go/internal/domain"
	"github.com/srunal/srunal-go/internal/format"
)

// OrderForm is the POST /api/orders body (OrderFormState in validate.ts).
type OrderForm struct {
	Product string `json:"product"`
	Works   int    `json:"works"`
	Units   int    `json:"units"`
	// WorkKind is required for "other": СРС, СӨЖ, практическая работа...
	WorkKind       string            `json:"workKind"`
	Topics         string            `json:"topics"`
	Notes          string            `json:"notes"`
	Language       string            `json:"language"`
	Institute      string            `json:"institute"`
	Department     string            `json:"department"`
	StudentName    string            `json:"studentName"`
	DisciplineCode string            `json:"disciplineCode"`
	TeacherName    string            `json:"teacherName"`
	Attachments    []domain.FileMeta `json:"attachments"`
	ContactName    string            `json:"contactName"`
	Consent        bool              `json:"consent"`
	PromoCode      *string           `json:"promoCode"`
}

// MaxWorks is a server-side sanity cap (the mock has none). Keeps amounts far
// from any overflow and blocks junk payloads; the message matches the mock.
const MaxWorks = 1000

// Form returns field -> message for every invalid field, like validateForm().
func Form(form *OrderForm) map[string]string {
	errors := map[string]string{}

	if !catalog.Known(form.Product) {
		errors["product"] = "Выберите вид работы"
	}

	if form.Works < 1 || form.Works > MaxWorks {
		errors["works"] = "Укажите количество работ"
	}

	if catalog.Known(form.Product) {
		product := catalog.Get(form.Product)
		if !product.Fixed {
			if form.Units < product.Min || form.Units > product.Max {
				errors["units"] = "Допустимо от " +
					format.GroupThousands(product.Min) + " до " +
					format.GroupThousands(product.Max) + " " + product.UnitMany
			}
		}
	}

	topics := strings.TrimSpace(form.Topics)
	if utf8.RuneCountInString(topics) < catalog.MinTopicsLen {
		errors["topics"] = "Укажите тему работы (минимум 3 символа)"
	} else if utf8.RuneCountInString(topics) > catalog.MaxTopicsLen {
		errors["topics"] = "Не больше 5000 символов"
	}

	if utf8.RuneCountInString(form.Notes) > catalog.MaxNotesLen {
		errors["notes"] = "Не больше 5000 символов"
	}

	// "Другой док." must say what it is (СРС, СӨЖ, практическая работа...): the
	// value lands on the title page and drives the generation.
	if form.Product == domain.ProductOther {
		kind := strings.TrimSpace(form.WorkKind)
		if utf8.RuneCountInString(kind) < 2 {
			errors["workKind"] = "Укажите, какая это работа (например, СРС, СӨЖ, практическая)"
		} else if utf8.RuneCountInString(kind) > 120 {
			errors["workKind"] = "Не больше 120 символов"
		}
	}

	if !validLanguage(form.Language) {
		errors["language"] = "Выберите язык работы"
	}

	textFields := []struct {
		key      string
		value    string
		required string
	}{
		{"institute", form.Institute, "Выберите институт"},
		{"department", form.Department, "Укажите кафедру"},
		{"studentName", form.StudentName, "Укажите ФИО студента"},
		{"disciplineCode", form.DisciplineCode, "Укажите код дисциплины"},
		{"teacherName", form.TeacherName, "Укажите ФИО преподавателя"},
	}
	for _, field := range textFields {
		value := strings.TrimSpace(field.value)
		if value == "" {
			errors[field.key] = field.required
		} else if utf8.RuneCountInString(value) > catalog.MaxTextLen {
			errors[field.key] = "Не больше 200 символов"
		}
	}

	if len(form.Attachments) > catalog.MaxFiles {
		errors["attachments"] = "Не больше 3 файлов"
	} else {
		for _, file := range form.Attachments {
			if file.Size > catalog.MaxFileSize {
				errors["attachments"] = "Файл больше 5 МБ"
				break
			}
			if !AllowedFileType(file.Type, file.Name) {
				errors["attachments"] = "Этот формат не поддерживается"
				break
			}
		}
	}

	contactName := strings.TrimSpace(form.ContactName)
	if contactName == "" {
		errors["contactName"] = "Укажите имя"
	} else if utf8.RuneCountInString(contactName) > catalog.MaxContactNameLen {
		errors["contactName"] = "Не больше 120 символов"
	}

	if !form.Consent {
		errors["consent"] = "Нужно согласие, чтобы создать заказ"
	}

	return errors
}

// SenderName validates the "Имя Ф." format used when the client claims a
// payment: at least two words, the last one up to two characters.
func SenderName(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "Укажите как в Kaspi: имя и фамилия одной буквой, например «Ануар А.»"
	}
	if utf8.RuneCountInString(trimmed) > catalog.MaxContactNameLen {
		return "Не больше 120 символов"
	}
	words := strings.Fields(trimmed)
	if len(words) < 2 || len(words) > catalog.MaxSenderWords {
		return "Укажите как в Kaspi: имя и фамилия одной буквой, например «Ануар А.»"
	}
	last := strings.TrimRight(words[len(words)-1], ".")
	if utf8.RuneCountInString(last) < 1 || utf8.RuneCountInString(last) > 2 {
		return "Укажите как в Kaspi: имя и фамилия одной буквой, например «Ануар А.»"
	}
	// The first word must look like a name, not a single letter.
	if utf8.RuneCountInString(words[0]) < 2 {
		return "Укажите как в Kaspi: имя и фамилия одной буквой, например «Ануар А.»"
	}
	return ""
}

// AllowedFileType mirrors ACCEPTED_FILE_TYPES in catalog.ts. The mime type is
// checked when present; the extension is the fallback the dropzone itself uses.
func AllowedFileType(mime, name string) bool {
	switch mime {
	case "application/pdf",
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		"text/plain",
		"image/png",
		"image/jpeg":
		return true
	}
	if mime != "" && mime != "application/octet-stream" {
		return false
	}
	return AllowedExtension(name)
}

// AllowedExtension checks the file name suffix.
func AllowedExtension(name string) bool {
	lower := strings.ToLower(name)
	for _, suffix := range []string{".pdf", ".docx", ".txt", ".png", ".jpg", ".jpeg"} {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}
	return false
}

// Review validates POST /api/reviews (10-1000 chars of text, rating 1-5).
func Review(input *domain.ReviewInput) map[string]string {
	errors := map[string]string{}

	if strings.TrimSpace(input.OrderNumber) == "" {
		errors["orderNumber"] = "Укажите номер заказа"
	}
	author := strings.TrimSpace(input.Author)
	if author == "" {
		errors["author"] = "Укажите имя"
	} else if utf8.RuneCountInString(author) > catalog.MaxContactNameLen {
		errors["author"] = "Не больше 120 символов"
	}
	if input.Rating < 1 || input.Rating > 5 {
		errors["rating"] = "Поставьте оценку от 1 до 5"
	}
	text := strings.TrimSpace(input.Text)
	if utf8.RuneCountInString(text) < 10 {
		errors["text"] = "Расскажите подробнее (минимум 10 символов)"
	} else if utf8.RuneCountInString(text) > 1000 {
		errors["text"] = "Не больше 1000 символов"
	}
	if strings.TrimSpace(input.Discipline) == "" {
		errors["discipline"] = "Укажите дисциплину"
	}
	if !catalog.Known(input.Product) {
		errors["product"] = "Выберите вид работы"
	}

	return errors
}

// Pricing validates the admin pricing form: integer prices > 0, min <= max.
func Pricing(config *domain.PricingConfig) map[string]string {
	errors := map[string]string{}
	if len(config.Products) == 0 {
		errors["products"] = "Добавьте хотя бы один вид работы"
	}
	seen := map[string]bool{}
	for _, product := range config.Products {
		label := product.Code
		if !catalog.Known(product.Code) {
			errors["products"] = "Неизвестный вид работы: " + product.Code
			continue
		}
		if seen[product.Code] {
			errors["products"] = "Вид работы указан дважды: " + product.Code
			continue
		}
		seen[product.Code] = true
		if product.UnitPrice <= 0 {
			errors["unitPrice."+label] = "Цена должна быть больше 0"
		}
		if product.Min < 1 {
			errors["min."+label] = "Минимум не меньше 1"
		}
		if product.Max < product.Min {
			errors["max."+label] = "Максимум должен быть не меньше минимума"
		}
	}
	for _, discount := range config.Discounts {
		if discount.Min < 1 {
			errors["discounts"] = "Порог скидки не меньше 1 работы"
			break
		}
		if discount.Pct < 1 || discount.Pct > 90 {
			errors["discounts"] = "Скидка должна быть от 1 до 90%"
			break
		}
	}
	return errors
}

// Promo validates the promo-code admin form (code >= 3 chars, 1-90 %).
func Promo(input *domain.PromoInput) map[string]string {
	errors := map[string]string{}
	code := strings.ToUpper(strings.TrimSpace(input.Code))
	if utf8.RuneCountInString(code) < 3 {
		errors["code"] = "Код промокода - минимум 3 символа"
	}
	if input.DiscountPct < 1 || input.DiscountPct > 90 {
		errors["discountPct"] = "Скидка должна быть от 1 до 90%"
	}
	if input.AppliesTo != "first" && input.AppliesTo != "any" {
		errors["appliesTo"] = "Выберите условие действия"
	}
	for _, product := range input.Products {
		if !catalog.Known(product) {
			errors["products"] = "Неизвестный вид работы: " + product
			break
		}
	}
	if input.MinAmount != nil && *input.MinAmount < 0 {
		errors["minAmount"] = "Сумма не может быть отрицательной"
	}
	if input.MinUnits != nil && *input.MinUnits < 1 {
		errors["minUnits"] = "Объём не меньше 1"
	}
	if input.UsageLimit != nil && *input.UsageLimit < 1 {
		errors["usageLimit"] = "Лимит не меньше 1"
	}
	return errors
}

func validLanguage(code string) bool {
	for _, language := range catalog.Languages {
		if language == code {
			return true
		}
	}
	return false
}
