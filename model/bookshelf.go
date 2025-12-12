package model

import (
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
)

// BookShelf 书架
type BookShelf struct {
	Version   int           `json:"version"`
	Books     []*BookRecord `json:"books"`
	UpdatedAt time.Time     `json:"updated_at"`
}

// BookRecord 书架中的书籍记录
type BookRecord struct {
	// 书籍标识
	ID       string `json:"id"`
	BookName string `json:"book_name"`
	Author   string `json:"author"`

	// 多书源信息
	Sources         []*BookSource `json:"sources"`
	CurrentSourceID int           `json:"current_source_id"`

	// 元信息
	CoverURL  string `json:"cover_url,omitempty"`
	Intro     string `json:"intro,omitempty"`
	Category  string `json:"category,omitempty"`
	Status    string `json:"status,omitempty"`
	WordCount string `json:"word_count,omitempty"`

	// 章节信息
	TotalChapters  int    `json:"total_chapters"`
	LatestChapter  string `json:"latest_chapter,omitempty"`
	LastUpdateTime string `json:"last_update_time,omitempty"`

	// 时间信息
	AddedAt       time.Time `json:"added_at"`
	LastReadAt    time.Time `json:"last_read_at"`
	LastCheckedAt time.Time `json:"last_checked_at,omitempty"`

	// 更新标记
	HasUpdate   bool `json:"has_update,omitempty"`
	NewChapters int  `json:"new_chapters,omitempty"`
}

// BookSource 书籍在某个书源的信息
type BookSource struct {
	SourceID      int    `json:"source_id"`
	SourceName    string `json:"source_name"`
	BookURL       string `json:"book_url"`
	TocURL        string `json:"toc_url,omitempty"`
	TotalChapters int    `json:"total_chapters"`
	LatestChapter string `json:"latest_chapter,omitempty"`
	LastUpdated   string `json:"last_updated,omitempty"`
	IsAvailable   bool   `json:"is_available"`
}

// NewBookShelf 创建空书架
func NewBookShelf() *BookShelf {
	return &BookShelf{
		Version:   1,
		Books:     make([]*BookRecord, 0),
		UpdatedAt: time.Now(),
	}
}

// NewBookRecord 从搜索结果创建书籍记录
func NewBookRecord(result *SearchResult, sourceName string) *BookRecord {
	record := &BookRecord{
		ID:             uuid.New().String(),
		BookName:       strings.TrimSpace(result.BookName),
		Author:         strings.TrimSpace(result.Author),
		Intro:          result.Intro,
		Category:       result.Category,
		Status:         result.Status,
		WordCount:      result.WordCount,
		LatestChapter:  result.LatestChapter,
		LastUpdateTime: result.LastUpdateTime,
		Sources:        make([]*BookSource, 0),
		AddedAt:        time.Now(),
		LastReadAt:     time.Now(),
	}

	// 添加初始书源
	record.AddSource(&BookSource{
		SourceID:      result.SourceID,
		SourceName:    sourceName,
		BookURL:       result.URL,
		LatestChapter: result.LatestChapter,
		LastUpdated:   result.LastUpdateTime,
		IsAvailable:   true,
	})
	record.CurrentSourceID = result.SourceID

	return record
}

// FindBook 根据书名和作者查找书籍
func (bs *BookShelf) FindBook(bookName, author string) *BookRecord {
	normalizedKey := NormalizeBookKey(bookName, author)
	for _, b := range bs.Books {
		if NormalizeBookKey(b.BookName, b.Author) == normalizedKey {
			return b
		}
	}
	return nil
}

// FindBookByID 根据ID查找书籍
func (bs *BookShelf) FindBookByID(id string) *BookRecord {
	for _, b := range bs.Books {
		if b.ID == id {
			return b
		}
	}
	return nil
}

