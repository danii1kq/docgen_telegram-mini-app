// Package seed ports srunal_vite/src/lib/seed.ts: 10 demo orders covering the
// whole lifecycle, 11 reviews, 6 promo codes, settings, pricing and the demo
// operator (@operator_almaty / see DemoOperatorPassword).
package seed

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/srunal/srunal-go/internal/auth"
	"github.com/srunal/srunal-go/internal/domain"
	"github.com/srunal/srunal-go/internal/store"
)

// DemoOperatorID is the handle the frontend login page pre-fills.
const DemoOperatorID = "@operator_almaty"

// DemoOperatorPassword is the password of the demo operator on a fresh
// database. Change it in production with:
//
//	go run ./cmd/api -operator @operator_almaty -password 'NewSecret!2026'
const DemoOperatorPassword = "Eshkere9113!xxx!"

const demoTeacher = "доц. Сериков А.Б."
const demoNotes = "Оформление строго по методичке кафедры."

// Import fills an empty database with the demo data. The pepper is the same
// one the service uses, so demo access tokens keep working.
func Import(ctx context.Context, s *store.Store, pepper string, log *slog.Logger) error {
	now := time.Now().UTC()

	tx, err := s.Pool().Begin(ctx)
	if err != nil {
		return fmt.Errorf("seed: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := importPricing(ctx, tx); err != nil {
		return err
	}
	if err := importSettings(ctx, tx); err != nil {
		return err
	}
	if err := importOrders(ctx, tx, now, pepper); err != nil {
		return err
	}
	if err := importReviews(ctx, tx, now); err != nil {
		return err
	}
	if err := importPromos(ctx, tx, now); err != nil {
		return err
	}
	if err := importOperator(ctx, tx); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`insert into counters (name, value) values ('order_number', 260390)
		 on conflict (name) do nothing`); err != nil {
		return fmt.Errorf("seed: counter: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("seed: commit: %w", err)
	}
	log.Info("demo data imported",
		"orders", 10, "reviews", 11, "promos", 6, "operator", DemoOperatorID)
	return nil
}

// ImportBase fills an empty database with just the working configuration: the
// price list (products + volume discounts) and the settings row. It is what a
// production start uses (SEED_ON_START=false): sane prices and an editable
// settings row, without any demo orders, reviews or operator.
func ImportBase(ctx context.Context, s *store.Store, log *slog.Logger) error {
	tx, err := s.Pool().Begin(ctx)
	if err != nil {
		return fmt.Errorf("seed: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := importPricing(ctx, tx); err != nil {
		return err
	}
	if err := importSettings(ctx, tx); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`insert into counters (name, value) values ('order_number', 260390)
		 on conflict (name) do nothing`); err != nil {
		return fmt.Errorf("seed: counter: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("seed: commit: %w", err)
	}
	log.Info("base configuration imported", "pricing", 4, "discounts", 3, "settings", 1)
	return nil
}

func importPricing(ctx context.Context, tx pgx.Tx) error {
	products := []struct {
		code  string
		min   int
		max   int
		price int
	}{
		{"essay", 4, 40, 80},
		{"lab", 1, 1, 1000},
		{"presentation", 5, 30, 240},
		{"other", 1, 1, 1000},
	}
	for _, product := range products {
		if _, err := tx.Exec(ctx,
			`insert into product_pricing (product_code, min_units, max_units, unit_price, active)
			 values ($1, $2, $3, $4, true) on conflict do nothing`,
			product.code, product.min, product.max, product.price); err != nil {
			return fmt.Errorf("seed: pricing %s: %w", product.code, err)
		}
	}
	discounts := []struct {
		min int
		pct int
	}{{5, 10}, {10, 15}, {20, 20}}
	for _, discount := range discounts {
		if _, err := tx.Exec(ctx,
			`insert into volume_discounts (min_works, pct) values ($1, $2) on conflict do nothing`,
			discount.min, discount.pct); err != nil {
			return fmt.Errorf("seed: discount: %w", err)
		}
	}
	return nil
}

func importSettings(ctx context.Context, tx pgx.Tx) error {
	requisites, err := json.Marshal(domain.Requisites{
		Recipient: "ИП «SruNal KZ»",
		Bank:      "Kaspi Bank",
		Card:      "4400 4300 1234 5678",
		IBAN:      "KZ75 125K ZT10 0130 0339",
		Note:      "В комментарии к переводу укажите код заказа (SN-XXXX).",
	})
	if err != nil {
		return fmt.Errorf("seed: requisites: %w", err)
	}
	_, err = tx.Exec(ctx,
		`insert into app_settings (id, requisites, payment_qr_url, payment_recipient, support_telegram, maintenance, queue_paused, payment_link)
		 values (1, $1, null, 'Даниил Л.', '@meownuarka', false, false, $2)
		 on conflict (id) do nothing`, requisites, DefaultPaymentLink)
	if err != nil {
		return fmt.Errorf("seed: settings: %w", err)
	}
	return nil
}

// DefaultPaymentLink is the Kaspi QR deep link the client app shows behind the
// «Оплатить через Kaspi» button on a fresh database. Operators can change it in
// the panel (Settings → Оплата).
const DefaultPaymentLink = "https://qr.kaspi.kz/19152067294640267577360626446900263009840"

// seedToken derives the demo access token from the order number. The mock in
// seed.ts mixed only digits[0] and digits[3], which produced the SAME token for
// every seeded order ("CLS28G"); the database enforces unique tokens, so every
// digit is used here. The result stays deterministic and typable.
func seedToken(number string) string {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	digits := make([]rune, 0, len(number))
	for _, r := range number {
		if r >= '0' && r <= '9' {
			digits = append(digits, r)
		}
	}
	if len(digits) == 0 {
		return "CLS28G"
	}
	out := make([]rune, 0, 6)
	for i := 0; i < 6; i++ {
		digit := int(digits[i%len(digits)] - '0')
		n := (digit + i*7) % len(alphabet)
		out = append(out, rune(alphabet[n]))
	}
	return string(out)
}

type seedOrder struct {
	number            string
	status            string
	product           string
	works             int
	units             int
	topics            string
	amount            int
	senderName        string
	client            string
	contact           string
	institute         string
	department        string
	discipline        string
	student           string
	createdMinutesAgo float64
	files             []domain.FileMeta
}

func seedOrders() []seedOrder {
	return []seedOrder{
		{
			number: "DG-260381", status: "payment_claimed", product: "lab", units: 1,
			topics: "Лабораторная по методичке • Kali Linux setup", amount: 1000,
			senderName: "Алихан С.", client: "Алихан", contact: "@alikhan_sat",
			institute: "ИЦИТ", department: "Информационная безопасность",
			discipline: "ИБ-2104", student: "Алихан С.", createdMinutesAgo: 5,
			files: []domain.FileMeta{{Name: "metodichka_kali.pdf", Size: 1468006, Type: "application/pdf"}},
		},
		{
			number: "DG-260379", status: "payment_claimed", product: "essay", units: 15,
			topics: "Устав ООН и суверенитет государств", amount: 2500,
			senderName: "Мадина К.", client: "Мадина К.", contact: "@madina_sat",
			institute: "ИЦИТ", department: "Программная инженерия",
			discipline: "ПИ-1203", student: "Мадина К.", createdMinutesAgo: 14,
		},
		{
			number: "DG-260375", status: "payment_claimed", product: "other", units: 1,
			topics: "Расчёт матриц • Python скрипт", amount: 4200,
			senderName: "Бахтияр Н.", client: "Бахтияр Н.", contact: "@bakha_sat",
			institute: "ИМА", department: "Автоматизация и управление",
			discipline: "АУ-2210", student: "Бахтияр Н.", createdMinutesAgo: 22,
		},
		{
			number: "DG-260370", status: "generating", product: "presentation", units: 18,
			topics: "Анализ Fintech KZ", amount: 3000,
			senderName: "Еркежан С.", client: "Еркежан С.", contact: "@erke_sat",
			institute: "ИМА", department: "Инженерная экономика",
			discipline: "ЭК-3301", student: "Еркежан С.", createdMinutesAgo: 28,
		},
		{
			number: "DG-260368", status: "completed", product: "lab", units: 1,
			topics: "Настройка VLAN & Cisco Packet Tracer", amount: 6500,
			senderName: "Данияр М.", client: "Данияр М.", contact: "@daniyar_stu",
			institute: "ИФТ", department: "Сети и телекоммуникации",
			discipline: "ТК-2107", student: "Данияр М.", createdMinutesAgo: 45,
			files: []domain.FileMeta{{Name: "packet_tracer_task.pdf", Size: 892110, Type: "application/pdf"}},
		},
		{
			number: "DG-260362", status: "completed", product: "essay", units: 12,
			topics: "Методы инклюзии в школе", amount: 1800,
			senderName: "Айнура Б.", client: "Айнура Б.", contact: "@ainur_abai",
			institute: "ИАС", department: "Прикладная математика",
			discipline: "ПМ-1409", student: "Айнура Б.", createdMinutesAgo: 72,
		},
		{
			number: "DG-260359", status: "queued", product: "other", units: 1,
			topics: "Комплексная нормализация БД PostgreSQL, 3НФ", amount: 8000,
			senderName: "Темирлан С.", client: "Темирлан С.", contact: "@temir_sat",
			institute: "ИЦИТ", department: "Компьютерные науки",
			discipline: "КН-2201", student: "Темирлан С.", createdMinutesAgo: 100,
		},
		{
			number: "DG-260355", status: "awaiting_payment", product: "essay", units: 10,
			topics: "Анализ современных методов шифрования данных в распределённых сетях", amount: 800,
			senderName: "Нурсултан Ж.", client: "Нурсултан Ж.", contact: "@nurs_sat",
			institute: "ИЦИТ", department: "Информационная безопасность",
			discipline: "ИБ-2308", student: "Нурсултан Ж.", createdMinutesAgo: 140,
		},
		{
			number: "DG-260348", status: "cancelled", product: "presentation", units: 10,
			topics: "Бизнес-план стартапа", amount: 1200,
			senderName: "Сая Т.", client: "Сая Т.", contact: "@saya_arch",
			institute: "ИАС", department: "Архитектура",
			discipline: "АР-1502", student: "Сая Т.", createdMinutesAgo: 220,
		},
		{
			number: "DG-260341", status: "payment_rejected", product: "other", units: 1,
			topics: "СРС по теоретической механике", amount: 1000,
			senderName: "Ерлан С.", client: "Ерлан С.", contact: "@erlan_gmi",
			institute: "ГМИ", department: "Разработка месторождений",
			discipline: "ГД-1206", student: "Ерлан С.", createdMinutesAgo: 260,
		},
	}
}

func importOrders(ctx context.Context, tx pgx.Tx, now time.Time, pepper string) error {
	minutesAgo := func(minutes float64) time.Time {
		return now.Add(-time.Duration(minutes * float64(time.Minute)))
	}

	for _, input := range seedOrders() {
		createdAt := minutesAgo(input.createdMinutesAgo)
		updatedMinutes := input.createdMinutesAgo - 2
		if updatedMinutes < 1 {
			updatedMinutes = 1
		}
		updatedAt := minutesAgo(updatedMinutes)
		claimAt := minutesAgo(max(1, input.createdMinutesAgo-1))

		type event struct {
			at    time.Time
			actor string
			text  string
		}
		events := []event{{createdAt, domain.ActorClient, "Заказ создан"}}
		if input.status != domain.StatusAwaitingPayment {
			events = append(events, event{claimAt, domain.ActorClient, "Клиент нажал «Я оплатил»"})
		}
		if contains([]string{"queued", "generating", "completed"}, input.status) {
			events = append(events, event{updatedAt, domain.ActorOperator, "Оплата подтверждена, заказ поставлен в очередь"})
		}
		if contains([]string{"generating", "completed"}, input.status) {
			events = append(events, event{updatedAt, domain.ActorSystem, "Генерация документа запущена"})
		}
		if input.status == "completed" {
			events = append(events, event{updatedAt, domain.ActorSystem, "Результат готов и доступен для скачивания"})
		}
		if input.status == "cancelled" {
			events = append(events, event{updatedAt, domain.ActorOperator, "Заказ отменён оператором"})
		}
		if input.status == "payment_rejected" {
			events = append(events, event{updatedAt, domain.ActorOperator, "Оплата не найдена: сумма не совпадает с чеком"})
		}

		var claimedAt *time.Time
		if input.status != domain.StatusAwaitingPayment {
			claimedAt = &claimAt
		}
		var confirmedAt *time.Time
		switch input.status {
		case "queued":
			value := minutesAgo(0.5)
			confirmedAt = &value
		case "generating":
			value := minutesAgo(0.4)
			confirmedAt = &value
		case "completed":
			confirmedAt = &updatedAt
		}
		var completedAt *time.Time
		if input.status == "completed" {
			completedAt = &updatedAt
		}
		var rejectReason *string
		if input.status == "payment_rejected" {
			reason := "Сумма перевода не совпадает с чеком. Переведите точную сумму и нажмите «Я оплатил»."
			rejectReason = &reason
		}
		clientNote := ""
		if input.status == "payment_claimed" {
			clientNote = "Чек Kaspi отправлен в бот."
		}

		works := input.works
		if works < 1 {
			works = 1
		}

		_, err := tx.Exec(ctx,
			`insert into orders (number, token_hash, status, sender_name, product_code, works, units,
			        topics, notes, language, institute, department, student_name, discipline_code,
			        teacher_name, contact_name, contact_type, contact, amount, quote_lines,
			        client_note, reject_reason, claimed_at, confirmed_at, completed_at, created_at, updated_at)
			 values ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'ru', $10, $11, $12, $13, $14, $15, 'telegram',
			         $16, $17, '[]'::jsonb, $18, $19, $20, $21, $22, $23, $24)`,
			input.number, tokenHash(pepper, input.number), input.status, input.senderName, input.product,
			works, input.units, input.topics, demoNotes, input.institute, input.department,
			input.student, input.discipline, demoTeacher, input.client, input.contact,
			input.amount, clientNote, rejectReason, claimedAt, confirmedAt, completedAt,
			createdAt, updatedAt)
		if err != nil {
			return fmt.Errorf("seed: order %s: %w", input.number, err)
		}

		for _, file := range input.files {
			if _, err := tx.Exec(ctx,
				`insert into order_files (order_number, kind, file_name, size_bytes, mime_type)
				 values ($1, 'client_material', $2, $3, $4)`,
				input.number, file.Name, file.Size, file.Type); err != nil {
				return fmt.Errorf("seed: attachment %s: %w", file.Name, err)
			}
		}

		for _, entry := range events {
			if _, err := tx.Exec(ctx,
				`insert into order_events (order_number, actor, text, at) values ($1, $2, $3, $4)`,
				input.number, entry.actor, entry.text, entry.at); err != nil {
				return fmt.Errorf("seed: event: %w", err)
			}
		}
	}
	return nil
}

func tokenHash(pepper, number string) string {
	return auth.HashToken(pepper, seedToken(number))
}

func importReviews(ctx context.Context, tx pgx.Tx, now time.Time) error {
	minutesAgo := func(minutes float64) time.Time {
		return now.Add(-time.Duration(minutes * float64(time.Minute)))
	}
	reviews := []struct {
		id          string
		orderNumber string
		author      string
		institute   string
		department  string
		discipline  string
		product     string
		rating      float64
		text        string
		createdAt   time.Time
		status      string
	}{
		{
			"r-1001", "DG-260381", "Арсен М.", "ИЦИТ", "Кафедра компьютерной инженерии",
			"Информационная безопасность", "lab", 5,
			"Сдавал 4-ю лабораторную по асимметричному шифрованию RSA. Преподаватель очень придирчивый к отчётам и блок-схемам. Оформили всё по ГОСТу, листинги на Python снабдили точными комментариями. Защитил за 5 минут, без доп. вопросов.",
			minutesAgo(60 * 24 * 40), "published",
		},
		{
			"r-1002", "DG-260375", "Данияр К.", "ИФТ", "Телекоммуникационные системы",
			"Сети и телекоммуникации", "other", 5,
			"Нужно было срочно поднять топологию в Packet Tracer и рассчитать маски переменной длины VLSM для СРСП. Сделали за сутки, конфигурационные файлы .pkt запустились с первого клика. Выручили перед рубежкой!",
			minutesAgo(60 * 24 * 43), "published",
		},
		{
			"r-1003", "DG-260359", "Мадина Т.", "ИЦИТ", "Системы баз данных",
			"Базы данных PostgreSQL", "other", 5,
			"Нормализация таблиц до третьей нормальной формы и триггеры с процедурами на PL/pgSQL. Грамотно составленные ER-диаграммы в Crow's Foot нотации. Сдала на максимальный балл группы.",
			minutesAgo(60 * 24 * 46), "published",
		},
		{
			"r-1004", "DG-260341", "Ерлан С.", "ГМИ", "Разработка месторождений",
			"Геология и горное дело", "essay", 4.8,
			"Реферат по гидрометаллургической переработке упорных золотосодержащих руд. Оригинальность после проверки вышла 88%, все ссылки на статьи Scopus и локальные казахстанские источники расставлены верно.",
			minutesAgo(60 * 24 * 49), "published",
		},
		{
			"r-1005", "DG-260348", "Санжар Б.", "ИАС", "Промышленное строительство",
			"BIM-технологии", "presentation", 5,
			"Подготовили презентацию к защите проекта на 18 слайдов в строгом инженерном стиле. Никакой воды: понятные графики, спецификации узлов и выжимка расчётов. Комиссия отдельно похвалила визуальную подачу.",
			minutesAgo(60 * 24 * 55), "published",
		},
		{
			"r-1006", "DG-260368", "Нурсултан Ж.", "ИЦИТ", "Программная инженерия",
			"Алгоритмы и структуры данных", "lab", 4,
			"Сложная тема с красно-чёрными деревьями и балансировкой АВЛ. Документация подробная, код рабочий. Небольшое замечание лектора по сложности в нотации O(log n), но оценку поставили хорошую.",
			minutesAgo(60 * 24 * 61), "published",
		},
		{
			"r-2001", "DG-260381", "Алихан Сатпаев", "Satbayev Univ", "Информационная безопасность",
			"Лабораторная по Kali Linux", "lab", 5,
			"Лабораторная по Kali Linux сдана на 98 баллов! Форматирование и структура команд полностью совпали с методичкой каф. ИБ. Спасибо за скорость!",
			minutesAgo(10), "pending",
		},
		{
			"r-2002", "DG-260379", "Мадина К.", "Satbayev Univ", "Программная инженерия",
			"Реферат по уставу ООН", "essay", 5,
			"Реферат по уставу ООН прошёл проверку оригинальности на 89% без правок. Быстро и чётко оформлен титульник.",
			minutesAgo(25), "pending",
		},
		{
			"r-2003", "DG-260370", "Еркежан С.", "Satbayev Univ", "Инженерная экономика",
			"Презентация по Fintech KZ", "presentation", 5,
			"Презентация по Fintech KZ - 18 слайдов, структура и графика на уровне. Защитила на 92 балла, комиссия отметила оформление диаграмм.",
			minutesAgo(48), "pending",
		},
		{
			"r-2004", "DG-260359", "Бахтияр Н.", "Satbayev Univ", "Автоматизация и управление",
			"Расчёт матриц, Python", "other", 4.6,
			"СРСП по расчёту матриц с Python-скриптом. Всё работает, преподаватель принял без замечаний, но пришлось один раз уточнять формат вывода.",
			minutesAgo(70), "pending",
		},
		{
			"r-3001", "DG-260341", "Аноним", "Satbayev Univ", "-",
			"-", "essay", 1,
			"Спам-отзыв без номера заказа, отклонён модератором.",
			minutesAgo(300), "rejected",
		},
	}

	for _, review := range reviews {
		if _, err := tx.Exec(ctx,
			`insert into reviews (id, order_number, author, institute, department, discipline,
			        product_code, rating, text, status, created_at)
			 values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
			review.id, review.orderNumber, review.author, review.institute, review.department,
			review.discipline, review.product, review.rating, review.text, review.status,
			review.createdAt); err != nil {
			return fmt.Errorf("seed: review %s: %w", review.id, err)
		}
	}
	return nil
}

func importPromos(ctx context.Context, tx pgx.Tx, now time.Time) error {
	createdAt := now.Add(-30 * 24 * time.Hour)
	expiry := func(value string) *time.Time {
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			return nil
		}
		return &parsed
	}

	promos := []struct {
		id         string
		code       string
		pct        int
		appliesTo  string
		products   []string
		minAmount  *int
		minUnits   *int
		usageLimit *int
		usedCount  int
		expiresAt  *time.Time
		active     bool
	}{
		{"promo-srunal10", "SRUNAL10", 10, "any", nil, nil, nil, nil, 7, nil, true},
		{"promo-first15", "FIRST15", 15, "first", nil, nil, nil, intPtr(50), 12, nil, true},
		{"promo-winter20", "WINTER20", 20, "any", []string{"essay", "presentation"}, nil,
			intPtr(5), intPtr(100), 31, expiry("2026-12-31T23:59:59Z"), true},
		{"promo-labhit10", "LABHIT10", 10, "any", []string{"lab"}, intPtr(1000),
			nil, intPtr(40), 9, nil, true},
		{"promo-spring25", "SPRING25", 25, "any", nil, nil, nil, intPtr(200), 4,
			expiry("2026-06-01T23:59:59Z"), true},
		{"promo-oldfive", "OLDFIVE", 5, "any", nil, nil, nil, intPtr(30), 30, nil, false},
	}

	for _, promo := range promos {
		if _, err := tx.Exec(ctx,
			`insert into promo_codes (id, code, discount_pct, applies_to, min_amount, min_units,
			        usage_limit, used_count, expires_at, active, created_at)
			 values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
			promo.id, promo.code, promo.pct, promo.appliesTo, promo.minAmount, promo.minUnits,
			promo.usageLimit, promo.usedCount, promo.expiresAt, promo.active, createdAt); err != nil {
			return fmt.Errorf("seed: promo %s: %w", promo.code, err)
		}
		for _, product := range promo.products {
			if _, err := tx.Exec(ctx,
				`insert into promo_code_products (promo_id, product_code) values ($1, $2)`,
				promo.id, product); err != nil {
				return fmt.Errorf("seed: promo product: %w", err)
			}
		}
	}
	return nil
}

func importOperator(ctx context.Context, tx pgx.Tx) error {
	hash, err := auth.HashPassword(DemoOperatorPassword)
	if err != nil {
		return fmt.Errorf("seed: operator hash: %w", err)
	}
	_, err = tx.Exec(ctx,
		`insert into operators (id, password_hash, display_name, active)
		 values ($1, $2, 'SruNal 2026 Admin Core', true)
		 on conflict (id) do nothing`, DemoOperatorID, hash)
	if err != nil {
		return fmt.Errorf("seed: operator: %w", err)
	}
	return nil
}

// TokenFor exposes the deterministic demo token (used by tests and the login hint).
func TokenFor(number string) string { return seedToken(number) }

func contains(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

func intPtr(value int) *int { return &value }
