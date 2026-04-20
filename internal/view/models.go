package view

import "fmt"

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
}

type HomeData struct {
	Categories []CategoryView
	Feeds      []FeedView
	Articles   []ArticleView
	Unread     int64
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
	Category  string
	ReadTime  string
	IsRead    bool
	IsStarred bool
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
	return state
}
