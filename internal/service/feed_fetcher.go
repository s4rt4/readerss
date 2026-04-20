package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/mmcdole/gofeed"

	"readress/internal/db/sqlc"
)

type FeedFetcher struct {
	queries *sqlc.Queries
	client  *http.Client
}

type FetchResult struct {
	FeedID       int64
	Inserted     int
	Skipped      int
	FeedTitle    string
	ErrorMessage string
}

var (
	scriptStyleRE = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>|<style[^>]*>.*?</style>|<iframe[^>]*>.*?</iframe>|<svg[^>]*>.*?</svg>|<noscript[^>]*>.*?</noscript>|<clipboard-copy[^>]*>.*?</clipboard-copy>`)
	doctypeRE     = regexp.MustCompile(`(?is)<!doctype[^>]*>`)
	commentRE     = regexp.MustCompile(`(?is)<!--.*?-->`)
	bodyOpenRE    = regexp.MustCompile(`(?is)</?(html|body)[^>]*>`)
	blockTagRE    = regexp.MustCompile(`(?is)</?(p|div|br|h[1-6]|li|ul|ol|blockquote|pre|figure|figcaption|section|article|table|tr)[^>]*>`)
	anyTagRE      = regexp.MustCompile(`(?is)<[^>]+>`)
	spaceRE       = regexp.MustCompile(`[ \t\r\f\v]+`)
	newlineRE     = regexp.MustCompile(`\n{3,}`)
	feedLinkRE    = regexp.MustCompile(`(?is)<link[^>]+(?:type=["']application/(?:rss|atom)\+xml["'][^>]+href=["']([^"']+)["']|href=["']([^"']+)["'][^>]+type=["']application/(?:rss|atom)\+xml["'])`)
	imgSrcRE      = regexp.MustCompile(`(?is)<img[^>]+src=["']([^"']+)["']`)
	ogImageRE     = regexp.MustCompile(`(?is)<meta[^>]+(?:property|name)=["'](?:og:image|twitter:image)["'][^>]+content=["']([^"']+)["']|<meta[^>]+content=["']([^"']+)["'][^>]+(?:property|name)=["'](?:og:image|twitter:image)["']`)
)

func NewFeedFetcher(queries *sqlc.Queries) *FeedFetcher {
	return &FeedFetcher{
		queries: queries,
		client: &http.Client{
			Timeout: 20 * time.Second,
		},
	}
}

func (f *FeedFetcher) DiscoverFeedURL(ctx context.Context, inputURL string) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, inputURL, nil)
	if err != nil {
		return inputURL
	}
	req.Header.Set("User-Agent", "ReadeRSS/0.1")
	resp, err := f.client.Do(req)
	if err != nil {
		return inputURL
	}
	defer resp.Body.Close()
	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	if strings.Contains(contentType, "xml") || strings.Contains(contentType, "rss") || strings.Contains(contentType, "atom") {
		return inputURL
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return inputURL
	}
	base, _ := url.Parse(inputURL)
	for _, match := range feedLinkRE.FindAllStringSubmatch(string(body), -1) {
		for _, candidate := range match[1:] {
			candidate = strings.TrimSpace(html.UnescapeString(candidate))
			if candidate == "" {
				continue
			}
			parsed, err := url.Parse(candidate)
			if err != nil {
				continue
			}
			return base.ResolveReference(parsed).String()
		}
	}
	return inputURL
}

func (f *FeedFetcher) FetchFeed(ctx context.Context, userID, feedID int64) (FetchResult, error) {
	feed, err := f.queries.GetFeed(ctx, sqlc.GetFeedParams{ID: feedID, UserID: userID})
	if err != nil {
		return FetchResult{}, err
	}

	result := FetchResult{FeedID: feed.ID, FeedTitle: feed.Title}
	parser := gofeed.NewParser()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feed.Url, nil)
	if err != nil {
		return result, err
	}
	req.Header.Set("User-Agent", "ReadeRSS/0.1 (+https://github.com/s4rt4/readerss)")
	if feed.Etag.Valid && feed.Etag.String != "" {
		req.Header.Set("If-None-Match", feed.Etag.String)
	}
	if feed.LastModified.Valid && feed.LastModified.String != "" {
		req.Header.Set("If-Modified-Since", feed.LastModified.String)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		msg := trimForDB(err.Error(), 240)
		_ = f.queries.UpdateFeedFetchError(ctx, sqlc.UpdateFeedFetchErrorParams{
			LastError: sql.NullString{String: msg, Valid: true},
			ID:        feed.ID,
			UserID:    userID,
		})
		result.ErrorMessage = msg
		return result, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotModified {
		if err := f.queries.UpdateFeedFetchedAt(ctx, sqlc.UpdateFeedFetchedAtParams{ID: feed.ID, UserID: userID}); err != nil {
			return result, err
		}
		return result, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := trimForDB(resp.Status, 240)
		_ = f.queries.UpdateFeedFetchError(ctx, sqlc.UpdateFeedFetchErrorParams{
			LastError: sql.NullString{String: msg, Valid: true},
			ID:        feed.ID,
			UserID:    userID,
		})
		result.ErrorMessage = msg
		return result, fmt.Errorf("fetch feed: %s", resp.Status)
	}

	parsed, err := parser.Parse(resp.Body)
	if err != nil {
		msg := trimForDB(err.Error(), 240)
		_ = f.queries.UpdateFeedFetchError(ctx, sqlc.UpdateFeedFetchErrorParams{
			LastError: sql.NullString{String: msg, Valid: true},
			ID:        feed.ID,
			UserID:    userID,
		})
		result.ErrorMessage = msg
		return result, err
	}

	title := firstNonEmpty(parsed.Title, feed.Title)
	siteURL := firstNonEmpty(parsed.Link, feed.SiteUrl.String)
	description := firstNonEmpty(parsed.Description, feed.Description.String)

	etag := firstNonEmpty(resp.Header.Get("ETag"), feed.Etag.String)
	lastModified := firstNonEmpty(resp.Header.Get("Last-Modified"), feed.LastModified.String)
	if err := f.queries.UpdateFeedFetchSuccess(ctx, sqlc.UpdateFeedFetchSuccessParams{
		Title:        title,
		SiteUrl:      nullString(siteURL),
		Description:  nullString(description),
		Etag:         nullString(etag),
		LastModified: nullString(lastModified),
		ID:           feed.ID,
		UserID:       userID,
	}); err != nil {
		return result, err
	}

	for _, item := range parsed.Items {
		article, ok := articleFromItem(feed.ID, item)
		if !ok {
			result.Skipped++
			continue
		}
		if len([]rune(article.Content.String)) < 220 || !article.ImageUrl.Valid {
			full, imageURL := f.fetchArticleExtras(ctx, article.Url)
			if full != "" && len([]rune(article.Content.String)) < 220 {
				article.Content = nullString(full)
				article.Excerpt = nullString(trimForDB(oneLine(full), 280))
			}
			if !article.ImageUrl.Valid && imageURL != "" {
				article.ImageUrl = nullString(imageURL)
			}
		}
		created, err := f.queries.CreateArticle(ctx, article)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				result.Skipped++
				continue
			}
			return result, fmt.Errorf("save article %q: %w", article.Title, err)
		}
		if err := f.applyFilterRules(ctx, userID, created.ID, created.FeedID, created.Url, created.Title, created.Content); err != nil {
			return result, err
		}
		result.Inserted++
	}

	result.FeedTitle = title
	return result, nil
}

