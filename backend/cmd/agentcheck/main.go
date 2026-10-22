// Command agentcheck runs one document-generation agent exactly the way the
// worker does (same arguments, working directory and environment) and prints
// its output. It exists to diagnose the agent CLI on a new machine:
//
//	go run ./cmd/agentcheck -cwd var/generator/work/DG-260403 -prompt "…"
//
// The prompt defaults to a short probe, so an unauthenticated or missing CLI is
// visible in seconds instead of after a queued order.
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

func main() {
	cwd := flag.String("cwd", ".", "working directory handed to the agent")
	prompt := flag.String("prompt", "Ответь ровно одним словом: DONE", "prompt handed to the agent")
	command := flag.String("command", os.Getenv("GENERATOR_COMMAND"), "agent CLI (default $GENERATOR_COMMAND or cline)")
	provider := flag.String("provider", envOr("GENERATOR_PROVIDER", "opencode-go"), "model provider")
	model := flag.String("model", envOr("GENERATOR_MODEL", "deepseek-v4.1-flash"), "model id")
	thinking := flag.String("thinking", envOr("GENERATOR_THINKING", "high"), "reasoning effort")
	timeout := flag.Duration("timeout", 5*time.Minute, "run timeout")
	listFiles := flag.Bool("files", false, "list the work directory before running")
	flag.Parse()

	if *command == "" {
		*command = "cline"
	}
	if *listFiles {
		entries, err := os.ReadDir(*cwd)
		if err != nil {
			fmt.Fprintf(os.Stderr, "cwd %s: %v\n", *cwd, err)
			os.Exit(1)
		}
		for _, entry := range entries {
			fmt.Printf("  %s\n", entry.Name())
		}
	}

	args := []string{
		"--json",
		"--auto-approve", "true",
		"--timeout", strconv.Itoa(int(timeout.Seconds())),
		"-c", *cwd,
	}
	if *provider != "" {
		args = append(args, "-P", *provider)
	}
	if *model != "" {
		args = append(args, "-m", *model)
	}
	if *thinking != "" {
		args = append(args, "--thinking", *thinking)
	}
	args = append(args, *prompt)

	executable := *command
	if runtime.GOOS == "windows" && strings.HasSuffix(strings.ToLower(executable), ".cmd") {
		args = append([]string{"/c", executable}, args...)
		executable = "cmd"
	}

	fmt.Printf("command: %s\nargs: %d\ntimeout: %s\n\n", executable, len(args)-1, timeout.String())

	ctx, cancel := context.WithTimeout(context.Background(), *timeout+30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Dir = *cwd
	cmd.Env = append(os.Environ(), "NO_COLOR=1")

	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	started := time.Now()
	err := cmd.Run()
	fmt.Printf("--- exit=%v in %s ---\n%s\n", err, time.Since(started).Round(time.Second), output.String())
	if err != nil {
		os.Exit(1)
	}
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
