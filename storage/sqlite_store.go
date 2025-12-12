package storage

import (
	"database/sql"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"

	"go-novel-reader/model"
)

const (
	dbFileName = "novel.db"
)

// SQLiteStore SQLite存储实现
type SQLiteStore struct {
	db      *sql.DB
	dataDir string
	dbPath  string

	// 缓存
	bookshelfCache *model.BookShelf
	progressCache  *model.ProgressStore

	// 并发保护
	mu sync.RWMutex
}

// NewSQLiteStore 创建SQLite存储
func NewSQLiteStore(dataDir string) (*SQLiteStore, error) {
	// 确保目录存在
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, err
	}

	dbPath := filepath.Join(dataDir, dbFileName)
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	store := &SQLiteStore{
		db:      db,
		dataDir: dataDir,
		dbPath:  dbPath,
	}

	// 初始化数据库
	if err := store.initSchema(); err != nil {
		db.Close()
		return nil, err
	}

	return store, nil
}

// initSchema 初始化数据库表结构
func (s *SQLiteStore) initSchema() error {
	// 启用外键约束和WAL模式
	pragmas := []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA journal_mode = WAL",
	}
	for _, pragma := range pragmas {
		if _, err := s.db.Exec(pragma); err != nil {
			return err
		}
	}

	schema := `
	-- 书架表
	CREATE TABLE IF NOT EXISTS books (
		id              TEXT PRIMARY KEY,
		book_name       TEXT NOT NULL,
		author          TEXT NOT NULL,
		current_source_id INTEGER NOT NULL DEFAULT 0,
		cover_url       TEXT,
		intro           TEXT,
		category        TEXT,
		status          TEXT,
		word_count      TEXT,
		total_chapters  INTEGER NOT NULL DEFAULT 0,
		latest_chapter  TEXT,
		last_update_time TEXT,
		added_at        INTEGER NOT NULL,
		last_read_at    INTEGER NOT NULL,
		last_checked_at INTEGER,
		has_update      INTEGER NOT NULL DEFAULT 0,
		new_chapters    INTEGER NOT NULL DEFAULT 0,
		UNIQUE(book_name, author)
	);

	-- 书源表
	CREATE TABLE IF NOT EXISTS book_sources (
		id              INTEGER PRIMARY KEY AUTOINCREMENT,
		book_id         TEXT NOT NULL,
		source_id       INTEGER NOT NULL,
		source_name     TEXT NOT NULL,
		book_url        TEXT NOT NULL,
		toc_url         TEXT,
		total_chapters  INTEGER NOT NULL DEFAULT 0,
		latest_chapter  TEXT,
		last_updated    TEXT,
		is_available    INTEGER NOT NULL DEFAULT 1,
		FOREIGN KEY (book_id) REFERENCES books(id) ON DELETE CASCADE,
		UNIQUE(book_id, source_id)
	);

	-- 阅读进度表
	CREATE TABLE IF NOT EXISTS reading_progress (
		book_id       TEXT PRIMARY KEY,
		source_id     INTEGER NOT NULL,
		chapter_index INTEGER NOT NULL,
		chapter_title TEXT NOT NULL,
		chapter_url   TEXT,
		line_offset   INTEGER NOT NULL DEFAULT 0,
		percentage    REAL NOT NULL DEFAULT 0,
		updated_at    INTEGER NOT NULL,
		FOREIGN KEY (book_id) REFERENCES books(id) ON DELETE CASCADE
	);

	-- 目录缓存表
	CREATE TABLE IF NOT EXISTS toc_cache (
		id            INTEGER PRIMARY KEY AUTOINCREMENT,
		book_id       TEXT NOT NULL,
		source_id     INTEGER NOT NULL,
		chapter_index INTEGER NOT NULL,
		chapter_title TEXT NOT NULL,
		chapter_url   TEXT NOT NULL,
		cached_at     INTEGER NOT NULL,
		FOREIGN KEY (book_id) REFERENCES books(id) ON DELETE CASCADE,
		UNIQUE(book_id, source_id, chapter_index)
	);

	-- 章节内容缓存表
	CREATE TABLE IF NOT EXISTS chapter_cache (
		book_id       TEXT NOT NULL,
		source_id     INTEGER NOT NULL,
		chapter_index INTEGER NOT NULL,
		content       TEXT NOT NULL,
		cached_at     INTEGER NOT NULL,
		PRIMARY KEY (book_id, source_id, chapter_index),
		FOREIGN KEY (book_id) REFERENCES books(id) ON DELETE CASCADE
	);

	-- 索引
	CREATE INDEX IF NOT EXISTS idx_books_last_read ON books(last_read_at DESC);
	CREATE INDEX IF NOT EXISTS idx_book_sources_book_id ON book_sources(book_id);
	CREATE INDEX IF NOT EXISTS idx_toc_cache_book_source ON toc_cache(book_id, source_id);
	CREATE INDEX IF NOT EXISTS idx_chapter_cache_book_source ON chapter_cache(book_id, source_id);
	`

	_, err := s.db.Exec(schema)
	return err
}

