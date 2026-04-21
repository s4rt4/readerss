package service

import (
	"strings"
	"testing"
)

func TestReadableArticleTextPrefersMainArticleContent(t *testing.T) {
	html := `<!doctype html>
<html>
	<body>
		<header>
			<a>Skip to main content</a>
			<button>Open menu</button>
			<a>Sign in</a>
			<a>View Profile</a>
			<form>Search</form>
		</header>
		<main>
			<article>
				<h1>How to draw comic panels</h1>
				<p>This practical guide explains panel rhythm, framing, and visual flow for artists.</p>
				<p>It keeps the useful tutorial copy while removing site chrome from the reader view.</p>
			</article>
		</main>
		<footer>Subscribe</footer>
	</body>
</html>`

	text := ReadableArticleText(html)
	for _, unwanted := range []string{"Skip to main content", "Open menu", "Sign in", "View Profile", "Search", "Subscribe"} {
		if strings.Contains(text, unwanted) {
			t.Fatalf("ReadableArticleText leaked boilerplate %q in %q", unwanted, text)
		}
	}
	for _, wanted := range []string{"How to draw comic panels", "panel rhythm", "site chrome"} {
		if !strings.Contains(text, wanted) {
			t.Fatalf("ReadableArticleText missing article text %q in %q", wanted, text)
		}
	}
}
