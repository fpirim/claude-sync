package sessions

import (
	"fmt"
	"os"
)

// Rename sets the friendly title in the sidecar.
func Rename(jsonlPath, title string) error {
	m, err := LoadMeta(jsonlPath)
	if err != nil {
		return err
	}
	m.Title = title
	return SaveMeta(jsonlPath, m)
}

// SetArchived flips the archived flag.
func SetArchived(jsonlPath string, archived bool) error {
	m, err := LoadMeta(jsonlPath)
	if err != nil {
		return err
	}
	m.Archived = archived
	return SaveMeta(jsonlPath, m)
}

// AddTag appends a tag if absent.
func AddTag(jsonlPath, tag string) error {
	m, err := LoadMeta(jsonlPath)
	if err != nil {
		return err
	}
	for _, t := range m.Tags {
		if t == tag {
			return nil
		}
	}
	m.Tags = append(m.Tags, tag)
	return SaveMeta(jsonlPath, m)
}

// Delete removes both the JSONL and its sidecar.
// Caller is responsible for confirmation.
func Delete(jsonlPath string) error {
	if err := os.Remove(jsonlPath); err != nil {
		return fmt.Errorf("remove %s: %w", jsonlPath, err)
	}
	if err := os.Remove(MetaPathFor(jsonlPath)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove sidecar: %w", err)
	}
	return nil
}
