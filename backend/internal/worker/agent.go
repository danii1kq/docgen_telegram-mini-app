package worker

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/srunal/srunal-go/internal/catalog"
	"github.com/srunal/srunal-go/internal/config"
	"github.com/srunal/srunal-go/internal/domain"
	"github.com/srunal/srunal-go/internal/service"
	"github.com/srunal/srunal-go/internal/store"
)

//go:embed skill/document.md
var builtinSkillFS embed.FS

// OOXML mime types of the results the agent produces.
const (
	docxMime = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	pptxMime = "application/vnd.openxmlformats-officedocument.presentationml.presentation"
)

// lookPath resolves an executable, keeping the npm shim path as-is.
func lookPath(command string) (string, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return "", fmt.Errorf("worker: empty command")
	}
	if strings.ContainsAny(command, `/\`) {
		info, err := os.Stat(command)
		if err != nil {
			return "", err
		}
		if info.IsDir() {
			return "", fmt.Errorf("worker: %s is a directory", command)
		}
		return command, nil
	}
	return exec.LookPath(command)
}

// agentGenerator runs an external AI agent (Cline by default) in a fresh
// working directory per order:
//
//	<GENERATOR_DIR>/work/<number>/
//	  .clinerules/01-skill.md   <- the skill (satbayev-gost or pptx)
//	  skill/                    <- the rest of the skill (references, scripts, assets)
//	  task.json                 <- the order payload and the expected output
//	  result.docx | result.pptx <- what the agent must produce
//
// The agent gets a bounded time budget; the produced file is validated (ZIP
// signature) and moved into the regular storage, so the rest of the pipeline
// (order files, signed links, notifications) works unchanged.
type agentGenerator struct {
	svc          *service.Service
	command      string
	builtinSkill []byte
}

func newAgentGenerator(svc *service.Service, command string) *agentGenerator {
	builtin, _ := builtinSkillFS.ReadFile("skill/document.md")
	return &agentGenerator{svc: svc, command: command, builtinSkill: builtin}
}

// agentResultNames are the files the agent must write for this order: a single
// result.docx/pptx, or one document per work (result_1.docx … result_N.docx)
// when the client ordered several works.
func agentResultNames(order *domain.Order) []string {
	extension := ".docx"
	if order.Product == domain.ProductPresentation {
		extension = ".pptx"
	}
	if order.Works <= 1 {
		return []string{"result" + extension}
	}
	names := make([]string, 0, order.Works)
	for index := 1; index <= order.Works; index++ {
		names = append(names, fmt.Sprintf("result_%d%s", index, extension))
	}
	return names
}

// workBrief is one ordered work: its topic and the document that must be built
// for it.
type workBrief struct {
	Index  int    `json:"index"`
	Topic  string `json:"topic"`
	Output string `json:"output"`
}

// worksOf splits the order into per-work briefs. The client lists the topics one
// per line (wizard step 2); a missing line repeats the previous topic.
func worksOf(order *domain.Order) []workBrief {
	outputs := agentResultNames(order)
	topics := splitTopicLines(order.Topics)
	briefs := make([]workBrief, 0, len(outputs))
	for index, output := range outputs {
		topic := strings.TrimSpace(order.Topics)
		if len(topics) > 0 {
			position := index
			if position >= len(topics) {
				position = len(topics) - 1
			}
			topic = topics[position]
		}
		briefs = append(briefs, workBrief{Index: index + 1, Topic: topic, Output: output})
	}
	return briefs
}

// splitTopicLines returns the non-empty topic lines of the order.
func splitTopicLines(topics string) []string {
	lines := strings.Split(strings.ReplaceAll(topics, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func agentResultMime(order *domain.Order) string {
	if order.Product == domain.ProductPresentation {
		return pptxMime
	}
	return docxMime
}

// Generate implements Generator.
func (g *agentGenerator) Generate(ctx context.Context, order *domain.Order) ([]store.OrderFileRow, string, error) {
	cfg := g.svc.Cfg.Worker
	workDir := filepath.Join(cfg.GeneratorDir, "work", order.Number)
	if err := os.MkdirAll(filepath.Join(workDir, ".clinerules"), 0o755); err != nil {
		return nil, "", fmt.Errorf("worker: work dir: %w", err)
	}

	skillName, skillDir, skillFound := skillFor(g.svc, order)
	skillLog := "скилл " + skillName
	if skillFound {
		if err := copyTree(skillDir, filepath.Join(workDir, "skill")); err != nil {
			g.svc.Log.Warn("worker: skill copy failed, using the built-in instructions",
				"skill", skillName, "error", err)
			skillFound = false
		}
	}
	if !skillFound {
		skillLog = "встроенная инструкция (скилл " + skillName + " не найден в " + cfg.SkillSourceDir + ")"
	}

	// .clinerules is what Cline loads automatically: the skill's SKILL.md plus
	// the built-in safety net so the output contract is always explicit.
	rules := new(bytes.Buffer)
	if skillFound {
		if content, err := os.ReadFile(filepath.Join(skillDir, "SKILL.md")); err == nil {
			rules.Write(content)
			rules.WriteString("\n\n---\n\n")
		}
	}
	rules.Write(g.builtinSkill)
	if err := os.WriteFile(filepath.Join(workDir, ".clinerules", "01-skill.md"), rules.Bytes(), 0o644); err != nil {
		return nil, "", fmt.Errorf("worker: write rules: %w", err)
	}

	outputNames := agentResultNames(order)
	task := buildAgentTask(order, outputNames, skillName, skillFound)
	taskJSON, err := json.MarshalIndent(task, "", "  ")
	if err != nil {
		return nil, "", fmt.Errorf("worker: encode task: %w", err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "task.json"), taskJSON, 0o644); err != nil {
		return nil, "", fmt.Errorf("worker: write task: %w", err)
	}

	// Stale files from a previous attempt must not be mistaken for the result.
	for _, name := range outputNames {
		_ = os.Remove(filepath.Join(workDir, name))
	}

	prompt := buildAgentPrompt(order, outputNames, skillName)
	runCtx, cancel := context.WithTimeout(ctx, cfg.GeneratorTimeout)
	defer cancel()

	started := time.Now()
	output, runErr := runAgentCommand(runCtx, g.command, cfg, workDir, prompt)
	elapsed := time.Since(started).Round(time.Second)
	// Fallback: if the live log could not be created, keep the captured tail.
	if _, statErr := os.Stat(filepath.Join(workDir, "agent.log")); statErr != nil {
		_ = os.WriteFile(filepath.Join(workDir, "agent.log"), []byte(output), 0o644)
	}
	// The cost of this run (the CLI reports it cumulatively in its stream) lands
	// on the order, so the operator panel shows what the order has cost.
	if cost := g.svc.AgentRunCost(order.Number); cost > 0 {
		g.svc.AddAgentCost(ctx, order.Number, cost)
	}
	summary := agentSummary(output)
	logLine := fmt.Sprintf("агент %s • %s • %s", filepath.Base(g.command), skillLog, elapsed)
	if cfg.GeneratorModel != "" {
		logLine += " • " + cfg.GeneratorProvider + "/" + cfg.GeneratorModel + " (thinking " + cfg.GeneratorThinking + ")"
	}
	if summary != "" {
		logLine += " • " + summary
	}

	if runErr != nil {
		return nil, logLine, fmt.Errorf("worker: agent run failed: %w (%s)", runErr, tailText(output, 800))
	}

	results, err := readResults(workDir, outputNames, order)
	if err != nil {
		return nil, logLine, err
	}
	files := make([]store.OrderFileRow, 0, len(results))
	for _, result := range results {
		file, err := g.svc.WriteResultFile(order.Number, result.Display, agentResultMime(order), result.Content)
		if err != nil {
			return nil, logLine, err
		}
		files = append(files, file)
	}
	return files, logLine, nil
}

// agentResult is one produced document read from the work directory.
type agentResult struct {
	Name    string
	Display string
	Content []byte
}

// readResults reads the produced documents (one per work for multi-work orders),
// validates them as OOXML packages and names them for the client. A missing
// document fails the attempt: half of the ordered works must not be delivered
// silently.
func readResults(workDir string, names []string, order *domain.Order) ([]agentResult, error) {
	results := make([]agentResult, 0, len(names))
	missing := make([]string, 0, len(names))
	for index, name := range names {
		content, err := os.ReadFile(filepath.Join(workDir, name))
		if err != nil {
			missing = append(missing, name)
			continue
		}
		if !looksLikeZip(content) {
			return nil, fmt.Errorf("worker: %s не является документом OOXML (нет ZIP-сигнатуры)", name)
		}
		if len(content) < 1024 {
			return nil, fmt.Errorf("worker: %s слишком мал (%d байт)", name, len(content))
		}
		display := "resultat_" + strings.ReplaceAll(order.Number, "-", "_")
		if len(names) > 1 {
			display += "_" + strconv.Itoa(index+1)
		}
		display += filepath.Ext(name)
		results = append(results, agentResult{Name: name, Display: display, Content: content})
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("worker: агент создал %d из %d документов, нет: %s",
			len(results), len(names), strings.Join(missing, ", "))
	}
	return results, nil
}

// buildAgentTask is the machine-readable brief handed to the agent.
func buildAgentTask(order *domain.Order, outputNames []string, skillName string, skillFound bool) map[string]any {
	product := catalog.Get(order.Product)
	task := map[string]any{
		"order": map[string]any{
			"number":         order.Number,
			"product":        order.Product,
			"productName":    product.Name,
			"workKind":       order.WorkKind,
			"works":          order.Works,
			"units":          order.Units,
			"unitWord":       product.UnitOne,
			"topics":         order.Topics,
			"notes":          order.Notes,
			"language":       order.Language,
			"institute":      order.Institute,
			"department":     order.Department,
			"disciplineCode": order.DisciplineCode,
			"studentName":    order.StudentName,
			"teacherName":    order.TeacherName,
			"amountKZT":      order.Amount,
		},
		"output": map[string]any{
			"file":    outputNames[0],
			"files":   outputNames,
			"note":    "готовые файлы положи в корень рабочего каталога, имена менять нельзя",
			"perWork": len(outputNames) > 1,
		},
		"skill": map[string]any{
			"name":      skillName,
			"rules":     ".clinerules/01-skill.md",
			"materials": "skill/",
			"found":     skillFound,
		},
		"requirements": []string{
			"язык документа - значение order.language",
			"титульный лист по стандарту Satbayev University",
			"оформление по ГОСТ РК (Times New Roman 14, интервал 1,5, поля 30/15/20/20 мм)",
			"объём - order.units " + product.UnitMany + " (±1)",
			"не меньше 5 реальных источников, список по ГОСТ 7.1-2003",
			"файл должен открываться в Word/LibreOffice (валидный OOXML-пакет)",
		},
	}
	if order.Product == domain.ProductOther && order.WorkKind != "" {
		task["requirements"] = append(task["requirements"].([]string),
			"вид работы: "+order.WorkKind+" - структура и титульный лист должны соответствовать этому виду")
	}
	// One document per ordered work: the client pays for separate works, so they
	// must not be merged into a single file.
	if briefs := worksOf(order); len(briefs) > 1 {
		task["order"].(map[string]any)["worksList"] = briefs
		task["requirements"] = append(task["requirements"].([]string),
			fmt.Sprintf("клиент заказал %d отдельные работы: на каждую тему - СВОЙ документ (%s), темы не объединять",
				len(briefs), strings.Join(outputNames, ", ")),
			"объём order.units относится к каждой работе отдельно")
	}
	if note := strings.TrimSpace(order.RevisionNote); note != "" {
		task["revision"] = map[string]any{
			"note":      note,
			"rule":      "это доработка: готовый файл уже лежит в каталоге, внеси правки и перезапиши его",
			"mustApply": true,
		}
		task["requirements"] = append(task["requirements"].([]string),
			"роль: ты студент (без упоминаний ИИ/агента/сервиса в тексте)",
			"не использовать длинное тире - только минус (-)")
	}
	return task
}

// buildAgentPrompt is the CLI task text (kept short: the details live in
// task.json and .clinerules).
func buildAgentPrompt(order *domain.Order, outputNames []string, skillName string) string {
	single := len(outputNames) == 1
	lines := []string{
		"Роль - ты студент: работу сдаёт студент, поэтому в тексте не должно быть упоминаний ИИ, агента, модели или сервиса.",
		"Сгенерируй готовый документ по заказу " + order.Number + " сервиса SruNal (КазНИТУ им. К.И. Сатпаева).",
		"1. Прочитай task.json в текущем каталоге - это данные заказа.",
		"2. Следуй правилам из .clinerules/01-skill.md (скилл " + skillName + "); материалы, шаблоны и скрипты скилла - в ./skill/.",
	}
	if single {
		lines = append(lines,
			"3. Собери файл "+outputNames[0]+" программно и положи его в корень рабочего каталога.",
			"4. Проверь, что файл валиден (ZIP-сигнатура PK) и открывается.",
			"5. Убери за собой: удали свои временные файлы (скрипты, черновики, промежуточные выгрузки), "+
				"в каталоге должны остаться только "+outputNames[0]+", task.json и служебные файлы сервиса "+
				"(agent.log, .clinerules/, skill/).",
		)
	} else {
		lines = append(lines,
			"3. Клиент заказал "+strconv.Itoa(len(outputNames))+" отдельные работы (task.json → order.worksList). "+
				"На КАЖДУЮ тему собери СВОЙ документ: "+strings.Join(outputNames, ", ")+". "+
				"Темы не объединять, каждый документ - самостоятельная работа со своим титульным листом и списком источников.",
			"4. Проверь, что каждый файл валиден (ZIP-сигнатура PK) и открывается.",
			"5. Убери за собой: удали свои временные файлы (скрипты, черновики, промежуточные выгрузки), "+
				"в каталоге должны остаться только "+strings.Join(outputNames, ", ")+", task.json и служебные файлы сервиса "+
				"(agent.log, .clinerules/, skill/).",
		)
	}
	lines = append(lines,
		"6. Никаких отчётов не создавай и не пиши: в конце выведи ровно одну строку DONE.",
		"7. Не используй длинное тире (—) и среднее тире (–): только обычный минус (-).",
	)
	if note := strings.TrimSpace(order.RevisionNote); note != "" {
		lines = append(lines,
			"8. Готовые файлы уже лежат в рабочем каталоге, и заказ вернули на доработку. Внеси правки, "+
				"сохранив остальное оформление и структуру, и перезапиши "+strings.Join(outputNames, ", ")+".",
			"Правки, которые обязательно учесть (каждую):\n"+note)
	}
	return strings.Join(lines, "\n")
}

// runAgentCommand executes the CLI in the work directory and returns its output. The provider/model/reasoning flags come from the configuration so the agent
// always runs on the pinned model (opencode-go / deepseek-v4.1-flash, thinking
// high by default).
func runAgentCommand(ctx context.Context, command string, cfg config.WorkerConfig, workDir, prompt string) (string, error) {
	args := []string{
		"--json",
		"--auto-approve", "true",
		"--timeout", strconv.Itoa(int(cfg.GeneratorTimeout.Seconds())),
		"-c", workDir,
	}
	if cfg.GeneratorProvider != "" {
		args = append(args, "-P", cfg.GeneratorProvider)
	}
	if cfg.GeneratorModel != "" {
		args = append(args, "-m", cfg.GeneratorModel)
	}
	if cfg.GeneratorThinking != "" {
		args = append(args, "--thinking", cfg.GeneratorThinking)
	}
	if cfg.GeneratorConfigDir != "" {
		args = append(args, "--config", cfg.GeneratorConfigDir)
	}
	if cfg.GeneratorDataDir != "" {
		args = append(args, "--data-dir", cfg.GeneratorDataDir)
	}
	args = append(args, cfg.GeneratorArgs...)
	args = append(args, prompt)

	// On Windows the npm shim is a .cmd: it must run through cmd /c.
	executable := command
	if runtime.GOOS == "windows" && isBatchShim(command) {
		args = append([]string{"/c", command}, args...)
		executable = "cmd"
	}

	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), "NO_COLOR=1")
	// A service has no interactive console. Give the CLI an explicit (empty)
	// stdin: on Windows the child would otherwise inherit the stdin handle of
	// the API process, and when that handle is closed (systemd, a CI runner, a
	// background shell) the CLI's own process spawning fails with an opaque
	// "uv_spawn ... ENOENT" before the first model call.
	cmd.Stdin = strings.NewReader("")
	// Optional BYOK key: the CLI reads it from the provider's key variable.
	if cfg.GeneratorAPIKey != "" {
		for _, name := range apiKeyEnvNames(cfg) {
			cmd.Env = append(cmd.Env, name+"="+cfg.GeneratorAPIKey)
		}
	}

	var output bytes.Buffer
	writers := []io.Writer{&tailWriter{buf: &output, limit: 512 * 1024}}
	// The full stream also lands in agent.log: the admin console tails that file
	// while the agent works, so generation can be watched live.
	if logFile, err := os.Create(filepath.Join(workDir, "agent.log")); err == nil {
		defer logFile.Close()
		writers = append(writers, newCappedWriter(logFile, agentLogLimit))
	}
	cmd.Stdout = io.MultiWriter(writers...)
	cmd.Stderr = cmd.Stdout
	err := cmd.Run()
	if ctx.Err() != nil {
		return output.String(), fmt.Errorf("agent timed out after %s", cfg.GeneratorTimeout)
	}
	return output.String(), err
}

// agentLogLimit caps the live agent log on disk (the console reads its tail).
const agentLogLimit = 8 << 20

// cappedWriter appends to a file and keeps it bounded: past the limit the file
// is rewritten with its own tail, so a long run cannot fill the disk.
type cappedWriter struct {
	mu    sync.Mutex
	file  *os.File
	limit int64
}

func newCappedWriter(file *os.File, limit int64) *cappedWriter {
	return &cappedWriter{file: file, limit: limit}
}

func (w *cappedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	n, err := w.file.Write(p)
	if err != nil {
		return n, err
	}
	if info, statErr := w.file.Stat(); statErr == nil && info.Size() > w.limit {
		w.compact()
	}
	return n, nil
}

// compact rewrites the file with its last half. Any failure leaves the file
// untouched: a too large log is better than a lost one.
func (w *cappedWriter) compact() {
	keep := w.limit / 2
	if _, err := w.file.Seek(-keep, io.SeekEnd); err != nil {
		return
	}
	buf := make([]byte, keep)
	read, err := io.ReadFull(w.file, buf)
	if err != nil {
		return
	}
	if err := w.file.Truncate(0); err != nil {
		return
	}
	if _, err := w.file.Seek(0, io.SeekStart); err != nil {
		return
	}
	if _, err := w.file.Write(buf[:read]); err != nil {
		return
	}
	_ = w.file.Sync()
}

// apiKeyEnvNames lists the environment variables the agent CLI reads the API key
// from. Cline resolves the key as: the stored provider config first, then the
// provider's apiKeyEnv variable - so the name depends on the provider
// (opencode-go -> OPENCODE_API_KEY, deepseek -> DEEPSEEK_API_KEY, ...).
// GENERATOR_API_KEY_ENV overrides the derivation with an explicit list.
func apiKeyEnvNames(cfg config.WorkerConfig) []string {
	if raw := strings.TrimSpace(cfg.GeneratorAPIKeyEnv); raw != "" {
		var names []string
		for _, name := range strings.Split(raw, ",") {
			if name = strings.TrimSpace(name); name != "" {
				names = append(names, name)
			}
		}
		if len(names) > 0 {
			return names
		}
	}
	switch strings.ToLower(strings.TrimSpace(cfg.GeneratorProvider)) {
	case "opencode-go", "opencode":
		return []string{"OPENCODE_API_KEY"}
	case "deepseek":
		return []string{"DEEPSEEK_API_KEY"}
	case "anthropic", "claude":
		return []string{"ANTHROPIC_API_KEY"}
	case "openrouter":
		return []string{"OPENROUTER_API_KEY"}
	case "google", "gemini":
		return []string{"GEMINI_API_KEY", "GOOGLE_API_KEY"}
	case "xai", "grok":
		return []string{"XAI_API_KEY"}
	case "mistral":
		return []string{"MISTRAL_API_KEY"}
	case "kilo":
		return []string{"KILO_API_KEY"}
	default:
		// openai and the remaining OpenAI-compatible providers.
		return []string{"OPENAI_API_KEY"}
	}
}

func isBatchShim(path string) bool {
	lower := strings.ToLower(path)
	return strings.HasSuffix(lower, ".cmd") || strings.HasSuffix(lower, ".bat")
}

// agentSummary extracts the final answer of the agent from the --json stream.
func agentSummary(output string) string {
	var finish string
	var text string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var event struct {
			Type         string `json:"type"`
			FinishReason string `json:"finishReason"`
			Text         string `json:"text"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}
		if event.Type == "run_result" {
			finish = event.FinishReason
			text = strings.Join(strings.Fields(event.Text), " ")
		}
	}
	if finish == "" && text == "" {
		return ""
	}
	if len(text) > 160 {
		text = text[:160] + "…"
	}
	return strings.TrimSpace("finish=" + finish + " " + text)
}

// tailText keeps the end of the agent output for the job log.
func tailText(output string, limit int) string {
	trimmed := strings.TrimSpace(output)
	if len(trimmed) <= limit {
		return trimmed
	}
	return "…" + trimmed[len(trimmed)-limit:]
}

// looksLikeZip checks the OOXML magic (docx/pptx are ZIP archives).
func looksLikeZip(content []byte) bool {
	return len(content) >= 4 && content[0] == 'P' && content[1] == 'K'
}

// tailWriter keeps the LAST bytes of the agent output: the end of a run carries
// the run_result (finish reason and the agent's report), while the beginning is
// a stream of reasoning tokens.
type tailWriter struct {
	buf   *bytes.Buffer
	limit int
}

func (w *tailWriter) Write(p []byte) (int, error) {
	if _, err := w.buf.Write(p); err != nil {
		return 0, err
	}
	if w.buf.Len() > w.limit {
		// Drop the oldest half so the buffer never grows unbounded.
		data := w.buf.Bytes()
		keep := data[len(data)-w.limit/2:]
		w.buf.Reset()
		_, _ = w.buf.Write(keep)
	}
	return len(p), nil
}
