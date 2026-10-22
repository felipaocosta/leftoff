package api

import "testing"

func TestEnrichRoutes(t *testing.T) {
	h := newTestAPI(t, "")
	id := addSeries(t, h)
	r := do(t, h, "POST", "/api/v1/series/"+id+"/enrich", "")
	ext, _ := r.json(t)["external"].(map[string]any)
	if r.status != 200 || ext == nil || ext["id"] != "" || ext["checked_at"] == nil {
		t.Errorf("enrich: %d %s", r.status, r.body)
	}
	r = do(t, h, "DELETE", "/api/v1/series/"+id+"/external", "")
	ext, _ = r.json(t)["external"].(map[string]any)
	if r.status != 200 || ext["ignored"] != true {
		t.Errorf("unlink: %d %s", r.status, r.body)
	}
	if r := do(t, h, "POST", "/api/v1/series/nope/enrich", ""); r.status != 404 {
		t.Errorf("unknown: %d", r.status)
	}
}
