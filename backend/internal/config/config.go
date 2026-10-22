// Package config loads the service configuration from environment variables
// and an optional .env file next to the binary (see .env.example).
package config

import (
	"bufio"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Config is the full runtime configuration. Everything has a development
// default so `go run ./cmd/api` works right after migrations.
type Config struct {
	Env         string // dev | prod
	HTTPAddr    string
	DatabaseURL string
	// CORS origins and the admin-panel origin used inside bot messages.
	CORSOrigins []string
	// PublicBaseURL is used to build absolute URLs (result files, QR images).
	PublicBaseURL string
	// FrontendBaseURL is the web app origin (links inside Telegram messages).
	FrontendBaseURL string
	// MiniAppDir is the built Telegram Mini App (the Vite dist folder). When
	// set, the API serves it as a single-page app on every non-/api route.
	MiniAppDir string

	// TokenPepper hardens the stored hashes of order access tokens and
	// admin session tokens. Change it in production.
	TokenPepper string
	// SessionTTL is the lifetime of an admin session (default 12h).
	SessionTTL time.Duration
	// ClientSessionTTL is the lifetime of a client session opened through the
	// bot (default 30 days: clients should stay signed in).
	ClientSessionTTL time.Duration

	// StorageDir keeps uploaded client materials, result documents and the
	// Kaspi QR image.
	StorageDir string
	// ResultTTL is how long a finished document stays downloadable (30 days).
	ResultTTL time.Duration

	// SeedOnStart inserts the demo data (seed.ts port) when the database is
	// empty. Disable in production.
	SeedOnStart bool

	// TrustProxy honours X-Forwarded-For for the client IP (enable behind a
	// reverse proxy you control; off by default so the header cannot be spoofed).
	TrustProxy bool

	// Rate limits (requests per window per client IP).
	CreateOrderPerHour int
	ReviewPerHour      int
	PromoCheckPerMin   int
	LoginMaxAttempts   int
	LoginLockout       time.Duration

	Worker WorkerConfig
	Bot    TelegramConfig
}

// WorkerConfig drives the background generation queue.
type WorkerConfig struct {
	Enabled bool
	// PollInterval is how often the queue is scanned (default 3s).
	PollInterval time.Duration
	// QueuePauseOnStart stops the worker from picking new orders.
	QueuePauseOnStart bool
	// GenerationMin/Max simulate document generation time with the built-in
	// generator. Real generation (the Cline agent) uses its own timeout.
	GenerationMin time.Duration
	GenerationMax time.Duration
	// ExpireAfter sets when an unpaid order becomes `expired` (default 24h).
	ExpireAfter time.Duration
	// StaleGeneratingAfter requeues orders stuck in `generating`. It is a floor,
	// not a ceiling: the worker raises it when the configured agent timeout and
	// retries need more room (a live run must never be requeued).
	StaleGeneratingAfter time.Duration

	// GeneratorMode selects the document generator:
	//   auto    - the agent command when it resolves, otherwise builtin (default)
	//   agent   - always the external command (fails when it is missing)
	//   builtin - the bundled .docx writer (demo/fallback)
	GeneratorMode string
	// GeneratorCommand is the external agent CLI, e.g. `cline` (Cline CLI).
	GeneratorCommand string
	// GeneratorArgs are extra arguments appended to the agent command line.
	GeneratorArgs []string
	// GeneratorProvider/GeneratorModel/GeneratorThinking pin the model the
	// agent runs on, e.g. opencode-go / deepseek-v4.1-flash / high.
	GeneratorProvider string
	GeneratorModel    string
	GeneratorThinking string
	// GeneratorAPIKey is optional: when set it is exported to the agent process
	// under the provider's key variable (see GeneratorAPIKeyEnv) for BYOK setups.
	GeneratorAPIKey string
	// GeneratorAPIKeyEnv names the environment variable(s) the key is exported
	// as; comma-separated. Empty derives the name from the provider, e.g.
	// opencode-go -> OPENCODE_API_KEY, deepseek -> DEEPSEEK_API_KEY.
	GeneratorAPIKeyEnv string
	// GeneratorConfigDir/GeneratorDataDir are passed to the agent CLI as
	// --config / --data-dir (Cline keeps credentials and state there).
	GeneratorConfigDir string
	GeneratorDataDir   string
	// GeneratorTimeout caps one agent run.
	GeneratorTimeout time.Duration
	// GeneratorDir keeps the per-order task files and agent output.
	GeneratorDir string

	// MaxAgents is how many orders are generated in parallel (2-3 keeps the
	// queue moving without hammering the model provider).
	MaxAgents int
	// AgentRetries is how many extra attempts an order gets after a failed run.
	AgentRetries int

	// SkillSourceDir is where the global agent skills live
	// (default ~/.agents/skills).
	SkillSourceDir string
	// SkillGOST is the skill for essay/lab/other (GOST documents).
	SkillGOST string
	// SkillPPTX is the skill for presentations.
	SkillPPTX string
}

// TelegramConfig configures the bot that runs inside the API server.
type TelegramConfig struct {
	Enabled bool
	// Token is the BotFather token, e.g. 123456789:AAH... This is the
	// "ключ" to paste: set TELEGRAM_BOT_TOKEN in .env.
	Token string
	// AdminChatIDs receive operator notifications.
	AdminChatIDs []int64
	// PollTimeout is the long-polling timeout for getUpdates (default 25s).
	PollTimeout time.Duration
	// Username is the bot handle without @ (used in deep links), optional.
	Username string
	// MiniAppShortName is the Mini App short name from BotFather
	// (t.me/<bot>/<short_name>). Together with Username it enables Mini App
	// deep links inside bot notifications.
	MiniAppShortName string
}

// IsDev reports whether the service runs in the development profile.
func (c *Config) IsDev() bool { return c.Env != "prod" }

// Load reads .env (if present) and then the process environment.
// Real environment variables always win over .env values.
func Load() (*Config, error) {
	loadDotEnv(".env")

	cfg := &Config{
		Env:         getEnv("APP_ENV", "dev"),
		HTTPAddr:    getEnv("HTTP_ADDR", ":8080"),
		DatabaseURL: getEnv("DATABASE_URL", ""),
		CORSOrigins: getEnvList("CORS_ORIGINS", []string{
			"http://localhost:5173",
			"http://127.0.0.1:5173",
		}),
		PublicBaseURL:    strings.TrimRight(getEnv("PUBLIC_BASE_URL", "http://localhost:8080"), "/"),
		FrontendBaseURL:  strings.TrimRight(getEnv("FRONTEND_BASE_URL", "http://localhost:5173"), "/"),
		MiniAppDir:       getEnv("MINIAPP_DIR", ""),
		TokenPepper:      getEnv("TOKEN_PEPPER", "dev-pepper-change-me"),
		SessionTTL:       getEnvDuration("SESSION_TTL", 12*time.Hour),
		ClientSessionTTL: getEnvDuration("CLIENT_SESSION_TTL", 30*24*time.Hour),
		StorageDir:       getEnv("STORAGE_DIR", filepath.Join("var", "storage")),
		ResultTTL:        getEnvDuration("RESULT_TTL", 30*24*time.Hour),
		SeedOnStart:      getEnvBool("SEED_ON_START", true),
		TrustProxy:       getEnvBool("TRUST_PROXY", false),

		CreateOrderPerHour: getEnvInt("RATE_LIMIT_ORDERS_PER_HOUR", 20),
		ReviewPerHour:      getEnvInt("RATE_LIMIT_REVIEWS_PER_HOUR", 10),
		PromoCheckPerMin:   getEnvInt("RATE_LIMIT_PROMO_PER_MIN", 60),
		LoginMaxAttempts:   getEnvInt("LOGIN_MAX_ATTEMPTS", 5),
		LoginLockout:       getEnvDuration("LOGIN_LOCKOUT", 15*time.Minute),

		Worker: WorkerConfig{
			Enabled:              getEnvBool("WORKER_ENABLED", true),
			PollInterval:         getEnvDuration("WORKER_POLL_INTERVAL", 3*time.Second),
			QueuePauseOnStart:    getEnvBool("WORKER_QUEUE_PAUSED_ON_START", false),
			GenerationMin:        getEnvDuration("WORKER_GENERATION_MIN", 45*time.Second),
			GenerationMax:        getEnvDuration("WORKER_GENERATION_MAX", 120*time.Second),
			ExpireAfter:          getEnvDuration("WORKER_EXPIRE_AFTER", 24*time.Hour),
			StaleGeneratingAfter: getEnvDuration("WORKER_STALE_GENERATING_AFTER", 30*time.Minute),

			GeneratorMode:    getEnv("GENERATOR_MODE", "auto"),
			GeneratorCommand: getEnv("GENERATOR_COMMAND", "cline"),
			GeneratorArgs:    getEnvList("GENERATOR_EXTRA_ARGS", nil),

			GeneratorProvider:  getEnv("GENERATOR_PROVIDER", "opencode-go"),
			GeneratorModel:     getEnv("GENERATOR_MODEL", "deepseek-v4.1-flash"),
			GeneratorThinking:  getEnv("GENERATOR_THINKING", "high"),
			GeneratorAPIKey:    getEnv("GENERATOR_API_KEY", ""),
			GeneratorAPIKeyEnv: getEnv("GENERATOR_API_KEY_ENV", ""),

			GeneratorConfigDir: getEnv("GENERATOR_CONFIG_DIR", ""),
			GeneratorDataDir:   getEnv("GENERATOR_DATA_DIR", ""),

			GeneratorTimeout: getEnvDuration("GENERATOR_TIMEOUT", 20*time.Minute),
			GeneratorDir:     getEnv("GENERATOR_DIR", filepath.Join("var", "generator")),

			MaxAgents:      getEnvInt("WORKER_MAX_AGENTS", 3),
			AgentRetries:   getEnvInt("WORKER_AGENT_RETRIES", 1),
			SkillSourceDir: getEnv("SKILL_SOURCE_DIR", defaultSkillsDir()),
			SkillGOST:      getEnv("GENERATOR_SKILL_GOST", "satbayev-gost"),
			SkillPPTX:      getEnv("GENERATOR_SKILL_PPTX", "pptx"),
		},

		Bot: TelegramConfig{
			Enabled:          getEnvBool("TELEGRAM_BOT_ENABLED", false),
			Token:            getEnv("TELEGRAM_BOT_TOKEN", ""),
			AdminChatIDs:     getEnvInt64List("TELEGRAM_ADMIN_CHAT_IDS", nil),
			PollTimeout:      getEnvDuration("TELEGRAM_POLL_TIMEOUT", 25*time.Second),
			Username:         strings.TrimPrefix(getEnv("TELEGRAM_BOT_USERNAME", ""), "@"),
			MiniAppShortName: strings.TrimPrefix(getEnv("TELEGRAM_MINIAPP_SHORT_NAME", ""), "@"),
		},
	}

	if cfg.DatabaseURL == "" {
		return nil, errors.New("DATABASE_URL is not set (see .env.example)")
	}
	if cfg.Worker.GenerationMax < cfg.Worker.GenerationMin {
		cfg.Worker.GenerationMax = cfg.Worker.GenerationMin
	}
	// A bot without a token is simply disabled, never an error: the API must
	// start even before the operator pastes the key.
	if cfg.Bot.Enabled && strings.TrimSpace(cfg.Bot.Token) == "" {
		cfg.Bot.Enabled = false
	}
	return cfg, nil
}

// BotConfigured reports whether the Telegram bot can start.
func (c *Config) BotConfigured() bool {
	return c.Bot.Enabled && strings.TrimSpace(c.Bot.Token) != ""
}

// MiniAppLink builds a t.me deep link that opens a specific Mini App screen,
// e.g. "order_DG-260391" ->
//
//	https://t.me/srunal_bot/srunal?startapp=order_DG-260391
//
// Empty when the bot handle or the Mini App short name is not configured;
// callers keep messages plain in that case.
func (c *Config) MiniAppLink(startParam string) string {
	if c.Bot.Username == "" || c.Bot.MiniAppShortName == "" {
		return ""
	}
	link := "https://t.me/" + c.Bot.Username + "/" + c.Bot.MiniAppShortName
	if trimmed := strings.TrimSpace(startParam); trimmed != "" {
		link += "?startapp=" + url.QueryEscape(trimmed)
	}
	return link
}

// loadDotEnv parses a minimal KEY=VALUE file. Missing file is not an error.
func loadDotEnv(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		eq := strings.IndexByte(line, '=')
		if eq <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		value := strings.TrimSpace(line[eq+1:])
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}
		if _, exists := os.LookupEnv(key); exists {
			continue // the real environment wins
		}
		_ = os.Setenv(key, value)
	}
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return fallback
}

