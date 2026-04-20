package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"html"
	"net/http"
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

func NewFeedFetcher(queries *sqlc.Queries) *FeedFetcher {
	return &FeedFetcher{
		queries: queries,
		client: &http.Client{
			Timeout: 20 * time.Second,
		},
	}
}

func (f *FeedFetcher) FetchFeed(ctx context.Context, userID, feedID int64) (FetchResult, error) {
	feed, err := f.queries.GetFeed(ctx, sqlc.GetFeedParams{ID: feedID, UserID: userID})
	if err != nil {
		return FetchResult{}, err
	}

	result := FetchResult{FeedID: feed.ID, FeedTitle: feed.Title}
	parser := gofeed.NewParser()
	parser.Client = f.client

	parsed, err := parser.ParseURLWithContext(feed.Url, ctx)
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

	if err := f.queries.UpdateFeedFetchSuccess(ctx, sqlc.UpdateFeedFetchSuccessParams{
		Title:        title,
		SiteUrl:      nullString(siteURL),
		Description:  nullString(description),
		Etag:         feed.Etag,
		LastModified: feed.LastModified,
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
		if _, err := f.queries.CreateArticle(ctx, article); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				result.Skipped++
				continue
			}
			return result, fmt.Errorf("save article %q: %w", article.Title, err)
		}
		result.Inserted++
	}

	result.FeedTitle = title
	return result, nil
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

	content := firstNonEmpty(item.Content, item.Description)
	excerpt := plainExcerpt(content, 280)

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
		PublishedAt: published,
	}, true
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
	value = html.UnescapeString(value)
	value = stripTags(value)
	value = strings.Join(strings.Fields(value), " ")
	return trimForDB(value, max)
}

func stripTags(value string) string {
	var out strings.Builder
	inTag := false
	for _, r := range value {
		switch r {
		case '<':
			inTag = true
		case '>':
			inTag = false
		default:
			if !inTag {
				out.WriteRune(r)
			}
		}
	}
	return out.String()
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
