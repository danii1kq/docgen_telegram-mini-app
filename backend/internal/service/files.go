package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/srunal/srunal-go/internal/domain"
	"github.com/srunal/srunal-go/internal/store"
)

// Signed result links: /api/orders/{number}/result?sig=<hmac>. The signature is
// derived from the server pepper, so the URL is unguessable but stable, and it
// works in a plain <a href> without JavaScript headers.
const resultSignatureContext = "result:"

// ResultSignature returns the URL signature of an order result.
func (s *Service) ResultSignature(number string) string {
	mac := hmac.New(sha256.New, []byte(s.Cfg.TokenPepper))
	mac.Write([]byte(resultSignatureContext + strings.ToUpper(strings.TrimSpace(number))))
	return hex.EncodeToString(mac.Sum(nil))[:32]
}

// VerifyResultSignature checks a signature from the query string.
func (s *Service) VerifyResultSignature(number, signature string) bool {
	expected := s.ResultSignature(number)
	return hmac.Equal([]byte(expected), []byte(strings.ToLower(strings.TrimSpace(signature))))
}

// ResultURL builds the download link of a finished order (the first document of
// the newest set; ResultURLFor targets one file of a multi-work order).
func (s *Service) ResultURL(number string) string {
	return s.ResultURLFor(number, 0)
}

// ResultURLFor builds a signed link to one result document (fileID 0 = the
// primary one). The signature covers the order, and the file must belong to it,
// so no extra signature is needed per file.
func (s *Service) ResultURLFor(number string, fileID int64) string {
	link := fmt.Sprintf("%s/api/orders/%s/result?sig=%s",
		s.Cfg.PublicBaseURL, strings.ToUpper(strings.TrimSpace(number)), s.ResultSignature(number))
	if fileID > 0 {
		link += "&file=" + strconv.FormatInt(fileID, 10)
	}
	return link
}

// attachResultURL fills Order.resultFile.url and every url of resultFiles.
func (s *Service) attachResultURL(order *domain.Order) {
	if order == nil {
		return
	}
	if order.ResultFile != nil {
		order.ResultFile.URL = s.ResultURLFor(order.Number, order.ResultFile.ID)
	}
	for i := range order.ResultFiles {
		order.ResultFiles[i].URL = s.ResultURLFor(order.Number, order.ResultFiles[i].ID)
	}
}

// WriteResultDocument stores the generated .docx and returns the file row the
// store must persist once the order flips to completed.
func (s *Service) WriteResultDocument(number string, content []byte) (store.OrderFileRow, error) {
	name := fmt.Sprintf("resultat_%s.docx", strings.ReplaceAll(number, "-", "_"))
	return s.WriteResultFile(number, name, docxMimeType, content)
}

// WriteResultFile stores any generated document (docx or pptx) in the storage
// and returns the row to persist. The extension of name decides the file on
// disk; mime is what the download endpoint reports. Multi-work orders pass an
// indexed name (resultat_DG-260391_2.docx) so every document keeps its own file.
func (s *Service) WriteResultFile(number, name, mime string, content []byte) (store.OrderFileRow, error) {
	extension := strings.ToLower(filepath.Ext(name))
	if extension == "" {
		extension = ".docx"
	}
	key := filepath.Join("results", resultStorageName(number, name, extension))
	path := s.StoragePath(key)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return store.OrderFileRow{}, fmt.Errorf("service: write result: %w", err)
	}
	expiresAt := time.Now().UTC().Add(s.Cfg.ResultTTL)
	return store.OrderFileRow{
		Number:     number,
		Kind:       "result",
		FileName:   name,
		SizeBytes:  int64(len(content)),
		MimeType:   mime,
		StorageKey: key,
		ExpiresAt:  &expiresAt,
	}, nil
}

// resultStorageName turns the display name into the storage file name:
// resultat_DG_260391.docx -> DG-260391.docx (the historical layout) and
// resultat_DG_260391_2.docx -> DG-260391_2.docx.
func resultStorageName(number, name, extension string) string {
	upper := strings.ToUpper(strings.TrimSpace(number))
	suffix := ""
	if trimmed := filepath.Base(name); trimmed != "" {
		suffix = strings.TrimSuffix(trimmed, filepath.Ext(trimmed))
		suffix = strings.TrimPrefix(suffix, "resultat_")
		suffix = strings.TrimPrefix(suffix, strings.ReplaceAll(upper, "-", "_"))
	}
	return upper + suffix + extension
}

// ResultFilePath resolves the stored file of an order result.
func (s *Service) ResultFilePath(file store.OrderFileRow) string {
	if file.StorageKey == "" {
		return ""
	}
	return s.StoragePath(file.StorageKey)
}

// GetResultFile returns the ready document (ErrNotFound when there is none).
func (s *Service) GetResultFile(ctx context.Context, number string) (store.OrderFileRow, error) {
	return s.Store.GetResultFile(ctx, NormalizeNumber(number))
}

// ListResultFiles returns the documents of the newest run (one per work).
func (s *Service) ListResultFiles(ctx context.Context, number string) ([]store.OrderFileRow, error) {
	return s.Store.ListResultFiles(ctx, NormalizeNumber(number))
}

// GetResultFileByID resolves one document of an order (per-file links).
func (s *Service) GetResultFileByID(ctx context.Context, number string, id int64) (store.OrderFileRow, error) {
	return s.Store.GetResultFileByID(ctx, NormalizeNumber(number), id)
}

const docxMimeType = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
