package tgbot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/srunal/srunal-go/internal/catalog"
	"github.com/srunal/srunal-go/internal/domain"
	"github.com/srunal/srunal-go/internal/format"
	"github.com/srunal/srunal-go/internal/service"
	"github.com/srunal/srunal-go/internal/store"
)

var (
	orderNumberPattern = regexp.MustCompile(`(?i)\bDG-\d{4,}\b`)
	accessTokenPattern = regexp.MustCompile(`\b[A-Z2-9]{6}\b`)
)

// telegramAdminID stands in for chats listed in TELEGRAM_ADMIN_CHAT_IDS that
// were never bound to a database operator.
const telegramAdminID = "@telegram_admin"

// handleMessage routes one incoming message.
func (b *Bot) handleMessage(ctx context.Context, message Message) {
	text := strings.TrimSpace(message.Text)
	chatID := message.Chat.ID
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return
	}
	command := strings.ToLower(fields[0])
	if at := strings.Index(command, "@"); at > 0 {
		command = command[:at] // /start@srunal_bot
	}
	// Log the routing decision (never the message body: it may be private).
	b.log.Info("telegram message",
		"chat_id", chatID,
		"chat_type", message.Chat.Type,
		"from", message.From.DisplayName(),
		"command", command)

	switch command {
	case "/start":
		payload := ""
		if len(fields) > 1 {
			payload = fields[1]
		}
		if strings.HasPrefix(payload, "login_") {
			b.handleLogin(ctx, chatID, message.From, strings.TrimPrefix(payload, "login_"))
			return
		}
		b.sendWithButtons(ctx, chatID, b.welcomeText(ctx, chatID), b.welcomeButtons())
	case "/help":
		if len(fields) > 1 && strings.EqualFold(fields[1], "op") {
			b.sendOperatorHelp(ctx, chatID)
			return
		}
		b.sendWithButtons(ctx, chatID, b.welcomeText(ctx, chatID), b.welcomeButtons())
	case "/id", "/chatid", "/myid":
		b.send(ctx, chatID, idReply(chatID, message.From, b.svc.Cfg.Bot.Username))
	case "/login":
		if len(fields) < 2 {
			b.send(ctx, chatID, "Отправьте код так: /login 123456")
			return
		}
		b.handleLogin(ctx, chatID, message.From, fields[1])
	case "/bind":
		b.handleBind(ctx, chatID, fields)
	case "/status":
		b.handleClientStatus(ctx, chatID)
	case "/orders":
		b.handleOperatorOrders(ctx, chatID)
	case "/order":
		b.handleOperatorOrder(ctx, chatID, fields)
	case "/confirm":
		b.handleOperatorTransition(ctx, chatID, fields, "confirm")
	case "/reject":
		b.handleOperatorTransition(ctx, chatID, fields, "reject")
	case "/cancel":
		b.handleOperatorTransition(ctx, chatID, fields, "cancel")
	case "/retry":
		b.handleOperatorTransition(ctx, chatID, fields, "retry")
	case "/stats":
		b.handleStats(ctx, chatID)
	case "/pause":
		b.handleQueue(ctx, chatID, true)
	case "/resume":
		b.handleQueue(ctx, chatID, false)
	default:
		// A plain message: try to link an order ("DG-260391 CLS28G").
		if b.handleOrderLink(ctx, chatID, message, text) {
			return
		}
		b.send(ctx, chatID, "Не понял команду. Отправьте /help, чтобы увидеть список команд.")
	}
}

// welcomeText is the /start and /help answer: client-facing only. Operator
// commands are listed by /help op, and /id stays undocumented on purpose.
func (b *Bot) welcomeText(ctx context.Context, chatID int64) string {
	lines := []string{
		"SruNal KZ - академические работы по стандартам Satbayev University.",
		"",
		"Реферат, лабораторная, презентация или СРС? Расчёт цены, оплата по Kaspi QR",
		"и готовый файл - в приложении. Этот чат получает все уведомления по заказу.",
		"",
		"Команды:",
		"/status - статусы ваших заказов",
		"/help - эта справка",
	}

	if link := b.svc.Cfg.MiniAppLink(""); link != "" {
		lines = append(lines, "", "Открыть приложение: "+link)
	}
	if operator, ok := b.operator(ctx, chatID); ok {
		lines = append(lines, "", "Вы оператор "+operator.ID+". Команды панели: /help op")
	}
	return strings.Join(lines, "\n")
}