// LoadBookShelf 加载书架
func (s *SQLiteStore) LoadBookShelf() (*model.BookShelf, error) {
	s.mu.RLock()
	if s.bookshelfCache != nil {
		defer s.mu.RUnlock()
		return s.bookshelfCache, nil
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()

	// 双重检查
	if s.bookshelfCache != nil {
		return s.bookshelfCache, nil
	}

	shelf := model.NewBookShelf()

	rows, err := s.db.Query(`
		SELECT id, book_name, author, current_source_id, cover_url, intro, category,
		       status, word_count, total_chapters, latest_chapter, last_update_time,
		       added_at, last_read_at, last_checked_at, has_update, new_chapters
		FROM books
		ORDER BY last_read_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var book model.BookRecord
		var addedAtUnix, lastReadAtUnix int64
		var lastCheckedAtUnix sql.NullInt64
		var coverURL, intro, category, status, wordCount, latestChapter, lastUpdateTime sql.NullString
		var hasUpdate int

		err := rows.Scan(
			&book.ID,
			&book.BookName,
			&book.Author,
			&book.CurrentSourceID,
			&coverURL,
			&intro,
			&category,
			&status,
			&wordCount,
			&book.TotalChapters,
			&latestChapter,
			&lastUpdateTime,
			&addedAtUnix,
			&lastReadAtUnix,
			&lastCheckedAtUnix,
			&hasUpdate,
			&book.NewChapters,
		)
		if err != nil {
			return nil, err
		}

		book.CoverURL = nullStringToString(coverURL)
		book.Intro = nullStringToString(intro)
		book.Category = nullStringToString(category)
		book.Status = nullStringToString(status)
		book.WordCount = nullStringToString(wordCount)
		book.LatestChapter = nullStringToString(latestChapter)
		book.LastUpdateTime = nullStringToString(lastUpdateTime)
		book.AddedAt = time.Unix(addedAtUnix, 0)
		book.LastReadAt = time.Unix(lastReadAtUnix, 0)
		if lastCheckedAtUnix.Valid {
			book.LastCheckedAt = time.Unix(lastCheckedAtUnix.Int64, 0)
		}
		book.HasUpdate = hasUpdate != 0

		// 加载书源
		sources, err := s.loadBookSources(book.ID)
		if err != nil {
			return nil, err
		}
		book.Sources = sources

		shelf.Books = append(shelf.Books, &book)
	}

	s.bookshelfCache = shelf
	return shelf, nil
}

// loadBookSources 加载书籍的书源
func (s *SQLiteStore) loadBookSources(bookID string) ([]*model.BookSource, error) {
	rows, err := s.db.Query(`
		SELECT source_id, source_name, book_url, toc_url, total_chapters,
		       latest_chapter, last_updated, is_available
		FROM book_sources
		WHERE book_id = ?
		ORDER BY id
	`, bookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sources []*model.BookSource
	for rows.Next() {
		var src model.BookSource
		var tocURL, latestChapter, lastUpdated sql.NullString
		var isAvailable int

		err := rows.Scan(
			&src.SourceID,
			&src.SourceName,
			&src.BookURL,
			&tocURL,
			&src.TotalChapters,
			&latestChapter,
			&lastUpdated,
			&isAvailable,
		)
		if err != nil {
			return nil, err
		}

		src.TocURL = nullStringToString(tocURL)
		src.LatestChapter = nullStringToString(latestChapter)
		src.LastUpdated = nullStringToString(lastUpdated)
		src.IsAvailable = isAvailable != 0

		sources = append(sources, &src)
	}

	return sources, nil
}

// SaveBookShelf 保存书架
func (s *SQLiteStore) SaveBookShelf(shelf *model.BookShelf) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 获取数据库中现有的书籍ID
	existingIDs := make(map[string]bool)
	rows, err := tx.Query("SELECT id FROM books")
	if err != nil {
		return err
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		existingIDs[id] = true
	}
	rows.Close()

	// 获取书架中的书籍ID
	shelfIDs := make(map[string]bool)
	for _, book := range shelf.Books {
		shelfIDs[book.ID] = true
	}

	// 删除不在书架中的书籍
	for id := range existingIDs {
		if !shelfIDs[id] {
			if _, err := tx.Exec("DELETE FROM books WHERE id = ?", id); err != nil {
				return err
			}
		}
	}

	// 保存/更新书籍
	for _, book := range shelf.Books {
		var lastCheckedAt interface{}
		if !book.LastCheckedAt.IsZero() {
			lastCheckedAt = book.LastCheckedAt.Unix()
		}

		hasUpdate := 0
		if book.HasUpdate {
			hasUpdate = 1
		}

		_, err := tx.Exec(`
			INSERT OR REPLACE INTO books (
				id, book_name, author, current_source_id, cover_url, intro, category,
				status, word_count, total_chapters, latest_chapter, last_update_time,
				added_at, last_read_at, last_checked_at, has_update, new_chapters
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`,
			book.ID, book.BookName, book.Author, book.CurrentSourceID,
			stringToNullString(book.CoverURL),
			stringToNullString(book.Intro),
			stringToNullString(book.Category),
			stringToNullString(book.Status),
			stringToNullString(book.WordCount),
			book.TotalChapters,
			stringToNullString(book.LatestChapter),
			stringToNullString(book.LastUpdateTime),
			book.AddedAt.Unix(), book.LastReadAt.Unix(), lastCheckedAt,
			hasUpdate, book.NewChapters,
		)
		if err != nil {
			return err
		}

		// 删除旧书源
		_, err = tx.Exec("DELETE FROM book_sources WHERE book_id = ?", book.ID)
		if err != nil {
			return err
		}

		// 插入新书源
		for _, src := range book.Sources {
			isAvailable := 0
			if src.IsAvailable {
				isAvailable = 1
			}

			_, err = tx.Exec(`
				INSERT INTO book_sources (
					book_id, source_id, source_name, book_url, toc_url,
					total_chapters, latest_chapter, last_updated, is_available
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
			`,
				book.ID, src.SourceID, src.SourceName, src.BookURL,
				stringToNullString(src.TocURL),
				src.TotalChapters,
				stringToNullString(src.LatestChapter),
				stringToNullString(src.LastUpdated),
				isAvailable,
			)
			if err != nil {
				return err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	s.bookshelfCache = shelf
	return nil
}

// LoadProgress 加载进度
func (s *SQLiteStore) LoadProgress() (*model.ProgressStore, error) {
	s.mu.RLock()
	if s.progressCache != nil {
		defer s.mu.RUnlock()
		return s.progressCache, nil
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.progressCache != nil {
		return s.progressCache, nil
	}

	store := model.NewProgressStore()

	rows, err := s.db.Query(`
		SELECT book_id, source_id, chapter_index, chapter_title, chapter_url,
		       line_offset, percentage, updated_at
		FROM reading_progress
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var p model.ReadingProgress
		var chapterURL sql.NullString
		var updatedAtUnix int64

		err := rows.Scan(
			&p.BookID,
			&p.SourceID,
			&p.ChapterIndex,
			&p.ChapterTitle,
			&chapterURL,
			&p.LineOffset,
			&p.Percentage,
			&updatedAtUnix,
		)
		if err != nil {
			return nil, err
		}

		p.ChapterURL = nullStringToString(chapterURL)
		p.UpdatedAt = time.Unix(updatedAtUnix, 0)
		store.Progresses[p.BookID] = &p
	}

	s.progressCache = store
	return store, nil
}

// SaveProgress 保存进度
func (s *SQLiteStore) SaveProgress(store *model.ProgressStore) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 获取有效的 book_id 列表
	validBookIDs := make(map[string]bool)
	rows, err := tx.Query("SELECT id FROM books")
	if err != nil {
		return err
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		validBookIDs[id] = true
	}
	rows.Close()

	// 清空旧数据
	_, err = tx.Exec("DELETE FROM reading_progress")
	if err != nil {
		return err
	}

	// 插入新数据（只插入有效的进度记录）
	for _, p := range store.Progresses {
		// 跳过无效的进度记录（book_id 不存在于 books 表中）
		if !validBookIDs[p.BookID] {
			continue
		}

		_, err := tx.Exec(`
			INSERT INTO reading_progress (
				book_id, source_id, chapter_index, chapter_title, chapter_url,
				line_offset, percentage, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`,
			p.BookID, p.SourceID, p.ChapterIndex, p.ChapterTitle,
			stringToNullString(p.ChapterURL),
			p.LineOffset, p.Percentage, p.UpdatedAt.Unix(),
		)
		if err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	s.progressCache = store
	return nil
}

// SaveReadingProgress 便捷方法：保存单本书的阅读进度
func (s *SQLiteStore) SaveReadingProgress(progress *model.ReadingProgress) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	progress.UpdatedAt = time.Now()

	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO reading_progress (
			book_id, source_id, chapter_index, chapter_title, chapter_url,
			line_offset, percentage, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`,
		progress.BookID, progress.SourceID, progress.ChapterIndex, progress.ChapterTitle,
		stringToNullString(progress.ChapterURL),
		progress.LineOffset, progress.Percentage, progress.UpdatedAt.Unix(),
	)
	if err != nil {
		return err
	}

	// 更新缓存
	if s.progressCache != nil {
		s.progressCache.Progresses[progress.BookID] = progress
		s.progressCache.UpdatedAt = time.Now()
	}

	return nil
}

// GetReadingProgress 获取单本书的阅读进度
func (s *SQLiteStore) GetReadingProgress(bookID string) (*model.ReadingProgress, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var p model.ReadingProgress
	var chapterURL sql.NullString
	var updatedAtUnix int64

	err := s.db.QueryRow(`
		SELECT book_id, source_id, chapter_index, chapter_title, chapter_url,
		       line_offset, percentage, updated_at
		FROM reading_progress
		WHERE book_id = ?
	`, bookID).Scan(
		&p.BookID,
		&p.SourceID,
		&p.ChapterIndex,
		&p.ChapterTitle,
		&chapterURL,
		&p.LineOffset,
		&p.Percentage,
		&updatedAtUnix,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	p.ChapterURL = nullStringToString(chapterURL)
	p.UpdatedAt = time.Unix(updatedAtUnix, 0)
	return &p, nil
}

// GetDataDir 获取数据目录
func (s *SQLiteStore) GetDataDir() string {
	return s.dataDir
}

// InvalidateCache 清除缓存
func (s *SQLiteStore) InvalidateCache() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bookshelfCache = nil
	s.progressCache = nil
}

// Close 关闭数据库连接
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// === 辅助函数 ===

func nullStringToString(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return ""
}

func stringToNullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

// === 章节内容缓存方法 ===

// GetChapterContent 获取缓存的章节内容
func (s *SQLiteStore) GetChapterContent(bookID string, sourceID int, chapterIndex int) (string, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var content string
	err := s.db.QueryRow(`
		SELECT content FROM chapter_cache
		WHERE book_id = ? AND source_id = ? AND chapter_index = ?
	`, bookID, sourceID, chapterIndex).Scan(&content)

	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}

	return content, true, nil
}

// SaveChapterContent 保存章节内容到缓存
func (s *SQLiteStore) SaveChapterContent(bookID string, sourceID int, chapterIndex int, content string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO chapter_cache (book_id, source_id, chapter_index, content, cached_at)
		VALUES (?, ?, ?, ?, ?)
	`, bookID, sourceID, chapterIndex, content, time.Now().Unix())

	return err
}

// DeleteBookCache 删除指定书籍的所有缓存（章节内容和目录）
func (s *SQLiteStore) DeleteBookCache(bookID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 删除章节内容缓存
	_, err = tx.Exec(`DELETE FROM chapter_cache WHERE book_id = ?`, bookID)
	if err != nil {
		return err
	}

	// 删除目录缓存
	_, err = tx.Exec(`DELETE FROM toc_cache WHERE book_id = ?`, bookID)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// DeleteSourceCache 删除指定书籍特定书源的缓存
func (s *SQLiteStore) DeleteSourceCache(bookID string, sourceID int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`DELETE FROM chapter_cache WHERE book_id = ? AND source_id = ?`, bookID, sourceID)
	if err != nil {
		return err
	}

	_, err = tx.Exec(`DELETE FROM toc_cache WHERE book_id = ? AND source_id = ?`, bookID, sourceID)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// CacheStats 缓存统计信息
type CacheStats struct {
	BookCount         int   // 缓存的书籍数
	TocCacheCount     int   // 目录缓存条目数
	ChapterCacheCount int   // 章节缓存条目数
	TotalSize         int64 // 章节内容总大小（字节）
}

// GetCacheStats 获取缓存统计信息
func (s *SQLiteStore) GetCacheStats() (CacheStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var stats CacheStats

	// 统计缓存的书籍数
	err := s.db.QueryRow(`
		SELECT COUNT(DISTINCT book_id) FROM chapter_cache
	`).Scan(&stats.BookCount)
	if err != nil && err != sql.ErrNoRows {
		return stats, err
	}

	// 统计目录缓存条目数
	err = s.db.QueryRow(`
		SELECT COUNT(*) FROM toc_cache
	`).Scan(&stats.TocCacheCount)
	if err != nil && err != sql.ErrNoRows {
		return stats, err
	}

	// 统计章节缓存条目数和总大小
	err = s.db.QueryRow(`
		SELECT COUNT(*), COALESCE(SUM(LENGTH(content)), 0) FROM chapter_cache
	`).Scan(&stats.ChapterCacheCount, &stats.TotalSize)
	if err != nil && err != sql.ErrNoRows {
		return stats, err
	}

	return stats, nil
}

// ClearAllCache 清除所有缓存
func (s *SQLiteStore) ClearAllCache() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`DELETE FROM chapter_cache`)
	if err != nil {
		return err
	}

	_, err = tx.Exec(`DELETE FROM toc_cache`)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// === 目录缓存方法 ===

// TocCacheItem 目录缓存项
type TocCacheItem struct {
	Index int    `json:"index"`
	Title string `json:"title"`
	URL   string `json:"url"`
}

// GetTocCache 获取缓存的目录
func (s *SQLiteStore) GetTocCache(bookID string, sourceID int) ([]TocCacheItem, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT chapter_index, chapter_title, chapter_url
		FROM toc_cache
		WHERE book_id = ? AND source_id = ?
		ORDER BY chapter_index
	`, bookID, sourceID)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()

	var items []TocCacheItem
	for rows.Next() {
		var item TocCacheItem
		if err := rows.Scan(&item.Index, &item.Title, &item.URL); err != nil {
			return nil, false, err
		}
		items = append(items, item)
	}

	if len(items) == 0 {
		return nil, false, nil
	}

	return items, true, nil
}

// SaveTocCache 保存目录到缓存
func (s *SQLiteStore) SaveTocCache(bookID string, sourceID int, items []TocCacheItem) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 删除旧缓存
	_, err = tx.Exec(`DELETE FROM toc_cache WHERE book_id = ? AND source_id = ?`, bookID, sourceID)
	if err != nil {
		return err
	}

	// 插入新缓存
	stmt, err := tx.Prepare(`
		INSERT INTO toc_cache (book_id, source_id, chapter_index, chapter_title, chapter_url, cached_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	now := time.Now().Unix()
	for _, item := range items {
		_, err = stmt.Exec(bookID, sourceID, item.Index, item.Title, item.URL, now)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// GetTocCacheTime 获取目录缓存的时间
func (s *SQLiteStore) GetTocCacheTime(bookID string, sourceID int) (time.Time, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var cachedAtUnix int64
	err := s.db.QueryRow(`
		SELECT cached_at FROM toc_cache
		WHERE book_id = ? AND source_id = ?
		LIMIT 1
	`, bookID, sourceID).Scan(&cachedAtUnix)

	if err == sql.ErrNoRows {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}

	return time.Unix(cachedAtUnix, 0), true, nil
}
