package validate

import (
	"strings"
	"testing"

	"github.com/srunal/srunal-go/internal/domain"
)

// validForm is a form that passes every rule.
func validForm() OrderForm {
	return OrderForm{
		Product:        "essay",
		Works:          1,
		Units:          10,
		Topics:         "Анализ методов шифрования",
		Notes:          "",
		Language:       "ru",
		Institute:      "ИЦИТ",
		Department:     "Программная инженерия",
		StudentName:    "Ануар Сатпаев",
		DisciplineCode: "CS-101",
		TeacherName:    "доц. Сериков А.Б.",
		Attachments:    []domain.FileMeta{},
		ContactName:    "Ануар",
		Consent:        true,
	}
}

// TestFormAcceptsValidInput guards against false positives.
func TestFormAcceptsValidInput(t *testing.T) {
	form := validForm()
	if fields := Form(&form); len(fields) != 0 {
		t.Fatalf("valid form rejected: %v", fields)
	}
}

// TestFormMessages pins the exact strings the frontend renders.
func TestFormMessages(t *testing.T) {
	form := validForm()
	form.Product = ""
	form.Works = 0
	form.Units = 2
	form.Topics = "ab"
	form.Language = "de"
	form.Institute = ""
	form.Department = ""
	form.StudentName = ""
	form.DisciplineCode = ""
	form.TeacherName = ""
	form.ContactName = ""
	form.Consent = false
	form.Attachments = []domain.FileMeta{
		{Name: "a.pdf", Size: 1, Type: "application/pdf"},
		{Name: "b.pdf", Size: 1, Type: "application/pdf"},
		{Name: "c.pdf", Size: 1, Type: "application/pdf"},
		{Name: "d.pdf", Size: 1, Type: "application/pdf"},
	}

	fields := Form(&form)
	wants := map[string]string{
		"product":        "Выберите вид работы",
		"works":          "Укажите количество работ",
		"topics":         "Укажите тему работы (минимум 3 символа)",
		"language":       "Выберите язык работы",
		"institute":      "Выберите институт",
		"department":     "Укажите кафедру",
		"studentName":    "Укажите ФИО студента",
		"disciplineCode": "Укажите код дисциплины",
		"teacherName":    "Укажите ФИО преподавателя",
		"contactName":    "Укажите имя",
		"consent":        "Нужно согласие, чтобы создать заказ",
		"attachments":    "Не больше 3 файлов",
	}
	for field, want := range wants {
		if fields[field] != want {
			t.Errorf("%s: got %q, want %q", field, fields[field], want)
		}
	}
}

// TestUnitsRangeMessage checks the product-specific range text.
func TestUnitsRangeMessage(t *testing.T) {
	form := validForm()
	form.Units = 3
	fields := Form(&form)
	if fields["units"] != "Допустимо от 4 до 40 страниц" {
		t.Errorf("essay range = %q", fields["units"])
	}

	form.Product = "presentation"
	form.Units = 31
	fields = Form(&form)
	if fields["units"] != "Допустимо от 5 до 30 слайдов" {
		t.Errorf("presentation range = %q", fields["units"])
	}

	// Fixed products ignore units entirely.
	form.Product = "lab"
	form.Units = 99
	if fields = Form(&form); fields["units"] != "" {
		t.Errorf("lab must ignore units, got %q", fields["units"])
	}
}

// TestFileRules checks size, count and type rules.
func TestFileRules(t *testing.T) {
	form := validForm()
	form.Attachments = []domain.FileMeta{{Name: "big.pdf", Size: 6 * 1024 * 1024, Type: "application/pdf"}}
	if fields := Form(&form); fields["attachments"] != "Файл больше 5 МБ" {
		t.Errorf("size rule = %q", fields["attachments"])
	}

	form.Attachments = []domain.FileMeta{{Name: "script.exe", Size: 100, Type: "application/x-msdownload"}}
	if fields := Form(&form); fields["attachments"] != "Этот формат не поддерживается" {
		t.Errorf("type rule = %q", fields["attachments"])
	}

	// The dropzone sometimes reports an empty mime type: the extension rules.
	form.Attachments = []domain.FileMeta{{Name: "material.DOCX", Size: 100, Type: ""}}
	if fields := Form(&form); fields["attachments"] != "" {
		t.Errorf("docx by extension rejected: %q", fields["attachments"])
	}
}

// TestWorkKindRule checks the extra field of "Другой док.".
func TestWorkKindRule(t *testing.T) {
	form := validForm()
	form.Product = "other"
	form.WorkKind = ""
	if fields := Form(&form); fields["workKind"] != "Укажите, какая это работа (например, СРС, СӨЖ, практическая)" {
		t.Errorf("missing kind = %q", fields["workKind"])
	}

	form.WorkKind = "СРС"
	if fields := Form(&form); fields["workKind"] != "" {
		t.Errorf("valid kind rejected: %q", fields["workKind"])
	}

	form.WorkKind = strings.Repeat("я", 121)
	if fields := Form(&form); fields["workKind"] != "Не больше 120 символов" {
		t.Errorf("long kind = %q", fields["workKind"])
	}

	// Other products never ask for the kind.
	essay := validForm()
	essay.WorkKind = ""
	if fields := Form(&essay); fields["workKind"] != "" {
		t.Errorf("essay must not require the kind: %q", fields["workKind"])
	}
}

// TestTopicsLimit checks the 5000 character ceiling.
func TestTopicsLimit(t *testing.T) {
	form := validForm()
	form.Topics = strings.Repeat("я", 5001)
	if fields := Form(&form); fields["topics"] != "Не больше 5000 символов" {
		t.Errorf("topics limit = %q", fields["topics"])
	}
}

// TestSenderName pins the «Ануар А.» rule.
func TestSenderName(t *testing.T) {
	cases := []struct {
		name  string
		valid bool
	}{
		{"Ануар А.", true},
		{"Ануар А", true},
		{"Мадина К.", true},
		{"Айнура Бек", false}, // the last word must be a single letter
		{"Ануар", false},      // needs two words
		{"", false},
		{"А Б.", false}, // the first word must be a name
		{"Ануар  А.  ", true},
	}
	for _, testCase := range cases {
		message := SenderName(testCase.name)
		if testCase.valid && message != "" {
			t.Errorf("SenderName(%q) rejected: %q", testCase.name, message)
		}
		if !testCase.valid && message == "" {
			t.Errorf("SenderName(%q) accepted, expected a message", testCase.name)
		}
		if !testCase.valid && message != "" &&
			message != "Укажите как в Kaspi: имя и фамилия одной буквой, например «Ануар А.»" &&
			message != "Не больше 120 символов" {
			t.Errorf("SenderName(%q) message = %q", testCase.name, message)
		}
	}
}
