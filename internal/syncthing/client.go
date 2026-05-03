package syncthing

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client is a thin HTTP wrapper around the local Syncthing REST API. All
// methods set the X-API-Key header and time out aggressively so a stalled
// daemon doesn't freeze the UI.
type Client struct {
	BaseURL string // e.g. http://127.0.0.1:8384
	APIKey  string
	HTTP    *http.Client
}

// New constructs a Client with sensible defaults.
func New(baseURL, apiKey string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		APIKey:  apiKey,
		HTTP:    &http.Client{Timeout: 5 * time.Second},
	}
}

func (c *Client) get(path string, q url.Values, out any) error {
	u := c.BaseURL + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-API-Key", c.APIKey)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("syncthing %s: %d %s", path, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// SystemStatus → /rest/system/status
func (c *Client) SystemStatus() (SystemStatus, error) {
	var s SystemStatus
	err := c.get("/rest/system/status", nil, &s)
	return s, err
}

// SystemVersion → /rest/system/version
func (c *Client) SystemVersion() (SystemVersion, error) {
	var v SystemVersion
	err := c.get("/rest/system/version", nil, &v)
	return v, err
}

// FolderStatus → /rest/db/status?folder=ID
func (c *Client) FolderStatus(folderID string) (FolderStatus, error) {
	var s FolderStatus
	q := url.Values{"folder": []string{folderID}}
	err := c.get("/rest/db/status", q, &s)
	return s, err
}

// Connections → /rest/system/connections
func (c *Client) Connections() (Connections, error) {
	var c2 Connections
	err := c.get("/rest/system/connections", nil, &c2)
	return c2, err
}

// Devices returns all configured peers (the local device is included).
func (c *Client) Devices() ([]Device, error) {
	var devs []Device
	err := c.get("/rest/config/devices", nil, &devs)
	return devs, err
}

// Folder → /rest/config/folders/ID
func (c *Client) Folder(id string) (Folder, error) {
	var f Folder
	err := c.get("/rest/config/folders/"+url.PathEscape(id), nil, &f)
	return f, err
}

// Completion → per-folder/per-device sync %.
func (c *Client) Completion(folderID, deviceID string) (Completion, error) {
	var cm Completion
	q := url.Values{"folder": []string{folderID}, "device": []string{deviceID}}
	err := c.get("/rest/db/completion", q, &cm)
	return cm, err
}

// Scan triggers a manual rescan of the given folder. POST with no body.
func (c *Client) Scan(folderID string) error {
	u := c.BaseURL + "/rest/db/scan?" + url.Values{"folder": []string{folderID}}.Encode()
	req, err := http.NewRequest("POST", u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-API-Key", c.APIKey)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("scan: %d %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

// Ping hits /rest/system/ping — returns nil if the daemon is reachable.
func (c *Client) Ping() error { return c.get("/rest/system/ping", nil, nil) }