// welcomeButtons opens the Mini App from the /start message.
func (b *Bot) welcomeButtons() json.RawMessage {
	link := b.svc.Cfg.MiniAppLink("")
	if link == "" {
		return nil
	}
	return json.RawMessage(service.URLButtonRow("🚀 Открыть приложение", link))
}

// bindHint is the /bind instruction for chats that are not operators yet. It
// shows a real handle: the literal "@operator" placeholder it used to print was
// taken as a login, so every /bind failed with «неверный логин или пароль».
const bindHint = "/bind @operator_almaty пароль"

// sendOperatorHelp lists the operator commands and refuses for everyone else.
func (b *Bot) sendOperatorHelp(ctx context.Context, chatID int64) {
	if _, ok := b.operator(ctx, chatID); !ok {
		b.send(ctx, chatID, "Команды панели доступны только операторам. Сначала привяжите чат: "+bindHint)
		return
	}
	b.send(ctx, chatID, strings.Join([]string{
		"Команды оператора:",
		"/orders - очередь на проверку оплат",
		"/order DG-260391 - карточка заказа",
		"/confirm DG-260391 - подтвердить оплату",
		"/reject DG-260391 причина - отклонить оплату",
		"/cancel DG-260391 - отменить заказ",
		"/retry DG-260391 - повторная генерация",
		"/pause и /resume - остановить и запустить очередь",
		"/stats - сводка",
	}, "\n"))
}

// idReply tells the chat its own id so the operator can paste it into
// TELEGRAM_ADMIN_CHAT_IDS (works in private chats, groups and supergroups).
func idReply(chatID int64, from *User, botUsername string) string {
	lines := []string{
		"🆔 ID этого чата: " + fmt.Sprint(chatID),
	}
	if from != nil && from.ID != 0 && from.ID != chatID {
		lines = append(lines, "Ваш личный ID: "+fmt.Sprint(from.ID))
	}
	bot := botUsername
	if bot == "" {
		bot = "srunal_bot"
	}
	lines = append(lines,
		"",
		"Чтобы получать уведомления оператора, вставьте это число в srunal_go/.env:",
		"TELEGRAM_ADMIN_CHAT_IDS="+fmt.Sprint(chatID),
		"для нескольких чатов - через запятую. Затем перезапустите API.",
		"",
		"Альтернатива без .env: привязать чат командой "+bindHint+" -",
		"тогда оператор получает уведомления автоматически. Подсказка: /help",
		"Бот: @"+bot,
	)
	return strings.Join(lines, "\n")
}

// clientTokenAlphabet matches auth.AccessToken.
const clientTokenAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// handleOrderLink links a chat to an order when the message carries the pair.
func (b *Bot) handleOrderLink(ctx context.Context, chatID int64, message Message, text string) bool {
	numbers := orderNumberPattern.FindAllString(text, -1)
	if len(numbers) == 0 {
		return false
	}
	if b.limitLinks(ctx, chatID) {
		b.send(ctx, chatID, "Слишком много попыток привязки. Подождите пару минут.")
		return true
	}

	candidates := accessTokenPattern.FindAllString(strings.ToUpper(text), -1)
	for _, number := range numbers {
		number = strings.ToUpper(number)
		for _, candidate := range candidates {
			if !strings.ContainsAny(candidate, clientTokenAlphabet) {
				continue
			}
			order, err := b.svc.GetOrderByToken(ctx, number, candidate)
			if err != nil || order == nil {
				continue
			}
			contact := ""
			if message.From != nil && message.From.Username != "" {
				contact = "@" + message.From.Username
			}
			if err := b.svc.Store.LinkOrderChat(ctx, order.Number, chatID, contact); err != nil {
				b.log.Error("telegram link order", "order", order.Number, "error", err)
				b.send(ctx, chatID, "Не удалось привязать заказ, попробуйте позже.")
				return true
			}
			_ = b.svc.Store.AppendEvent(ctx, order.Number, domain.ActorClient,
				"Клиент привязал Telegram-чат для уведомлений")
			b.send(ctx, chatID, "✅ Заказ "+order.Number+" привязан. "+
				"Теперь я буду присылать сюда статусы и готовый файл.")
			return true
		}
	}

	b.send(ctx, chatID, "Не нашёл заказ с таким номером и кодом доступа. "+
		"Проверьте: номер вида DG-260391 и код из 6 символов со страницы заказа.")
	return true
}

