package sessions

import (
	"encoding/json"
	"os"
	"strings"
)

// Meta is the sidecar file <session>.meta.json.
type Meta struct {
	Title    string   `json:"title,omitempty"`
	Tags     []string `json:"tags,omitempty"`
	Archived bool     `json:"archived,omitempty"`
	Note     string   `json:"note,omitempty"`
}

// MetaPathFor returns the sidecar path for a JSONL file.
func MetaPathFor(jsonlPath string) string {
	return strings.TrimSuffix(jsonlPath, ".jsonl") + ".meta.json"
}

// LoadMeta returns the sidecar metadata, or zero value if absent.
func LoadMeta(jsonlPath string) (Meta, error) {
	p := MetaPathFor(jsonlPath)
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return Meta{}, nil
		}
		return Meta{}, err
	}
	var m Meta
	if err := json.Unmarshal(b, &m); err != nil {
		return Meta{}, err
	}
	return m, nil
}

// IsZero reports whether the meta has no information worth persisting.
func (m Meta) IsZero() bool {
	return m.Title == "" && len(m.Tags) == 0 && !m.Archived && m.Note == ""
}

// SaveMeta writes the sidecar (atomic via temp + rename). Empty meta deletes it.
func SaveMeta(jsonlPath string, m Meta) error {
	p := MetaPathFor(jsonlPath)
	if m.IsZero() {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}
