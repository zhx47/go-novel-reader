package model

import "time"

// ReadingProgress 阅读进度
type ReadingProgress struct {
	BookID       string    `json:"book_id"`
	SourceID     int       `json:"source_id"`
	ChapterIndex int       `json:"chapter_index"`
	ChapterTitle string    `json:"chapter_title"`
	ChapterURL   string    `json:"chapter_url,omitempty"`
	LineOffset   int       `json:"line_offset"`
	Percentage   float64   `json:"percentage"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// ProgressStore 进度存储
type ProgressStore struct {
	Version    int                         `json:"version"`
	Progresses map[string]*ReadingProgress `json:"progresses"`
	UpdatedAt  time.Time                   `json:"updated_at"`
}

// NewProgressStore 创建进度存储
func NewProgressStore() *ProgressStore {
	return &ProgressStore{
		Version:    1,
		Progresses: make(map[string]*ReadingProgress),
		UpdatedAt:  time.Now(),
	}
}

// GetProgress 获取书籍阅读进度
func (ps *ProgressStore) GetProgress(bookID string) *ReadingProgress {
	return ps.Progresses[bookID]
}

// SaveProgress 保存阅读进度
func (ps *ProgressStore) SaveProgress(progress *ReadingProgress) {
	progress.UpdatedAt = time.Now()
	ps.Progresses[progress.BookID] = progress
	ps.UpdatedAt = time.Now()
}

// RemoveProgress 删除阅读进度
func (ps *ProgressStore) RemoveProgress(bookID string) {
	delete(ps.Progresses, bookID)
	ps.UpdatedAt = time.Now()
}

// NewReadingProgress 创建新的阅读进度
func NewReadingProgress(bookID string, sourceID int, chapterIndex int, chapterTitle string) *ReadingProgress {
	return &ReadingProgress{
		BookID:       bookID,
		SourceID:     sourceID,
		ChapterIndex: chapterIndex,
		ChapterTitle: chapterTitle,
		LineOffset:   0,
		Percentage:   0,
		UpdatedAt:    time.Now(),
	}
}

// UpdatePosition 更新阅读位置
func (rp *ReadingProgress) UpdatePosition(lineOffset int, totalLines int) {
	rp.LineOffset = lineOffset
	if totalLines > 0 {
		rp.Percentage = float64(lineOffset) / float64(totalLines) * 100
	}
	rp.UpdatedAt = time.Now()
}

// UpdateChapter 更新章节
func (rp *ReadingProgress) UpdateChapter(chapterIndex int, chapterTitle string, chapterURL string) {
	rp.ChapterIndex = chapterIndex
	rp.ChapterTitle = chapterTitle
	rp.ChapterURL = chapterURL
	rp.LineOffset = 0
	rp.Percentage = 0
	rp.UpdatedAt = time.Now()
}

// SwitchSource 切换书源时更新
func (rp *ReadingProgress) SwitchSource(sourceID int, newChapterIndex int, newChapterTitle string) {
	rp.SourceID = sourceID
	rp.ChapterIndex = newChapterIndex
	rp.ChapterTitle = newChapterTitle
	rp.LineOffset = 0
	rp.Percentage = 0
	rp.UpdatedAt = time.Now()
}
