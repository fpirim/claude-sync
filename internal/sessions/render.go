package sessions

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// Turn is a single user or assistant message extracted from a JSONL session.
// Tool uses, file snapshots, and other meta records are intentionally dropped
// or summarized so the rendered transcript stays scannable.
type Turn struct {
	Role string // "user" | "assistant"
	When time.Time
	Text string // plain-text body, joined from all text parts
	Note string // optional inline note for tool calls etc. ("[tool: Read]")
}

// LoadTranscript streams a JSONL file and returns its user+assistant turns
// in order. Limit caps the number of turns returned (0 = unlimited);
// callers can use it to keep memory bounded for very long sessions.
func LoadTranscript(jsonlPath string, limit int) ([]Turn, error) {
	f, err := os.Open(jsonlPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 8*1024*1024)

	var out []Turn
	for sc.Scan() {
		line := sc.Bytes()
		var rec rawRec
		if err := json.Unmarshal(line, &rec); err != nil {
			continue
		}
		switch rec.Type {
		case "user":
			t := Turn{Role: "user", Text: extractText(rec.Message.Content)}
			if rec.Timestamp != "" {
				if ts, err := time.Parse(time.RFC3339Nano, rec.Timestamp); err == nil {
					t.When = ts
				}
			}
			out = append(out, t)
		case "assistant":
			text, tools := extractAssistant(rec.Message.Content)
			t := Turn{Role: "assistant", Text: text}
			if len(tools) > 0 {
				t.Note = "uses " + strings.Join(tools, ", ")
			}
			if rec.Timestamp != "" {
				if ts, err := time.Parse(time.RFC3339Nano, rec.Timestamp); err == nil {
					t.When = ts
				}
			}
			out = append(out, t)
		}
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, sc.Err()
}

type rawRec struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	Message   struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

// extractAssistant returns the text content (joined) plus the list of tool
// names invoked in this turn.
func extractAssistant(raw json.RawMessage) (string, []string) {
	if len(raw) == 0 {
		return "", nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s, nil
	}
	var arr []struct {
		Type  string `json:"type"`
		Text  string `json:"text"`
		Name  string `json:"name"`
		Input json.RawMessage `json:"input"`
	}
	if err := json.Unmarshal(raw, &arr); err == nil {
		var parts []string
		var tools []string
		for _, p := range arr {
			switch p.Type {
			case "text":
				if p.Text != "" {
					parts = append(parts, p.Text)
				}
			case "tool_use":
				if p.Name != "" {
					tools = append(tools, p.Name)
				}
			case "thinking":
				// skip silently — too long to render in the preview pane
			}
		}
		return strings.Join(parts, "\n\n"), tools
	}
	return "", nil
}

// FormatTranscript renders turns as a readable transcript. Width is the
// target column count; styler is called for role labels so the caller can
// inject lipgloss colors. styler may be nil for plain output.
func FormatTranscript(turns []Turn, width int, styler func(role string) string) string {
	if width < 20 {
		width = 80
	}
	var sb strings.Builder
	for i, t := range turns {
		role := t.Role
		if styler != nil {
			role = styler(t.Role)
		}
		stamp := ""
		if !t.When.IsZero() {
			stamp = " " + t.When.Format("01-02 15:04")
		}
		sb.WriteString(role + stamp)
		if t.Note != "" {
			sb.WriteString("  " + t.Note)
		}
		sb.WriteString("\n")

		body := t.Text
		if body == "" {
			body = "(empty)"
		}
		sb.WriteString(wordWrap(body, width))
		if i != len(turns)-1 {
			sb.WriteString("\n\n")
		} else {
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

// wordWrap breaks text to fit within width columns, preserving paragraph
// breaks (blank lines). It handles unicode reasonably well for our purposes
// (treating runes as one column each is an approximation but good enough for
// preview text).
func wordWrap(s string, width int) string {
	var out strings.Builder
	for li, line := range strings.Split(s, "\n") {
		if li > 0 {
			out.WriteByte('\n')
		}
		if line == "" {
			continue
		}
		// Don't wrap lines that look like fenced code or tool output: leave
		// them as-is, the user can scroll.
		if strings.HasPrefix(line, "```") || strings.HasPrefix(line, "    ") {
			out.WriteString(line)
			continue
		}
		col := 0
		for wi, w := range strings.Fields(line) {
			if wi > 0 {
				if col+1+runeLen(w) > width {
					out.WriteByte('\n')
					col = 0
				} else {
					out.WriteByte(' ')
					col++
				}
			}
			out.WriteString(w)
			col += runeLen(w)
		}
	}
	return out.String()
}

func runeLen(s string) int {
	n := 0
	for range s {
		n++
	}
	return n
}

// PreviewLine returns a single-line summary of a turn — used in compact
// listings. Currently unused at runtime; kept as a small public helper.
func PreviewLine(t Turn) string {
	body := strings.ReplaceAll(t.Text, "\n", " ")
	if len(body) > 120 {
		body = body[:120] + "…"
	}
	return fmt.Sprintf("[%s] %s", t.Role, body)
}
