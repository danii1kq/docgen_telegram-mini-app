package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/srunal/srunal-go/internal/auth"
	"github.com/srunal/srunal-go/internal/domain"
	"github.com/srunal/srunal-go/internal/store"
)

// The admin console shows the agent run as a readable event stream instead of
// the raw JSON: streaming deltas are dropped, meaningful events are rendered.
const (
	consoleIteration = "iteration"
	consoleText      = "text"
	consoleReasoning = "reasoning"
	consoleTool      = "tool"
	consoleUsage     = "usage"
	consoleError     = "error"
	consoleResult    = "result"
	consoleEvent     = "event"
)

// consoleTailBytes is how much of agent.log is parsed per request and
// consoleMaxLines caps the rendered events (older ones are dropped).
const (
	consoleTailBytes = 256 << 10
	consoleReadBytes = 4 << 20
	consoleWantLines = 60
	consoleMaxLines  = 400
)

// ConsoleLine is one rendered event of a generation run.
type ConsoleLine struct {
	At   string `json:"at,omitempty"`
	Kind string `json:"kind"`
	Text string `json:"text"`
}

// OrderConsole is the live view of one order's generation.
type OrderConsole struct {
	Number     string        `json:"number"`
	Status     string        `json:"status"`
	Attempt    int           `json:"attempt"`
	JobStatus  string        `json:"jobStatus"`
	StartedAt  *time.Time    `json:"startedAt,omitempty"`
	FinishedAt *time.Time    `json:"finishedAt,omitempty"`
	UpdatedAt  *time.Time    `json:"updatedAt,omitempty"`
	Lines      []ConsoleLine `json:"lines"`
	Truncated  bool          `json:"truncated"`
	// LogURL is a relative signed link to the standalone log viewer (the
	// operator opens it in a browser where initData is absent).
	LogURL string `json:"logUrl,omitempty"`
	// CostUSD is what the runs of this order have cost so far (from the log),
	// CostKZT the same amount in tenge at the current rate.
	CostUSD float64 `json:"costUsd"`
	CostKZT float64 `json:"costKzt"`
}

// LogTokenSecret signs the standalone log links (the bot token: server-side).
func (s *Service) LogTokenSecret() string {
	return s.Cfg.Bot.Token
}

// ConsoleLogLink is the relative signed link to the log viewer of one order.
func (s *Service) ConsoleLogLink(number string) string {
	expires := time.Now().Add(12 * time.Hour)
	token := auth.LogLinkToken(s.LogTokenSecret(), number, expires)
	return "/api/admin/orders/" + url.PathEscape(number) + "/console/log?token=" + url.QueryEscape(token)
}

// ConsoleLogPath is the raw agent log of an order (written live during a run).
func (s *Service) ConsoleLogPath(number string) string {
	return filepath.Join(s.Cfg.Worker.GeneratorDir, "work", number, "agent.log")
}

// AdminOrderConsole returns the latest generation attempt of an order together
// with the readable tail of the agent log.
func (s *Service) AdminOrderConsole(ctx context.Context, number string) (OrderConsole, error) {
	order, err := s.AdminOrder(ctx, number)
	if err != nil {
		return OrderConsole{}, err
	}

	console := OrderConsole{Number: order.Number, Status: order.Status}
	// A completed order is explained by the attempt that finished it, not by a
	// later service retry that failed on the state transition.
	jobStatus := ""
	if order.Status == domain.StatusCompleted {
		jobStatus = "done"
	}
	job, err := s.Store.LatestJob(ctx, order.Number, jobStatus)
	if err != nil && jobStatus != "" {
		job, err = s.Store.LatestJob(ctx, order.Number, "")
	}
	if err == nil {
		started := job.StartedAt
		console.Attempt = job.Attempt
		console.JobStatus = job.Status
		console.StartedAt = &started
		console.FinishedAt = job.FinishedAt
	} else if !errors.Is(err, store.ErrNotFound) {
		return OrderConsole{}, err
	}

	logPath := s.ConsoleLogPath(order.Number)
	if info, err := os.Stat(logPath); err == nil {
		updated := info.ModTime().UTC()
		console.UpdatedAt = &updated
	}
	lines, truncated, err := readConsoleEvents(logPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return OrderConsole{}, err
	}
	console.Lines = lines
	console.Truncated = truncated
	console.LogURL = s.ConsoleLogLink(order.Number)
	// Prefer the stored total; a live run may already be ahead of it in the log,
	// and the raw CLI fields cover runs recorded before the cost tracking.
	console.CostUSD = order.AgentCostUSD
	for _, candidate := range []float64{maxTotalCost(lines), s.AgentRunCost(order.Number)} {
		if candidate > console.CostUSD {
			console.CostUSD = candidate
		}
	}
	// Orders generated before the tracking existed (or recorded with an older log
	// shape) heal themselves the first time the operator opens the console.
	if console.CostUSD > order.AgentCostUSD+0.000001 {
		s.AddAgentCost(ctx, order.Number, console.CostUSD-order.AgentCostUSD)
	}
	console.CostKZT = s.convertUSDToKZT(ctx, console.CostUSD)
	return console, nil
}

