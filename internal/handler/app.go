package handler

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"

	"readress/internal/db/sqlc"
	"readress/internal/service"
	"readress/internal/view"
)

type App struct {
	db      *sql.DB
	queries *sqlc.Queries
	fetcher *service.FeedFetcher
	logger  *slog.Logger
	userID  int64
}

func NewApp(db *sql.DB, logger *slog.Logger, userID int64) *App {
	queries := sqlc.New(db)
	return &App{
		db:      db,
		queries: queries,
		fetcher: service.NewFeedFetcher(queries),
		logger:  logger,
		userID:  userID,
	}
}

func (a *App) Routes(r chi.Router) {
	r.Get("/", a.home)
	r.Get("/login", a.login)
	r.Get("/feeds/manage", a.feedManagement)
	r.Post("/feeds", a.createFeed)
	r.Post("/feeds/refresh", a.refreshAllFeeds)
	r.Get("/feeds/{id}/edit", a.editFeed)
	r.Post("/feeds/{id}", a.updateFeed)
	r.Post("/feeds/{id}/delete", a.deleteFeed)
	r.Post("/feeds/{id}/refresh", a.refreshFeed)
	r.Get("/settings", a.settings)
	r.Get("/search", a.search)
	r.Get("/feed-health", a.feedHealth)
	r.Get("/healthz", a.healthz)
}

func (a *App) home(w http.ResponseWriter, r *http.Request) {
	data, err := a.homeData(r)
	if err != nil {
		a.serverError(w, "load home data", err)
		return
	}
	render(w, r, view.Home(data))
}

func (a *App) login(w http.ResponseWriter, r *http.Request) {
	render(w, r, view.Login())
}

func (a *App) feedManagement(w http.ResponseWriter, r *http.Request) {
	data, err := a.feedManagementData(r, view.FeedFormData{Interval: 60}, r.URL.Query().Get("error"), r.URL.Query().Get("notice"))
	if err != nil {
		a.serverError(w, "load feed management", err)
		return
	}
	render(w, r, view.FeedManagement(data))
}

func (a *App) editFeed(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(w, r)
	if !ok {
		return
	}

	feed, err := a.queries.GetFeed(r.Context(), sqlc.GetFeedParams{ID: id, UserID: a.userID})
	if err != nil {
		a.redirectFeedManage(w, r, "Feed not found.", "")
		return
	}

	form := view.FeedFormData{
		ID:       feed.ID,
		URL:      feed.Url,
		SiteURL:  feed.SiteUrl.String,
		Title:    feed.Title,
		Interval: feed.FetchIntervalMinutes,
	}
	if feed.CategoryID.Valid {
		form.CategoryID = feed.CategoryID.Int64
	}

	data, err := a.feedManagementData(r, form, "", "Editing "+feed.Title)
	if err != nil {
		a.serverError(w, "load edit feed", err)
		return
	}
	render(w, r, view.FeedManagement(data))
}

func (a *App) createFeed(w http.ResponseWriter, r *http.Request) {
	form, err := parseFeedForm(r)
	if err != nil {
		a.redirectFeedManage(w, r, err.Error(), "")
		return
	}

	feed, err := a.queries.CreateFeed(r.Context(), sqlc.CreateFeedParams{
		UserID:               a.userID,
		CategoryID:           nullInt64(form.CategoryID),
		Url:                  form.URL,
		SiteUrl:              nullString(form.SiteURL),
		Title:                form.Title,
		Description:          sql.NullString{},
		IconUrl:              sql.NullString{},
		FetchIntervalMinutes: form.Interval,
	})
	if err != nil {
		a.redirectFeedManage(w, r, "Could not save feed. It may already exist.", "")
		return
	}

	if _, err := a.fetcher.FetchFeed(r.Context(), a.userID, feed.ID); err != nil {
		a.logger.Warn("initial feed fetch failed", "feed_id", feed.ID, "err", err)
		a.redirectFeedManage(w, r, "Feed saved, but the first fetch failed. Check feed health.", "")
		return
	}

	a.redirectFeedManage(w, r, "", "Feed subscribed.")
}

func (a *App) updateFeed(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(w, r)
	if !ok {
		return
	}

	form, err := parseFeedForm(r)
	if err != nil {
		a.redirectFeedManage(w, r, err.Error(), "")
		return
	}

	_, err = a.queries.UpdateFeed(r.Context(), sqlc.UpdateFeedParams{
		CategoryID:           nullInt64(form.CategoryID),
		Url:                  form.URL,
		SiteUrl:              nullString(form.SiteURL),
		Title:                form.Title,
		Description:          sql.NullString{},
		FetchIntervalMinutes: form.Interval,
		ID:                   id,
		UserID:               a.userID,
	})
	if err != nil {
		a.redirectFeedManage(w, r, "Could not update feed.", "")
		return
	}

	a.redirectFeedManage(w, r, "", "Feed updated.")
}

