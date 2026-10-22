package worker

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/srunal/srunal-go/internal/config"
	"github.com/srunal/srunal-go/internal/domain"
	"github.com/srunal/srunal-go/internal/service"
)

// testService builds a service with a throw-away storage and generator dir.
func testService(t *testing.T) *service.Service {
	t.Helper()
	dir := t.TempDir()
	cfg := &config.Config{
		TokenPepper: "test-pepper",
		StorageDir:  filepath.Join(dir, "storage"),
		ResultTTL:   24 * time.Hour,
		Worker: config.WorkerConfig{
			GeneratorMode:    "agent",
			GeneratorCommand: "unused",
			GeneratorTimeout: 30 * time.Second,
			GeneratorDir:     filepath.Join(dir, "generator"),
			SkillSourceDir:   filepath.Join(dir, "skills"),
			SkillGOST:        "satbayev-gost",
			SkillPPTX:        "pptx",
			GenerationMin:    time.Millisecond,
			GenerationMax:    time.Millisecond,
			MaxAgents:        2,
		},
	}
	log := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
	svc := service.New(nil, cfg, log)
	if err := svc.EnsureStorage(); err != nil {
		t.Fatalf("EnsureStorage: %v", err)
	}
	return svc
}

// writeSkill creates a fake global skill with a SKILL.md and one reference.
func writeSkill(t *testing.T, svc *service.Service, name, marker string) {
	t.Helper()
	dir := filepath.Join(svc.Cfg.Worker.SkillSourceDir, name)
	if err := os.MkdirAll(filepath.Join(dir, "references"), 0o755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# "+marker+"\n"), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "references", "ref.md"), []byte("ref"), 0o644); err != nil {
		t.Fatalf("write skill ref: %v", err)
	}
}

func testOrder(product string) *domain.Order {
	order := &domain.Order{
		Number:         "DG-260391",
		Status:         domain.StatusGenerating,
		Product:        product,
		Works:          1,
		Units:          10,
		Topics:         "Тестовая тема работы",
		Language:       "ru",
		Institute:      "ИЦИТ",
		Department:     "Программная инженерия",
		StudentName:    "Тест Тестович",
		DisciplineCode: "CS-101",
		TeacherName:    "Доц. Тестов Т.Т.",
		Amount:         720,
	}
	if product == domain.ProductOther {
		order.WorkKind = "СРС"
	}
	return order
}

// writeFakeAgent creates a script that plays the agent: it copies a prepared
// archive to the expected result file (or exits with an error).
func writeFakeAgent(t *testing.T, dir, source, resultName string, exitCode int) string {
	t.Helper()
	return writeFakeAgentMulti(t, dir, source, []string{resultName}, exitCode)
}

// writeFakeAgentMulti is writeFakeAgent for orders with several documents: the
// script copies the prepared archive to every expected file name.
func writeFakeAgentMulti(t *testing.T, dir, source string, resultNames []string, exitCode int) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		path := filepath.Join(dir, "fake-agent.cmd")
		script := "@echo off\r\n" +
			"echo {\"type\":\"run_result\",\"finishReason\":\"completed\",\"text\":\"fake agent done\"}\r\n"
		for _, name := range resultNames {
			script += "copy /Y \"" + source + "\" \"" + name + "\" >nul\r\n"
		}
		script += "exit /b " + strconv.Itoa(exitCode) + "\r\n"
		if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
			t.Fatalf("write fake agent: %v", err)
		}
		return path
	}
	path := filepath.Join(dir, "fake-agent.sh")
	script := "#!/bin/sh\n" +
		"echo '{\"type\":\"run_result\",\"finishReason\":\"completed\",\"text\":\"fake agent done\"}'\n"
	for _, name := range resultNames {
		script += "cp \"" + source + "\" \"" + name + "\"\n"
	}
	script += "exit " + strconv.Itoa(exitCode) + "\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake agent: %v", err)
	}
	return path
}

