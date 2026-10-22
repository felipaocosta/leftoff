package recipe

import (
	"context"
	"testing"

	"github.com/felipaocosta/leftoff/internal/testfixture"

	"github.com/felipaocosta/leftoff/internal/fetch"
)

// FuzzRecipe: a recipe file is refused or, once valid, runs on a real page
// without panicking (user recipes are edited by hand and written by the LLM).
func FuzzRecipe(f *testing.F) {
	for _, name := range []string{"ravenscans.org.json", "mgeko.cc.json"} {
		data, err := testfixture.ReadFile(f, "../../../testdata/recipes/"+name)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(data)
	}
	f.Add([]byte(`{"schema":1,"domain":"ravenscans.org","series":{"title":{"css":"h1"},"alt_titles":{"css":"span","split":""}},"chapters":{"item":{"css":"li"},"list_url":"../x","number_regex":"(\\d*)","date":{"css":"*","attr":"x"}}}`))
	html, err := testfixture.ReadFile(f, "../../../testdata/fixtures/ravenscans.org/the-extras-academy-survival-guide.html")
	if err != nil {
		f.Fatal(err)
	}
	const u = "https://ravenscans.org/manga/the-extras-academy-survival-guide/"
	f.Fuzz(func(t *testing.T, data []byte) {
		r, err := Parse(data)
		if err != nil {
			return
		}
		p := page(t, u, html)
		// Other pages (list_url) are not found: an error, never a request.
		_, _, _ = Test(context.Background(), r, &fetch.Static{}, p, captured) // only panics matter here
	})
}