func (a *App) deleteFeed(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(w, r)
	if !ok {
		return
	}
	if err := a.queries.DeleteFeed(r.Context(), sqlc.DeleteFeedParams{ID: id, UserID: a.userID}); err != nil {
		a.redirectFeedManage(w, r, "Could not delete feed.", "")
		return
	}
	a.redirectFeedManage(w, r, "", "Feed deleted.")
}

func (a *App) refreshFeed(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(w, r)
	if !ok {
		return
	}
	result, err := a.fetcher.FetchFeed(r.Context(), a.userID, id)
	if err != nil {
		message := result.ErrorMessage
		if message == "" {
			message = err.Error()
		}
		a.redirectFeedManage(w, r, "Refresh failed: "+message, "")
		return
	}
	a.redirectFeedManage(w, r, "", fmt.Sprintf("Fetched %s: %d new articles.", result.FeedTitle, result.Inserted))
}

func (a *App) refreshAllFeeds(w http.ResponseWriter, r *http.Request) {
	feeds, err := a.queries.ListFeeds(r.Context(), a.userID)
	if err != nil {
		a.serverError(w, "list feeds for refresh", err)
		return
	}
	inserted := 0
	failed := 0
	for _, feed := range feeds {
		result, err := a.fetcher.FetchFeed(r.Context(), a.userID, feed.ID)
		inserted += result.Inserted
		if err != nil {
			failed++
			a.logger.Warn("feed refresh failed", "feed_id", feed.ID, "err", err)
		}
	}
	if failed > 0 {
		a.redirectFeedManage(w, r, fmt.Sprintf("Refresh finished with %d failed feeds and %d new articles.", failed, inserted), "")
		return
	}
	a.redirectFeedManage(w, r, "", fmt.Sprintf("Refresh complete: %d new articles.", inserted))
}

func (a *App) settings(w http.ResponseWriter, r *http.Request) {
	render(w, r, view.Settings())
}

func (a *App) search(w http.ResponseWriter, r *http.Request) {
	render(w, r, view.SearchResults())
}

func (a *App) feedHealth(w http.ResponseWriter, r *http.Request) {
	render(w, r, view.FeedHealth())
}