// TestAgentGeneratorProducesResult checks the whole agent path: per-order work
// dir, skill copied into it, task.json, the produced file stored as a result.
func TestAgentGeneratorProducesResult(t *testing.T) {
	svc := testService(t)
	writeSkill(t, svc, "satbayev-gost", "SATBAYEV-GOST-SKILL-MARKER")

	order := testOrder(domain.ProductOther)
	source, err := BuildDocument(order)
	if err != nil {
		t.Fatalf("BuildDocument: %v", err)
	}
	sourcePath := filepath.Join(t.TempDir(), "agent-output.docx")
	if err := os.WriteFile(sourcePath, source, 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	generator := newAgentGenerator(svc, writeFakeAgent(t, t.TempDir(), sourcePath, "result.docx", 0))
	files, logText, err := generator.Generate(context.Background(), order)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("files = %d, want 1", len(files))
	}
	file := files[0]
	if file.FileName != "resultat_DG_260391.docx" {
		t.Errorf("file name = %q", file.FileName)
	}
	if file.MimeType != docxMime {
		t.Errorf("mime = %q", file.MimeType)
	}
	if file.SizeBytes != int64(len(source)) {
		t.Errorf("size = %d, want %d", file.SizeBytes, len(source))
	}
	if !strings.Contains(logText, "satbayev-gost") {
		t.Errorf("log does not mention the skill: %q", logText)
	}
	if !strings.Contains(logText, "finish=completed") {
		t.Errorf("log does not carry the agent summary: %q", logText)
	}

	// The work directory is self-contained: rules, skill and the task brief.
	workDir := filepath.Join(svc.Cfg.Worker.GeneratorDir, "work", order.Number)
	for _, name := range []string{
		filepath.Join(".clinerules", "01-skill.md"),
		filepath.Join("skill", "SKILL.md"),
		filepath.Join("skill", "references", "ref.md"),
		"task.json",
	} {
		if _, err := os.Stat(filepath.Join(workDir, name)); err != nil {
			t.Errorf("work dir misses %s: %v", name, err)
		}
	}

	rules, err := os.ReadFile(filepath.Join(workDir, ".clinerules", "01-skill.md"))
	if err != nil {
		t.Fatalf("read rules: %v", err)
	}
	if !strings.Contains(string(rules), "SATBAYEV-GOST-SKILL-MARKER") {
		t.Error("rules do not include the global skill")
	}
	if !strings.Contains(string(rules), "result.docx") {
		t.Error("rules do not include the built-in output contract")
	}

	task, err := os.ReadFile(filepath.Join(workDir, "task.json"))
	if err != nil {
		t.Fatalf("read task: %v", err)
	}
	for _, needle := range []string{"\"workKind\": \"СРС\"", "\"file\": \"result.docx\"", "satbayev-gost"} {
		if !strings.Contains(string(task), needle) {
			t.Errorf("task.json misses %s", needle)
		}
	}

	// The stored result is a real file on disk.
	if _, err := os.Stat(svc.StoragePath(file.StorageKey)); err != nil {
		t.Errorf("stored result missing: %v", err)
	}
}

// TestAgentGeneratorPresentation checks the pptx branch: different skill,
// different result file and mime.
func TestAgentGeneratorPresentation(t *testing.T) {
	svc := testService(t)
	writeSkill(t, svc, "pptx", "PPTX-SKILL-MARKER")

	order := testOrder(domain.ProductPresentation)
	source, err := BuildDocument(testOrder(domain.ProductEssay)) // any ZIP payload
	if err != nil {
		t.Fatalf("BuildDocument: %v", err)
	}
	sourcePath := filepath.Join(t.TempDir(), "agent-output.pptx")
	if err := os.WriteFile(sourcePath, source, 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	generator := newAgentGenerator(svc, writeFakeAgent(t, t.TempDir(), sourcePath, "result.pptx", 0))
	files, logText, err := generator.Generate(context.Background(), order)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("files = %d, want 1", len(files))
	}
	file := files[0]
	if file.FileName != "resultat_DG_260391.pptx" {
		t.Errorf("file name = %q", file.FileName)
	}
	if file.MimeType != pptxMime {
		t.Errorf("mime = %q", file.MimeType)
	}
	if !strings.Contains(logText, "pptx") {
		t.Errorf("log does not mention the pptx skill: %q", logText)
	}
}

