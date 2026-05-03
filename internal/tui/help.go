package tui

import "strings"

// helpFor returns the multi-line help body for the active tab plus the small
// set of global shortcuts. Rendered inside an info modal when the user
// presses '?'.
func helpFor(tab Tab, sessionsPreviewFocused bool) string {
	global := []string{
		"  1 / 2 / 3 / 4   switch tabs",
		"  tab             cycle tabs",
		"  r               refresh",
		"  q / ctrl+c      quit",
	}

	var tabKeys []string
	var title string
	switch tab {
	case TabProjects:
		title = "Projects"
		tabKeys = []string{
			"  j / k        navigate",
			"  /            filter",
			"  enter        open sessions for the selected project",
			"  a            add or update a project",
			"  r            repair (idempotent)",
			"  D            delete (this host / from config / stray)",
		}
	case TabSessions:
		title = "Sessions"
		if sessionsPreviewFocused {
			title += " — preview focus"
			tabKeys = []string{
				"  j / k          line up / down",
				"  pgup / pgdn    page",
				"  u / d          half page",
				"  g / G          top / bottom",
				"  esc / backspace  back to list",
			}
		} else {
			tabKeys = []string{
				"  j / k          navigate",
				"  pgup / pgdn    page up / down",
				"  shift+↑ / ↓    resize the divider",
				"  enter          focus the preview pane",
				"  c              continue session (claude --resume)",
				"  R              rename session",
				"  A              archive / unarchive",
				"  D              delete session",
				"  backspace      back to projects",
			}
		}
	case TabSync:
		title = "Sync"
		tabKeys = []string{
			"  s            trigger a manual rescan of the folder",
			"  (auto-polls every 2s while the tab is mounted)",
		}
	case TabConfig:
		title = "Config"
		tabKeys = []string{
			"  e            edit config.yml in $EDITOR",
			"  pgup / pgdn  scroll",
		}
	}

	body := []string{
		Styles.Key.Render(title),
		strings.Join(tabKeys, "\n"),
		"",
		Styles.Key.Render("Global"),
		strings.Join(global, "\n"),
	}
	return strings.Join(body, "\n")
}
