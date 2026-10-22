package library

import (
	"context"
	"encoding/json"
	"testing"
)

// FuzzImport: any file is either refused or merged; importing the same
// file twice adds nothing the second time, and no chapter read before the
// import ends up unread.
func FuzzImport(f *testing.F) {
	f.Add([]byte(`{"format":"leftoff-export","schema":2,"series":[{"id":"01A","title":"T","source":{"url":"https://site.org/manga/x"},"chapters":[{"key":"1","number":1,"read":true,"read_at":"2026-10-01T00:00:00Z"}]}]}`))
	f.Add([]byte(`{"schema":1,"series":[{"title":"Y","source":{"url":"https://other.org/y"}}]}`))
	f.Add([]byte(`{"schema":2,"series":[{"title":"Z","source":{"url":"https://site.org/manga/x"},"chapters":[{"key":"2","read":true},{"key":"2","read":false}]}]}`))
	f.Add([]byte(`{"schema":3,"series":[]}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		e := newEnv(t)
		s := e.add(t)
		ctx := context.Background()
		if _, err := e.svc.MarkReadUpTo(ctx, s.ID, 1); err != nil {
			t.Fatal(err)
		}
		if _, err := e.svc.Import(ctx, data); err != nil {
			return
		}
		got, err := e.svc.Get(s.ID)
		if err != nil {
			t.Fatalf("series gone after import: %v", err)
		}
		for _, c := range got.Chapters {
			if c.Key == "1" && !c.Read {
				t.Fatal("import unread a chapter")
			}
		}
		before, _ := json.Marshal(e.svc.Export().Series)
		again, err := e.svc.Import(ctx, data)
		if err != nil {
			t.Fatalf("second import of an accepted file: %v", err)
		}
		after, _ := json.Marshal(e.svc.Export().Series)
		if again.Added != 0 || again.ChaptersRead != 0 || string(before) != string(after) {
			t.Fatalf("second import changed the library: %+v\n%s\n%s", again, before, after)
		}
	})
}
