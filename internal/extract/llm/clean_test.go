package llm

import (
	"fmt"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func parse(t *testing.T, s string) *goquery.Document {
	t.Helper()
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(s))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestCleanHTML_KeepsDataAndDropsNoise(t *testing.T) {
	page := `<html><head><title>Solo</title><script>var x</script><style>.a{}</style>
<meta property="og:image" content="https://s.org/c.jpg"><meta name="viewport" content="w">
<link rel="stylesheet" href="a.css"></head>
<body><nav><a href="/">Home</a></nav><!-- ad -->
<h1 class="entry-title md:text-xl w-[140px]" style="color:red" onclick="x()">Solo Leveling</h1>
<img class="cover" src="data:image/gif;base64,AAA" data-src="https://s.org/c.jpg">
<div class="empty"></div><svg><path d="M0"/></svg>
<p>` + strings.Repeat("long ", 100) + `</p><footer>©</footer></body></html>`
	doc := parse(t, page)
	got, err := CleanHTML(doc, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`<title>Solo</title>`, `<meta property="og:image" content="https://s.org/c.jpg"/>`,
		`<h1 class="entry-title">Solo Leveling</h1>`, `<img class="cover" data-src="https://s.org/c.jpg"/>`, "…",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	for _, unwanted := range []string{"script", "var x", "style", "viewport", "stylesheet", "Home", "<!-- ad", "empty", "path", "©", "onclick", "data:image"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("kept %q in\n%s", unwanted, got)
		}
	}
	if doc.Find("script").Length() != 1 {
		t.Error("CleanHTML changed the original page")
	}
}

func TestCleanHTML_CollapsesLongRuns(t *testing.T) {
	var items strings.Builder
	for i := 1; i <= 50; i++ {
		fmt.Fprintf(&items, `<li class="ch"><a href="/c/%d">Chapter %d</a></li>`, i, i)
	}
	got, err := CleanHTML(parse(t, `<ul>`+items.String()+`</ul><ul><li>a</li><li>b</li></ul>`), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(got, `<li class="ch">`); n != keepRepeated {
		t.Errorf("kept %d chapter items, want %d", n, keepRepeated)
	}
	if !strings.Contains(got, "<!-- 46 more similar <li> elements -->") {
		t.Errorf("no note about the removed items:\n%s", got)
	}
	if !strings.Contains(got, "<li>a</li><li>b</li>") {
		t.Errorf("short runs must stay whole:\n%s", got)
	}
}

func TestCleanHTML_Truncates(t *testing.T) {
	got, err := CleanHTML(parse(t, `<p>`+strings.Repeat("é", 100)+`</p>`), 40)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(got, "<!-- truncated -->") || len(got) > 40+len("\n<!-- truncated -->") {
		t.Errorf("got %q", got)
	}
}

func TestSimpleClasses(t *testing.T) {
	tests := []struct{ in, want string }{
		{"a b c d e", "a b c d"},
		{"md:flex w-[1px] bg-x/50 real", "real"},
		{"  ", ""},
	}
	for _, tt := range tests {
		if got := simpleClasses(tt.in); got != tt.want {
			t.Errorf("simpleClasses(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
