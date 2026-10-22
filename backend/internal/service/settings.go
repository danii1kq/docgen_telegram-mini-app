package service

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/srunal/srunal-go/internal/domain"
	"github.com/srunal/srunal-go/internal/validate"
)

// GetSettings returns the public settings. A locally stored QR image is
// exposed as an absolute URL the frontend can put into <img src>.
func (s *Service) GetSettings(ctx context.Context) (domain.Settings, error) {
	settings, err := s.Store.GetSettings(ctx)
	if err != nil {
		return settings, err
	}
	s.ensureQrURL(&settings)
	return settings, nil
}

// SaveSettings is PUT /api/admin/settings.
func (s *Service) SaveSettings(ctx context.Context, settings domain.Settings) (domain.Settings, error) {
	if strings.TrimSpace(settings.PaymentRecipient) == "" {
		return settings, ValidationError(map[string]string{
			"paymentRecipient": "Укажите получателя платежа",
		})
	}
	// The payment link is optional but must be a real URL when it is set: it
	// sits behind the «Оплатить через Kaspi» button in the Mini App.
	if link := strings.TrimSpace(settings.PaymentLink); link != "" {
		parsed, err := url.Parse(link)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return settings, ValidationError(map[string]string{
				"paymentLink": "Ссылка должна начинаться с https:// (например https://qr.kaspi.kz/…)",
			})
		}
		settings.PaymentLink = strings.TrimSpace(link)
	}
	// The old frontend sent the QR as a data URL; keep it usable but bounded.
	if settings.PaymentQr != nil && strings.HasPrefix(*settings.PaymentQr, "data:") {
		if len(*settings.PaymentQr) > 2*1024*1024 {
			return settings, ValidationError(map[string]string{
				"paymentQr": "QR слишком большой, загрузите файл до 1.5 МБ",
			})
		}
	}
	if err := s.Store.SaveSettings(ctx, settings); err != nil {
		return settings, err
	}
	return s.GetSettings(ctx)
}

// ensureQrURL points paymentQr at the stored upload when there is one.
func (s *Service) ensureQrURL(settings *domain.Settings) {
	if settings.PaymentQr != nil && *settings.PaymentQr != "" {
		return // an explicit URL (or data URL) already stored
	}
	path := s.qrFilePath()
	if path == "" {
		settings.PaymentQr = nil // frontend falls back to the bundled poster
		return
	}
	info, err := os.Stat(path)
	if err != nil {
		settings.PaymentQr = nil
		return
	}
	url := fmt.Sprintf("%s/api/settings/payment-qr?v=%d", s.Cfg.PublicBaseURL, info.ModTime().Unix())
	settings.PaymentQr = &url
}

// qrFilePath finds the uploaded QR image, if any.
func (s *Service) qrFilePath() string {
	for _, ext := range []string{".png", ".jpg", ".jpeg", ".webp", ".svg"} {
		path := s.StoragePath("settings", "kaspi_qr"+ext)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path
		}
	}
	return ""
}

// QrFilePath exposes the current QR file for the download handler.
func (s *Service) QrFilePath() string { return s.qrFilePath() }

// SaveQrImage validates the upload by magic bytes, stores it and returns the
// URL the admin panel will show.
func (s *Service) SaveQrImage(ctx context.Context, data []byte) (string, error) {
	const maxSize = 1536 * 1024 // 1.5 MB (README section 5.5)
	if len(data) == 0 {
		return "", ValidationError(map[string]string{"file": "Файл пустой"})
	}
	if len(data) > maxSize {
		return "", ValidationError(map[string]string{"file": "Файл больше 1.5 МБ"})
	}
	ext, err := imageExt(data)
	if err != nil {
		return "", ValidationError(map[string]string{
			"file": "Поддерживаются PNG, JPG, WEBP и SVG",
		})
	}

	dir := s.StoragePath("settings")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("service: qr dir: %w", err)
	}
	// Remove previous versions so only one file is served.
	for _, known := range []string{".png", ".jpg", ".jpeg", ".webp", ".svg"} {
		_ = os.Remove(filepath.Join(dir, "kaspi_qr"+known))
	}
	target := filepath.Join(dir, "kaspi_qr"+ext)
	if err := os.WriteFile(target, data, 0o644); err != nil {
		return "", fmt.Errorf("service: write qr: %w", err)
	}

	url := fmt.Sprintf("%s/api/settings/payment-qr?v=%d", s.Cfg.PublicBaseURL, len(data))
	if err := s.Store.SetPaymentQr(ctx, url); err != nil {
		return "", err
	}
	return url, nil
}

// imageExt sniffs the image type by content, never by the file name alone.
func imageExt(data []byte) (string, error) {
	detected := http.DetectContentType(data)
	switch {
	case strings.HasPrefix(detected, "image/png"):
		return ".png", nil
	case strings.HasPrefix(detected, "image/jpeg"):
		return ".jpg", nil
	case strings.HasPrefix(detected, "image/webp"):
		return ".webp", nil
	}
	// http.DetectContentType reports SVG as text/xml or text/plain.
	head := strings.ToLower(strings.TrimSpace(string(data[:min(256, len(data))])))
	if strings.HasPrefix(head, "<svg") || (strings.HasPrefix(head, "<?xml") && strings.Contains(head, "<svg")) {
		return ".svg", nil
	}
	return "", fmt.Errorf("service: unsupported image type %q", detected)
}

// Pricing returns the admin price table.
func (s *Service) Pricing(ctx context.Context) (domain.PricingConfig, error) {
	return s.Store.GetPricing(ctx)
}

// SavePricing validates and stores the price table.
func (s *Service) SavePricing(ctx context.Context, config domain.PricingConfig) (domain.PricingConfig, error) {
	if fields := validate.Pricing(&config); len(fields) > 0 {
		return config, ValidationError(fields)
	}
	if err := s.Store.SavePricing(ctx, config); err != nil {
		return config, err
	}
	return s.Store.GetPricing(ctx)
}

// QrContentType reports the mime type of the stored QR image.
func (s *Service) QrContentType() string {
	path := s.qrFilePath()
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	case ".svg":
		return "image/svg+xml"
	default:
		return "application/octet-stream"
	}
}
