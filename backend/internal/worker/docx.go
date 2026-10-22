package worker

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/srunal/srunal-go/internal/catalog"
	"github.com/srunal/srunal-go/internal/domain"
	"github.com/srunal/srunal-go/internal/format"
)

// BuildDocument produces a minimal but valid .docx (OPC package) with the title
// page and the task brief of the order. Word, LibreOffice and Google Docs all
// open it. This replaces the demo file of the frontend and gives the operator a
// real artefact until the external generation service is wired in.
func BuildDocument(order *domain.Order) ([]byte, error) {
	product := catalog.Get(order.Product)

	paragraphs := []paragraph{
		{style: "Title", text: "«SruNal KZ» • Satbayev University"},
		{style: "Subtitle", text: "Заказ " + order.Number},
		{style: "", text: "Вид работы: " + product.Name},
		{style: "", text: "Тема: " + order.Topics},
		{style: "", text: "Заказчик: " + order.StudentName},
		{style: "", text: "Институт: " + order.Institute + ", кафедра: " + order.Department},
		{style: "", text: "Дисциплина: " + order.DisciplineCode + ", преподаватель: " + order.TeacherName},
		{style: "", text: "Язык: " + languageLabel(order.Language)},
		{style: "", text: "Объём: " + unitsLabel(order, product)},
		{style: "", text: "Сумма: " + format.Money(order.Amount)},
		{style: "", text: "Дата: " + time.Now().Format("02.01.2006 15:04")},
		{style: "", text: ""},
		{style: "Heading1", text: "Задание"},
		{style: "", text: order.Topics},
		{style: "", text: ""},
		{style: "Heading1", text: "Требования"},
		{style: "", text: "Оформление по ГОСТ РК. Титульный лист и рамки по стандарту Satbayev University."},
	}
	if strings.TrimSpace(order.Notes) != "" {
		paragraphs = append(paragraphs, paragraph{style: "", text: "Примечания: " + order.Notes})
	}
	paragraphs = append(paragraphs,
		paragraph{style: "Heading1", text: "Структура работы"},
		paragraph{style: "", text: "1. Титульный лист."},
		paragraph{style: "", text: "2. Введение и постановка задачи."},
		paragraph{style: "", text: "3. Основная часть с расчётами и выводами."},
		paragraph{style: "", text: "4. Заключение."},
		paragraph{style: "", text: "5. Список использованных источников (по ГОСТ 7.1-2003)."},
	)

	documentXML, err := renderDocument(paragraphs)
	if err != nil {
		return nil, err
	}

	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)

	parts := []struct {
		name string
		body string
	}{
		{"[Content_Types].xml", contentTypesXML},
		{"_rels/.rels", relsXML},
		{"word/document.xml", documentXML},
		{"docProps/core.xml", fmt.Sprintf(corePropsXML, order.Number, time.Now().UTC().Format(time.RFC3339))},
	}
	for _, part := range parts {
		writer, err := archive.Create(part.name)
		if err != nil {
			return nil, fmt.Errorf("worker: docx zip %s: %w", part.name, err)
		}
		if _, err := writer.Write([]byte(part.body)); err != nil {
			return nil, fmt.Errorf("worker: docx write %s: %w", part.name, err)
		}
	}
	if err := archive.Close(); err != nil {
		return nil, fmt.Errorf("worker: docx close: %w", err)
	}
	return buffer.Bytes(), nil
}

type paragraph struct {
	style string
	text  string
}

// renderDocument builds word/document.xml with one paragraph per line.
func renderDocument(paragraphs []paragraph) (string, error) {
	var body strings.Builder
	for _, entry := range paragraphs {
		body.WriteString("<w:p>")
		if entry.style != "" {
			body.WriteString(fmt.Sprintf(
				`<w:pPr><w:pStyle w:val="%s"/></w:pPr>`, entry.style))
		}
		if entry.text != "" {
			escaped, err := escapeXML(entry.text)
			if err != nil {
				return "", err
			}
			body.WriteString(`<w:r><w:rPr><w:sz w:val="28"/></w:rPr><w:t xml:space="preserve">`)
			body.WriteString(escaped)
			body.WriteString("</w:t></w:r>")
		}
		body.WriteString("</w:p>")
	}
	return strings.Replace(documentXMLTemplate, "{{BODY}}", body.String(), 1), nil
}

func escapeXML(value string) (string, error) {
	var out bytes.Buffer
	if err := xml.EscapeText(&out, []byte(value)); err != nil {
		return "", fmt.Errorf("worker: escape: %w", err)
	}
	return out.String(), nil
}

func unitsLabel(order *domain.Order, product catalog.Product) string {
	if product.Fixed {
		return "1 работа"
	}
	word := format.Plural(order.Units, product.UnitOne, product.UnitFew, product.UnitMany)
	return fmt.Sprintf("%d %s", order.Units, word)
}

func languageLabel(code string) string {
	switch code {
	case "kk":
		return "Қазақша"
	case "en":
		return "English"
	default:
		return "Русский"
	}
}

// removeFile is os.Remove with a tolerant contract (used for expired results).
func removeFile(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

const documentXMLTemplate = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
<w:body>{{BODY}}<w:sectPr><w:pgSz w:w="11906" w:h="16838"/><w:pgMar w:top="1134" w:right="850" w:bottom="1134" w:left="1701"/></w:sectPr></w:body>
</w:document>`

const contentTypesXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
<Default Extension="xml" ContentType="application/xml"/>
<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
<Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/>
</Types>`

const relsXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/>
</Relationships>`

const corePropsXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:dcterms="http://purl.org/dc/terms/" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
<dc:title>SruNal KZ • %s</dc:title>
<dc:creator>SruNal KZ</dc:creator>
<dcterms:created xsi:type="dcterms:W3CDTF">%s</dcterms:created>
</cp:coreProperties>`