// totalCostPattern matches the cumulative cost of the Cline usage line:
// "токены: вход 154571, выход 262, стоимость $0.0009 (всего $0.1128)".
var totalCostPattern = regexp.MustCompile(`всего \$([0-9]+(?:\.[0-9]+)?)`)

// maxTotalCost reads the last cumulative cost from the parsed usage lines.
func maxTotalCost(lines []ConsoleLine) float64 {
	best := 0.0
	for _, line := range lines {
		match := totalCostPattern.FindStringSubmatch(line.Text)
		if len(match) != 2 {
			continue
		}
		if value, err := strconv.ParseFloat(match[1], 64); err == nil && value > best {
			best = value
		}
	}
	return best
}

// agentCostFieldPattern matches the cumulative cost field of the CLI JSON stream
// ("totalCost":0.121123446) - that is what the agent writes after every turn,
// and the console renders it as the «всего $x» usage line.
var agentCostFieldPattern = regexp.MustCompile(`"totalCost"\s*:\s*([0-9]+(?:\.[0-9]+)?)`)

// AgentRunCost returns the cumulative cost of the agent run of one order in USD.
// The raw JSON fields are the primary source (the largest "totalCost" is the
// final total), the rendered usage text is a fallback for other log shapes.
// Reading only the tail keeps multi-megabyte logs cheap to scan.
func (s *Service) AgentRunCost(number string) float64 {
	data, err := os.ReadFile(s.ConsoleLogPath(number))
	if err != nil {
		return 0
	}
	const tail = 4 << 20
	if len(data) > tail {
		data = data[len(data)-tail:]
	}
	text := string(data)

	best := 0.0
	for _, match := range agentCostFieldPattern.FindAllStringSubmatch(text, -1) {
		if value, err := strconv.ParseFloat(match[1], 64); err == nil && value > best {
			best = value
		}
	}
	for _, match := range totalCostPattern.FindAllStringSubmatch(text, -1) {
		if value, err := strconv.ParseFloat(match[1], 64); err == nil && value > best {
			best = value
		}
	}
	return best
}

// readConsoleEvents walks the log backwards in chunks until it collected enough
// events: a long-running tool can fill megabytes with stream chunks that carry
// no events, and the console must still show what the agent did before them.
func readConsoleEvents(path string) ([]ConsoleLine, bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, false, err
	}

	var (
		data   []byte
		offset = info.Size()
	)
	for {
		read := int64(consoleTailBytes)
		if offset < read {
			read = offset
		}
		if read <= 0 {
			break
		}
		buf := make([]byte, read)
		if _, err := file.ReadAt(buf, offset-read); err != nil {
			return nil, false, err
		}
		data = append(buf, data...)
		offset -= read

		lines, truncated := parseAgentLog(data)
		if len(lines) >= consoleWantLines || info.Size()-offset >= consoleReadBytes {
			return lines, truncated || offset > 0, nil
		}
	}
	lines, truncated := parseAgentLog(data)
	return lines, truncated, nil
}

