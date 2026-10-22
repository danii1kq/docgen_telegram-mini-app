package store

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/srunal/srunal-go/internal/domain"
)

const promoColumns = `id, code, discount_pct, applies_to, min_amount, min_units,
	usage_limit, used_count, expires_at, active, created_at`

func scanPromo(row rowScanner) (domain.PromoCode, error) {
	var promo domain.PromoCode
	err := row.Scan(&promo.ID, &promo.Code, &promo.DiscountPct, &promo.AppliesTo,
		&promo.MinAmount, &promo.MinUnits, &promo.UsageLimit, &promo.UsedCount,
		&promo.ExpiresAt, &promo.Active, &promo.CreatedAt)
	if err != nil {
		return promo, err
	}
	promo.Products = []string{}
	promo.ExpiresAt = utcPtr(promo.ExpiresAt)
	promo.CreatedAt = utc(promo.CreatedAt)
	return promo, nil
}

// NormalizePromoCode uppercases and trims a promo code (api.ts normalizePromoCode).
func NormalizePromoCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

// ListPromos returns every promo code with its product restriction.
func (s *Store) ListPromos(ctx context.Context) ([]domain.PromoCode, error) {
	rows, err := s.pool.Query(ctx, `select `+promoColumns+` from promo_codes order by code`)
	if err != nil {
		return nil, fmt.Errorf("store: list promos: %w", err)
	}
	defer rows.Close()

	promos := []domain.PromoCode{}
	byID := map[string]int{}
	for rows.Next() {
		promo, err := scanPromo(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan promo: %w", err)
		}
		byID[promo.ID] = len(promos)
		promos = append(promos, promo)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: promo rows: %w", err)
	}
	if len(promos) == 0 {
		return promos, nil
	}
	return s.attachPromoProducts(ctx, promos, byID)
}

func (s *Store) attachPromoProducts(ctx context.Context, promos []domain.PromoCode, byID map[string]int) ([]domain.PromoCode, error) {
	ids := make([]string, 0, len(promos))
	for _, promo := range promos {
		ids = append(ids, promo.ID)
	}
	rows, err := s.pool.Query(ctx,
		`select promo_id, product_code from promo_code_products
		  where promo_id = any($1::text[]) order by product_code`, ids)
	if err != nil {
		return nil, fmt.Errorf("store: promo products: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var promoID, product string
		if err := rows.Scan(&promoID, &product); err != nil {
			return nil, fmt.Errorf("store: scan promo product: %w", err)
		}
		if index, ok := byID[promoID]; ok {
			promos[index].Products = append(promos[index].Products, product)
		}
	}
	return promos, rows.Err()
}

// GetPromoByCode finds a promo by its normalized code (ErrNotFound otherwise).
func (s *Store) GetPromoByCode(ctx context.Context, code string) (domain.PromoCode, error) {
	var promo domain.PromoCode
	row := s.pool.QueryRow(ctx,
		`select `+promoColumns+` from promo_codes where code = $1`, NormalizePromoCode(code))
	var err error
	if promo, err = scanPromo(row); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return promo, ErrNotFound
		}
		return promo, fmt.Errorf("store: get promo: %w", err)
	}

	rows, err := s.pool.Query(ctx,
		`select product_code from promo_code_products where promo_id = $1 order by product_code`,
		promo.ID)
	if err != nil {
		return promo, fmt.Errorf("store: promo products: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var product string
		if err := rows.Scan(&product); err != nil {
			return promo, fmt.Errorf("store: scan promo product: %w", err)
		}
		promo.Products = append(promo.Products, product)
	}
	return promo, rows.Err()
}

// GetPromoByID finds a promo by its primary key.
func (s *Store) GetPromoByID(ctx context.Context, id string) (domain.PromoCode, error) {
	var promo domain.PromoCode
	row := s.pool.QueryRow(ctx, `select `+promoColumns+` from promo_codes where id = $1`, id)
	var err error
	if promo, err = scanPromo(row); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return promo, ErrNotFound
		}
		return promo, fmt.Errorf("store: get promo by id: %w", err)
	}
	rows, err := s.pool.Query(ctx,
		`select product_code from promo_code_products where promo_id = $1 order by product_code`, id)
	if err != nil {
		return promo, fmt.Errorf("store: promo products: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var product string
		if err := rows.Scan(&product); err != nil {
			return promo, fmt.Errorf("store: scan promo product: %w", err)
		}
		promo.Products = append(promo.Products, product)
	}
	return promo, rows.Err()
}

