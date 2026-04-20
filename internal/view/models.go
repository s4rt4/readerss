package view

import (
	"encoding/json"
	"fmt"
	"strings"
)

type CategoryView struct {
	ID    int64
	Name  string
	Count int
}

type FeedView struct {
	ID          int64
	Title       string
	URL         string
	SiteURL     string
	CategoryID  int64
	Category    string
	Interval    int64
	Status      string
	StatusTone  string
	UnreadCount int
	ErrorCount  int64
	LastFetched string
}

type HomeData struct {
	Categories []CategoryView
	Feeds      []FeedView
	Boards     []BoardView
	Articles   []ArticleView
	Unread     int64
	All        int64
	Starred    int64
	ReadLater  int64
	Errors     int
	Filter     string
}

type FeedManagementData struct {
	Categories []CategoryView
	Feeds      []FeedView
	Form       FeedFormData
	Error      string
	Notice     string
}

type FeedFormData struct {
	ID         int64
	URL        string
	SiteURL    string
	Title      string
	CategoryID int64
	Interval   int64
}

type ArticleView struct {
	ID        int64
	FeedID    int64
	Title     string
	URL       string
	Source    string
	Time      string
	Summary   string
	Content   string
	ImageURL  string
	Category  string
	ReadTime  string
	IsRead    bool
	IsStarred bool
	ReadLater bool
}

type BoardView struct {
	ID          int64
	Name        string
	Description string
	Count       int64
	CreatedAt   string
}

type BoardsData struct {
	Boards []BoardView
	Notice string
	Error  string
}

type BoardDetailData struct {
	Board    BoardView
	Articles []ArticleView
	Boards   []BoardView
}

type SearchData struct {
	Query   string
	Results []SearchResultView
}

type SearchResultView struct {
	Title   string
	Source  string
	Snippet string
	Tag     string
	Time    string
	URL     string
}

type LoginData struct {
	Next  string
	Error string
}

type SettingsData struct {
	DefaultFetchInterval int64
	RetentionDays        int64
	Theme                string
	Density              string
	RespectCacheHeaders  bool
	FilterRules          []FilterRuleView
	Feeds                []FeedView
	Notice               string
	Error                string
}

type FilterRuleView struct {
	ID        int64
	FeedID    int64
	FeedTitle string
	MatchType string
	Pattern   string
	Action    string
}

type FeedHealthData struct {
	Feeds       []FeedView
	Healthy     int
	Warnings    int
	Errors      int
	Total       int
	LastChecked string
}

func (f FeedFormData) IsEditing() bool {
	return f.ID > 0
}

func (f FeedFormData) Action() string {
	if f.IsEditing() {
		return fmt.Sprintf("/feeds/%d", f.ID)
	}
	return "/feeds"
}

func (f FeedFormData) SubmitLabel() string {
	if f.IsEditing() {
		return "Save feed"
	}
	return "Subscribe feed"
}

func initials(value string) string {
	runes := []rune(value)
	if len(runes) == 0 {
		return "FD"
	}
	if len(runes) == 1 {
		return string(runes[0])
	}
	return string(runes[:2])
}

func selectedInt(value, current int64) bool {
	return value == current
}

func selectedString(value, current string) bool {
	return value == current
}

func checkedBool(value bool) bool {
	return value
}

func categoryDotClass(index int) string {
	classes := []string{"dot dot-blue", "dot dot-green", "dot dot-amber"}
	return classes[index%len(classes)]
}

func articleState(article ArticleView, active bool) string {
	state := ""
	if active {
		state += "active "
	}
	if !article.IsRead {
		state += "unread "
	}
	if article.IsStarred {
		state += "starred "
	}
	if article.ReadLater {
		state += "read-later "
	}
	return state
}

func filterSelected(filter, current string) bool {
	return filter == current
}

func navState(filter, current string) string {
	if filter == current {
		return "active"
	}
	return ""
}

func articleReadAction(article ArticleView) string {
	if article.IsRead {
		return fmt.Sprintf("/articles/%d/unread", article.ID)
	}
	return fmt.Sprintf("/articles/%d/read", article.ID)
}

func articleStarAction(article ArticleView) string {
	if article.IsStarred {
		return fmt.Sprintf("/articles/%d/unstar", article.ID)
	}
	return fmt.Sprintf("/articles/%d/star", article.ID)
}

func articleReadLaterAction(article ArticleView) string {
	if article.ReadLater {
		return fmt.Sprintf("/articles/%d/read-later/remove", article.ID)
	}
	return fmt.Sprintf("/articles/%d/read-later", article.ID)
}

func articleBoardAction(article ArticleView) string {
	return fmt.Sprintf("/articles/%d/boards", article.ID)
}

func boardLink(board BoardView) string {
	return fmt.Sprintf("/boards/%d", board.ID)
}

func boardDeleteAction(board BoardView) string {
	return fmt.Sprintf("/boards/%d/delete", board.ID)
}

func boardRemoveArticleAction(board BoardView, article ArticleView) string {
	return fmt.Sprintf("/boards/%d/articles/%d/remove", board.ID, article.ID)
}

func articleOriginalURL(articles []ArticleView) string {
	if len(articles) == 0 || articles[0].URL == "" {
		return "#"
	}
	return articles[0].URL
}

func articleParagraphs(content, fallback string) []string {
	content = strings.TrimSpace(content)
	if content == "" {
		content = fallback
	}
	parts := strings.Split(content, "\n")
	paragraphs := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			paragraphs = append(paragraphs, part)
		}
	}
	if len(paragraphs) == 0 && strings.TrimSpace(fallback) != "" {
		paragraphs = append(paragraphs, strings.TrimSpace(fallback))
	}
	return paragraphs
}

func articleJSONParagraphs(content, fallback string) string {
	encoded, err := json.Marshal(articleParagraphs(content, fallback))
	if err != nil {
		return "[]"
	}
	return string(encoded)
}

func percent(part, total int) string {
	if total == 0 {
		return "0%"
	}
	return fmt.Sprintf("%d%%", part*100/total)
}
