package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestExternal_KeepsUnknownFieldsAndFound(t *testing.T) {
	in := `{"source":"anilist","id":"1","url":"u","title":"T","status":"ongoing","genres":["Action"],"year":2020,"synopsis":"","ignored":false,"checked_at":"2026-10-20T00:00:00Z","score":87}`
	var e External
	if err := json.Unmarshal([]byte(in), &e); err != nil {
		t.Fatal(err)
	}
	out, err := json.Marshal(e)
	if err != nil || !strings.Contains(string(out), `"score":87`) || *e.Year != 2020 {
		t.Errorf("round trip = %s, %v", out, err)
	}
	var none *External
	for _, tt := range []struct {
		e    *External
		want bool
	}{{&e, true}, {none, false}, {&External{}, false}, {&External{ID: "1", Ignored: true}, false}} {
		if got := tt.e.Found(); got != tt.want {
			t.Errorf("Found(%+v) = %v", tt.e, got)
		}
	}
}
