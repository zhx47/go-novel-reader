package storage

import (
	"go-novel-reader/model"
	"time"
)

// Store 存储接口
type Store interface {
	// 书架操作
	LoadBookShelf() (*model.BookShelf, error)
	SaveBookShelf(shelf *model.BookShelf) error

	// 进度操作
	LoadProgress() (*model.ProgressStore, error)
	SaveProgress(store *model.ProgressStore) error

	// 便捷方法
	SaveReadingProgress(progress *model.ReadingProgress) error
	GetReadingProgress(bookID string) (*model.ReadingProgress, error)

	// 路径获取
	GetDataDir() string

	// 缓存管理
	InvalidateCache()
}

// CacheStore 缓存存储接口（SQLite 特有）
type CacheStore interface {
	Store

	// 章节内容缓存
	GetChapterContent(bookID string, sourceID int, chapterIndex int) (content string, exists bool, err error)
	SaveChapterContent(bookID string, sourceID int, chapterIndex int, content string) error
	DeleteBookCache(bookID string) error
	DeleteSourceCache(bookID string, sourceID int) error
	GetCacheStats() (CacheStats, error)
	ClearAllCache() error

	// 目录缓存
	GetTocCache(bookID string, sourceID int) (items []TocCacheItem, exists bool, err error)
	SaveTocCache(bookID string, sourceID int, items []TocCacheItem) error
	GetTocCacheTime(bookID string, sourceID int) (cachedAt time.Time, exists bool, err error)
}
