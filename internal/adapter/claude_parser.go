package adapter

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/code-all-remote/car/internal/domain"
)

// OutputParser normalizes raw agent output into domain events.
type OutputParser struct {
	mu                    interface{} // dummy for sync.Mutex when needed
	compatibilityDegraded bool
	knownPatterns         []Pattern
}

// Pattern defines a regex pattern for recognizing output.
type Pattern struct {
	Name    string
	Regex   *regexp.Regexp
	Handler func(match []string) (*ParsedEvent, error)
}

// ParsedEvent represents a normalized event from parsing.
type ParsedEvent struct {
	Type    AdapterSignalType
	Payload any
}

// NewOutputParser creates a new output parser.
func NewOutputParser() *OutputParser {
	p := &OutputParser{
		knownPatterns: make([]Pattern, 0),
	}
	p.registerDefaultPatterns()
	return p
}

// SetCompatibilityDegraded sets the degraded mode flag.
func (p *OutputParser) SetCompatibilityDegraded(degraded bool) {
	p.compatibilityDegraded = degraded
}

// IsCompatibilityDegraded returns true if in degraded mode.
func (p *OutputParser) IsCompatibilityDegraded() bool {
	return p.compatibilityDegraded
}

// registerDefaultPatterns registers patterns for Claude Code output.
func (p *OutputParser) registerDefaultPatterns() {
	// Pattern for approval requests
	p.knownPatterns = append(p.knownPatterns, Pattern{
		Name:  "approval_request",
		Regex: regexp.MustCompile(`(?i)approval\s+(?:required|requested|needed)[:\s]+(.+)`),
		Handler: func(match []string) (*ParsedEvent, error) {
			if len(match) < 2 {
				return nil, fmt.Errorf("insufficient match groups")
			}
			return &ParsedEvent{
				Type: SignalApprovalRequest,
				Payload: ApprovalRequestPayload{
					HumanReadableContext: match[1],
				},
			}, nil
		},
	})

	// Pattern for tool/exec commands
	p.knownPatterns = append(p.knownPatterns, Pattern{
		Name:  "tool_execution",
		Regex: regexp.MustCompile(`(?i)(?:executing|running)\s+(?:command|tool)[:\s]+["']?([^"'\n]+)["']?`),
		Handler: func(match []string) (*ParsedEvent, error) {
			if len(match) < 2 {
				return nil, fmt.Errorf("insufficient match groups")
			}
			return &ParsedEvent{
				Type: SignalOutput,
				Payload: OutputPayload{
					Content: fmt.Sprintf("Executing: %s", match[1]),
					Stream:  "stdout",
				},
			}, nil
		},
	})

	// Pattern for errors
	p.knownPatterns = append(p.knownPatterns, Pattern{
		Name:  "error",
		Regex: regexp.MustCompile(`(?i)^error[:\s]+(.+)$`),
		Handler: func(match []string) (*ParsedEvent, error) {
			if len(match) < 2 {
				return nil, fmt.Errorf("insufficient match groups")
			}
			return &ParsedEvent{
				Type: SignalError,
				Payload: ErrorPayload{
					Message: match[1],
				},
			}, nil
		},
	})

	// Pattern for JSON output (structured events)
	p.knownPatterns = append(p.knownPatterns, Pattern{
		Name:  "json_event",
		Regex: regexp.MustCompile(`^\s*(\{.*\})\s*$`),
		Handler: func(match []string) (*ParsedEvent, error) {
			if len(match) < 2 {
				return nil, fmt.Errorf("insufficient match groups")
			}

			var event struct {
				Type    string `json:"type"`
				Payload any    `json:"payload"`
			}
			if err := json.Unmarshal([]byte(match[1]), &event); err != nil {
				return nil, fmt.Errorf("parsing JSON event: %w", err)
			}

			signalType := SignalOutput
			switch event.Type {
			case "approval":
				signalType = SignalApprovalRequest
			case "error":
				signalType = SignalError
			case "completion":
				signalType = SignalCompletion
			}

			return &ParsedEvent{
				Type:    signalType,
				Payload: event.Payload,
			}, nil
		},
	})
}

