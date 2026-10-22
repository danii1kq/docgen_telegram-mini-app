package worker

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/srunal/srunal-go/internal/catalog"
	"github.com/srunal/srunal-go/internal/domain"
)

// TestBuildDocument checks that the generated .docx is a structurally valid
// OPC package containing the order data.
func TestBuildDocument(t *testing.T) {
	promoPct := 10
	order := &domain.Order{
		Number:           "DG-260391",
		Status:           domain.StatusGenerating,
		Product:          "essay",
		Works:            1,
		Units:            10,
		Topics:           "Анализ методов шифрования & защита <данных>",
		Notes:            "Оформление по методичке",
		Language:         "ru",
		Institute:        "ИЦИТ",
		Department:       "Программная инженерия",
		StudentName:      "Ануар Сатпаев",
		DisciplineCode:   "CS-101",
		TeacherName:      "доц. Сериков А.Б.",
		Amount:           720,
		PromoCode:        strPtr("SRUNAL10"),
		PromoDiscountPct: &promoPct,
		CreatedAt:        time.Now().UTC(),
	}

	data, err := BuildDocument(order)
	if err != nil {
		t.Fatalf("BuildDocument: %v", err)
	}
	if len(data) < 1000 {
		t.Fatalf("document too small: %d bytes", len(data))
	}

	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("not a zip archive: %v", err)
	}
	required := map[string]bool{
		"[Content_Types].xml": false,
		"_rels/.rels":         false,
		"word/document.xml":   false,
	}
	for _, file := range archive.File {
		if _, ok := required[file.Name]; ok {
			required[file.Name] = true
		}
	}
	for name, found := range required {
		if !found {
			t.Errorf("missing part %s", name)
		}
	}

	reader, err := archive.Open("word/document.xml")
	if err != nil {
		t.Fatalf("open document.xml: %v", err)
	}
	defer reader.Close()
	var body bytes.Buffer
	if _, err := body.ReadFrom(reader); err != nil {
		t.Fatalf("read document.xml: %v", err)
	}
	xml := body.String()

	if !strings.Contains(xml, "DG-260391") {
		t.Error("order number missing from the document")
	}
	if !strings.Contains(xml, "Анализ методов шифрования") {
		t.Error("topic missing from the document")
	}
	// XML special characters must be escaped, never emitted raw.
	if strings.Contains(xml, "<данных>") {
		t.Error("angle brackets were not escaped")
	}
	if !strings.Contains(xml, "&amp;") {
		t.Error("ampersand was not escaped")
	}
	if !strings.Contains(xml, "720") {
		t.Error("amount missing from the document")
	}
}

// TestUnitsLabel covers the Russian plural forms inside the document.
func TestUnitsLabel(t *testing.T) {
	product := catalog.Get("essay")
	cases := map[int]string{4: "4 страницы", 5: "5 страниц", 21: "21 страница"}
	for units, want := range cases {
		order := &domain.Order{Product: "essay", Units: units}
		if got := unitsLabel(order, product); got != want {
			t.Errorf("units=%d: got %q, want %q", units, got, want)
		}
	}

	lab := catalog.Get("lab")
	if got := unitsLabel(&domain.Order{Product: "lab", Units: 7}, lab); got != "1 работа" {
		t.Errorf("fixed product label = %q", got)
	}
}

func strPtr(value string) *string { return &value }
