package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/srunal/srunal-go/internal/catalog"
	"github.com/srunal/srunal-go/internal/domain"
	"github.com/srunal/srunal-go/internal/format"
)

// Bot notification templates (design.md §8.2, canonical copy).
//
// Messages are plain text (no parse mode) so a topic full of <, > or & cannot
// break the Bot API call. They are queued in telegram_outbox and flushed by the
// bot loop: a disabled bot never blocks an API request, and a finished document
// is delivered with sendDocument by the same loop.
//
// Rules of thumb:
//   - one event = one message;
//   - the first line is the headline, then 2-4 short lines, then buttons;
//   - money, dates and numbers use the app formats ("800 ₸", "17 окт, 14:05");
//   - buttons are deep links into the Mini App screens (no tokens in messages).

// notifyAdmins queues a message for every operator chat (config + bound chats).
func (s *Service) notifyAdmins(ctx context.Context, text string) {
	s.notifyAdminsWithButtons(ctx, text, nil)
}

// notifyAdminsWithButtons is notifyAdmins plus an inline keyboard (used by the
// payment-claimed message: «Подтвердить» / «Отклонить»).
func (s *Service) notifyAdminsWithButtons(ctx context.Context, text string, buttons []byte) {
	seen := map[int64]bool{}
	ids := append([]int64{}, s.Cfg.Bot.AdminChatIDs...)
	if extra, err := s.Store.OperatorChatIDs(ctx); err == nil {
		ids = append(ids, extra...)
	} else {
		s.Log.Error("notify: operator chats", "error", err)
	}
	for _, chatID := range ids {
		if chatID == 0 || seen[chatID] {
			continue
		}
		seen[chatID] = true
		if err := s.Store.EnqueueTelegram(ctx, chatID, text, "", buttons); err != nil {
			s.Log.Error("notify: enqueue admin", "error", err)
		}
	}
}

// notifyClient queues a message for the order's linked chat.
func (s *Service) notifyClient(ctx context.Context, order *domain.Order, text string) {
	s.notifyClientWithButtons(ctx, order, text, nil)
}

// notifyClientWithButtons is notifyClient plus an inline keyboard. Buttons are
// optional: nil keeps the message plain (e.g. before the Mini App short name
// is configured in .env).
func (s *Service) notifyClientWithButtons(ctx context.Context, order *domain.Order, text string, buttons []byte) {
	if order == nil || order.TelegramChatID == 0 {
		return
	}
	if err := s.Store.EnqueueTelegram(ctx, order.TelegramChatID, text, order.Number, buttons); err != nil {
		s.Log.Error("notify: enqueue client", "error", err)
	}
}

// notifyClientDocument delivers a message with a file attached (a finished
// document arrives as a Telegram document, not only as a link).
func (s *Service) notifyClientDocument(ctx context.Context, order *domain.Order, text, path, name string, buttons []byte) {
	if order == nil || order.TelegramChatID == 0 {
		return
	}
	if strings.TrimSpace(path) == "" {
		s.notifyClientWithButtons(ctx, order, text, buttons)
		return
	}
	if err := s.Store.EnqueueTelegramDocument(ctx, order.TelegramChatID, text, order.Number, path, name, buttons); err != nil {
		s.Log.Error("notify: enqueue client document", "error", err)
	}
}

// inlineButton is one callback button of an inline keyboard.
type inlineButton struct {
	Text string `json:"text"`
	Data string `json:"callback_data"`
}

// inlineKeyboard renders reply_markup for inline buttons; data is limited to
// 64 bytes by Telegram, so it carries only "<action>:<order number>".
func inlineKeyboard(rows ...[]inlineButton) []byte {
	markup := struct {
		InlineKeyboard [][]inlineButton `json:"inline_keyboard"`
	}{InlineKeyboard: rows}
	encoded, err := json.Marshal(markup)
	if err != nil {
		return nil
	}
	return encoded
}

// inlineURLButton opens a link. t.me deep links launch the Mini App directly
// inside Telegram, which is how notifications lead back into the exact screen.
type inlineURLButton struct {
	Text string `json:"text"`
	URL  string `json:"url"`
}

// urlInlineKeyboard renders a reply markup with URL buttons.
func urlInlineKeyboard(rows ...[]inlineURLButton) []byte {
	markup := struct {
		InlineKeyboard [][]inlineURLButton `json:"inline_keyboard"`
	}{InlineKeyboard: rows}
	encoded, err := json.Marshal(markup)
	if err != nil {
		return nil
	}
	return encoded
}

// ClaimedPaymentButtons is the keyboard of the operator's payment message.
func ClaimedPaymentButtons(number string) []byte {
	return inlineKeyboard(
		[]inlineButton{
			{Text: "✅ Подтвердить", Data: "confirm:" + number},
			{Text: "❌ Отклонить", Data: "reject:" + number},
		},
	)
}

