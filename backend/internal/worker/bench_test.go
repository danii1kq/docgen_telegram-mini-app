package worker

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/srunal/srunal-go/internal/domain"
)

// BenchmarkCappedWriter measures the log writer of a generation run. The agent
// emits thousands of small JSON chunks per order, so the per-write cost is on
// the hot path of every generation.
//
// Baseline (Stat() after every write): see the commit body of the optimization.
func BenchmarkCappedWriter(b *testing.B) {
	dir := b.TempDir()
	path := filepath.Join(dir, "agent.log")

	file, err := os.Create(path)
	if err != nil {
		b.Fatal(err)
	}
	defer file.Close()

	// A realistic chunk: one streamed JSON event of the agent.
	chunk := []byte(`{"ts":"2026-11-02T18:00:00.123Z","type":"agent_event","event":{"type":"content_update","contentType":"text","text":"` +
		"фрагмент рассуждения модели, примерно сто байт полезной нагрузки" + `"}}` + "\n")

	writer := newCappedWriter(file, agentLogLimit)
	b.SetBytes(int64(len(chunk)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := writer.Write(chunk); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCappedWriterCompaction measures the write path across a compaction:
// the log is capped, so a long run rewrites the file with its own tail.
func BenchmarkCappedWriterCompaction(b *testing.B) {
	dir := b.TempDir()
	path := filepath.Join(dir, "agent.log")

	file, err := os.Create(path)
	if err != nil {
		b.Fatal(err)
	}
	defer file.Close()

	chunk := make([]byte, 4096)
	for i := range chunk {
		chunk[i] = 'x'
	}
	chunk[len(chunk)-1] = '\n'

	// A small limit forces a compaction every few writes.
	writer := newCappedWriter(file, 64<<10)
	b.SetBytes(int64(len(chunk)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := writer.Write(chunk); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkAgentResultNames is the per-order allocation of the output contract.
func BenchmarkAgentResultNames(b *testing.B) {
	order := testOrder(domain.ProductPresentation)
	order.Works = 5
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if names := agentResultNames(order); len(names) != 5 {
			b.Fatalf("names = %d", len(names))
		}
	}
}

// BenchmarkSplitTopicLines measures the topic parsing of one order.
func BenchmarkSplitTopicLines(b *testing.B) {
	topics := ""
	for i := 0; i < 8; i++ {
		topics += "Тема работы номер " + strconv.Itoa(i) + "\r\n"
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if lines := splitTopicLines(topics); len(lines) != 8 {
			b.Fatalf("lines = %d", len(lines))
		}
	}
}

// BenchmarkSkillPrepare measures how the skill reaches the work directory of an
// order: a full copy of the skill tree versus a symlink. The pptx skill is the
// worst case (~1.1 MB, 56 files) and it is prepared for every generation.
func BenchmarkSkillPrepare(b *testing.B) {
	// go test runs with the package directory as the working directory.
	src := filepath.Join("..", "..", "skills", "pptx")
	if _, err := os.Stat(src); err != nil {
		b.Skipf("skill source not available: %v", err)
	}

	b.Run("copy", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			dst := filepath.Join(b.TempDir(), "skill")
			if err := copyTree(src, dst); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("symlink", func(b *testing.B) {
		abs, err := filepath.Abs(src)
		if err != nil {
			b.Fatal(err)
		}
		for i := 0; i < b.N; i++ {
			dst := filepath.Join(b.TempDir(), "skill")
			if err := linkSkill(abs, dst); err != nil {
				b.Fatal(err)
			}
		}
	})
}