// TestAgentGeneratorPerWorkDocuments checks that an order with several works
// produces one document per work (result_1..N) instead of one merged file, and
// that a short set fails the attempt instead of shipping half of the works.
func TestAgentGeneratorPerWorkDocuments(t *testing.T) {
	svc := testService(t)
	writeSkill(t, svc, "satbayev-gost", "SATBAYEV-GOST-SKILL-MARKER")

	order := testOrder(domain.ProductEssay)
	order.Works = 3
	order.Topics = "Тема первая\nТема вторая\nТема третья"

	source, err := BuildDocument(order)
	if err != nil {
		t.Fatalf("BuildDocument: %v", err)
	}
	sourcePath := filepath.Join(t.TempDir(), "agent-output.docx")
	if err := os.WriteFile(sourcePath, source, 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	names := []string{"result_1.docx", "result_2.docx", "result_3.docx"}
	generator := newAgentGenerator(svc, writeFakeAgentMulti(t, t.TempDir(), sourcePath, names, 0))
	files, logText, err := generator.Generate(context.Background(), order)
	if err != nil {
		t.Fatalf("Generate: %v (%s)", err, logText)
	}
	if len(files) != 3 {
		t.Fatalf("files = %d, want 3", len(files))
	}
	for index, file := range files {
		wantName := "resultat_DG_260391_" + strconv.Itoa(index+1) + ".docx"
		if file.FileName != wantName {
			t.Errorf("file[%d].FileName = %q, want %q", index, file.FileName, wantName)
		}
		wantKey := filepath.Join("results", "DG-260391_"+strconv.Itoa(index+1)+".docx")
		if file.StorageKey != wantKey {
			t.Errorf("file[%d].StorageKey = %q, want %q", index, file.StorageKey, wantKey)
		}
	}

	// The brief hands the agent one entry per work with its own output file.
	taskJSON, err := os.ReadFile(filepath.Join(svc.Cfg.Worker.GeneratorDir, "work", order.Number, "task.json"))
	if err != nil {
		t.Fatalf("read task.json: %v", err)
	}
	for _, needle := range []string{"Тема вторая", "result_3.docx", "worksList"} {
		if !strings.Contains(string(taskJSON), needle) {
			t.Errorf("task.json misses %q", needle)
		}
	}

	// A short set (2 of 3 documents) must fail the attempt: the client ordered
	// three works and must not be delivered two of them silently.
	shortOrder := *order
	shortOrder.Number = "DG-260392"
	short := newAgentGenerator(svc, writeFakeAgentMulti(t, t.TempDir(), sourcePath, names[:2], 0))
	if _, _, err := short.Generate(context.Background(), &shortOrder); err == nil {
		t.Fatal("Generate with 2 of 3 documents must fail")
	} else if !strings.Contains(err.Error(), "2 из 3") {
		t.Errorf("error = %v, want it to mention 2 из 3", err)
	}
}

// TestAgentGeneratorFailure checks that a failed run reports the agent output.
func TestAgentGeneratorFailure(t *testing.T) {
	svc := testService(t)
	writeSkill(t, svc, "satbayev-gost", "SKILL")

	order := testOrder(domain.ProductEssay)
	generator := newAgentGenerator(svc, writeFakeAgent(t, t.TempDir(), "nonexistent.docx", "result.docx", 3))
	_, logText, err := generator.Generate(context.Background(), order)
	if err == nil {
		t.Fatal("a failing agent was accepted")
	}
	if logText == "" {
		t.Error("no log line for a failed run")
	}
}

// TestAgentGeneratorRejectsNonDocument checks the OOXML validation.
func TestAgentGeneratorRejectsNonDocument(t *testing.T) {
	svc := testService(t)
	writeSkill(t, svc, "satbayev-gost", "SKILL")

	order := testOrder(domain.ProductEssay)
	junk := filepath.Join(t.TempDir(), "junk.docx")
	if err := os.WriteFile(junk, []byte("это не документ, а просто текст"), 0o644); err != nil {
		t.Fatalf("write junk: %v", err)
	}
	generator := newAgentGenerator(svc, writeFakeAgent(t, t.TempDir(), junk, "result.docx", 0))
	_, _, err := generator.Generate(context.Background(), order)
	if err == nil || !strings.Contains(err.Error(), "OOXML") {
		t.Fatalf("expected an OOXML error, got %v", err)
	}
}

// TestResolveGeneratorFallback checks that a missing CLI falls back to the
// built-in writer in auto mode.
func TestResolveGeneratorFallback(t *testing.T) {
	svc := testService(t)
	svc.Cfg.Worker.GeneratorMode = "auto"
	svc.Cfg.Worker.GeneratorCommand = "definitely-not-a-real-agent-cli-xyz"

	generator := resolveGenerator(svc)
	if _, ok := generator.(*builtinGenerator); !ok {
		t.Fatalf("expected the built-in generator, got %T", generator)
	}
}

// TestSkillSelection checks which skill each product asks for.
func TestSkillSelection(t *testing.T) {
	svc := testService(t)
	writeSkill(t, svc, "satbayev-gost", "GOST")
	writeSkill(t, svc, "pptx", "PPTX")

	cases := map[string]string{
		domain.ProductEssay:        "satbayev-gost",
		domain.ProductLab:          "satbayev-gost",
		domain.ProductOther:        "satbayev-gost",
		domain.ProductPresentation: "pptx",
	}
	for product, want := range cases {
		name, dir, ok := skillFor(svc, testOrder(product))
		if !ok {
			t.Errorf("%s: skill not found", product)
		}
		if name != want {
			t.Errorf("%s: skill = %q, want %q", product, name, want)
		}
		if !strings.HasSuffix(dir, want) {
			t.Errorf("%s: dir = %q", product, dir)
		}
	}

	// A missing skill directory is reported, not fatal.
	svc.Cfg.Worker.SkillGOST = "not-installed"
	if _, _, ok := skillFor(svc, testOrder(domain.ProductEssay)); ok {
		t.Error("a missing skill must not be reported as found")
	}
}

// TestAPIKeyEnvNames checks where a BYOK key is exported: the variable must
// match the provider (Cline reads apiKeyEnv of the selected provider), and
// GENERATOR_API_KEY_ENV overrides the derivation.
func TestAPIKeyEnvNames(t *testing.T) {
	cases := map[string][]string{
		"opencode-go": {"OPENCODE_API_KEY"},
		"opencode":    {"OPENCODE_API_KEY"},
		"deepseek":    {"DEEPSEEK_API_KEY"},
		"anthropic":   {"ANTHROPIC_API_KEY"},
		"openrouter":  {"OPENROUTER_API_KEY"},
		"google":      {"GEMINI_API_KEY", "GOOGLE_API_KEY"},
		"openai":      {"OPENAI_API_KEY"},
		"":            {"OPENAI_API_KEY"},
	}
	for provider, want := range cases {
		got := apiKeyEnvNames(config.WorkerConfig{GeneratorProvider: provider})
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("provider %q: got %v, want %v", provider, got, want)
		}
	}

	override := apiKeyEnvNames(config.WorkerConfig{
		GeneratorProvider:  "opencode-go",
		GeneratorAPIKeyEnv: " MY_KEY , OTHER_KEY ",
	})
	if strings.Join(override, ",") != "MY_KEY,OTHER_KEY" {
		t.Errorf("override = %v", override)
	}
}

