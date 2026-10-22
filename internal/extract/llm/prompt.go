package llm

import (
	"encoding/json"
	"fmt"
	"strings"
)

// systemPrompt explains the recipe format (SPEC §5.3) to the model.
const systemPrompt = `You write extraction recipes for a manga tracker. A recipe tells a program,
with CSS selectors, how to read one site's series page: the title, cover,
status, synopsis, alternative titles and the list of chapters.

Each field is {"css": selector, "attr": attribute, "split": separator}:
- "css" selects the FIRST matching element; "" means the field is absent.
- "attr" reads that attribute (e.g. "src", "data-src", "href", "content");
  "" reads the element's text.
- "split" cuts the text into a list (only for alt_titles, e.g. ","); else "".

Chapters:
- "item" selects EVERY chapter row of the list (e.g. "ul.chapters li").
- "link", "label" and "date" are selected INSIDE each item; link "css": ""
  means the item itself is the <a>. link "attr" is normally "href".
- "number_regex" is a Go regular expression with ONE capture group for the
  chapter number, applied to the label and then to the link URL, e.g.
  "(?i)chapter\\s*(\\d+(?:\\.\\d+)?)". Use "" when the label already says
  "Chapter 12" or similar.
- "list_url" is "" when the chapters are on this page; otherwise the
  relative URL of the page that lists them (e.g. "all-chapters/").

Rules:
- Use only selectors that match the HTML you are given. Prefer stable
  classes and ids; avoid positions like :nth-child and generated class
  names such as "css-1x2y3z".
- Selectors may use :contains("text") to find a labelled value, e.g.
  ".info:contains(\"Status\") span".
- Long runs of similar elements were shortened to a few examples followed by
  a comment such as "<!-- 120 more similar <li> elements -->"; the real page
  has all of them.
- Answer with the JSON object only.`

// userPrompt carries the page to the model.
func userPrompt(pageURL, cleaned string) string {
	return fmt.Sprintf("Series page: %s\n\nHTML (cleaned):\n%s", pageURL, cleaned)
}

// retryPrompt reports the problems of the previous answer.
func retryPrompt(problems []string) string {
	return "That recipe did not work on the page:\n- " + strings.Join(problems, "\n- ") +
		"\n\nLook at the HTML again and answer with the whole corrected recipe."
}

// fieldSchema is the JSON Schema of a recipe field.
const fieldSchema = `{"type":"object","additionalProperties":false,"required":["css","attr","split"],
"properties":{"css":{"type":"string"},"attr":{"type":"string"},"split":{"type":"string"}}}`

// draftSchema constrains the model's answer: the parts of a recipe it
// writes. The rest (schema, domain, generated_by, updated_at) is filled in
// by the generator.
var draftSchema = json.RawMessage(strings.NewReplacer("FIELD", fieldSchema).Replace(`{
"type":"object","additionalProperties":false,"required":["series","chapters"],
"properties":{
 "series":{"type":"object","additionalProperties":false,
  "required":["title","cover","status","synopsis","alt_titles"],
  "properties":{"title":FIELD,"cover":FIELD,"status":FIELD,"synopsis":FIELD,"alt_titles":FIELD}},
 "chapters":{"type":"object","additionalProperties":false,
  "required":["list_url","item","link","label","number_regex","date"],
  "properties":{"list_url":{"type":"string"},"item":FIELD,"link":FIELD,"label":FIELD,
   "number_regex":{"type":"string"},"date":FIELD}}}}`))