func (a *App) healthz(w http.ResponseWriter, r *http.Request) {
	if err := a.db.PingContext(r.Context()); err != nil {
		a.logger.Error("database ping failed", "err", err)
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

func render(w http.ResponseWriter, r *http.Request, component templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := component.Render(r.Context(), w); err != nil {
		http.Error(w, "render failed", http.StatusInternalServerError)
	}
}

func (a *App) homeData(r *http.Request) (view.HomeData, error) {
	categories, feeds, err := a.loadLibrary(r)
	if err != nil {
		return view.HomeData{}, err
	}
	articles, err := a.loadRecentArticles(r)
	if err != nil {
		return view.HomeData{}, err
	}
	unread, err := a.queries.CountUnreadArticles(r.Context(), a.userID)
	if err != nil {
		return view.HomeData{}, err
	}
	return view.HomeData{Categories: categories, Feeds: feeds, Articles: articles, Unread: unread}, nil
}

func (a *App) feedManagementData(r *http.Request, form view.FeedFormData, formError, notice string) (view.FeedManagementData, error) {
	categories, feeds, err := a.loadLibrary(r)
	if err != nil {
		return view.FeedManagementData{}, err
	}
	return view.FeedManagementData{
		Categories: categories,
		Feeds:      feeds,
		Form:       form,
		Error:      formError,
		Notice:     notice,
	}, nil
}

func (a *App) loadLibrary(r *http.Request) ([]view.CategoryView, []view.FeedView, error) {
	dbCategories, err := a.queries.ListCategories(r.Context(), a.userID)
	if err != nil {
		return nil, nil, err
	}
	dbFeeds, err := a.queries.ListFeeds(r.Context(), a.userID)
	if err != nil {
		return nil, nil, err
	}

	categoryByID := make(map[int64]string, len(dbCategories))
	countByCategory := make(map[int64]int)
	categories := make([]view.CategoryView, 0, len(dbCategories))
	for _, category := range dbCategories {
		categoryByID[category.ID] = category.Name
		categories = append(categories, view.CategoryView{
			ID:   category.ID,
			Name: category.Name,
		})
	}

	feeds := make([]view.FeedView, 0, len(dbFeeds))
	for _, feed := range dbFeeds {
		categoryName := "Uncategorized"
		var categoryID int64
		if feed.CategoryID.Valid {
			categoryID = feed.CategoryID.Int64
			if name := categoryByID[feed.CategoryID.Int64]; name != "" {
				categoryName = name
			}
			countByCategory[feed.CategoryID.Int64]++
		}
		status := "Healthy"
		tone := "ok"
		if feed.LastError.Valid && strings.TrimSpace(feed.LastError.String) != "" {
			status = feed.LastError.String
			tone = "error"
		} else if feed.ErrorCount > 0 {
			status = fmt.Sprintf("%d recent errors", feed.ErrorCount)
			tone = "warn"
		}
		feeds = append(feeds, view.FeedView{
			ID:         feed.ID,
			Title:      feed.Title,
			URL:        feed.Url,
			SiteURL:    feed.SiteUrl.String,
			CategoryID: categoryID,
			Category:   categoryName,
			Interval:   feed.FetchIntervalMinutes,
			Status:     status,
			StatusTone: tone,
		})
	}

	for i := range categories {
		categories[i].Count = countByCategory[categories[i].ID]
	}

	return categories, feeds, nil
}

func (a *App) loadRecentArticles(r *http.Request) ([]view.ArticleView, error) {
	rows, err := a.queries.ListRecentArticles(r.Context(), sqlc.ListRecentArticlesParams{
		UserID: a.userID,
		Limit:  50,
		Offset: 0,
	})
	if err != nil {
		return nil, err
	}

	articles := make([]view.ArticleView, 0, len(rows))
	for _, row := range rows {
		content := row.Content.String
		summary := row.Excerpt.String
		if summary == "" {
			summary = strings.TrimSpace(content)
		}
		articles = append(articles, view.ArticleView{
			ID:        row.ID,
			FeedID:    row.FeedID,
			Title:     row.Title,
			URL:       row.Url,
			Source:    row.FeedTitle,
			Time:      articleTime(row.PublishedAt, row.CreatedAt),
			Summary:   summary,
			Content:   content,
			Category:  row.Category,
			ReadTime:  readTime(content),
			IsRead:    row.IsRead != 0,
			IsStarred: row.IsStarred != 0,
		})
	}
	return articles, nil
}

func (a *App) serverError(w http.ResponseWriter, msg string, err error) {
	a.logger.Error(msg, "err", err)
	http.Error(w, "internal server error", http.StatusInternalServerError)
}

func parseFeedForm(r *http.Request) (view.FeedFormData, error) {
	if err := r.ParseForm(); err != nil {
		return view.FeedFormData{}, errors.New("Could not read form.")
	}

	feedURL := strings.TrimSpace(r.PostForm.Get("url"))
	if feedURL == "" {
		return view.FeedFormData{}, errors.New("Feed URL is required.")
	}
	parsed, err := url.ParseRequestURI(feedURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return view.FeedFormData{}, errors.New("Enter a valid feed URL.")
	}

	title := strings.TrimSpace(r.PostForm.Get("title"))
	if title == "" {
		title = parsed.Host
	}

	interval, err := strconv.ParseInt(r.PostForm.Get("fetch_interval_minutes"), 10, 64)
	if err != nil || interval <= 0 {
		interval = 60
	}

	categoryID, _ := strconv.ParseInt(r.PostForm.Get("category_id"), 10, 64)

	return view.FeedFormData{
		URL:        feedURL,
		SiteURL:    strings.TrimSpace(r.PostForm.Get("site_url")),
		Title:      title,
		CategoryID: categoryID,
		Interval:   interval,
	}, nil
}

func parseIDParam(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return 0, false
	}
	return id, true
}

func (a *App) redirectFeedManage(w http.ResponseWriter, r *http.Request, formError, notice string) {
	values := url.Values{}
	if formError != "" {
		values.Set("error", formError)
	}
	if notice != "" {
		values.Set("notice", notice)
	}
	target := "/feeds/manage"
	if encoded := values.Encode(); encoded != "" {
		target += "?" + encoded
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func nullString(value string) sql.NullString {
	value = strings.TrimSpace(value)
	return sql.NullString{String: value, Valid: value != ""}
}

func nullInt64(value int64) sql.NullInt64 {
	return sql.NullInt64{Int64: value, Valid: value > 0}
}

func articleTime(published sql.NullTime, created time.Time) string {
	t := created
	if published.Valid {
		t = published.Time
	}
	duration := time.Since(t)
	switch {
	case duration < time.Minute:
		return "just now"
	case duration < time.Hour:
		return fmt.Sprintf("%d min ago", int(duration.Minutes()))
	case duration < 24*time.Hour:
		return fmt.Sprintf("%d hours ago", int(duration.Hours()))
	default:
		return t.Format("Jan 2")
	}
}

func readTime(content string) string {
	words := len(strings.Fields(content))
	if words == 0 {
		return "1 min"
	}
	minutes := words / 220
	if minutes < 1 {
		minutes = 1
	}
	return fmt.Sprintf("%d min", minutes)
}