func (f *FeedFetcher) fetchArticleExtras(ctx context.Context, articleURL string) (string, string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, articleURL, nil)
	if err != nil {
		return "", ""
	}
	req.Header.Set("User-Agent", "ReadeRSS/0.1")
	resp, err := f.client.Do(req)
	if err != nil {
		return "", ""
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", ""
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", ""
	}
	htmlValue := string(body)
	return ReadableText(htmlValue), absoluteURL(articleURL, firstNonEmpty(extractMetaImage(htmlValue), extractFirstImage(htmlValue)))
}

func (f *FeedFetcher) applyFilterRules(ctx context.Context, userID, articleID, feedID int64, articleURL, titleValue string, contentValue sql.NullString) error {
	rules, err := f.queries.ListFilterRules(ctx, userID)
	if err != nil {
		return err
	}
	title := strings.ToLower(titleValue)
	urlValue := strings.ToLower(articleURL)
	content := strings.ToLower(contentValue.String)
	for _, rule := range rules {
		if rule.FeedID.Valid && rule.FeedID.Int64 != feedID {
			continue
		}
		pattern := strings.ToLower(strings.TrimSpace(rule.Pattern))
		matched := false
		switch rule.MatchType {
		case "url_contains":
			matched = strings.Contains(urlValue, pattern)
		case "content_contains":
			matched = strings.Contains(content, pattern)
		default:
			matched = strings.Contains(title, pattern)
		}
		if !matched {
			continue
		}
		switch rule.Action {
		case "star":
			if err := f.queries.StarArticle(ctx, sqlc.StarArticleParams{IsStarred: 1, ID: articleID, UserID: userID}); err != nil {
				return err
			}
		case "delete":
			if err := f.queries.DeleteArticle(ctx, sqlc.DeleteArticleParams{ID: articleID, UserID: userID}); err != nil {
				return err
			}
			return nil
		default:
			if err := f.queries.MarkArticleRead(ctx, sqlc.MarkArticleReadParams{IsRead: 1, ID: articleID, UserID: userID}); err != nil {
				return err
			}
		}
	}
	return nil
}

