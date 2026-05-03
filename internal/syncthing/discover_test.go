package syncthing

import "testing"

func TestMatchTag(t *testing.T) {
	xml := `<config><gui enabled="true" tls="false" address="127.0.0.1:8384"><apikey>SECRET-KEY</apikey></gui></config>`
	if got := matchTag(xml, "apikey"); got != "SECRET-KEY" {
		t.Errorf("apikey = %q", got)
	}
	if got := matchAttr(xml, "gui", "address"); got != "127.0.0.1:8384" {
		t.Errorf("address = %q", got)
	}
	if got := matchAttr(xml, "gui", "tls"); got != "false" {
		t.Errorf("tls = %q", got)
	}
}
