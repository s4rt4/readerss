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

// TestReadableArticleTextStripsFutureChrome reproduces the Creative Bloq / Future
// plc layout: a <body> whose class contains "navigation" (which must not cause
// the whole page to be dropped), a mega-menu, and an #article-body that embeds a
// share/utility bar and an affiliate ("hawk") promo around the real copy.
func TestReadableArticleTextStripsFutureChrome(t *testing.T) {
	html := `<!doctype html>
<html>
	<body class="news-page sticky-navigation has-kiosq">
		<nav><a>Art &amp; Design</a><a>Digital Crafting</a><a>Daily inspiration for creative people</a></nav>
		<section class="content-wrapper">
			<div id="article-body" class="text-copy bodyCopy">
				<div id="utility-bar" data-component-name="UtilityBar">
					<a>Facebook</a><a>Whatsapp</a><a>Flipboard</a><span>Follow us</span>
				</div>
				<p>The pro shares how panel rhythm and framing drive the reader's eye across the page.</p>
				<div class="hawk-promotion-item">Subscribe to ImagineFX and save today.</div>
				<p>Final inking adjustments give the artwork a more organic and lively feel.</p>
			</div>
		</section>
	</body>
</html>`

	text := ReadableArticleText(html)
	for _, unwanted := range []string{
		"Daily inspiration for creative people", "Digital Crafting", "Art & Design",
		"Facebook", "Whatsapp", "Flipboard", "Follow us", "Subscribe to ImagineFX",
	} {
		if strings.Contains(text, unwanted) {
			t.Fatalf("ReadableArticleText leaked chrome %q in %q", unwanted, text)
		}
	}
	for _, wanted := range []string{"panel rhythm and framing", "organic and lively feel"} {
		if !strings.Contains(text, wanted) {
			t.Fatalf("ReadableArticleText missing article text %q in %q", wanted, text)
		}
	}
}
