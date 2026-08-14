package extract

import (
	"html"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

var whitespaceRun = regexp.MustCompile(`\s+`)

func collapseWhitespace(s string) string {
	return strings.TrimSpace(whitespaceRun.ReplaceAllString(s, " "))
}

// htmlToLines is Stage 1 (Normalize) from §6: strip nav/footer/script/style,
// and flatten the rest to lines that preserve heading and list structure —
// "## heading text" and "- item text" — since Stage 2 (Segment) depends on
// those markers to find section boundaries.
//
// It selects h1-h6, li, and p specifically rather than every element,
// because selecting a generic container (div, section) alongside its own
// descendants would double-count text via goquery's .Text(), which
// concatenates all descendant text nodes. This matches how ATS rich-text
// editors (Greenhouse, Lever) actually emit content — paragraphs in <p>,
// list items in <li> — so it's a pragmatic Phase 1 choice, not a general
// HTML-to-text solution. Content sitting directly in a <div> with no <p>
// wrapper would be missed; none of the 5 seed companies' postings do this,
// but it's a known limitation.
//
// Greenhouse's `content` field (confirmed against a real Riot Games
// posting) comes back HTML-entity-escaped — the string contains literal
// "&lt;p&gt;" rather than "<p>" — so an HTML parser sees one giant text
// node with zero real tags and htmlToLines silently returned nothing.
// html.UnescapeString decodes that one layer before parsing. Running it
// unconditionally is safe for already-plain HTML (Lever's list content):
// there's nothing double-escaped to undo, so it's a no-op there.
func htmlToLines(rawHTML string) ([]string, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html.UnescapeString(rawHTML)))
	if err != nil {
		return nil, err
	}

	doc.Find("script, style, nav, footer").Remove()

	var lines []string
	doc.Find("h1, h2, h3, h4, h5, h6, li, p").Each(func(_ int, sel *goquery.Selection) {
		text := collapseWhitespace(sel.Text())
		if text == "" {
			return
		}
		switch goquery.NodeName(sel) {
		case "h1", "h2", "h3", "h4", "h5", "h6":
			lines = append(lines, "## "+text)
		case "li":
			lines = append(lines, "- "+text)
		default: // p
			if headingText, ok := pseudoHeadingText(sel, text); ok {
				lines = append(lines, "## "+headingText)
			} else {
				lines = append(lines, text)
			}
		}
	})
	return lines, nil
}

const maxPseudoHeadingLen = 80

// pseudoHeadingText recognizes a <p> whose entire content is a single
// <strong>/<b> child — e.g. `<p><strong>You Will</strong></p>` — as a
// heading line, the same way a real <h1>-<h6> is. Confirmed against a real
// Roblox posting: Roblox uses exactly this pattern instead of semantic
// heading tags, so without this the whole body collapsed into one
// unclassified block and Stage 2 never saw a section boundary at all.
//
// Guarded two ways against false positives: the bold child must be the
// paragraph's ONLY content (no surrounding text — a paragraph that merely
// starts with a bolded word keeps going as a normal paragraph), and the
// text must be short (<=80 chars) so a fully-bolded multi-sentence
// paragraph doesn't get mistaken for a heading.
func pseudoHeadingText(sel *goquery.Selection, fullText string) (string, bool) {
	children := sel.Children()
	if children.Length() != 1 {
		return "", false
	}
	child := children.First()
	tag := goquery.NodeName(child)
	if tag != "strong" && tag != "b" {
		return "", false
	}
	childText := collapseWhitespace(child.Text())
	if childText == "" || childText != fullText || len(childText) > maxPseudoHeadingLen {
		return "", false
	}
	return childText, true
}