// AddBook 添加书籍到书架
func (bs *BookShelf) AddBook(record *BookRecord) {
	existing := bs.FindBook(record.BookName, record.Author)
	if existing != nil {
		// 合并书源
		for _, src := range record.Sources {
			existing.AddSource(src)
		}
		// 更新元信息（如果现有的为空）
		if existing.Intro == "" && record.Intro != "" {
			existing.Intro = record.Intro
		}
		if existing.CoverURL == "" && record.CoverURL != "" {
			existing.CoverURL = record.CoverURL
		}
		return
	}
	bs.Books = append(bs.Books, record)
	bs.UpdatedAt = time.Now()
}

// RemoveBook 从书架移除书籍
func (bs *BookShelf) RemoveBook(id string) bool {
	for i, b := range bs.Books {
		if b.ID == id {
			bs.Books = append(bs.Books[:i], bs.Books[i+1:]...)
			bs.UpdatedAt = time.Now()
			return true
		}
	}
	return false
}

// GetBooksSortedByLastRead 按最后阅读时间排序获取书籍
func (bs *BookShelf) GetBooksSortedByLastRead() []*BookRecord {
	// 复制切片避免修改原数据
	books := make([]*BookRecord, len(bs.Books))
	copy(books, bs.Books)

	// 按最后阅读时间降序排序
	for i := 0; i < len(books)-1; i++ {
		for j := i + 1; j < len(books); j++ {
			if books[j].LastReadAt.After(books[i].LastReadAt) {
				books[i], books[j] = books[j], books[i]
			}
		}
	}
	return books
}

// AddSource 为书籍添加新书源
func (br *BookRecord) AddSource(source *BookSource) {
	for i, s := range br.Sources {
		if s.SourceID == source.SourceID {
			br.Sources[i] = source
			return
		}
	}
	br.Sources = append(br.Sources, source)
}

// GetCurrentSource 获取当前使用的书源
func (br *BookRecord) GetCurrentSource() *BookSource {
	for _, s := range br.Sources {
		if s.SourceID == br.CurrentSourceID {
			return s
		}
	}
	if len(br.Sources) > 0 {
		return br.Sources[0]
	}
	return nil
}

// SwitchSource 切换书源
func (br *BookRecord) SwitchSource(sourceID int) bool {
	for _, s := range br.Sources {
		if s.SourceID == sourceID {
			br.CurrentSourceID = sourceID
			return true
		}
	}
	return false
}

// GetAvailableSources 获取所有可用书源
func (br *BookRecord) GetAvailableSources() []*BookSource {
	var available []*BookSource
	for _, s := range br.Sources {
		if s.IsAvailable {
			available = append(available, s)
		}
	}
	return available
}

// UpdateLastRead 更新最后阅读时间
func (br *BookRecord) UpdateLastRead() {
	br.LastReadAt = time.Now()
}

// MarkUpdate 标记有更新
func (br *BookRecord) MarkUpdate(newChapters int, latestChapter string) {
	br.HasUpdate = true
	br.NewChapters = newChapters
	br.LatestChapter = latestChapter
	br.LastCheckedAt = time.Now()
}

// ClearUpdateMark 清除更新标记
func (br *BookRecord) ClearUpdateMark() {
	br.HasUpdate = false
	br.NewChapters = 0
}

// NormalizeBookKey 标准化书名+作者作为唯一键
func NormalizeBookKey(bookName, author string) string {
	bookName = removeAllWhitespace(bookName)
	author = removeAllWhitespace(author)
	bookName = strings.ToLower(bookName)
	author = strings.ToLower(author)
	bookName = removeDecorations(bookName)
	return bookName + "|" + author
}

// removeAllWhitespace 移除所有空白字符
func removeAllWhitespace(s string) string {
	var result strings.Builder
	for _, r := range s {
		if !unicode.IsSpace(r) {
			result.WriteRune(r)
		}
	}
	return result.String()
}

// removeDecorations 移除常见修饰字符
func removeDecorations(s string) string {
	decorations := []string{
		"【", "】", "[", "]", "(", ")", "（", "）",
		"《", "》", "「", "」", "『", "』",
		"完本", "全本", "精校版", "出版", "网络版",
	}
	for _, d := range decorations {
		s = strings.ReplaceAll(s, d, "")
	}
	return s
}
