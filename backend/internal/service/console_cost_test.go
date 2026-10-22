package service

import (
	"io"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/srunal/srunal-go/internal/config"
)

// TestAgentRunCostParsesCLITotal checks the cost extraction from the raw CLI
// stream: "totalCost" is the field the agent writes after every turn (and the
// one the console renders as «всего $x»), the rendered text is a fallback.
func TestAgentRunCostParsesCLITotal(t *testing.T) {
	dir := t.TempDir()
	svc := &Service{
		Cfg: &config.Config{Worker: config.WorkerConfig{GeneratorDir: dir}},
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	work := filepath.Join(dir, "work", "DG-260391")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	stream := `{"type":"usage","inputTokens":10,"outputTokens":2,"cost":0.0009,"totalCost":0.0009}
{"type":"say","text":"работаю"}
{"type":"usage","inputTokens":154571,"outputTokens":262,"cost":0.0009,"totalCost":0.121123446}
{"type":"run_result","finishReason":"completed"}`
	if err := os.WriteFile(filepath.Join(work, "agent.log"), []byte(stream), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}

	if got := svc.AgentRunCost("DG-260391"); math.Abs(got-0.121123446) > 1e-9 {
		t.Fatalf("cost = %v, want 0.121123446", got)
	}
	if got := svc.AgentRunCost("DG-260000"); got != 0 {
		t.Fatalf("missing log -> %v, want 0", got)
	}
}