// TestStaleGeneratingAfterCoversWorstCase checks the requeue threshold: it must
// outlast every attempt running to its timeout, otherwise a slow order is
// requeued while its agent is still writing the document.
func TestStaleGeneratingAfterCoversWorstCase(t *testing.T) {
	cfg := config.WorkerConfig{
		StaleGeneratingAfter: 30 * time.Minute,
		GeneratorTimeout:     20 * time.Minute,
		AgentRetries:         1,
	}
	got := staleGeneratingAfter(cfg)
	worstCase := 2*(cfg.GeneratorTimeout+time.Minute) + time.Minute
	if got < worstCase {
		t.Errorf("stale threshold %s is below the worst case %s", got, worstCase)
	}

	// A larger configured value still wins (it is a floor, not a ceiling).
	cfg.StaleGeneratingAfter = 3 * time.Hour
	if staleGeneratingAfter(cfg) != 3*time.Hour {
		t.Errorf("configured threshold ignored: %s", staleGeneratingAfter(cfg))
	}

	// No retries: one attempt still fits under the floor.
	cfg = config.WorkerConfig{GeneratorTimeout: 20 * time.Minute, AgentRetries: 0}
	if got := staleGeneratingAfter(cfg); got < cfg.GeneratorTimeout+time.Minute {
		t.Errorf("single-attempt floor too small: %s", got)
	}
}