// handleClientStatus lists the orders linked to this chat (design.md §8.2
// template 10: status emoji, number, amount, status label + app button).
func (b *Bot) handleClientStatus(ctx context.Context, chatID int64) {
	orders, err := b.svc.Store.ListOrdersForChat(ctx, chatID, 10)
	if err != nil {
		b.log.Error("telegram client status", "error", err)
		b.send(ctx, chatID, "Не удалось получить список заказов, попробуйте позже.")
		return
	}
	if len(orders) == 0 {
		lines := []string{
			"Пока нет заказов в этом чате.",
			"Оформите первую работу в приложении - расчёт цены займёт около трёх минут.",
		}
		b.sendWithButtons(ctx, chatID, strings.Join(lines, "\n"), b.welcomeButtons())
		return
	}

	lines := []string{"Ваши заказы:"}
	for _, order := range orders {
		status := catalog.Statuses[order.Status].Label
		lines = append(lines, fmt.Sprintf("%s %s · %s · %s",
			statusEmoji(order.Status), order.Number, format.Money(order.Amount), status))
	}
	var buttons json.RawMessage
	if link := b.svc.Cfg.MiniAppLink("orders"); link != "" {
		buttons = json.RawMessage(service.URLButtonRow("🚀 Открыть приложение", link))
	}
	b.sendWithButtons(ctx, chatID, strings.Join(lines, "\n"), buttons)
}

// statusEmoji is the icon of a status inside bot messages (design.md §8.2).
func statusEmoji(status string) string {
	switch status {
	case domain.StatusAwaitingPayment:
		return "⏳"
	case domain.StatusPaymentClaimed:
		return "🔎"
	case domain.StatusPaymentRejected:
		return "❌"
	case domain.StatusQueued:
		return "🕒"
	case domain.StatusGenerating:
		return "⌛"
	case domain.StatusCompleted:
		return "🎓"
	case domain.StatusFailedFinal:
		return "⚠️"
	case domain.StatusExpired:
		return "⌛"
	case domain.StatusCancelled:
		return "🚫"
	default:
		return "•"
	}
}

// ---------------------------------------------------------------------------
// operator commands
// ---------------------------------------------------------------------------

// operator resolves the operator behind a chat. Chats listed in
// TELEGRAM_ADMIN_CHAT_IDS are trusted without /bind; a bot without a store
// (unit tests) only sees those trusted chats.
func (b *Bot) operator(ctx context.Context, chatID int64) (store.Operator, bool) {
	if b.svc != nil && b.svc.Store != nil {
		if operator, err := b.svc.Store.OperatorByChatID(ctx, chatID); err == nil {
			return operator, true
		}
	}
	for _, admin := range b.svc.Cfg.Bot.AdminChatIDs {
		if admin == chatID {
			return store.Operator{ID: telegramAdminID, DisplayName: "Telegram admin"}, true
		}
	}
	return store.Operator{}, false
}

// handleBind links the chat to an operator after checking the password.
func (b *Bot) handleBind(ctx context.Context, chatID int64, fields []string) {
	if len(fields) < 3 {
		b.send(ctx, chatID, "Формат: /bind @operator_almaty пароль")
		return
	}
	if err := b.svc.BindOperatorChat(ctx, fields[1], fields[2], chatID); err != nil {
		var apiErr *service.APIError
		if errors.As(err, &apiErr) {
			b.send(ctx, chatID, "Не удалось привязать: "+apiErr.Message)
			return
		}
		b.log.Error("telegram bind", "error", err)
		b.send(ctx, chatID, "Не удалось привязать чат, попробуйте позже.")
		return
	}
	b.send(ctx, chatID, "✅ Чат привязан к оператору "+fields[1]+
		". Команды панели: /help op")
}