// CreatePromo inserts a promo code, ErrConflict when the code exists.
func (s *Store) CreatePromo(ctx context.Context, promo domain.PromoCode) (domain.PromoCode, error) {
	if promo.ID == "" {
		promo.ID = "promo-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	promo.Code = NormalizePromoCode(promo.Code)
	promo.CreatedAt = time.Now().UTC()

	err := s.withTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			`insert into promo_codes (id, code, discount_pct, applies_to, min_amount, min_units,
			        usage_limit, used_count, expires_at, active, created_at)
			 values ($1, $2, $3, $4, $5, $6, $7, 0, $8, $9, $10)`,
			promo.ID, promo.Code, promo.DiscountPct, promo.AppliesTo, promo.MinAmount,
			promo.MinUnits, promo.UsageLimit, promo.ExpiresAt, promo.Active, promo.CreatedAt,
		); err != nil {
			return classify(fmt.Errorf("store: insert promo: %w", err))
		}
		return replacePromoProducts(ctx, tx, promo.ID, promo.Products)
	})
	if err != nil {
		return domain.PromoCode{}, err
	}
	return s.GetPromoByCode(ctx, promo.Code)
}

// UpdatePromo patches an existing promo (admin edit and enable/disable switch).
func (s *Store) UpdatePromo(ctx context.Context, id string, patch domain.PromoInput) (domain.PromoCode, error) {
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		var code string
		if err := tx.QueryRow(ctx,
			`update promo_codes
			    set code = $2, discount_pct = $3, applies_to = $4, min_amount = $5,
			        min_units = $6, usage_limit = $7, expires_at = $8, active = $9
			  where id = $1
			  returning code`,
			id, NormalizePromoCode(patch.Code), patch.DiscountPct, patch.AppliesTo,
			patch.MinAmount, patch.MinUnits, patch.UsageLimit, patch.ExpiresAt, patch.Active,
		).Scan(&code); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return classify(fmt.Errorf("store: update promo: %w", err))
		}
		return replacePromoProducts(ctx, tx, id, patch.Products)
	})
	if err != nil {
		return domain.PromoCode{}, err
	}

	var promo domain.PromoCode
	row := s.pool.QueryRow(ctx, `select `+promoColumns+` from promo_codes where id = $1`, id)
	if promo, err = scanPromo(row); err != nil {
		return promo, fmt.Errorf("store: reload promo: %w", err)
	}
	rows, err := s.pool.Query(ctx,
		`select product_code from promo_code_products where promo_id = $1 order by product_code`, id)
	if err != nil {
		return promo, fmt.Errorf("store: promo products: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var product string
		if err := rows.Scan(&product); err != nil {
			return promo, fmt.Errorf("store: scan promo product: %w", err)
		}
		promo.Products = append(promo.Products, product)
	}
	return promo, rows.Err()
}

// DeletePromo removes a promo code and its redemption history. Orders keep
// their frozen discount.
func (s *Store) DeletePromo(ctx context.Context, id string) error {
	return s.withTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `delete from promo_code_usages where promo_id = $1`, id); err != nil {
			return fmt.Errorf("store: delete promo usages: %w", err)
		}
		tag, err := tx.Exec(ctx, `delete from promo_codes where id = $1`, id)
		if err != nil {
			return fmt.Errorf("store: delete promo: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// CountActivePromos counts the codes that can still be applied.
func (s *Store) CountActivePromos(ctx context.Context) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx,
		`select count(*) from promo_codes
		  where active and (expires_at is null or expires_at > now())
		    and (usage_limit is null or used_count < usage_limit)`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("store: count promos: %w", err)
	}
	return count, nil
}

func replacePromoProducts(ctx context.Context, tx pgx.Tx, promoID string, products []string) error {
	if _, err := tx.Exec(ctx, `delete from promo_code_products where promo_id = $1`, promoID); err != nil {
		return fmt.Errorf("store: clear promo products: %w", err)
	}
	for _, product := range products {
		if _, err := tx.Exec(ctx,
			`insert into promo_code_products (promo_id, product_code) values ($1, $2)
			 on conflict do nothing`, promoID, product,
		); err != nil {
			return fmt.Errorf("store: insert promo product: %w", err)
		}
	}
	return nil
}