// URLButtonRow renders a one-button URL keyboard. Exported because the bot
// answers (/start, /status) also link into the Mini App.
func URLButtonRow(label, link string) []byte {
	return urlInlineKeyboard([]inlineURLButton{{Text: label, URL: link}})
}

// supportHandle returns the Telegram handle clients should write to.
func (s *Service) supportHandle(ctx context.Context) string {
	if settings, err := s.Store.GetSettings(ctx); err == nil && strings.TrimSpace(settings.SupportTelegram) != "" {
		return strings.TrimSpace(settings.SupportTelegram)
	}
	return "@meownuarka"
}

// supportButton renders the "write to support" URL button (nil when the handle
// is not usable as a t.me link).
func (s *Service) supportButton(ctx context.Context, label string) []inlineURLButton {
	handle := strings.TrimPrefix(s.supportHandle(ctx), "@")
	if len(handle) < 5 {
		return nil
	}
	return []inlineURLButton{{Text: label, URL: "https://t.me/" + handle}}
}

// orderButtons builds the "open this order in the Mini App" keyboard. nil when
// the deep link is not configured (TELEGRAM_BOT_USERNAME plus
// TELEGRAM_MINIAPP_SHORT_NAME), so the messages simply stay button-less.
func (s *Service) orderButtons(order *domain.Order, labels ...string) []byte {
	link := s.Cfg.MiniAppLink("order_" + order.Number)
	if link == "" || len(labels) == 0 {
		return nil
	}
	buttons := make([]inlineURLButton, 0, len(labels))
	for _, label := range labels {
		buttons = append(buttons, inlineURLButton{Text: label, URL: link})
	}
	return urlInlineKeyboard(buttons)
}

// reviewButton is the "leave a review" deep link of a finished order.
func (s *Service) reviewButton(order *domain.Order, label string) []inlineURLButton {
	link := s.Cfg.MiniAppLink("review_" + order.Number)
	if link == "" {
		return nil
	}
	return []inlineURLButton{{Text: label, URL: link}}
}

// newOrderButton opens the app on the first wizard step.
func (s *Service) newOrderButton(label string) []inlineURLButton {
	link := s.Cfg.MiniAppLink("new")
	if link == "" {
		return nil
	}
	return []inlineURLButton{{Text: label, URL: link}}
}

// ProductLabel renders "Другой док. (СРС)" when the client named the work kind.
func ProductLabel(order *domain.Order) string {
	label := catalog.Get(order.Product).Name
	if strings.TrimSpace(order.WorkKind) != "" {
		label += " (" + order.WorkKind + ")"
	}
	return label
}

// ---------------------------------------------------------------------------
// client notifications
// ---------------------------------------------------------------------------

// notifyNewOrder tells the operators about a fresh order and hands the client
// the payment instructions (design.md §8.2 template 2).
func (s *Service) notifyNewOrder(ctx context.Context, order *domain.Order) {
	product := catalog.Get(order.Product)
	s.notifyAdmins(ctx, strings.Join([]string{
		"🆕 Новый заказ " + order.Number,
		fmt.Sprintf("%s · %s · %s", ProductLabel(order), unitsLine(order, product), format.Money(order.Amount)),
		"Тема: " + short(order.Topics, 120),
		"Клиент: " + order.ContactName,
		"Статус: ожидает оплату",
	}, "\n"))

	s.notifyClientWithButtons(ctx, order, strings.Join([]string{
		"📄 Заказ " + order.Number + " создан",
		fmt.Sprintf("%s · %s · %s", product.Name, unitsLine(order, product), format.Money(order.Amount)),
		"Оплатите по Kaspi QR и нажмите «Я оплатил» - оператор проверит поступление.",
	}, "\n"), s.orderButtons(order, "💳 Перейти к оплате", "👀 Детали заказа"))
}

// notifyPaymentClaimed pings the operators: the client says they paid
// (template 3) and confirms to the client that the check is running.
func (s *Service) notifyPaymentClaimed(ctx context.Context, order *domain.Order) {
	product := catalog.Get(order.Product)
	rows := [][]inlineButton{
		{
			{Text: "✅ Подтвердить", Data: "confirm:" + order.Number},
			{Text: "❌ Отклонить", Data: "reject:" + order.Number},
		},
	}
	s.notifyAdminsWithButtons(ctx, strings.Join([]string{
		"💰 Оплата на проверку",
		fmt.Sprintf("%s · %s · отправитель: %s", order.Number, format.Money(order.Amount), order.SenderName),
		fmt.Sprintf("%s · %s · создан %s", product.Name, unitsLine(order, product), shortDate(order.CreatedAt)),
	}, "\n"), inlineKeyboard(rows...))

	s.notifyClient(ctx, order, strings.Join([]string{
		"🔎 Оплата отмечена",
		"Оператор проверяет поступление по заказу " + order.Number + ". Обычно это занимает несколько минут.",
		"Мы пришлём сообщение, когда оплата подтвердится.",
	}, "\n"))
}