// handleLogin confirms a waiting web session (/login 123456).
//
// Client codes need no binding: the chat becomes the client identity and the
// orders created in that browser attach to it. Operator codes still require a
// chat that is bound to an operator (or a handle pre-filled by the page).
func (b *Bot) handleLogin(ctx context.Context, chatID int64, from *User, code string) {
	code = strings.TrimSpace(code)
	if len(code) != 6 {
		b.send(ctx, chatID, "Код состоит из 6 цифр. Формат: /login 123456")
		return
	}

	challenge, err := b.svc.Store.GetLoginChallenge(ctx, code)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			b.send(ctx, chatID, "Код не найден или истёк. Обновите страницу входа и запросите новый.")
			return
		}
		b.log.Error("telegram login lookup", "error", err)
		b.send(ctx, chatID, "Не удалось подтвердить вход, попробуйте позже.")
		return
	}

	if challenge.Kind == store.ChallengeOperator {
		// The panel session can only open for a known operator: the one this
		// chat is bound to, or the one the login page pre-filled.
		operatorID := challenge.OperatorID
		if operator, err := b.svc.Store.OperatorByChatID(ctx, chatID); err == nil {
			operatorID = operator.ID
		}
		if operatorID == "" {
			b.send(ctx, chatID, strings.Join([]string{
				"Этот чат не привязан к оператору, поэтому вход в панель не откроется.",
				"Отправьте " + bindHint + " (логин - как в панели) и повторите /login " + code,
			}, "\n"))
			return
		}
	}

	username := ""
	if from != nil && from.Username != "" {
		username = "@" + from.Username
	}
	if err := b.svc.Store.ConfirmLoginChallenge(ctx, code, chatID, username); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			b.send(ctx, chatID, "Код не найден или истёк. Обновите страницу входа и запросите новый.")
			return
		}
		b.log.Error("telegram login confirm", "error", err)
		b.send(ctx, chatID, "Не удалось подтвердить вход, попробуйте позже.")
		return
	}
	if challenge.Kind == store.ChallengeOperator {
		b.send(ctx, chatID, "✅ Вход подтверждён. Вернитесь на страницу входа - панель откроется автоматически.")
		return
	}
	b.send(ctx, chatID, strings.Join([]string{
		"✅ Вход подтверждён. Вернитесь на сайт - окно закроется само.",
		"Заказы из этого браузера будут приходить в этот чат: статусы, оплата и готовый файл.",
	}, "\n"))
}

// handleOperatorOrders lists the orders that need a payment check.
func (b *Bot) handleOperatorOrders(ctx context.Context, chatID int64) {
	if _, ok := b.operator(ctx, chatID); !ok {
		b.send(ctx, chatID, "Команда доступна операторам. Привяжите чат: "+bindHint)
		return
	}
	orders, err := b.svc.Store.ListOrdersByStatus(ctx, []string{domain.StatusPaymentClaimed}, 10)
	if err != nil {
		b.log.Error("telegram operator orders", "error", err)
		b.send(ctx, chatID, "Не удалось получить список, попробуйте позже.")
		return
	}
	if len(orders) == 0 {
		b.send(ctx, chatID, "Очередь проверки оплат пуста ✅")
		return
	}

	lines := []string{fmt.Sprintf("Оплаты на проверку: %d", len(orders))}
	for _, order := range orders {
		product := catalog.Get(order.Product)
		lines = append(lines, fmt.Sprintf("• %s • %s • %s • отправитель: %s",
			order.Number, product.Name, format.Money(order.Amount), fallback(order.SenderName, "не указан")))
		lines = append(lines, "  /confirm "+order.Number+" или /reject "+order.Number+" причина")
	}
	b.send(ctx, chatID, strings.Join(lines, "\n"))
}

