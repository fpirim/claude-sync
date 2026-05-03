// Package syncthing wraps a slice of Syncthing's REST API. Only the endpoints
// claude-sync actually consumes are implemented — keeping the surface small
// makes it cheap to maintain across Syncthing version bumps.
package syncthing

// SystemStatus is the trimmed shape of GET /rest/system/status.
type SystemStatus struct {
	MyID         string `json:"myID"`
	Uptime       int    `json:"uptime"`
	Version      string `json:"version,omitempty"`
	GoVersion    string `json:"goVersion,omitempty"`
	GoOS         string `json:"goOS,omitempty"`
	StartTime    string `json:"startTime"`
	Connections  int    `json:"connectionServiceStatus,omitempty"`
}

// SystemVersion comes from GET /rest/system/version (the version field on
// /rest/system/status is empty in some releases, so we fall back to this).
type SystemVersion struct {
	Version string `json:"version"`
	Arch    string `json:"arch"`
	OS      string `json:"os"`
}

// FolderStatus is the trimmed shape of GET /rest/db/status?folder=ID.
type FolderStatus struct {
	GlobalBytes   int64  `json:"globalBytes"`
	GlobalFiles   int    `json:"globalFiles"`
	LocalBytes    int64  `json:"localBytes"`
	LocalFiles    int    `json:"localFiles"`
	NeedBytes     int64  `json:"needBytes"`
	NeedFiles     int    `json:"needFiles"`
	NeedDeletes   int    `json:"needDeletes,omitempty"`
	State         string `json:"state"` // idle | syncing | scanning | error
	StateChanged  string `json:"stateChanged,omitempty"`
	Errors        int    `json:"errors,omitempty"`
}

// Connections wraps GET /rest/system/connections. The "connections" map is
// keyed by device ID.
type Connections struct {
	Total       ConnectionStats            `json:"total"`
	Connections map[string]ConnectionStats `json:"connections"`
}

// ConnectionStats is what each device entry looks like.
type ConnectionStats struct {
	Connected     bool   `json:"connected"`
	Paused        bool   `json:"paused"`
	Address       string `json:"address,omitempty"`
	ClientVersion string `json:"clientVersion,omitempty"`
	Type          string `json:"type,omitempty"`
	InBytesTotal  int64  `json:"inBytesTotal,omitempty"`
	OutBytesTotal int64  `json:"outBytesTotal,omitempty"`
	StartedAt     string `json:"startedAt,omitempty"`
}

// Device is a peer entry from /rest/config/devices.
type Device struct {
	DeviceID string   `json:"deviceID"`
	Name     string   `json:"name"`
	Addresses []string `json:"addresses"`
}

// Folder is a folder entry from /rest/config/folders.
type Folder struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Path    string `json:"path"`
	Type    string `json:"type"`
	Devices []FolderDevice `json:"devices"`
}

type FolderDevice struct {
	DeviceID string `json:"deviceID"`
}

// Completion is GET /rest/db/completion?folder=ID&device=DEV — needed to
// build a per-peer % column.
type Completion struct {
	Completion  float64 `json:"completion"`
	NeedBytes   int64   `json:"needBytes"`
	NeedDeletes int     `json:"needDeletes,omitempty"`
	NeedItems   int     `json:"needItems,omitempty"`
	GlobalBytes int64   `json:"globalBytes"`
}