// notifyPaymentConfirmed tells the client the queue has started (template 4).
func (s *Service) notifyPaymentConfirmed(ctx context.Context, order *domain.Order) {
	s.notifyClientWithButtons(ctx, order, strings.Join([]string{
		"✅ Оплата подтверждена",
		"Заказ " + order.Number + " принят в работу. Обычно подготовка занимает от 3 до 20 минут.",
		"Пришлём файл этим сообщением.",
	}, "\n"), s.orderButtons(order, "👀 Следить за статусом"))
}

// notifyPaymentRejected forwards the operator's reason and gives the client a
// retry path (template 5).
func (s *Service) notifyPaymentRejected(ctx context.Context, order *domain.Order, reason string) {
	rows := make([][]inlineURLButton, 0, 1)
	buttonRow := make([]inlineURLButton, 0, 2)
	buttonRow = append(buttonRow, s.orderButtonsList(order, "🔁 Я оплатил ещё раз")...)
	if support := s.supportButton(ctx, "💬 Поддержка"); support != nil {
		buttonRow = append(buttonRow, support...)
	}
	if len(buttonRow) > 0 {
		rows = append(rows, buttonRow)
	}
	s.notifyClientWithButtons(ctx, order, strings.Join([]string{
		"❌ Оплата не найдена",
		"Причина: " + reason,
		"Проверьте данные платежа и попробуйте ещё раз или напишите в поддержку.",
	}, "\n"), urlInlineKeyboard(rows...))
}

// notifyCompleted hands the finished document to the client (template 6).
func (s *Service) notifyCompleted(ctx context.Context, order *domain.Order) {
	product := catalog.Get(order.Product)
	caption := strings.Join([]string{
		"🎓 Работа готова!",
		fmt.Sprintf("%s · %s · %s", order.Number, product.Name, unitsLine(order, product)),
		"Файл во вложении. Доступен 30 дней. Правки преподавателя внесём бесплатно.",
	}, "\n")

	rows := make([][]inlineURLButton, 0, 1)
	buttonRow := make([]inlineURLButton, 0, 2)
	if review := s.reviewButton(order, "⭐ Оставить отзыв"); review != nil {
		buttonRow = append(buttonRow, review...)
	}
	buttonRow = append(buttonRow, s.orderButtonsList(order, "👀 Открыть заказ")...)
	if len(buttonRow) > 0 {
		rows = append(rows, buttonRow)
	}
	buttons := urlInlineKeyboard(rows...)

	// The stored files are the primary delivery path; when they are gone (e.g.
	// the TTL already elapsed) the caption still tells the client what happened.
	// An order with several works has one document per work.
	attachments := s.resultAttachments(ctx, order)
	if len(attachments) == 0 {
		link := ""
		if order.ResultFile != nil {
			link = order.ResultFile.URL
		}
		caption = strings.TrimSpace(caption + "\n" + link)
		s.notifyClientWithButtons(ctx, order, caption, buttons)
		return
	}
	if len(attachments) > 1 {
		caption = strings.Replace(caption, "Файл во вложении.",
			fmt.Sprintf("Документов во вложении: %d - по одному на каждую работу.", len(attachments)), 1)
	}
	for index, attachment := range attachments {
		text := caption
		fileButtons := buttons
		if index > 0 {
			text = fmt.Sprintf("%s · документ %d из %d: %s",
				order.Number, index+1, len(attachments), attachment.Name)
			fileButtons = nil // the buttons ride on the first document only
		}
		s.notifyClientDocument(ctx, order, text, attachment.Path, attachment.Name, fileButtons)
	}
}

// notifyFailed warns the operators with the technical reason and tells the
// client honestly that the work needs more time (template 7).
func (s *Service) notifyFailed(ctx context.Context, order *domain.Order, reason string) {
	s.notifyAdmins(ctx, strings.Join([]string{
		"⚠️ Сбой генерации: " + order.Number,
		"Причина: " + short(reason, 200),
		"Откройте заказ и запустите повторную генерацию.",
	}, "\n"))

	rows := [][]inlineURLButton{}
	if support := s.supportButton(ctx, "💬 Поддержка"); support != nil {
		rows = append(rows, support)
	}
	s.notifyClientWithButtons(ctx, order, strings.Join([]string{
		"⚠️ Работа задерживается",
		"По заказу " + order.Number + " возникла техническая проблема - оператор уже разбирается.",
		"Пришлём сообщение, как только всё будет готово, или напишите в поддержку.",
	}, "\n"), urlInlineKeyboard(rows...))
}