// handleOperatorOrder shows one order card.
func (b *Bot) handleOperatorOrder(ctx context.Context, chatID int64, fields []string) {
	if _, ok := b.operator(ctx, chatID); !ok {
		b.send(ctx, chatID, "Команда доступна операторам. Привяжите чат: "+bindHint)
		return
	}
	if len(fields) < 2 {
		b.send(ctx, chatID, "Формат: /order DG-260391")
		return
	}
	order, err := b.svc.AdminOrder(ctx, fields[1])
	if err != nil {
		var apiErr *service.APIError
		if errors.As(err, &apiErr) {
			b.send(ctx, chatID, apiErr.Message)
			return
		}
		b.send(ctx, chatID, "Не удалось открыть заказ, попробуйте позже.")
		return
	}

	lines := []string{
		"Заказ " + order.Number + " • " + catalog.Statuses[order.Status].Label,
		fmt.Sprintf("%s • %s • %s", service.ProductLabel(order), order.Topics, format.Money(order.Amount)),
		"Студент: " + order.StudentName + " (" + order.Institute + ", " + order.Department + ")",
		"Контакт: " + fallback(order.Contact, order.ContactName),
		"Создан: " + order.CreatedAt.Local().Format("02.01.2006 15:04"),
	}
	if order.SenderName != "" {
		lines = append(lines, "Отправитель Kaspi: "+order.SenderName)
	}
	if order.PaymentRejectedReason != nil {
		lines = append(lines, "Отклонён ранее: "+*order.PaymentRejectedReason)
	}
	if len(order.Events) > 0 {
		lines = append(lines, "Последнее событие: "+order.Events[0].Text)
	}
	var buttons json.RawMessage
	if link := b.svc.Cfg.MiniAppLink("order_" + order.Number); link != "" {
		buttons = json.RawMessage(service.URLButtonRow("👀 Открыть в приложении", link))
	} else {
		lines = append(lines, "Панель: "+b.svc.Cfg.FrontendBaseURL+"/admin/orders/"+order.Number)
	}
	b.sendWithButtons(ctx, chatID, strings.Join(lines, "\n"), buttons)
}

// handleOperatorTransition runs /confirm, /reject, /cancel and /retry.
func (b *Bot) handleOperatorTransition(ctx context.Context, chatID int64, fields []string, action string) {
	operator, ok := b.operator(ctx, chatID)
	if !ok {
		b.send(ctx, chatID, "Команда доступна операторам. Привяжите чат: "+bindHint)
		return
	}
	if len(fields) < 2 {
		b.send(ctx, chatID, "Укажите номер заказа, например: /"+action+" DG-260391")
		return
	}
	number := strings.ToUpper(strings.TrimSpace(fields[1]))

	var (
		order      *domain.Order
		err        error
		successMsg string
	)
	switch action {
	case "confirm":
		order, err = b.svc.ConfirmPayment(ctx, number, operator.ID)
		successMsg = "✅ Оплата подтверждена, заказ " + number + " в очереди."
	case "reject":
		reason := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(strings.Join(fields[1:], " ")), number))
		if reason == "" {
			b.send(ctx, chatID, "Укажите причину: /reject "+number+" сумма не совпадает")
			return
		}
		order, err = b.svc.RejectPayment(ctx, number, reason)
		successMsg = "❌ Оплата отклонена, клиент увидит причину."
	case "cancel":
		order, err = b.svc.CancelOrder(ctx, number, "отменён оператором через бота")
		successMsg = "🚫 Заказ " + number + " отменён."
	case "retry":
		order, err = b.svc.RetryGeneration(ctx, number)
		successMsg = "🔄 Заказ " + number + " снова в очереди."
	}
	if err != nil {
		var apiErr *service.APIError
		if errors.As(err, &apiErr) {
			b.send(ctx, chatID, apiErr.Message)
			return
		}
		b.log.Error("telegram transition", "action", action, "order", number, "error", err)
		b.send(ctx, chatID, "Не удалось выполнить действие, попробуйте позже.")
		return
	}
	b.send(ctx, chatID, successMsg)
	_ = order
}

