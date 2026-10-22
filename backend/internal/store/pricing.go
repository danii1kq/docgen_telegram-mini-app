package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/srunal/srunal-go/internal/catalog"
	"github.com/srunal/srunal-go/internal/domain"
)

// GetPricing returns the price table and volume discounts (admin -> Цены).
func (s *Store) GetPricing(ctx context.Context) (domain.PricingConfig, error) {
	config := domain.PricingConfig{}

	rows, err := s.pool.Query(ctx,
		`select product_code, min_units, max_units, unit_price, active
		   from product_pricing`)
	if err != nil {
		return config, fmt.Errorf("store: pricing: %w", err)
	}
	defer rows.Close()

	byCode := map[string]domain.PricingProduct{}
	for rows.Next() {
		var product domain.PricingProduct
		if err := rows.Scan(&product.Code, &product.Min, &product.Max, &product.UnitPrice, &product.Active); err != nil {
			return config, fmt.Errorf("store: scan pricing: %w", err)
		}
		byCode[product.Code] = product
	}
	if err := rows.Err(); err != nil {
		return config, fmt.Errorf("store: pricing rows: %w", err)
	}

	// Keep the canonical catalog order so the admin table and the quote lines
	// stay stable, then append any custom codes.
	for _, product := range catalog.Products {
		if configured, ok := byCode[product.Code]; ok {
			config.Products = append(config.Products, configured)
			delete(byCode, product.Code)
		} else {
			config.Products = append(config.Products, domain.PricingProduct{
				Code:      product.Code,
				Min:       product.Min,
				Max:       product.Max,
				UnitPrice: product.PricePerUnit,
				Active:    true,
			})
		}
	}
	for _, left := range byCode {
		config.Products = append(config.Products, left)
	}

	discountRows, err := s.pool.Query(ctx,
		`select min_works, pct from volume_discounts order by min_works`)
	if err != nil {
		return config, fmt.Errorf("store: discounts: %w", err)
	}
	defer discountRows.Close()
	for discountRows.Next() {
		var discount domain.VolumeDiscount
		if err := discountRows.Scan(&discount.Min, &discount.Pct); err != nil {
			return config, fmt.Errorf("store: scan discount: %w", err)
		}
		config.Discounts = append(config.Discounts, discount)
	}
	return config, discountRows.Err()
}

// SavePricing upserts the products and replaces the volume-discount tiers.
// Already created orders keep their frozen amount and quote lines.
func (s *Store) SavePricing(ctx context.Context, config domain.PricingConfig) error {
	return s.withTx(ctx, func(tx pgx.Tx) error {
		for _, product := range config.Products {
			if _, err := tx.Exec(ctx,
				`insert into product_pricing (product_code, min_units, max_units, unit_price, active, updated_at)
				 values ($1, $2, $3, $4, $5, now())
				 on conflict (product_code) do update
				   set min_units = excluded.min_units,
				       max_units = excluded.max_units,
				       unit_price = excluded.unit_price,
				       active = excluded.active,
				       updated_at = now()`,
				product.Code, product.Min, product.Max, product.UnitPrice, product.Active,
			); err != nil {
				return classify(fmt.Errorf("store: save product %s: %w", product.Code, err))
			}
		}
		if _, err := tx.Exec(ctx, `delete from volume_discounts`); err != nil {
			return fmt.Errorf("store: clear discounts: %w", err)
		}
		for _, discount := range config.Discounts {
			if _, err := tx.Exec(ctx,
				`insert into volume_discounts (min_works, pct) values ($1, $2)
				 on conflict (min_works) do update set pct = excluded.pct`,
				discount.Min, discount.Pct,
			); err != nil {
				return classify(fmt.Errorf("store: save discount: %w", err))
			}
		}
		return nil
	})
}

// GetSettings returns the single settings row (defaults when it is missing).
func (s *Store) GetSettings(ctx context.Context) (domain.Settings, error) {
	settings := domain.Settings{Requisites: domain.Requisites{}}
	var requisites []byte

	err := s.pool.QueryRow(ctx,
		`select requisites, payment_qr_url, payment_recipient, support_telegram, maintenance, queue_paused, payment_link
		   from app_settings where id = 1`,
	).Scan(&requisites, &settings.PaymentQr, &settings.PaymentRecipient,
		&settings.SupportTelegram, &settings.Maintenance, &settings.QueuePaused, &settings.PaymentLink)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return settings, nil
		}
		return settings, fmt.Errorf("store: settings: %w", err)
	}
	if len(requisites) > 0 {
		if err := json.Unmarshal(requisites, &settings.Requisites); err != nil {
			s.log.Warn("settings: bad requisites json", "error", err)
		}
	}
	return settings, nil
}

// SaveSettings writes the single settings row.
func (s *Store) SaveSettings(ctx context.Context, settings domain.Settings) error {
	requisites, err := json.Marshal(settings.Requisites)
	if err != nil {
		return fmt.Errorf("store: encode requisites: %w", err)
	}
	_, err = s.pool.Exec(ctx,
		`insert into app_settings (id, requisites, payment_qr_url, payment_recipient, support_telegram, maintenance, queue_paused, payment_link)
		 values (1, $1, $2, $3, $4, $5, $6, $7)
		 on conflict (id) do update
		   set requisites = excluded.requisites,
		       payment_qr_url = excluded.payment_qr_url,
		       payment_recipient = excluded.payment_recipient,
		       support_telegram = excluded.support_telegram,
		       maintenance = excluded.maintenance,
		       queue_paused = excluded.queue_paused,
		       payment_link = excluded.payment_link`,
		requisites, settings.PaymentQr, settings.PaymentRecipient,
		settings.SupportTelegram, settings.Maintenance, settings.QueuePaused, settings.PaymentLink,
	)
	if err != nil {
		return fmt.Errorf("store: save settings: %w", err)
	}
	return nil
}

// SetPaymentQr updates only the QR image URL (admin upload).
func (s *Store) SetPaymentQr(ctx context.Context, url string) error {
	_, err := s.pool.Exec(ctx,
		`update app_settings set payment_qr_url = $1 where id = 1`, url)
	if err != nil {
		return fmt.Errorf("store: set payment qr: %w", err)
	}
	return nil
}

// SetQueuePaused toggles the generation-queue flag (used by the bot and admin).
func (s *Store) SetQueuePaused(ctx context.Context, paused bool) error {
	_, err := s.pool.Exec(ctx,
		`update app_settings set queue_paused = $1 where id = 1`, paused)
	if err != nil {
		return fmt.Errorf("store: set queue paused: %w", err)
	}
	return nil
}