// notifyExpired is sent when the payment window elapsed (template 8).
func (s *Service) notifyExpired(ctx context.Context, order *domain.Order) {
	rows := [][]inlineURLButton{}
	if button := s.newOrderButton("🔄 Новый заказ"); button != nil {
		rows = append(rows, button)
	}
	s.notifyClientWithButtons(ctx, order, strings.Join([]string{
		"⌛ Срок оплаты истёк",
		"Заказ " + order.Number + " отменён. Оформите новый заказ - цены и скидки сохранились.",
	}, "\n"), urlInlineKeyboard(rows...))
}

// notifyCancelled informs the client about an operator cancellation (template 9).
func (s *Service) notifyCancelled(ctx context.Context, order *domain.Order) {
	rows := [][]inlineURLButton{}
	if support := s.supportButton(ctx, "💬 Поддержка"); support != nil {
		rows = append(rows, support)
	}
	s.notifyClientWithButtons(ctx, order, strings.Join([]string{
		"🚫 Заказ " + order.Number + " отменён",
		"Если это ошибка - напишите в поддержку, поможем восстановить.",
	}, "\n"), urlInlineKeyboard(rows...))
}

// notifyQueued is used after an operator retry.
func (s *Service) notifyQueued(ctx context.Context, order *domain.Order) {
	s.notifyClientWithButtons(ctx, order, strings.Join([]string{
		"🔄 Заказ " + order.Number + " снова в очереди",
		"Обычно подготовка занимает от 3 до 20 минут. Пришлём файл этим сообщением.",
	}, "\n"), s.orderButtons(order, "👀 Следить за статусом"))
}

// NotifyCompleted is called by the worker when a document is ready.
func (s *Service) NotifyCompleted(ctx context.Context, order *domain.Order) {
	s.attachResultURL(order)
	s.notifyCompleted(ctx, order)
}

// NotifyFailed is called by the worker when generation failed.
func (s *Service) NotifyFailed(ctx context.Context, order *domain.Order, reason string) {
	s.notifyFailed(ctx, order, reason)
}

// NotifyExpired is called by the worker when the payment window elapsed.
func (s *Service) NotifyExpired(ctx context.Context, order *domain.Order) {
	s.notifyExpired(ctx, order)
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// orderButtonsList returns the order-screen button as a plain slice, so it can
// be combined with the support/review buttons on one row.
func (s *Service) orderButtonsList(order *domain.Order, label string) []inlineURLButton {
	link := s.Cfg.MiniAppLink("order_" + order.Number)
	if link == "" {
		return nil
	}
	return []inlineURLButton{{Text: label, URL: link}}
}

// resultAttachment is one document the bot uploads to the client.
type resultAttachment struct {
	Path string
	Name string
}

// resultAttachments resolves the stored documents of the newest run to paths
// the bot can upload (several works -> several documents, oldest first).
func (s *Service) resultAttachments(ctx context.Context, order *domain.Order) []resultAttachment {
	files, err := s.Store.ListResultFiles(ctx, order.Number)
	if err != nil || len(files) == 0 {
		return nil
	}
	out := make([]resultAttachment, 0, len(files))
	for _, file := range files {
		if file.StorageKey == "" {
			continue
		}
		path := s.ResultFilePath(file)
		if path == "" {
			continue
		}
		name := file.FileName
		if name == "" {
			name = "resultat_" + strings.ReplaceAll(order.Number, "-", "_")
		}
		out = append(out, resultAttachment{Path: path, Name: name})
	}
	return out
}

func unitsLine(order *domain.Order, product catalog.Product) string {
	if product.Fixed {
		return "1 работа"
	}
	word := format.Plural(order.Units, product.UnitOne, product.UnitFew, product.UnitMany)
	return fmt.Sprintf("%d %s", order.Units, word)
}

func short(value string, limit int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit-1]) + "…"
}

// almaty is the display timezone of the service (design.md §10.3). Kazakhstan
// is on UTC+5 with no DST, so a fixed zone keeps the bot independent of the
// host's tzdata.
var almaty = time.FixedZone("Asia/Almaty", 5*60*60)

var monthsShort = [...]string{
	"янв", "фев", "мар", "апр", "мая", "июн",
	"июл", "авг", "сен", "окт", "ноя", "дек",
}

// shortDate renders "17 окт, 14:05" / "сегодня, 14:05" / "вчера, 09:40".
func shortDate(at time.Time) string {
	local := at.In(almaty)
	now := time.Now().In(almaty)
	if local.Format("2006-01-02") == now.Format("2006-01-02") {
		return "сегодня, " + local.Format("15:04")
	}
	if local.Format("2006-01-02") == now.AddDate(0, 0, -1).Format("2006-01-02") {
		return "вчера, " + local.Format("15:04")
	}
	return fmt.Sprintf("%d %s, %s", local.Day(), monthsShort[int(local.Month())-1], local.Format("15:04"))
}