func articleFromItem(feedID int64, item *gofeed.Item) (sqlc.CreateArticleParams, bool) {
	if item == nil {
		return sqlc.CreateArticleParams{}, false
	}

	title := strings.TrimSpace(html.UnescapeString(item.Title))
	link := strings.TrimSpace(item.Link)
	if title == "" && link == "" {
		return sqlc.CreateArticleParams{}, false
	}
	if title == "" {
		title = link
	}

	guid := strings.TrimSpace(item.GUID)
	if guid == "" {
		guid = link
	}
	if guid == "" {
		guid = title
	}

	rawContent := firstNonEmpty(item.Content, item.Description)
	content := ReadableText(rawContent)
	excerpt := trimForDB(oneLine(content), 280)
	imageURL := itemImageURL(item, rawContent, link)

	var published sql.NullTime
	if item.PublishedParsed != nil {
		published = sql.NullTime{Time: *item.PublishedParsed, Valid: true}
	} else if item.UpdatedParsed != nil {
		published = sql.NullTime{Time: *item.UpdatedParsed, Valid: true}
	}

	return sqlc.CreateArticleParams{
		FeedID:      feedID,
		Guid:        guid,
		Url:         firstNonEmpty(link, guid),
		Title:       title,
		Author:      nullString(authorName(item)),
		Content:     nullString(content),
		Excerpt:     nullString(excerpt),
		ImageUrl:    nullString(imageURL),
		PublishedAt: published,
	}, true
}

func itemImageURL(item *gofeed.Item, rawContent, baseURL string) string {
	if item.Image != nil && item.Image.URL != "" {
		return absoluteURL(baseURL, item.Image.URL)
	}
	for _, enclosure := range item.Enclosures {
		if strings.HasPrefix(strings.ToLower(enclosure.Type), "image/") && enclosure.URL != "" {
			return absoluteURL(baseURL, enclosure.URL)
		}
	}
	if media := item.Extensions["media"]; media != nil {
		for _, name := range []string{"thumbnail", "content"} {
			for _, ext := range media[name] {
				if ext.Attrs != nil {
					if value := ext.Attrs["url"]; value != "" {
						return absoluteURL(baseURL, value)
					}
				}
			}
		}
	}
	return absoluteURL(baseURL, extractFirstImage(rawContent))
}

func extractFirstImage(value string) string {
	match := imgSrcRE.FindStringSubmatch(value)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(html.UnescapeString(match[1]))
}

func extractMetaImage(value string) string {
	match := ogImageRE.FindStringSubmatch(value)
	if len(match) == 0 {
		return ""
	}
	for _, candidate := range match[1:] {
		candidate = strings.TrimSpace(html.UnescapeString(candidate))
		if candidate != "" {
			return candidate
		}
	}
	return ""
}

func absoluteURL(baseURL, candidate string) string {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return ""
	}
	parsed, err := url.Parse(candidate)
	if err != nil {
		return ""
	}
	if parsed.IsAbs() {
		return parsed.String()
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return candidate
	}
	return base.ResolveReference(parsed).String()
}

func authorName(item *gofeed.Item) string {
	if item.Author != nil {
		return item.Author.Name
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func nullString(value string) sql.NullString {
	value = strings.TrimSpace(value)
	return sql.NullString{String: value, Valid: value != ""}
}

func plainExcerpt(value string, max int) string {
	value = oneLine(ReadableText(value))
	return trimForDB(value, max)
}

func ReadableText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = doctypeRE.ReplaceAllString(value, "")
	value = commentRE.ReplaceAllString(value, "")
	value = scriptStyleRE.ReplaceAllString(value, "")
	value = bodyOpenRE.ReplaceAllString(value, "")
	value = blockTagRE.ReplaceAllString(value, "\n")
	value = anyTagRE.ReplaceAllString(value, "")
	value = html.UnescapeString(value)
	value = strings.ReplaceAll(value, "\u00a0", " ")
	value = spaceRE.ReplaceAllString(value, " ")
	lines := strings.Split(value, "\n")
	clean := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			clean = append(clean, line)
		}
	}
	value = strings.Join(clean, "\n\n")
	value = newlineRE.ReplaceAllString(value, "\n\n")
	return strings.TrimSpace(value)
}

func oneLine(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func trimForDB(value string, max int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	if max <= 1 {
		return string(runes[:max])
	}
	return string(runes[:max-1]) + "..."
}
