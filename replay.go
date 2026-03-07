package main

import (
	"bytes"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"time"

	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
)

func runReplayCommand(args []string) {
	fs := flag.NewFlagSet("replay", flag.ExitOnError)
	configPath := fs.String("config", "config.toml", "path to config file")
	noCall := fs.Bool("no-call", false, "skip calling the Grok API")
	fullContext := fs.Bool("full-context", false, "show full content in diffs")
	fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintf(os.Stderr, "usage: discord-grok replay [flags] <logfile>\n")
		os.Exit(1)
	}
	logFilePath := fs.Arg(0)

	log, err := LoadPromptLog(logFilePath)
	if err != nil {
		slog.Error("failed to load prompt log", "error", err)
		os.Exit(1)
	}

	// Parse the log's timestamp and restore the original timezone
	logTime, err := time.Parse(time.RFC3339, log.Timestamp)
	if err != nil {
		slog.Error("failed to parse log timestamp", "error", err)
		os.Exit(1)
	}
	loc := time.FixedZone(log.Timezone, log.TimezoneOffset)
	logTime = logTime.In(loc)

	// Reconstruct prompt from structured data
	cb := &ContextBuilder{}
	newSystem, newUserContent := cb.ToXMLPrompt(log.ContextMessages, log.TriggerMsgID, logTime)

	// Diff system prompts
	showDiff("System Prompt", log.Prompt.System, newSystem, *fullContext, highlightXML)

	// Diff user content text (first content part)
	var origText, newText string
	if len(log.Prompt.UserContent) > 0 {
		origText = log.Prompt.UserContent[0].Text
	}
	if len(newUserContent) > 0 {
		newText = newUserContent[0].Text
	}
	showDiff("User Content", origText, newText, *fullContext, highlightXML)

	if *noCall {
		return
	}

	// Call Grok API with reconstructed prompt
	cfg, err := LoadConfig(*configPath)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	grokClient := NewGrokClient(cfg.XAIAPIKey)
	messages := []ChatMessage{
		{Role: "system", Content: newSystem},
		{Role: "user", Content: newUserContent},
	}

	maxTokens := maxResponseBytes / bytesPerToken
	opts := SendOptions{MaxTokens: maxTokens}
	response, err := grokClient.SendMessage(messages, opts)
	if err != nil {
		slog.Error("failed to call Grok API", "error", err)
		os.Exit(1)
	}

	showDiff("Response", log.Response, response, *fullContext, nil)
}

func showDiff(label string, original string, reconstructed string, fullContext bool, highlight func(string) string) {
	fmt.Printf("\033[1;36m=== %s ===\033[0m\n", label)

	if original == reconstructed {
		if fullContext {
			if highlight != nil {
				fmt.Print(highlight(original))
			} else {
				fmt.Print(original)
			}
			if original != "" && original[len(original)-1] != '\n' {
				fmt.Println()
			}
		}
		fmt.Println("(identical)")
		fmt.Println()
		return
	}

	origFile, err := os.CreateTemp("", "replay-orig-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp file: %v\n", err)
		return
	}
	defer os.Remove(origFile.Name())

	reconFile, err := os.CreateTemp("", "replay-recon-*")
	if err != nil {
		origFile.Close()
		fmt.Fprintf(os.Stderr, "failed to create temp file: %v\n", err)
		return
	}
	defer os.Remove(reconFile.Name())

	origFile.WriteString(original)
	origFile.Close()
	reconFile.WriteString(reconstructed)
	reconFile.Close()

	diffFlag := "-u"
	if fullContext {
		diffFlag = "-U9999"
	}
	cmd := exec.Command("diff", diffFlag, "--color=always",
		"--label", "original",
		"--label", "reconstructed",
		origFile.Name(), reconFile.Name())
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run() // exit code 1 when files differ is expected
	fmt.Println()
}

func highlightXML(text string) string {
	lexer := lexers.Get("xml")
	style := styles.Get("tango")
	formatter := formatters.Get("terminal")
	iterator, err := lexer.Tokenise(nil, text)
	if err != nil {
		return text
	}
	var buf bytes.Buffer
	if err := formatter.Format(&buf, style, iterator); err != nil {
		return text
	}
	return buf.String()
}
