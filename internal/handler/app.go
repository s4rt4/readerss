package handler

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"

	"readress/internal/db/sqlc"
	"readress/internal/service"
	"readress/internal/view"
)

type App struct {
	db            *sql.DB
	queries       *sqlc.Queries
	fetcher       *service.FeedFetcher
	logger        *slog.Logger
	userID        int64
	sessionKey    []byte
	loginAttempts map[string]loginAttempt
	loginMu       sync.Mutex
}

type loginAttempt struct {
	Count      int
	WindowEnds time.Time
}

type opmlDocument struct {
	XMLName xml.Name `xml:"opml"`
	Version string   `xml:"version,attr"`
	Head    opmlHead `xml:"head"`
	Body    opmlBody `xml:"body"`
}

type opmlHead struct {
	Title string `xml:"title"`
}

type opmlBody struct {
	Outlines []opmlOutline `xml:"outline"`
}

type opmlOutline struct {
	Text     string        `xml:"text,attr,omitempty"`
	Title    string        `xml:"title,attr,omitempty"`
	Type     string        `xml:"type,attr,omitempty"`
	XMLURL   string        `xml:"xmlUrl,attr,omitempty"`
	HTMLURL  string        `xml:"htmlUrl,attr,omitempty"`
	Outlines []opmlOutline `xml:"outline"`
}

func NewApp(db *sql.DB, logger *slog.Logger, userID int64) *App {
	queries := sqlc.New(db)
	return &App{
		db:            db,
		queries:       queries,
		fetcher:       service.NewFeedFetcher(queries),
		logger:        logger,
		userID:        userID,
		sessionKey:    []byte(firstNonEmpty(os.Getenv("READRESS_SESSION_KEY"), "readress-local-dev-session-key-change-me")),
		loginAttempts: map[string]loginAttempt{},
	}
}