// handleStats shows the operational summary.
func (b *Bot) handleStats(ctx context.Context, chatID int64) {
	if _, ok := b.operator(ctx, chatID); !ok {
		b.send(ctx, chatID, "Команда доступна операторам. Привяжите чат: "+bindHint)
		return
	}
	stats, err := b.svc.Stats(ctx)
	if err != nil {
		b.log.Error("telegram stats", "error", err)
		b.send(ctx, chatID, "Не удалось собрать статистику, попробуйте позже.")
		return
	}
	settings, _ := b.svc.GetSettings(ctx)

	lines := []string{
		"📊 Сводка SruNal",
		"Всего заказов: " + fmt.Sprint(stats.OrdersTotal),
		"Оплаты на проверку: " + fmt.Sprint(stats.OrdersByStatus[domain.StatusPaymentClaimed]),
		"В очереди: " + fmt.Sprint(stats.OrdersByStatus[domain.StatusQueued]),
		"Генерируется: " + fmt.Sprint(stats.OrdersByStatus[domain.StatusGenerating]),
		"Готово: " + fmt.Sprint(stats.OrdersByStatus[domain.StatusCompleted]),
		"Проблемные: " + fmt.Sprint(stats.OrdersByStatus[domain.StatusFailedFinal]),
		"Подтверждённая выручка: " + format.Money(stats.ConfirmedRevenue),
		"Заказов сегодня: " + fmt.Sprint(stats.CompletedToday),
	}
	if stats.QueueWarning {
		lines = append(lines, "⚠️ Старейший заказ в очереди ждёт больше 30 минут")
	}
	queue := "идёт ✅"
	if settings.QueuePaused {
		queue = "на паузе ⏸ (/resume)"
	}
	lines = append(lines, "Очередь: "+queue)
	b.send(ctx, chatID, strings.Join(lines, "\n"))
}

// handleQueue pauses or resumes the generation queue.
func (b *Bot) handleQueue(ctx context.Context, chatID int64, paused bool) {
	if _, ok := b.operator(ctx, chatID); !ok {
		b.send(ctx, chatID, "Команда доступна операторам. Привяжите чат: "+bindHint)
		return
	}
	if err := b.svc.Store.SetQueuePaused(ctx, paused); err != nil {
		b.log.Error("telegram queue switch", "error", err)
		b.send(ctx, chatID, "Не удалось переключить очередь, попробуйте позже.")
		return
	}
	if paused {
		b.send(ctx, chatID, "⏸ Очередь генерации остановлена. Текущие задания завершатся. Запуск: /resume")
	} else {
		b.send(ctx, chatID, "▶️ Очередь генерации запущена.")
	}
}

// ---------------------------------------------------------------------------
// inline buttons (payment confirmation)
// ---------------------------------------------------------------------------

// callbackPrefixes of the inline buttons the bot sends.
const (
	callbackConfirm = "confirm:"
	callbackReject  = "reject:"
)

// handleCallback processes an inline-button press. Payment buttons are attached
// to the operator's notification, so the chat must be an operator's; the answer
// is shown as a toast and the original message is rewritten without buttons.
func (b *Bot) handleCallback(ctx context.Context, query CallbackQuery) {
	chatID := int64(0)
	messageID := int64(0)
	if query.Message != nil {
		chatID = query.Message.Chat.ID
		messageID = query.Message.MessageID
	}

	action := ""
	number := ""
	switch {
	case strings.HasPrefix(query.Data, callbackConfirm):
		action, number = "confirm", strings.TrimPrefix(query.Data, callbackConfirm)
	case strings.HasPrefix(query.Data, callbackReject):
		action, number = "reject", strings.TrimPrefix(query.Data, callbackReject)
	}
	if action == "" || !strings.HasPrefix(number, "DG-") {
		b.answerCallback(ctx, query.ID, "Не понял действие")
		return
	}

	operator, ok := b.operator(ctx, chatID)
	if !ok {
		b.answerCallback(ctx, query.ID, "Подтверждать оплату могут только операторы")
		return
	}

	switch action {
	case "confirm":
		if _, err := b.svc.ConfirmPayment(ctx, number, operator.ID); err != nil {
			b.answerCallback(ctx, query.ID, b.apiErrorMessage(err))
			return
		}
		b.answerCallback(ctx, query.ID, "Оплата подтверждена, заказ в очереди")
		b.editCallbackMessage(ctx, chatID, messageID, strings.Join([]string{
			"✅ Оплата подтверждена",
			"Заказ " + number + " отправлен на генерацию (" + operator.ID + ").",
		}, "\n"))
	case "reject":
		reason := b.defaultRejectReason(ctx)
		if _, err := b.svc.RejectPayment(ctx, number, reason); err != nil {
			b.answerCallback(ctx, query.ID, b.apiErrorMessage(err))
			return
		}
		b.answerCallback(ctx, query.ID, "Оплата отклонена, клиент уведомлён")
		b.editCallbackMessage(ctx, chatID, messageID, strings.Join([]string{
			"❌ Оплата отклонена",
			"Заказ " + number + " (" + operator.ID + "). Клиент получил сообщение с причиной.",
		}, "\n"))
	}
}