// parseStreamJSONLine recognizes the real Claude Code `--output-format stream-json`
// NDJSON events documented in ADR-009 and maps them to ParsedEvents. It returns
// (event, true) when the line is a recognized stream-json event (event may be nil
// for events we deliberately suppress), and (nil, false) when the line is not a
// recognized stream-json event so ParseLine can fall back to the regex patterns.
// ADR-009: the exact field names are pending operator verification against a real
// claude stream.
func (p *OutputParser) parseStreamJSONLine(line string) (*ParsedEvent, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || trimmed[0] != '{' {
		return nil, false
	}

	var raw struct {
		Type    string          `json:"type"`
		Subtype string          `json:"subtype"`
		Result  string          `json:"result"`
		Message json.RawMessage `json:"message"`
		Event   json.RawMessage `json:"event"`
	}
	if err := json.Unmarshal([]byte(trimmed), &raw); err != nil {
		return nil, false
	}

	switch raw.Type {
	case "system":
		// Session metadata; the adapter already emits an active status separately.
		return nil, true
	case "user":
		// Typically tool_result echoes; recognize but suppress.
		return nil, true
	case "result":
		if raw.Result != "" {
			return &ParsedEvent{Type: SignalOutput, Payload: OutputPayload{Content: raw.Result, Stream: "stdout"}}, true
		}
		return nil, true
	case "assistant":
		var msg struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
				Name string `json:"name"`
			} `json:"content"`
		}
		if err := json.Unmarshal(raw.Message, &msg); err != nil {
			return nil, true
		}
		var texts []string
		for _, b := range msg.Content {
			if b.Type == "text" {
				texts = append(texts, b.Text)
			}
		}
		if joined := strings.Join(texts, ""); joined != "" {
			return &ParsedEvent{Type: SignalOutput, Payload: OutputPayload{Content: joined, Stream: "stdout"}}, true
		}
		for _, b := range msg.Content {
			if b.Type == "tool_use" {
				return &ParsedEvent{Type: SignalDiagnostic, Payload: DiagnosticPayload{Level: "info", Message: "tool_use: " + b.Name}}, true
			}
		}
		return nil, true
	case "stream_event":
		var ev struct {
			Type  string `json:"type"`
			Delta struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"delta"`
		}
		if err := json.Unmarshal(raw.Event, &ev); err != nil {
			return nil, true
		}
		if ev.Delta.Type == "text_delta" && ev.Delta.Text != "" {
			return &ParsedEvent{Type: SignalOutput, Payload: OutputPayload{Content: ev.Delta.Text, Stream: "stdout"}}, true
		}
		return nil, true
	default:
		// Unknown top-level type: fall back to existing regex patterns.
		return nil, false
	}
}

// ParseLine parses a single line of output and returns a normalized event. It
// tries the real stream-json events first (A-3, ADR-009), then falls back to the
// regex-based terminal-text patterns.
func (p *OutputParser) ParseLine(line string) *ParsedEvent {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}

	if ev, ok := p.parseStreamJSONLine(line); ok {
		return ev
	}

	for _, pattern := range p.knownPatterns {
		match := pattern.Regex.FindStringSubmatch(line)
		if match != nil {
			event, err := pattern.Handler(match)
			if err == nil {
				return event
			}
			// Continue to next pattern on error
		}
	}

	// No pattern matched - treat as plain output
	return &ParsedEvent{
		Type: SignalOutput,
		Payload: OutputPayload{
			Content: line,
			Stream:  "stdout",
		},
	}
}

// ParseStream parses a stream of output bytes into events.
func (p *OutputParser) ParseStream(data []byte) []ParsedEvent {
	var events []ParsedEvent

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		if event := p.ParseLine(line); event != nil {
			events = append(events, *event)
		}
	}

	return events
}

// ParseBuffer parses accumulated output buffer, handling partial lines.
func (p *OutputParser) ParseBuffer(buffer []byte, isComplete bool) ([]ParsedEvent, []byte) {
	var events []ParsedEvent

	// Find complete lines
	lines := bytes.Split(buffer, []byte("\n"))

	// All but last are complete lines
	for i := 0; i < len(lines)-1; i++ {
		line := string(bytes.TrimSpace(lines[i]))
		if line != "" {
			if event := p.ParseLine(line); event != nil {
				events = append(events, *event)
			}
		}
	}

	// Last might be partial
	var remaining []byte
	if len(lines) > 0 {
		last := lines[len(lines)-1]
		if !isComplete && len(last) > 0 {
			// Keep partial line for next parse
			remaining = last
		} else if len(last) > 0 {
			if event := p.ParseLine(string(last)); event != nil {
				events = append(events, *event)
			}
		}
	}

	return events, remaining
}

// RecognizeApproval attempts to recognize approval patterns in text.
func (p *OutputParser) RecognizeApproval(text string) *ApprovalRequestPayload {
	text = strings.TrimSpace(text)

	// Look for common approval phrases
	phrases := []string{
		"do you want to",
		"would you like to",
		"should i",
		"shall i",
		"permission to",
		"approval to",
		"may i",
	}

	for _, phrase := range phrases {
		if strings.Contains(strings.ToLower(text), phrase) {
			return &ApprovalRequestPayload{
				Category:             "command_execution",
				ActionKind:           "execute",
				HumanReadableContext: text,
				StructuredPayload: map[string]any{
					"trigger_phrase": phrase,
				},
				ExpiresIn: 5 * time.Minute,
			}
		}
	}

	return nil
}

// NormalizePayload converts raw output to appropriate payload type.
func (p *OutputParser) NormalizePayload(raw string, signalType AdapterSignalType) (any, error) {
	switch signalType {
	case SignalOutput:
		return OutputPayload{Content: raw, Stream: "stdout"}, nil
	case SignalError:
		return ErrorPayload{Message: raw}, nil
	case SignalDiagnostic:
		return DiagnosticPayload{Level: "info", Message: raw}, nil
	case SignalCompletion:
		// Try to parse exit code
		if strings.Contains(raw, "exit code") {
			var code int
			fmt.Sscanf(raw, "exit code %d", &code)
			return CompletionPayload{ExitCode: code}, nil
		}
		return CompletionPayload{}, nil
	default:
		return map[string]any{"raw": raw}, nil
	}
}

// IsSignalline determines if a line should generate a structured event.
func (p *OutputParser) IsSignalLine(line string) bool {
	line = strings.TrimSpace(line)
	for _, pattern := range p.knownPatterns {
		if pattern.Regex.MatchString(line) {
			return true
		}
	}
	return false
}

// ShouldEmitRaw determines if raw output should also be emitted.
func (p *OutputParser) ShouldEmitRaw() bool {
	return !p.compatibilityDegraded
}

// ExtractChangedFiles attempts to extract changed file information from output.
func (p *OutputParser) ExtractChangedFiles(output string) []ChangedFilePayload {
	var files []ChangedFilePayload

	// Pattern for "Modified: path/to/file"
	modifiedRe := regexp.MustCompile(`(?i)(?:modified|changed|updated)[:\s]+([^\n]+)`)
	for _, match := range modifiedRe.FindAllStringSubmatch(output, -1) {
		if len(match) >= 2 {
			files = append(files, ChangedFilePayload{
				Path:      strings.TrimSpace(match[1]),
				Operation: "modify",
			})
		}
	}

	// Pattern for "Created: path/to/file"
	createdRe := regexp.MustCompile(`(?i)(?:created|added|new)[:\s]+([^\n]+)`)
	for _, match := range createdRe.FindAllStringSubmatch(output, -1) {
		if len(match) >= 2 {
			files = append(files, ChangedFilePayload{
				Path:      strings.TrimSpace(match[1]),
				Operation: "create",
			})
		}
	}

	// Pattern for "Deleted: path/to/file"
	deletedRe := regexp.MustCompile(`(?i)(?:deleted|removed)[:\s]+([^\n]+)`)
	for _, match := range deletedRe.FindAllStringSubmatch(output, -1) {
		if len(match) >= 2 {
			files = append(files, ChangedFilePayload{
				Path:      strings.TrimSpace(match[1]),
				Operation: "delete",
			})
		}
	}

	return files
}

// RecognizeStatusChange attempts to recognize state changes in output.
func (p *OutputParser) RecognizeStatusChange(text string) *StatusChangePayload {
	text = strings.TrimSpace(text)
	lower := strings.ToLower(text)

	// Started patterns
	if strings.Contains(lower, "starting") || strings.Contains(lower, "beginning") {
		return &StatusChangePayload{
			OldState: "pending",
			NewState: domain.RunStateStarting,
		}
	}

	// Running/active patterns
	if strings.Contains(lower, "ready") || strings.Contains(lower, "running") {
		return &StatusChangePayload{
			OldState: domain.RunStateStarting,
			NewState: domain.RunStateActive,
		}
	}

	// Completion patterns
	if strings.Contains(lower, "finished") || strings.Contains(lower, "completed") || strings.Contains(lower, "done") {
		return &StatusChangePayload{
			OldState: domain.RunStateActive,
			NewState: domain.RunStateCompleted,
		}
	}

	// Error patterns
	if strings.Contains(lower, "failed") || strings.Contains(lower, "error") {
		return &StatusChangePayload{
			OldState: domain.RunStateActive,
			NewState: domain.RunStateFailed,
			Reason:   text,
		}
	}

	return nil
}

// RedactSecrets removes common secret patterns from output.
func (p *OutputParser) RedactSecrets(output string, secrets []string) string {
	result := output
	for _, secret := range secrets {
		if secret != "" && len(secret) >= 4 {
			result = strings.ReplaceAll(result, secret, "[REDACTED]")
		}
	}
	return result
}