// agentLogLine is the subset of Cline's JSON stream the console understands.
type agentLogLine struct {
	TS    string `json:"ts"`
	Type  string `json:"type"`
	Event struct {
		Type        string `json:"type"`
		ContentType string `json:"contentType"`
		Text        string `json:"text"`
		Reasoning   string `json:"reasoning"`
		ToolName    string `json:"toolName"`
		Update      struct {
			Query string `json:"query"`
		} `json:"update"`
		InputTokens  int     `json:"inputTokens"`
		OutputTokens int     `json:"outputTokens"`
		Cost         float64 `json:"cost"`
		TotalCost    float64 `json:"totalCost"`
		Iteration    int     `json:"iteration"`
	} `json:"event"`
	HookEventName string `json:"hookEventName"`
	Message       string `json:"message"`
	FinishReason  string `json:"finishReason"`
	Iterations    int    `json:"iterations"`
	Usage         struct {
		TotalCost float64 `json:"totalCost"`
	} `json:"usage"`
}

// parseAgentLog turns the raw stream into readable lines: the streaming deltas
// (hundreds per run) are dropped, the events that explain the work stay.
func parseAgentLog(raw []byte) ([]ConsoleLine, bool) {
	lines := []ConsoleLine{}
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 0, 64<<10), 4<<20)

	iteration := 0
	for scanner.Scan() {
		var entry agentLogLine
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue // a partial line at the tail cut
		}

		line, ok := renderAgentLine(entry, &iteration)
		if !ok {
			continue
		}
		// Cline repeats a tool update once per command in a batch: collapse the
		// identical consecutive lines so the console stays readable.
		if len(lines) > 0 {
			last := lines[len(lines)-1]
			if line.Kind == consoleTool && last.Kind == consoleTool && last.Text == line.Text {
				continue
			}
		}
		lines = append(lines, line)
	}

	truncated := false
	if len(lines) > consoleMaxLines {
		lines = lines[len(lines)-consoleMaxLines:]
		truncated = true
	}
	return lines, truncated
}

// renderAgentLine renders one stream entry, or reports that it is noise.
func renderAgentLine(entry agentLogLine, iteration *int) (ConsoleLine, bool) {
	at := entry.TS
	switch entry.Type {
	case "error":
		return ConsoleLine{At: at, Kind: consoleError, Text: entry.Message}, entry.Message != ""

	case "run_result":
		text := "агент завершил работу: " + entry.FinishReason
		if entry.Iterations > 0 {
			text += fmt.Sprintf(", итераций %d", entry.Iterations)
		}
		if entry.Usage.TotalCost > 0 {
			text += fmt.Sprintf(", стоимость $%.4f", entry.Usage.TotalCost)
		}
		return ConsoleLine{At: at, Kind: consoleResult, Text: text}, true

	case "hook_event":
		switch entry.HookEventName {
		case "agent_end":
			return ConsoleLine{At: at, Kind: consoleEvent, Text: "агент остановлен"}, true
		case "agent_error":
			return ConsoleLine{At: at, Kind: consoleError, Text: "ошибка агента"}, true
		default:
			return ConsoleLine{}, false
		}

	case "agent_event":
		event := entry.Event
		switch event.Type {
		case "iteration_start":
			*iteration++
			return ConsoleLine{At: at, Kind: consoleIteration,
				Text: fmt.Sprintf("итерация %d", *iteration)}, true

		case "content_end":
			text := strings.TrimSpace(event.Text)
			if text != "" {
				return ConsoleLine{At: at, Kind: consoleText, Text: text}, true
			}
			if reasoning := strings.TrimSpace(event.Reasoning); reasoning != "" {
				return ConsoleLine{At: at, Kind: consoleReasoning, Text: reasoning}, true
			}
			return ConsoleLine{}, false

		case "content_update":
			query := strings.TrimSpace(event.Update.Query)
			if query == "" {
				return ConsoleLine{}, false // stdout chunks are noise
			}
			return ConsoleLine{At: at, Kind: consoleTool,
				Text: event.ToolName + ": " + oneLine(query, 400)}, true

		case "usage":
			return ConsoleLine{At: at, Kind: consoleUsage, Text: fmt.Sprintf(
				"токены: вход %d, выход %d, стоимость $%.4f (всего $%.4f)",
				event.InputTokens, event.OutputTokens, event.Cost, event.TotalCost)}, true

		case "done":
			return ConsoleLine{At: at, Kind: consoleResult, Text: "агент закончил работу"}, true
		}
	}
	return ConsoleLine{}, false
}

// oneLine flattens and truncates a command for the console.
func oneLine(value string, limit int) string {
	flat := strings.Join(strings.Fields(value), " ")
	if len(flat) <= limit {
		return flat
	}
	return flat[:limit] + "…"
}
