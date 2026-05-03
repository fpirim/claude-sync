// Package sessions reads Claude Code JSONL session files from
// ~/.claude/projects/_shared/<project>/ and exposes them with friendly metadata.
//
// Schema (empirically): each line is a JSON object with a "type" field. We
// care about the first object whose type=="user" — its message.content (string
// or array of {type,text}) is shown as the preview, and its timestamp serves
// as FirstAt. Counting "user" + "assistant" lines yields a msg count that
// roughly matches what a human thinks of as conversation turns.
package sessions

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Session is one conversation file.
type Session struct {
	Project   string    // logical project name
	UUID      string    // basename without .jsonl
	Path      string    // absolute path to the JSONL
	Title     string    // from .meta.json (empty if none)
	Tags      []string
	Archived  bool
	Preview   string    // first user message, single-line, trimmed
	MsgCount  int       // user+assistant lines
	FirstAt   time.Time // timestamp of first user message (zero if absent)
	LastAt    time.Time // mtime of file as fallback for "last activity"
	SizeBytes int64
	Cwd       string // cwd field from any line — useful for "from machine"
}

type firstScan struct {
	Preview  string
	MsgCount int
	FirstAt  time.Time
	Cwd      string
}

// ListProject returns sessions for one project under sharedRoot.
// sharedRoot = ~/.claude/projects/_shared. Returns empty slice (not error)
// if the project directory is missing.
func ListProject(sharedRoot, project string) ([]Session, error) {
	return ListDir(filepath.Join(sharedRoot, project), project)
}

// ListDir reads sessions from any directory containing JSONL files. Used for
// stray dirs (read-only view of un-adopted history) where the data lives at
// ~/.claude/projects/<encoded>/ rather than under _shared/.
func ListDir(dir, label string) ([]Session, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Session
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		// Skip Syncthing conflict copies and .conflict-* migration leftovers.
		if strings.Contains(e.Name(), ".sync-conflict-") || strings.Contains(e.Name(), ".conflict-") {
			continue
		}
		uuid := strings.TrimSuffix(e.Name(), ".jsonl")
		full := filepath.Join(dir, e.Name())
		info, err := e.Info()
		if err != nil {
			continue
		}
		s := Session{
			Project:   label,
			UUID:      uuid,
			Path:      full,
			LastAt:    info.ModTime(),
			SizeBytes: info.Size(),
		}
		if fs, err := scanJSONL(full); err == nil {
			s.Preview = fs.Preview
			s.MsgCount = fs.MsgCount
			s.FirstAt = fs.FirstAt
			s.Cwd = fs.Cwd
		}
		// Overlay sidecar metadata.
		if m, err := LoadMeta(full); err == nil {
			s.Title = m.Title
			s.Tags = m.Tags
			s.Archived = m.Archived
		}
		out = append(out, s)
	}
	// Most recent first.
	sort.Slice(out, func(i, j int) bool { return out[i].LastAt.After(out[j].LastAt) })
	return out, nil
}

// ListAll walks _shared/ and returns sessions across all projects.
func ListAll(sharedRoot string) ([]Session, error) {
	ents, err := os.ReadDir(sharedRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Session
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		ss, err := ListProject(sharedRoot, e.Name())
		if err != nil {
			return nil, err
		}
		out = append(out, ss...)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastAt.After(out[j].LastAt) })
	return out, nil
}

// scanJSONL streams the file once and extracts what we need.
func scanJSONL(path string) (firstScan, error) {
	var fs firstScan
	f, err := os.Open(path)
	if err != nil {
		return fs, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	// JSONL lines can be very long (large skill listings, etc).
	sc.Buffer(make([]byte, 64*1024), 8*1024*1024)

	gotFirstUser := false
	for sc.Scan() {
		line := sc.Bytes()
		// Cheap probe: extract type without full parse first.
		typ := jsonField(line, "type")
		switch typ {
		case "user", "assistant":
			fs.MsgCount++
		}
		if !gotFirstUser && typ == "user" {
			var rec userRec
			if err := json.Unmarshal(line, &rec); err == nil {
				fs.Preview = extractPreview(rec.Message.Content)
				if rec.Timestamp != "" {
					if t, err := time.Parse(time.RFC3339Nano, rec.Timestamp); err == nil {
						fs.FirstAt = t
					}
				}
				if rec.Cwd != "" && fs.Cwd == "" {
					fs.Cwd = rec.Cwd
				}
				gotFirstUser = true
			}
		}
		if fs.Cwd == "" {
			if c := jsonField(line, "cwd"); c != "" {
				fs.Cwd = c
			}
		}
	}
	return fs, sc.Err()
}

type userRec struct {
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"`
	Cwd       string          `json:"cwd"`
	Message   struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

// extractPreview is extractText clipped to ~200 chars for list rendering.
func extractPreview(raw json.RawMessage) string {
	return clip(extractText(raw))
}

// extractText returns the raw text body from a message.content payload, which
// may be either a plain string or an array of {type:"text",text:"..."} parts.
// Non-text parts (tool_use, thinking) are dropped.
func extractText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var arr []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &arr); err == nil {
		var parts []string
		for _, p := range arr {
			if p.Type == "text" && p.Text != "" {
				parts = append(parts, p.Text)
			}
		}
		return strings.Join(parts, "\n\n")
	}
	return ""
}

func clip(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	return s
}

// jsonField is a tiny non-allocating extractor for top-level string fields.
// Returns "" if the field is missing or not a simple string.
func jsonField(line []byte, key string) string {
	needle := []byte(`"` + key + `":"`)
	i := indexOf(line, needle)
	if i < 0 {
		return ""
	}
	start := i + len(needle)
	end := start
	for end < len(line) && line[end] != '"' {
		if line[end] == '\\' && end+1 < len(line) {
			end += 2
			continue
		}
		end++
	}
	if end >= len(line) {
		return ""
	}
	return string(line[start:end])
}

func indexOf(haystack, needle []byte) int {
	if len(needle) == 0 || len(haystack) < len(needle) {
		return -1
	}
outer:
	for i := 0; i <= len(haystack)-len(needle); i++ {
		for j := 0; j < len(needle); j++ {
			if haystack[i+j] != needle[j] {
				continue outer
			}
		}
		return i
	}
	return -1
}
