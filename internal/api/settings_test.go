package api

import (
	"strings"
	"testing"
)

func TestSettingsAPI(t *testing.T) {
	h := newTestAPI(t, "")
	r := do(t, h, "GET", "/api/v1/settings", "")
	n, _ := r.json(t)["notifications"].(map[string]any)
	if r.status != 200 || n["ntfy_url"] != "" || n["telegram"].(map[string]any)["chat_id"] != "42" {
		t.Fatalf("get: %d %s", r.status, r.body)
	}
	if locked, _ := n["locked"].([]any); len(locked) != 1 || locked[0] != "telegram.chat_id" {
		t.Errorf("locked = %v", n["locked"])
	}

	body := `{"notifications":{"ntfy_url":"https://ntfy.sh/manga-x",
		"discord_webhook":"https://discord.com/api/webhooks/1/secret-abcd",
		"telegram":{"token":"","chat_id":""},"only_reading":true}}`
	r = do(t, h, "PUT", "/api/v1/settings", body)
	n, _ = r.json(t)["notifications"].(map[string]any)
	if r.status != 200 || n["discord_webhook"] != "••••abcd" || n["only_reading"] != true {
		t.Errorf("put: %d %s", r.status, r.body)
	}
	if strings.Contains(string(do(t, h, "GET", "/api/v1/settings", "").body), "secret") {
		t.Error("GET must not reveal the webhook")
	}

	r = do(t, h, "PUT", "/api/v1/settings", `{"notifications":{"ntfy_url":"not a url"}}`)
	if r.status != 400 || r.errorCode(t) != "invalid_input" {
		t.Errorf("invalid: %d %s", r.status, r.body)
	}
	if r := do(t, h, "PUT", "/api/v1/settings", `{"notifications":{"typo":1}}`); r.status != 400 {
		t.Errorf("unknown field: %d %s", r.status, r.body)
	}

	// No channel is set (the Discord webhook is a fake address): saving
	// one and testing it reports the failure per channel.
	if r := do(t, h, "PUT", "/api/v1/settings", `{"notifications":{}}`); r.status != 200 {
		t.Fatalf("clear: %d %s", r.status, r.body)
	}
	if r := do(t, h, "POST", "/api/v1/settings/notifications/test", ""); r.status != 400 || r.errorCode(t) != "invalid_input" {
		t.Errorf("test without channels: %d %s", r.status, r.body)
	}
	do(t, h, "PUT", "/api/v1/settings", `{"notifications":{"ntfy_url":"http://127.0.0.1:1/manga"}}`)
	r = do(t, h, "POST", "/api/v1/settings/notifications/test", "")
	results, _ := r.json(t)["results"].([]any)
	if r.status != 200 || len(results) != 1 || results[0].(map[string]any)["ok"] != false {
		t.Errorf("test: %d %s", r.status, r.body)
	}
}