// defaultSkillsDir is where the global agent skills live on this machine
// (~/.agents/skills; on Windows %USERPROFILE%\.agents\skills).
func defaultSkillsDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".agents", "skills")
	}
	return filepath.Join(home, ".agents", "skills")
}

func getEnvInt(key string, fallback int) int {
	raw := os.Getenv(key)
	if strings.TrimSpace(raw) == "" {
		return fallback
	}
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return fallback
	}
	return value
}

func getEnvBool(key string, fallback bool) bool {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	switch raw {
	case "1", "true", "yes", "on", "y":
		return true
	case "0", "false", "no", "off", "n":
		return false
	default:
		return fallback
	}
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	if value, err := time.ParseDuration(raw); err == nil {
		return value
	}
	// Bare numbers are seconds: WORKER_POLL_INTERVAL=3
	if seconds, err := strconv.Atoi(raw); err == nil {
		return time.Duration(seconds) * time.Second
	}
	return fallback
}

func getEnvList(key string, fallback []string) []string {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	if len(out) == 0 {
		return fallback
	}
	return out
}

func getEnvInt64List(key string, fallback []int64) []int64 {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	out := make([]int64, 0)
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		value, err := strconv.ParseInt(part, 10, 64)
		if err != nil {
			fmt.Fprintf(os.Stderr, "config: %s: ignoring %q: %v\n", key, part, err)
			continue
		}
		out = append(out, value)
	}
	if len(out) == 0 {
		return fallback
	}
	return out
}