func (a *App) Routes(r chi.Router) {
	r.Get("/login", a.login)
	r.Post("/login", a.loginPost)
	r.Post("/logout", a.logout)
	r.Group(func(protected chi.Router) {
		protected.Use(a.requireAuth)
		protected.Use(a.csrfProtection)
		protected.Get("/", a.home)
		protected.Get("/feeds/manage", a.feedManagement)
		protected.Post("/feeds", a.createFeed)
		protected.Post("/feeds/refresh", a.refreshAllFeeds)
		protected.Get("/feeds/{id}/edit", a.editFeed)
		protected.Post("/feeds/{id}", a.updateFeed)
		protected.Post("/feeds/{id}/delete", a.deleteFeed)
		protected.Post("/feeds/{id}/refresh", a.refreshFeed)
		protected.Post("/feeds/{id}/mark-read", a.markFeedRead)
		protected.Post("/categories", a.createCategory)
		protected.Post("/categories/{id}/mark-read", a.markCategoryRead)
		protected.Post("/articles/{id}/read", a.markArticleRead)
		protected.Post("/articles/{id}/unread", a.markArticleUnread)
		protected.Post("/articles/{id}/star", a.starArticle)
		protected.Post("/articles/{id}/unstar", a.unstarArticle)
		protected.Post("/articles/{id}/read-later", a.addReadLater)
		protected.Post("/articles/{id}/read-later/remove", a.removeReadLater)
		protected.Post("/articles/{id}/boards", a.addArticleToBoard)
		protected.Post("/articles/mark-all-read", a.markAllArticlesRead)
		protected.Get("/boards", a.boards)
		protected.Post("/boards", a.createBoard)
		protected.Get("/boards/{id}", a.boardDetail)
		protected.Post("/boards/{id}/delete", a.deleteBoard)
		protected.Post("/boards/{id}/articles/{articleID}/remove", a.removeArticleFromBoard)
		protected.Get("/settings", a.settings)
		protected.Post("/settings", a.updateSettings)
		protected.Get("/settings/opml/export", a.exportOPML)
		protected.Post("/settings/opml/import", a.importOPML)
		protected.Post("/settings/filter-rules", a.createFilterRule)
		protected.Post("/settings/filter-rules/{id}", a.updateFilterRule)
		protected.Post("/settings/filter-rules/{id}/delete", a.deleteFilterRule)
		protected.Get("/search", a.search)
		protected.Get("/feed-health", a.feedHealth)
		protected.Get("/events", a.events)
	})
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
	if a.authenticated(r) {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	render(w, r, view.Login(view.LoginData{Next: r.URL.Query().Get("next"), Error: r.URL.Query().Get("error")}))
}

func (a *App) loginPost(w http.ResponseWriter, r *http.Request) {
	if !a.allowLoginAttempt(r) {
		http.Redirect(w, r, "/login?error=Too+many+login+attempts.+Try+again+in+one+minute.", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/login?error=Could+not+read+login+form.", http.StatusSeeOther)
		return
	}
	username := strings.TrimSpace(r.PostForm.Get("username"))
	password := r.PostForm.Get("password")
	user, err := a.queries.GetUserByUsername(r.Context(), username)
	if err != nil || !validPassword(user.PasswordHash, password) {
		a.recordLoginFailure(r)
		http.Redirect(w, r, "/login?error=Invalid+username+or+password.", http.StatusSeeOther)
		return
	}
	a.clearLoginAttempts(r)
	a.setSession(w, user.ID, r.PostForm.Get("remember") == "1")
	target := r.PostForm.Get("next")
	if target == "" || !strings.HasPrefix(target, "/") || strings.HasPrefix(target, "//") {
		target = "/"
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func (a *App) logout(w http.ResponseWriter, r *http.Request) {
	clearSession(w)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
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
	form.URL = a.fetcher.DiscoverFeedURL(r.Context(), form.URL)

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

func (a *App) createCategory(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		redirectBack(w, r)
		return
	}
	name := strings.TrimSpace(r.PostForm.Get("name"))
	if name == "" {
		redirectBack(w, r)
		return
	}
	categories, err := a.queries.ListCategories(r.Context(), a.userID)
	if err != nil {
		a.serverError(w, "list categories", err)
		return
	}
	for _, category := range categories {
		if strings.EqualFold(category.Name, name) {
			redirectBack(w, r)
			return
		}
	}
	if _, err := a.queries.CreateCategory(r.Context(), sqlc.CreateCategoryParams{
		UserID:    a.userID,
		Name:      name,
		SortOrder: int64(len(categories) + 1),
	}); err != nil {
		a.serverError(w, "create category", err)
		return
	}
	redirectBack(w, r)
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
	data, err := a.settingsData(r, r.URL.Query().Get("notice"), r.URL.Query().Get("error"))
	if err != nil {
		a.serverError(w, "load settings", err)
		return
	}
	render(w, r, view.Settings(data))
}

func (a *App) updateSettings(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		a.redirectSettings(w, r, "", "Could not read settings form.")
		return
	}

	params := sqlc.UpsertReaderSettingsParams{
		UserID:                      a.userID,
		DefaultFetchIntervalMinutes: parseFormInt(r, "default_fetch_interval_minutes", 60),
		RetentionDays:               parseFormInt(r, "retention_days", 90),
		Theme:                       normalizeChoice(r.PostForm.Get("theme"), "system", "system", "light", "dark"),
		Density:                     normalizeChoice(r.PostForm.Get("density"), "balanced", "comfortable", "balanced", "compact"),
		RespectCacheHeaders:         boolInt(r.PostForm.Get("respect_cache_headers") == "1"),
	}
	if _, err := a.queries.UpsertReaderSettings(r.Context(), params); err != nil {
		a.redirectSettings(w, r, "", "Could not save settings.")
		return
	}
	a.redirectSettings(w, r, "Settings saved.", "")
}

func (a *App) exportOPML(w http.ResponseWriter, r *http.Request) {
	_, feeds, err := a.loadLibrary(r)
	if err != nil {
		a.serverError(w, "load feeds for opml", err)
		return
	}

	byCategory := map[string][]view.FeedView{}
	order := []string{}
	for _, feed := range feeds {
		category := feed.Category
		if category == "" {
			category = "Uncategorized"
		}
		if _, ok := byCategory[category]; !ok {
			order = append(order, category)
		}
		byCategory[category] = append(byCategory[category], feed)
	}

	doc := opmlDocument{Version: "2.0", Head: opmlHead{Title: "ReadeRSS subscriptions"}}
	for _, category := range order {
		group := opmlOutline{Text: category, Title: category}
		for _, feed := range byCategory[category] {
			group.Outlines = append(group.Outlines, opmlOutline{
				Text:    feed.Title,
				Title:   feed.Title,
				Type:    "rss",
				XMLURL:  feed.URL,
				HTMLURL: feed.SiteURL,
			})
		}
		doc.Body.Outlines = append(doc.Body.Outlines, group)
	}

	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="readerss-subscriptions.opml"`)
	if _, err := io.WriteString(w, xml.Header); err != nil {
		return
	}
	encoder := xml.NewEncoder(w)
	encoder.Indent("", "  ")
	_ = encoder.Encode(doc)
}

func (a *App) importOPML(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(4 << 20); err != nil {
		a.redirectSettings(w, r, "", "Could not read OPML upload.")
		return
	}
	file, _, err := r.FormFile("opml")
	if err != nil {
		a.redirectSettings(w, r, "", "Choose an OPML file first.")
		return
	}
	defer file.Close()

	var doc opmlDocument
	if err := xml.NewDecoder(file).Decode(&doc); err != nil {
		a.redirectSettings(w, r, "", "Could not parse OPML file.")
		return
	}

	imported, skipped := a.importOPMLOutlines(r, doc.Body.Outlines, "")
	notice := fmt.Sprintf("OPML import finished: %d feeds added, %d skipped.", imported, skipped)
	a.redirectSettings(w, r, notice, "")
}

func (a *App) createFilterRule(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		a.redirectSettings(w, r, "", "Could not read filter rule form.")
		return
	}
	pattern := strings.TrimSpace(r.PostForm.Get("pattern"))
	if pattern == "" {
		a.redirectSettings(w, r, "", "Filter rule pattern is required.")
		return
	}
	_, err := a.queries.CreateFilterRule(r.Context(), sqlc.CreateFilterRuleParams{
		UserID:    a.userID,
		FeedID:    nullInt64(parseFormInt(r, "feed_id", 0)),
		MatchType: normalizeChoice(r.PostForm.Get("match_type"), "title_contains", "title_contains", "url_contains", "content_contains"),
		Pattern:   pattern,
		Action:    normalizeChoice(r.PostForm.Get("action"), "mark_read", "mark_read", "star", "delete"),
	})
	if err != nil {
		a.redirectSettings(w, r, "", "Could not save filter rule.")
		return
	}
	a.redirectSettings(w, r, "Filter rule added.", "")
}

func (a *App) updateFilterRule(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		a.redirectSettings(w, r, "", "Could not read filter rule form.")
		return
	}
	pattern := strings.TrimSpace(r.PostForm.Get("pattern"))
	if pattern == "" {
		a.redirectSettings(w, r, "", "Filter rule pattern is required.")
		return
	}
	if err := a.queries.UpdateFilterRule(r.Context(), sqlc.UpdateFilterRuleParams{
		ID:        id,
		UserID:    a.userID,
		FeedID:    nullInt64(parseFormInt(r, "feed_id", 0)),
		MatchType: normalizeChoice(r.PostForm.Get("match_type"), "title_contains", "title_contains", "url_contains", "content_contains"),
		Pattern:   pattern,
		Action:    normalizeChoice(r.PostForm.Get("action"), "mark_read", "mark_read", "star", "delete"),
	}); err != nil {
		a.redirectSettings(w, r, "", "Could not update filter rule.")
		return
	}
	a.redirectSettings(w, r, "Filter rule updated.", "")
}

func (a *App) deleteFilterRule(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(w, r)
	if !ok {
		return
	}
	if err := a.queries.DeleteFilterRule(r.Context(), sqlc.DeleteFilterRuleParams{ID: id, UserID: a.userID}); err != nil {
		a.redirectSettings(w, r, "", "Could not delete filter rule.")
		return
	}
	a.redirectSettings(w, r, "Filter rule deleted.", "")
}

func (a *App) search(w http.ResponseWriter, r *http.Request) {
	data, err := a.searchData(r)
	if err != nil {
		a.serverError(w, "search articles", err)
		return
	}
	render(w, r, view.SearchResults(data))
}

func (a *App) feedHealth(w http.ResponseWriter, r *http.Request) {
	data, err := a.feedHealthData(r)
	if err != nil {
		a.serverError(w, "load feed health", err)
		return
	}
	render(w, r, view.FeedHealth(data))
}

func (a *App) boards(w http.ResponseWriter, r *http.Request) {
	data, err := a.boardsData(r, r.URL.Query().Get("notice"), r.URL.Query().Get("error"))
	if err != nil {
		a.serverError(w, "load boards", err)
		return
	}
	render(w, r, view.Boards(data))
}

func (a *App) createBoard(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		a.redirectBoards(w, r, "", "Could not read board form.")
		return
	}
	name := strings.TrimSpace(r.PostForm.Get("name"))
	if name == "" {
		a.redirectBoards(w, r, "", "Board name is required.")
		return
	}
	_, err := a.queries.CreateBoard(r.Context(), sqlc.CreateBoardParams{
		UserID:      a.userID,
		Name:        name,
		Description: nullString(r.PostForm.Get("description")),
	})
	if err != nil {
		a.serverError(w, "create board", err)
		return
	}
	a.redirectBoards(w, r, "Board saved.", "")
}

func (a *App) deleteBoard(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(w, r)
	if !ok {
		return
	}
	if err := a.queries.DeleteBoard(r.Context(), sqlc.DeleteBoardParams{ID: id, UserID: a.userID}); err != nil {
		a.serverError(w, "delete board", err)
		return
	}
	a.redirectBoards(w, r, "Board deleted.", "")
}

func (a *App) boardDetail(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(w, r)
	if !ok {
		return
	}
	board, err := a.queries.GetBoard(r.Context(), sqlc.GetBoardParams{ID: id, UserID: a.userID})
	if err != nil {
		http.NotFound(w, r)
		return
	}
	rows, err := a.queries.ListBoardArticles(r.Context(), sqlc.ListBoardArticlesParams{ID: id, UserID: a.userID, Limit: 100, Offset: 0})
	if err != nil {
		a.serverError(w, "list board articles", err)
		return
	}
	articles := make([]view.ArticleView, 0, len(rows))
	for _, row := range rows {
		articles = append(articles, articleViewFromRow(row.ID, row.FeedID, row.Url, row.Title, row.Content, row.Excerpt, row.ImageUrl, row.PublishedAt, row.CreatedAt, row.IsRead, row.IsStarred, row.IsReadLater, row.FeedTitle, row.Category))
	}
	boards, err := a.loadBoards(r)
	if err != nil {
		a.serverError(w, "list boards", err)
		return
	}
	render(w, r, view.BoardDetail(view.BoardDetailData{
		Board: view.BoardView{
			ID:          board.ID,
			Name:        board.Name,
			Description: board.Description.String,
			CreatedAt:   articleTime(sql.NullTime{}, board.CreatedAt),
		},
		Articles: articles,
		Boards:   boards,
	}))
}

func (a *App) events(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	send := func() bool {
		unread, err := a.queries.CountUnreadArticles(r.Context(), a.userID)
		if err != nil {
			return false
		}
		_, _ = fmt.Fprintf(w, "event: unread\ndata: %d\n\n", unread)
		flusher.Flush()
		return true
	}
	if !send() {
		return
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			if !send() {
				return
			}
		}
	}
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

func (a *App) csrfProtection(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := a.ensureCSRFToken(w, r)
		if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch || r.Method == http.MethodDelete {
			submitted := r.Header.Get("X-CSRF-Token")
			if submitted == "" {
				submitted = r.FormValue("csrf_token")
			}
			if submitted == "" || submitted != token {
				http.Error(w, "invalid csrf token", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) ensureCSRFToken(w http.ResponseWriter, r *http.Request) string {
	if cookie, err := r.Cookie("readress_csrf"); err == nil && len(cookie.Value) >= 32 {
		return cookie.Value
	}
	token := randomToken()
	http.SetCookie(w, &http.Cookie{
		Name:     "readress_csrf",
		Value:    token,
		Path:     "/",
		MaxAge:   86400,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil,
	})
	return token
}

func (a *App) homeData(r *http.Request) (view.HomeData, error) {
	filter := normalizeArticleFilter(r.URL.Query().Get("filter"))
	categories, feeds, err := a.loadLibrary(r)
	if err != nil {
		return view.HomeData{}, err
	}
	articles, hasMore, err := a.loadRecentArticles(r, filter)
	if err != nil {
		return view.HomeData{}, err
	}
	offset := parseQueryInt(r, "offset", 0)
	boards, err := a.loadBoards(r)
	if err != nil {
		return view.HomeData{}, err
	}
	unread, err := a.queries.CountUnreadArticles(r.Context(), a.userID)
	if err != nil {
		return view.HomeData{}, err
	}
	all, _ := a.queries.CountAllArticles(r.Context(), a.userID)
	starred, _ := a.queries.CountStarredArticles(r.Context(), a.userID)
	readLater, _ := a.queries.CountReadLaterArticles(r.Context(), a.userID)
	settings, _ := a.readerSettings(r)
	errors := 0
	for _, feed := range feeds {
		if feed.StatusTone == "error" {
			errors++
		}
	}
	return view.HomeData{Categories: categories, Feeds: feeds, Boards: boards, Articles: articles, Unread: unread, All: all, Starred: starred, ReadLater: readLater, Errors: errors, Filter: filter, Density: settings.Density, Offset: offset, NextOffset: offset + articlePageSize, HasMore: hasMore}, nil
}

func (a *App) boardsData(r *http.Request, notice, formError string) (view.BoardsData, error) {
	boards, err := a.loadBoards(r)
	if err != nil {
		return view.BoardsData{}, err
	}
	return view.BoardsData{Boards: boards, Notice: notice, Error: formError}, nil
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

func (a *App) settingsData(r *http.Request, notice, formError string) (view.SettingsData, error) {
	settings, err := a.readerSettings(r)
	if err != nil {
		return view.SettingsData{}, err
	}
	_, feeds, err := a.loadLibrary(r)
	if err != nil {
		return view.SettingsData{}, err
	}
	return view.SettingsData{
		DefaultFetchInterval: settings.DefaultFetchIntervalMinutes,
		RetentionDays:        settings.RetentionDays,
		Theme:                settings.Theme,
		Density:              settings.Density,
		RespectCacheHeaders:  settings.RespectCacheHeaders != 0,
		FilterRules:          a.filterRuleViews(r),
		Feeds:                feeds,
		Notice:               notice,
		Error:                formError,
	}, nil
}

func (a *App) readerSettings(r *http.Request) (sqlc.ReaderSetting, error) {
	settings, err := a.queries.GetReaderSettings(r.Context(), a.userID)
	if err == nil {
		return settings, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return sqlc.ReaderSetting{}, err
	}
	return a.queries.UpsertReaderSettings(r.Context(), sqlc.UpsertReaderSettingsParams{
		UserID:                      a.userID,
		DefaultFetchIntervalMinutes: 60,
		RetentionDays:               90,
		Theme:                       "system",
		Density:                     "balanced",
		RespectCacheHeaders:         1,
	})
}

func (a *App) feedHealthData(r *http.Request) (view.FeedHealthData, error) {
	_, feeds, err := a.loadLibrary(r)
	if err != nil {
		return view.FeedHealthData{}, err
	}
	data := view.FeedHealthData{Feeds: feeds, Total: len(feeds), LastChecked: "Never"}
	for _, feed := range feeds {
		switch feed.StatusTone {
		case "error":
			data.Errors++
		case "warn":
			data.Warnings++
		default:
			data.Healthy++
		}
		if feed.LastFetched != "Never" {
			data.LastChecked = feed.LastFetched
		}
	}
	return data, nil
}

func (a *App) filterRuleViews(r *http.Request) []view.FilterRuleView {
	rules, err := a.queries.ListFilterRules(r.Context(), a.userID)
	if err != nil {
		a.logger.Warn("list filter rules failed", "err", err)
		return nil
	}
	_, feeds, err := a.loadLibrary(r)
	feedNames := map[int64]string{}
	for _, feed := range feeds {
		feedNames[feed.ID] = feed.Title
	}
	views := make([]view.FilterRuleView, 0, len(rules))
	for _, rule := range rules {
		feedTitle := "All feeds"
		feedID := int64(0)
		if rule.FeedID.Valid {
			feedID = rule.FeedID.Int64
			if name := feedNames[feedID]; name != "" {
				feedTitle = name
			}
		}
		views = append(views, view.FilterRuleView{
			ID:        rule.ID,
			FeedID:    feedID,
			FeedTitle: feedTitle,
			MatchType: rule.MatchType,
			Pattern:   rule.Pattern,
			Action:    rule.Action,
		})
	}
	return views
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
	unreadByFeedRows, _ := a.queries.CountUnreadArticlesByFeed(r.Context(), a.userID)
	unreadByFeed := make(map[int64]int64, len(unreadByFeedRows))
	for _, row := range unreadByFeedRows {
		unreadByFeed[row.FeedID] = row.UnreadCount
	}
	unreadByCategoryRows, _ := a.queries.CountUnreadArticlesByCategory(r.Context(), a.userID)
	unreadByCategory := make(map[int64]int64, len(unreadByCategoryRows))
	for _, row := range unreadByCategoryRows {
		if row.CategoryID.Valid {
			unreadByCategory[row.CategoryID.Int64] = row.UnreadCount
		}
	}

	categoryByID := make(map[int64]string, len(dbCategories))
	categories := make([]view.CategoryView, 0, len(dbCategories))
	for _, category := range dbCategories {
		categoryByID[category.ID] = category.Name
		categories = append(categories, view.CategoryView{
			ID:    category.ID,
			Name:  category.Name,
			Count: int(unreadByCategory[category.ID]),
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
		}
		status := "Healthy"
		tone := "ok"
		lastFetched := "Never"
		if feed.LastFetchedAt.Valid {
			lastFetched = articleTime(feed.LastFetchedAt, time.Now())
		}
		if feed.LastError.Valid && strings.TrimSpace(feed.LastError.String) != "" {
			status = feed.LastError.String
			tone = "error"
		} else if feed.ErrorCount > 0 {
			status = fmt.Sprintf("%d recent errors", feed.ErrorCount)
			tone = "warn"
		} else if !feed.LastFetchedAt.Valid {
			status = "Waiting for first fetch"
			tone = "warn"
		}
		feeds = append(feeds, view.FeedView{
			ID:          feed.ID,
			Title:       feed.Title,
			URL:         feed.Url,
			SiteURL:     feed.SiteUrl.String,
			CategoryID:  categoryID,
			Category:    categoryName,
			Interval:    feed.FetchIntervalMinutes,
			Status:      status,
			StatusTone:  tone,
			UnreadCount: int(unreadByFeed[feed.ID]),
			ErrorCount:  feed.ErrorCount,
			LastFetched: lastFetched,
		})
	}

	return categories, feeds, nil
}

func (a *App) loadBoards(r *http.Request) ([]view.BoardView, error) {
	rows, err := a.queries.ListBoards(r.Context(), a.userID)
	if err != nil {
		return nil, err
	}
	boards := make([]view.BoardView, 0, len(rows))
	for _, row := range rows {
		boards = append(boards, view.BoardView{
			ID:          row.ID,
			Name:        row.Name,
			Description: row.Description.String,
			Count:       row.ArticleCount,
			CreatedAt:   articleTime(sql.NullTime{}, row.CreatedAt),
		})
	}
	return boards, nil
}

const articlePageSize = int64(30)

func (a *App) loadRecentArticles(r *http.Request, filter string) ([]view.ArticleView, bool, error) {
	offset := parseQueryInt(r, "offset", 0)
	params := sqlc.ListRecentArticlesParams{UserID: a.userID, Limit: articlePageSize + 1, Offset: offset}
	var articles []view.ArticleView
	switch filter {
	case "starred":
		starred, err := a.queries.ListStarredArticles(r.Context(), sqlc.ListStarredArticlesParams(params))
		if err != nil {
			return nil, false, err
		}
		for _, row := range starred {
			articles = append(articles, articleViewFromRow(row.ID, row.FeedID, row.Url, row.Title, row.Content, row.Excerpt, row.ImageUrl, row.PublishedAt, row.CreatedAt, row.IsRead, row.IsStarred, row.IsReadLater, row.FeedTitle, row.Category))
		}
	case "all":
		rows, err := a.queries.ListRecentArticles(r.Context(), params)
		if err != nil {
			return nil, false, err
		}
		for _, row := range rows {
			articles = append(articles, articleViewFromRow(row.ID, row.FeedID, row.Url, row.Title, row.Content, row.Excerpt, row.ImageUrl, row.PublishedAt, row.CreatedAt, row.IsRead, row.IsStarred, row.IsReadLater, row.FeedTitle, row.Category))
		}
	case "read-later":
		rows, err := a.queries.ListReadLaterArticles(r.Context(), sqlc.ListReadLaterArticlesParams(params))
		if err != nil {
			return nil, false, err
		}
		for _, row := range rows {
			articles = append(articles, articleViewFromRow(row.ID, row.FeedID, row.Url, row.Title, row.Content, row.Excerpt, row.ImageUrl, row.PublishedAt, row.CreatedAt, row.IsRead, row.IsStarred, row.IsReadLater, row.FeedTitle, row.Category))
		}
	default:
		if strings.HasPrefix(filter, "feed:") {
			feedID, _ := strconv.ParseInt(strings.TrimPrefix(filter, "feed:"), 10, 64)
			rows, err := a.queries.ListRecentArticlesByFeed(r.Context(), sqlc.ListRecentArticlesByFeedParams{UserID: a.userID, ID: feedID, Limit: articlePageSize + 1, Offset: offset})
			if err != nil {
				return nil, false, err
			}
			for _, row := range rows {
				articles = append(articles, articleViewFromRow(row.ID, row.FeedID, row.Url, row.Title, row.Content, row.Excerpt, row.ImageUrl, row.PublishedAt, row.CreatedAt, row.IsRead, row.IsStarred, row.IsReadLater, row.FeedTitle, row.Category))
			}
			return trimArticlePage(articles)
		}
		if strings.HasPrefix(filter, "category:") {
			categoryID, _ := strconv.ParseInt(strings.TrimPrefix(filter, "category:"), 10, 64)
			rows, err := a.queries.ListRecentArticlesByCategory(r.Context(), sqlc.ListRecentArticlesByCategoryParams{UserID: a.userID, CategoryID: nullInt64(categoryID), Limit: articlePageSize + 1, Offset: offset})
			if err != nil {
				return nil, false, err
			}
			for _, row := range rows {
				articles = append(articles, articleViewFromRow(row.ID, row.FeedID, row.Url, row.Title, row.Content, row.Excerpt, row.ImageUrl, row.PublishedAt, row.CreatedAt, row.IsRead, row.IsStarred, row.IsReadLater, row.FeedTitle, row.Category))
			}
			return trimArticlePage(articles)
		}
		if strings.HasPrefix(filter, "board:") {
			boardID, _ := strconv.ParseInt(strings.TrimPrefix(filter, "board:"), 10, 64)
			rows, err := a.queries.ListBoardArticles(r.Context(), sqlc.ListBoardArticlesParams{ID: boardID, UserID: a.userID, Limit: articlePageSize + 1, Offset: offset})
			if err != nil {
				return nil, false, err
			}
			for _, row := range rows {
				articles = append(articles, articleViewFromRow(row.ID, row.FeedID, row.Url, row.Title, row.Content, row.Excerpt, row.ImageUrl, row.PublishedAt, row.CreatedAt, row.IsRead, row.IsStarred, row.IsReadLater, row.FeedTitle, row.Category))
			}
			return trimArticlePage(articles)
		}
		unread, err := a.queries.ListUnreadArticles(r.Context(), sqlc.ListUnreadArticlesParams(params))
		if err != nil {
			return nil, false, err
		}
		for _, row := range unread {
			articles = append(articles, articleViewFromRow(row.ID, row.FeedID, row.Url, row.Title, row.Content, row.Excerpt, row.ImageUrl, row.PublishedAt, row.CreatedAt, row.IsRead, row.IsStarred, row.IsReadLater, row.FeedTitle, row.Category))
		}
	}
	return trimArticlePage(articles)
}

func trimArticlePage(articles []view.ArticleView) ([]view.ArticleView, bool, error) {
	if int64(len(articles)) <= articlePageSize {
		return articles, false, nil
	}
	return articles[:articlePageSize], true, nil
}

func articleViewFromRow(id, feedID int64, urlValue, title string, contentValue, excerpt, imageURL sql.NullString, publishedAt sql.NullTime, createdAt time.Time, isRead, isStarred, isReadLater int64, feedTitle, category string) view.ArticleView {
	content := service.ReadableText(contentValue.String)
	summary := service.ReadableText(excerpt.String)
	if summary == "" {
		summary = strings.TrimSpace(content)
	}
	summary = trimDisplay(summary, 280)
	return view.ArticleView{
		ID:        id,
		FeedID:    feedID,
		Title:     title,
		URL:       urlValue,
		Source:    feedTitle,
		Time:      articleTime(publishedAt, createdAt),
		Summary:   summary,
		Content:   content,
		ImageURL:  imageURL.String,
		Category:  category,
		ReadTime:  readTime(content),
		IsRead:    isRead != 0,
		IsStarred: isStarred != 0,
		ReadLater: isReadLater != 0,
	}
}

func (a *App) searchData(r *http.Request) (view.SearchData, error) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	data := view.SearchData{Query: query}
	if query == "" {
		return data, nil
	}
	rows, err := a.queries.SearchArticles(r.Context(), sqlc.SearchArticlesParams{
		Content: ftsQuery(query),
		UserID:  a.userID,
	})
	if err != nil {
		return a.searchDataLike(r, data, query)
	}
	for _, row := range rows {
		snippet := row.ContentSnippet
		data.Results = append(data.Results, view.SearchResultView{
			Title:   stripMarks(row.TitleSnippet, row.Title),
			Source:  row.FeedTitle,
			Snippet: snippet,
			Tag:     "Article",
			Time:    articleTime(row.PublishedAt, time.Now()),
			URL:     row.Url,
		})
	}
	return data, nil
}

func (a *App) searchDataLike(r *http.Request, data view.SearchData, query string) (view.SearchData, error) {
	rows, err := a.db.QueryContext(r.Context(), `
SELECT a.title, a.url, COALESCE(a.excerpt, ''), a.published_at, f.title
FROM articles a
JOIN feeds f ON f.id = a.feed_id
WHERE f.user_id = ?
  AND (a.title LIKE '%' || ? || '%' OR a.content LIKE '%' || ? || '%' OR a.author LIKE '%' || ? || '%')
ORDER BY COALESCE(a.published_at, a.created_at) DESC, a.id DESC
LIMIT 50`, a.userID, query, query, query)
	if err != nil {
		return data, err
	}
	defer rows.Close()
	for rows.Next() {
		var title, urlValue, snippet, feedTitle string
		var published sql.NullTime
		if err := rows.Scan(&title, &urlValue, &snippet, &published, &feedTitle); err != nil {
			return data, err
		}
		data.Results = append(data.Results, view.SearchResultView{
			Title:   title,
			Source:  feedTitle,
			Snippet: highlightSnippet(trimDisplay(service.ReadableText(snippet), 320), query),
			Tag:     "Article",
			Time:    articleTime(published, time.Now()),
			URL:     urlValue,
		})
	}
	return data, rows.Err()
}

func (a *App) markArticleRead(w http.ResponseWriter, r *http.Request) {
	a.setArticleRead(w, r, 1)
}

func (a *App) markArticleUnread(w http.ResponseWriter, r *http.Request) {
	a.setArticleRead(w, r, 0)
}

func (a *App) setArticleRead(w http.ResponseWriter, r *http.Request, value int64) {
	id, ok := parseIDParam(w, r)
	if !ok {
		return
	}
	if err := a.queries.MarkArticleRead(r.Context(), sqlc.MarkArticleReadParams{IsRead: value, ID: id, UserID: a.userID}); err != nil {
		a.serverError(w, "mark article read", err)
		return
	}
	redirectBack(w, r)
}

func (a *App) starArticle(w http.ResponseWriter, r *http.Request) {
	a.setArticleStarred(w, r, 1)
}

func (a *App) unstarArticle(w http.ResponseWriter, r *http.Request) {
	a.setArticleStarred(w, r, 0)
}

func (a *App) setArticleStarred(w http.ResponseWriter, r *http.Request, value int64) {
	id, ok := parseIDParam(w, r)
	if !ok {
		return
	}
	if err := a.queries.StarArticle(r.Context(), sqlc.StarArticleParams{IsStarred: value, ID: id, UserID: a.userID}); err != nil {
		a.serverError(w, "star article", err)
		return
	}
	redirectBack(w, r)
}

func (a *App) addReadLater(w http.ResponseWriter, r *http.Request) {
	a.setArticleReadLater(w, r, 1)
}

func (a *App) removeReadLater(w http.ResponseWriter, r *http.Request) {
	a.setArticleReadLater(w, r, 0)
}

func (a *App) setArticleReadLater(w http.ResponseWriter, r *http.Request, value int64) {
	id, ok := parseIDParam(w, r)
	if !ok {
		return
	}
	if err := a.queries.SetArticleReadLater(r.Context(), sqlc.SetArticleReadLaterParams{IsReadLater: value, ID: id, UserID: a.userID}); err != nil {
		a.serverError(w, "set article read later", err)
		return
	}
	redirectBack(w, r)
}

func (a *App) addArticleToBoard(w http.ResponseWriter, r *http.Request) {
	articleID, ok := parseIDParam(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		a.serverError(w, "read board form", err)
		return
	}
	boardID, err := strconv.ParseInt(r.PostForm.Get("board_id"), 10, 64)
	if err != nil || boardID <= 0 {
		http.Error(w, "invalid board", http.StatusBadRequest)
		return
	}
	if err := a.queries.AddArticleToBoard(r.Context(), sqlc.AddArticleToBoardParams{
		BoardID:   boardID,
		ArticleID: articleID,
		OwnerID:   a.userID,
	}); err != nil {
		a.serverError(w, "add article to board", err)
		return
	}
	redirectBack(w, r)
}

func (a *App) removeArticleFromBoard(w http.ResponseWriter, r *http.Request) {
	boardID, ok := parseIDParam(w, r)
	if !ok {
		return
	}
	articleID, err := strconv.ParseInt(chi.URLParam(r, "articleID"), 10, 64)
	if err != nil || articleID <= 0 {
		http.Error(w, "invalid article id", http.StatusBadRequest)
		return
	}
	if err := a.queries.RemoveArticleFromBoard(r.Context(), sqlc.RemoveArticleFromBoardParams{BoardID: boardID, ArticleID: articleID, UserID: a.userID}); err != nil {
		a.serverError(w, "remove article from board", err)
		return
	}
	redirectBack(w, r)
}

func (a *App) markAllArticlesRead(w http.ResponseWriter, r *http.Request) {
	if err := a.queries.MarkAllArticlesRead(r.Context(), a.userID); err != nil {
		a.serverError(w, "mark all read", err)
		return
	}
	redirectBack(w, r)
}

func (a *App) markFeedRead(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(w, r)
	if !ok {
		return
	}
	if err := a.queries.MarkFeedArticlesRead(r.Context(), sqlc.MarkFeedArticlesReadParams{FeedID: id, UserID: a.userID}); err != nil {
		a.serverError(w, "mark feed read", err)
		return
	}
	redirectBack(w, r)
}

func (a *App) markCategoryRead(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(w, r)
	if !ok {
		return
	}
	if err := a.queries.MarkCategoryArticlesRead(r.Context(), sqlc.MarkCategoryArticlesReadParams{UserID: a.userID, CategoryID: sql.NullInt64{Int64: id, Valid: true}}); err != nil {
		a.serverError(w, "mark category read", err)
		return
	}
	redirectBack(w, r)
}

func (a *App) importOPMLOutlines(r *http.Request, outlines []opmlOutline, categoryName string) (int, int) {
	imported := 0
	skipped := 0
	for _, outline := range outlines {
		label := firstNonEmpty(outline.Title, outline.Text)
		if strings.TrimSpace(outline.XMLURL) == "" {
			childImported, childSkipped := a.importOPMLOutlines(r, outline.Outlines, label)
			imported += childImported
			skipped += childSkipped
			continue
		}

		feedURL := strings.TrimSpace(outline.XMLURL)
		parsed, err := url.ParseRequestURI(feedURL)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			skipped++
			continue
		}

		categoryID := int64(0)
		if strings.TrimSpace(categoryName) != "" {
			categoryID, _ = a.findOrCreateCategory(r, categoryName)
		}
		title := strings.TrimSpace(label)
		if title == "" {
			title = parsed.Host
		}
		feed, err := a.queries.CreateFeed(r.Context(), sqlc.CreateFeedParams{
			UserID:               a.userID,
			CategoryID:           nullInt64(categoryID),
			Url:                  feedURL,
			SiteUrl:              nullString(outline.HTMLURL),
			Title:                title,
			Description:          sql.NullString{},
			IconUrl:              sql.NullString{},
			FetchIntervalMinutes: 60,
		})
		if err != nil {
			skipped++
			continue
		}
		imported++
		if _, err := a.fetcher.FetchFeed(r.Context(), a.userID, feed.ID); err != nil {
			a.logger.Warn("opml feed fetch failed", "feed_id", feed.ID, "err", err)
		}
	}
	return imported, skipped
}

func (a *App) findOrCreateCategory(r *http.Request, name string) (int64, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return 0, nil
	}
	categories, err := a.queries.ListCategories(r.Context(), a.userID)
	if err != nil {
		return 0, err
	}
	for _, category := range categories {
		if strings.EqualFold(category.Name, name) {
			return category.ID, nil
		}
	}
	category, err := a.queries.CreateCategory(r.Context(), sqlc.CreateCategoryParams{
		UserID:    a.userID,
		Name:      name,
		SortOrder: int64(len(categories) + 1),
	})
	if err != nil {
		return 0, err
	}
	return category.ID, nil
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

func (a *App) redirectSettings(w http.ResponseWriter, r *http.Request, notice, formError string) {
	values := url.Values{}
	if notice != "" {
		values.Set("notice", notice)
	}
	if formError != "" {
		values.Set("error", formError)
	}
	target := "/settings"
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

func parseFormInt(r *http.Request, name string, fallback int64) int64 {
	value, err := strconv.ParseInt(r.PostForm.Get(name), 10, 64)
	if err != nil || value < 0 {
		return fallback
	}
	return value
}

func parseQueryInt(r *http.Request, name string, fallback int64) int64 {
	value, err := strconv.ParseInt(r.URL.Query().Get(name), 10, 64)
	if err != nil || value < 0 {
		return fallback
	}
	return value
}

func boolInt(value bool) int64 {
	if value {
		return 1
	}
	return 0
}

func validPassword(stored, password string) bool {
	// Compatibility for databases created before auth was wired.
	if stored == "local-dev-password-placeholder" && password == "readerss" {
		return true
	}
	return stored == passwordDigest(password)
}

func normalizeChoice(value, fallback string, allowed ...string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	for _, option := range allowed {
		if value == option {
			return value
		}
	}
	return fallback
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func randomToken() string {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return base64.RawURLEncoding.EncodeToString(bytes)
}

func normalizeArticleFilter(value string) string {
	if strings.HasPrefix(value, "feed:") || strings.HasPrefix(value, "category:") || strings.HasPrefix(value, "board:") {
		return value
	}
	switch value {
	case "all", "starred", "read-later":
		return value
	default:
		return "unread"
	}
}

func (a *App) redirectBoards(w http.ResponseWriter, r *http.Request, notice, formError string) {
	values := url.Values{}
	if notice != "" {
		values.Set("notice", notice)
	}
	if formError != "" {
		values.Set("error", formError)
	}
	target := "/boards"
	if encoded := values.Encode(); encoded != "" {
		target += "?" + encoded
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func redirectBack(w http.ResponseWriter, r *http.Request) {
	target := r.Header.Get("Referer")
	if target == "" {
		target = "/"
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func (a *App) allowLoginAttempt(r *http.Request) bool {
	key := clientKey(r)
	now := time.Now()
	a.loginMu.Lock()
	defer a.loginMu.Unlock()
	attempt := a.loginAttempts[key]
	if now.After(attempt.WindowEnds) {
		return true
	}
	return attempt.Count < 5
}

func (a *App) recordLoginFailure(r *http.Request) {
	key := clientKey(r)
	now := time.Now()
	a.loginMu.Lock()
	defer a.loginMu.Unlock()
	attempt := a.loginAttempts[key]
	if now.After(attempt.WindowEnds) {
		attempt = loginAttempt{WindowEnds: now.Add(time.Minute)}
	}
	attempt.Count++
	a.loginAttempts[key] = attempt
}

func (a *App) clearLoginAttempts(r *http.Request) {
	key := clientKey(r)
	a.loginMu.Lock()
	defer a.loginMu.Unlock()
	delete(a.loginAttempts, key)
}

func clientKey(r *http.Request) string {
	if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); forwarded != "" {
		return strings.TrimSpace(strings.Split(forwarded, ",")[0])
	}
	host, _, found := strings.Cut(r.RemoteAddr, ":")
	if found && host != "" {
		return host
	}
	return r.RemoteAddr
}

func highlightSnippet(value, query string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return value
	}
	lower := strings.ToLower(value)
	needle := strings.ToLower(query)
	index := strings.Index(lower, needle)
	if index < 0 {
		return value
	}
	end := index + len(query)
	return value[:index] + "<mark>" + value[index:end] + "</mark>" + value[end:]
}

func ftsQuery(query string) string {
	parts := strings.Fields(query)
	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.Trim(part, `"*'`)
		if part != "" {
			clean = append(clean, `"`+strings.ReplaceAll(part, `"`, `""`)+`"*`)
		}
	}
	if len(clean) == 0 {
		return `""`
	}
	return strings.Join(clean, " AND ")
}

func stripMarks(value, fallback string) string {
	value = strings.ReplaceAll(value, "<mark>", "")
	value = strings.ReplaceAll(value, "</mark>", "")
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
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

func trimDisplay(value string, max int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	if max <= 3 {
		return string(runes[:max])
	}
	return string(runes[:max-3]) + "..."
}