// defaultRejectReason is what the client sees when the operator presses
// «Отклонить»: the wording already points to the tech support chat.
func (b *Bot) defaultRejectReason(ctx context.Context) string {
	support := "@meownuarka"
	if settings, err := b.svc.GetSettings(ctx); err == nil && strings.TrimSpace(settings.SupportTelegram) != "" {
		support = strings.TrimSpace(settings.SupportTelegram)
	}
	return "Оплата не найдена в Kaspi: проверьте отправителя и сумму перевода. " +
		"Если это ошибка, обратитесь в техподдержку " + support
}

// apiErrorMessage extracts the Russian message of a service error.
func (b *Bot) apiErrorMessage(err error) string {
	var apiErr *service.APIError
	if errors.As(err, &apiErr) {
		return apiErr.Message
	}
	b.log.Error("telegram callback action failed", "error", err)
	return "Не удалось выполнить действие, попробуйте позже"
}

// answerCallback shows a short toast on the pressed button.
func (b *Bot) answerCallback(ctx context.Context, id, text string) {
	sender := b.sender
	if sender == nil {
		sender = b.client
	}
	callCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := sender.answerCallbackQuery(callCtx, id, text); err != nil {
		b.log.Error("telegram answer callback failed", "error", err)
	}
}

// editCallbackMessage rewrites the message the button belonged to (the keyboard
// disappears: the payment is already handled).
func (b *Bot) editCallbackMessage(ctx context.Context, chatID, messageID int64, text string) {
	if chatID == 0 || messageID == 0 {
		return
	}
	sender := b.sender
	if sender == nil {
		sender = b.client
	}
	callCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := sender.editMessageText(callCtx, chatID, messageID, text); err != nil {
		b.log.Error("telegram edit callback message failed", "error", err)
	}
}

// sendWithButtons sends a reply immediately, with an optional inline keyboard.
func (b *Bot) sendWithButtons(ctx context.Context, chatID int64, text string, markup json.RawMessage) {
	sender := b.sender
	if sender == nil {
		sender = b.client
	}
	sendCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if err := sender.sendMessage(sendCtx, chatID, text, markup); err != nil {
		b.log.Error("telegram reply failed", "chat_id", chatID, "error", err)
	}
}

// limitLinks throttles order-link attempts per chat: ten tries per ten minutes.
func (b *Bot) limitLinks(ctx context.Context, chatID int64) bool {
	const window = 10 * time.Minute
	const maxAttempts = 10

	key := "link_attempts:" + fmt.Sprint(chatID)
	raw, _, err := b.svc.Store.KVGet(ctx, key)
	if err == nil && raw != "" {
		parts := strings.SplitN(raw, ":", 2)
		if len(parts) == 2 {
			var count int
			var startedAt int64
			_, scanCount := fmt.Sscan(parts[0], &count)
			_, scanTime := fmt.Sscan(parts[1], &startedAt)
			if scanCount == nil && scanTime == nil {
				if time.Since(time.Unix(startedAt, 0)) < window && count >= maxAttempts {
					return true
				}
			}
		}
	}
	now := time.Now().Unix()
	_ = b.svc.Store.KVSet(ctx, key, fmt.Sprintf("1:%d", now))
	return false
}

func fallback(value, empty string) string {
	if strings.TrimSpace(value) == "" {
		return empty
	}
	return value
}
