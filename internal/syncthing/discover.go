package syncthing

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// Discover finds the local Syncthing endpoint + API key without bothering
// the user. It walks the standard config locations for the platform, reads
// config.xml, and pulls the <apikey> and <gui address="..."> values.
func Discover() (baseURL, apiKey string, err error) {
	for _, dir := range configDirs() {
		x := filepath.Join(dir, "config.xml")
		b, e := os.ReadFile(x)
		if e != nil {
			continue
		}
		text := string(b)
		apiKey = matchTag(text, "apikey")
		addr := matchAttr(text, "gui", "address")
		if apiKey == "" {
			continue
		}
		if addr == "" {
			addr = "127.0.0.1:8384"
		}
		// gui.address is "host:port"; we need a URL.
		if !strings.HasPrefix(addr, "http") {
			scheme := "http"
			// If the gui has tls="true", switch to https. Cheap probe: look
			// for tls="true" inside the gui open tag.
			gui := matchTagBlock(text, "gui")
			if strings.Contains(gui, `tls="true"`) {
				scheme = "https"
			}
			addr = scheme + "://" + addr
		}
		baseURL = addr
		return
	}
	err = errors.New("no Syncthing config.xml found in standard locations")
	return
}

// configDirs lists the locations Syncthing might keep its config.xml on the
// current platform, in priority order.
func configDirs() []string {
	home, _ := os.UserHomeDir()
	if home == "" {
		return nil
	}
	switch runtime.GOOS {
	case "darwin":
		return []string{
			filepath.Join(home, "Library/Application Support/Syncthing"),
			filepath.Join(home, ".config/syncthing"),
			filepath.Join(home, ".local/state/syncthing"),
		}
	default: // linux + others
		return []string{
			filepath.Join(home, ".local/state/syncthing"),
			filepath.Join(home, ".config/syncthing"),
		}
	}
}

var (
	tagRE   = regexp.MustCompile(`<([a-zA-Z]+)>([^<]*)</[a-zA-Z]+>`)
	openRE  = regexp.MustCompile(`<([a-zA-Z]+)([^>]*)>`)
)

// matchTag returns the inner text of <tag>...</tag> for a single occurrence.
func matchTag(s, tag string) string {
	for _, m := range tagRE.FindAllStringSubmatch(s, -1) {
		if m[1] == tag {
			return strings.TrimSpace(m[2])
		}
	}
	return ""
}

// matchAttr returns the value of attr on the first <tag ...> open element.
func matchAttr(s, tag, attr string) string {
	for _, m := range openRE.FindAllStringSubmatch(s, -1) {
		if m[1] != tag {
			continue
		}
		// crude: find attr="..." inside m[2]
		needle := attr + `="`
		i := strings.Index(m[2], needle)
		if i < 0 {
			continue
		}
		rest := m[2][i+len(needle):]
		j := strings.Index(rest, `"`)
		if j < 0 {
			continue
		}
		return rest[:j]
	}
	return ""
}

// matchTagBlock returns the substring of s between <tag ...> and </tag>.
// Used for cheap attribute scans on multi-line elements.
func matchTagBlock(s, tag string) string {
	open := "<" + tag
	close := "</" + tag + ">"
	i := strings.Index(s, open)
	if i < 0 {
		return ""
	}
	j := strings.Index(s[i:], close)
	if j < 0 {
		return ""
	}
	return s[i : i+j]
}
