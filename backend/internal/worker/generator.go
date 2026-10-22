package worker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/srunal/srunal-go/internal/domain"
	"github.com/srunal/srunal-go/internal/service"
	"github.com/srunal/srunal-go/internal/store"
)

// Generator turns an order into a ready document: either the bundled .docx
// writer or an external AI agent (Cline) that follows a skill.
type Generator interface {
	// Generate returns the stored result files (one per work when the order has
	// several) plus a short log line for the job.
	Generate(ctx context.Context, order *domain.Order) ([]store.OrderFileRow, string, error)
}

// builtinGenerator is the demo/fallback path: a real (but simple) .docx with
// the order brief. It also simulates generation time so the queue behaves like
// production when no agent CLI is installed.
type builtinGenerator struct {
	svc  *service.Service
	rand *rand.Rand
}

func (g *builtinGenerator) Generate(ctx context.Context, order *domain.Order) ([]store.OrderFileRow, string, error) {
	minDur := g.svc.Cfg.Worker.GenerationMin
	maxDur := g.svc.Cfg.Worker.GenerationMax
	delay := minDur
	if maxDur > minDur {
		delay = minDur + time.Duration(g.rand.Int63n(int64(maxDur-minDur)))
	}
	select {
	case <-ctx.Done():
		return nil, "генерация отменена", ctx.Err()
	case <-time.After(delay):
	}

	briefs := worksOf(order)
	files := make([]store.OrderFileRow, 0, len(briefs))
	for index, brief := range briefs {
		workOrder := *order
		if len(briefs) > 1 {
			workOrder.Topics = brief.Topic
		}
		document, err := BuildDocument(&workOrder)
		if err != nil {
			return nil, "", err
		}
		name := "resultat_" + strings.ReplaceAll(order.Number, "-", "_")
		if len(briefs) > 1 {
			name += fmt.Sprintf("_%d", index+1)
		}
		name += ".docx"
		file, err := g.svc.WriteResultFile(order.Number, name, docxMime, document)
		if err != nil {
			return nil, "", err
		}
		files = append(files, file)
	}
	logLine := "встроенный генератор: собран .docx с заданием заказа"
	if len(files) > 1 {
		logLine = fmt.Sprintf("встроенный генератор: собрано документов - %d", len(files))
	}
	return files, logLine, nil
}

// resolveGenerator picks the generator for this process.
func resolveGenerator(svc *service.Service) Generator {
	cfg := svc.Cfg.Worker
	mode := strings.ToLower(strings.TrimSpace(cfg.GeneratorMode))
	builtin := &builtinGenerator{svc: svc, rand: rand.New(rand.NewSource(time.Now().UnixNano()))}

	if mode == "builtin" {
		svc.Log.Info("generator: built-in .docx writer", "mode", mode)
		return builtin
	}

	path, err := lookAgentCommand(cfg.GeneratorCommand)
	switch {
	case err == nil:
		svc.Log.Info("generator: AI agent", "mode", mode, "command", path,
			"max_agents", cfg.MaxAgents, "timeout", cfg.GeneratorTimeout.String())
		return newAgentGenerator(svc, path)
	case mode == "agent":
		// Explicitly requested: do not silently fall back, the operator must
		// see why every generation fails.
		svc.Log.Error("generator: GENERATOR_MODE=agent but the command was not found",
			"command", cfg.GeneratorCommand, "error", err)
		return newAgentGenerator(svc, cfg.GeneratorCommand)
	default:
		svc.Log.Warn("generator: agent CLI not found, using the built-in writer",
			"command", cfg.GeneratorCommand, "hint", "install it (npm i -g cline) or set GENERATOR_MODE=builtin")
		return builtin
	}
}

// lookAgentCommand resolves the agent executable (and reports whether it is a
// batch shim that needs cmd /c on Windows).
func lookAgentCommand(command string) (string, error) {
	if strings.TrimSpace(command) == "" {
		return "", errors.New("worker: empty generator command")
	}
	return lookPath(command)
}

// skillFor returns the skill name and directory for an order: presentations use
// the pptx skill, everything else the GOST document skill.
func skillFor(svc *service.Service, order *domain.Order) (name, dir string, ok bool) {
	cfg := svc.Cfg.Worker
	name = cfg.SkillGOST
	if order.Product == domain.ProductPresentation {
		name = cfg.SkillPPTX
	}
	dir = filepath.Join(cfg.SkillSourceDir, name)
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return name, dir, false
	}
	return name, dir, true
}

// copyTree copies a directory tree, skipping the noise (caches, evals, VCS).
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return os.MkdirAll(dst, 0o755)
		}
		switch entry.Name() {
		case "__pycache__", ".git", "node_modules", "evals":
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(dst, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
