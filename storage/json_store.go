package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"go-novel-reader/model"
)

const (
	bookshelfFile = "bookshelf.json"
	progressFile  = "progress.json"
)

// JSONStore JSON文件存储实现
type JSONStore struct {
	dataDir       string
	bookshelfPath string
	progressPath  string

	// 缓存
	bookshelfCache *model.BookShelf
	progressCache  *model.ProgressStore

	// 并发保护
	mu sync.RWMutex
}

// NewJSONStore 创建JSON存储
func NewJSONStore(dataDir string) (*JSONStore, error) {
	// 确保目录存在
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, err
	}

	return &JSONStore{
		dataDir:       dataDir,
		bookshelfPath: filepath.Join(dataDir, bookshelfFile),
		progressPath:  filepath.Join(dataDir, progressFile),
	}, nil
}

// LoadBookShelf 加载书架
func (s *JSONStore) LoadBookShelf() (*model.BookShelf, error) {
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

	data, err := os.ReadFile(s.bookshelfPath)
	if os.IsNotExist(err) {
		s.bookshelfCache = shelf
		return shelf, nil
	}
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(data, shelf); err != nil {
		return nil, err
	}

	s.bookshelfCache = shelf
	return shelf, nil
}

// SaveBookShelf 保存书架
func (s *JSONStore) SaveBookShelf(shelf *model.BookShelf) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.MarshalIndent(shelf, "", "  ")
	if err != nil {
		return err
	}

	// 先写临时文件，再原子替换
	tmpPath := s.bookshelfPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}

	if err := os.Rename(tmpPath, s.bookshelfPath); err != nil {
		os.Remove(tmpPath)
		return err
	}

	s.bookshelfCache = shelf
	return nil
}

// LoadProgress 加载进度
func (s *JSONStore) LoadProgress() (*model.ProgressStore, error) {
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

	data, err := os.ReadFile(s.progressPath)
	if os.IsNotExist(err) {
		s.progressCache = store
		return store, nil
	}
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(data, store); err != nil {
		return nil, err
	}

	s.progressCache = store
	return store, nil
}

// SaveProgress 保存进度
func (s *JSONStore) SaveProgress(store *model.ProgressStore) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}

	tmpPath := s.progressPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}

	if err := os.Rename(tmpPath, s.progressPath); err != nil {
		os.Remove(tmpPath)
		return err
	}

	s.progressCache = store
	return nil
}

// SaveReadingProgress 便捷方法：保存单本书的阅读进度
func (s *JSONStore) SaveReadingProgress(progress *model.ReadingProgress) error {
	store, err := s.LoadProgress()
	if err != nil {
		return err
	}
	store.SaveProgress(progress)
	return s.SaveProgress(store)
}

// GetReadingProgress 获取单本书的阅读进度
func (s *JSONStore) GetReadingProgress(bookID string) (*model.ReadingProgress, error) {
	store, err := s.LoadProgress()
	if err != nil {
		return nil, err
	}
	return store.GetProgress(bookID), nil
}

// GetDataDir 获取数据目录
func (s *JSONStore) GetDataDir() string {
	return s.dataDir
}

// InvalidateCache 清除缓存
func (s *JSONStore) InvalidateCache() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bookshelfCache = nil
	s.progressCache = nil
}
