package service

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestParseAgentLog checks the console rendering: the streaming deltas (hundreds
// per run) are dropped, the events that explain the work survive - iterations,
// tool commands, errors and the final result.
func TestParseAgentLog(t *testing.T) {
	raw := strings.Join([]string{
		`{"ts":"2026-10-14T01:00:00Z","type":"agent_event","event":{"type":"content_start","contentType":"reasoning","reasoning":" thin"}}`,
		`{"ts":"2026-10-14T01:00:01Z","type":"agent_event","event":{"type":"iteration_start"}}`,
		`{"ts":"2026-10-14T01:00:02Z","type":"agent_event","event":{"type":"content_end","contentType":"text","text":"Собираю документ"}}`,
		`{"ts":"2026-10-14T01:00:03Z","type":"agent_event","event":{"type":"content_update","contentType":"tool","toolName":"run_commands","update":{"stream":"stdout","chunk":"x"}}}`,
		`{"ts":"2026-10-14T01:00:04Z","type":"agent_event","event":{"type":"content_update","contentType":"tool","toolName":"run_commands","update":{"query":"python3 build.py\n# вторая строка"}}}`,
		`{"ts":"2026-10-14T01:00:04Z","type":"agent_event","event":{"type":"content_update","contentType":"tool","toolName":"run_commands","update":{"query":"python3 build.py\n# вторая строка"}}}`,
		`{"ts":"2026-10-14T01:00:05Z","type":"agent_event","event":{"type":"usage","inputTokens":100,"outputTokens":20,"cost":0.001,"totalCost":0.002}}`,
		`{"ts":"2026-10-14T01:00:06Z","type":"error","message":"Upstream request failed"}`,
		`{"ts":"2026-10-14T01:00:07Z","type":"run_result","finishReason":"completed","iterations":3,"usage":{"totalCost":0.12}}`,
		`{"broken`, // a partial line from the tail cut
	}, "\n")

	lines, truncated := parseAgentLog([]byte(raw))
	if truncated {
		t.Error("nothing should be truncated here")
	}

	kinds := []string{}
	for _, line := range lines {
		kinds = append(kinds, line.Kind)
		if strings.Contains(line.Text, "\n") {
			t.Errorf("console text must be flattened: %q", line.Text)
		}
	}
	want := []string{consoleIteration, consoleText, consoleTool, consoleUsage, consoleError, consoleResult}
	if strings.Join(kinds, ",") != strings.Join(want, ",") {
		t.Fatalf("kinds = %v, want %v", kinds, want)
	}
	if !strings.Contains(lines[1].Text, "Собираю документ") {
		t.Errorf("text line = %q", lines[1].Text)
	}
	if !strings.Contains(lines[2].Text, "run_commands: python3 build.py # вторая строка") {
		t.Errorf("tool line = %q", lines[2].Text)
	}
	if !strings.Contains(lines[5].Text, "$0.1200") {
		t.Errorf("result line = %q", lines[5].Text)
	}
}

// TestParseAgentLogCaps checks that a long stream keeps only the last events.
func TestParseAgentLogCaps(t *testing.T) {
	var raw strings.Builder
	for i := 0; i < consoleMaxLines+50; i++ {
		raw.WriteString(`{"type":"agent_event","event":{"type":"content_end","contentType":"text","text":"шаг"}}`)
		raw.WriteString("\n")
	}
	lines, truncated := parseAgentLog([]byte(raw.String()))
	if !truncated {
		t.Error("expected the older events to be dropped")
	}
	if len(lines) != consoleMaxLines {
		t.Errorf("lines = %d, want %d", len(lines), consoleMaxLines)
	}
}

// TestReadConsoleEventsSkipsNoiseTail checks the chunked reader: when the tail of
// the log is filled with stream chunks (a long tool output), the events that came
// before them are still rendered instead of an empty console.
func TestReadConsoleEventsSkipsNoiseTail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.log")

	var buf bytes.Buffer
	buf.WriteString(`{"ts":"2026-10-14T01:00:01Z","type":"agent_event","event":{"type":"iteration_start"}}` + "\n")
	buf.WriteString(`{"ts":"2026-10-14T01:00:02Z","type":"agent_event","event":{"type":"content_end","contentType":"text","text":"Готовлю документ"}}` + "\n")

	noise := `{"ts":"2026-10-14T01:00:03Z","type":"agent_event","event":{"type":"content_update","contentType":"tool","toolName":"run_commands","update":{"stream":"stdout","chunk":"` +
		strings.Repeat("x", 900) + `"}}}` + "\n"
	for buf.Len() < consoleTailBytes+consoleTailBytes/2 {
		buf.WriteString(noise)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}

	lines, _, err := readConsoleEvents(path)
	if err != nil {
		t.Fatalf("readConsoleEvents: %v", err)
	}
	if len(lines) < 2 {
		t.Fatalf("events before the noise tail were lost: %d lines", len(lines))
	}
	if lines[0].Kind != consoleIteration || lines[1].Kind != consoleText {
		t.Errorf("lines = %+v", lines[:2])
	}
}
