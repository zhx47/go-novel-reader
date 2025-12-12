package service

import (
	"fmt"
	"sync"

	"go-novel-reader/httpclient"
	"go-novel-reader/model"
	"go-novel-reader/parser"
	"go-novel-reader/storage"
)

// ChapterPreloader 章节预加载器
type ChapterPreloader struct {
	store       storage.CacheStore
	httpClient  *httpclient.Client
	preloadSize int

	mu      sync.Mutex
	loading map[string]bool
}

// NewChapterPreloader 创建章节预加载器
func NewChapterPreloader(store storage.CacheStore, httpClient *httpclient.Client, preloadSize int) *ChapterPreloader {
	if preloadSize <= 0 {
		preloadSize = 3 // 默认预加载3章
	}
	return &ChapterPreloader{
		store:       store,
		httpClient:  httpClient,
		preloadSize: preloadSize,
		loading:     make(map[string]bool),
	}
}

// PreloadAhead 预加载后续章节
// bookID: 书籍ID
// sourceID: 书源ID
// currentIndex: 当前章节索引
// chapters: 章节列表
// rule: 当前书源规则
func (p *ChapterPreloader) PreloadAhead(bookID string, sourceID int, currentIndex int, chapters []*model.Chapter, rule *model.Rule) {
	if p.store == nil || rule == nil || len(chapters) == 0 {
		return
	}

	go func() {
		for i := 1; i <= p.preloadSize; i++ {
			nextIndex := currentIndex + i
			if nextIndex >= len(chapters) {
				break
			}

			// 检查是否已缓存
			_, exists, err := p.store.GetChapterContent(bookID, sourceID, nextIndex)
			if err == nil && exists {
				continue
			}

			// 检查是否正在加载
			key := fmt.Sprintf("%s:%d:%d", bookID, sourceID, nextIndex)
			p.mu.Lock()
			if p.loading[key] {
				p.mu.Unlock()
				continue
			}
			p.loading[key] = true
			p.mu.Unlock()

			// 获取章节内容
			chapter := chapters[nextIndex]
			chapterParser := parser.NewChapterParser(rule, p.httpClient)
			err = chapterParser.Parse(chapter)

			// 完成后移除加载标记
			p.mu.Lock()
			delete(p.loading, key)
			p.mu.Unlock()

			if err != nil {
				continue
			}

			// 保存到缓存
			if chapter.Content != "" {
				p.store.SaveChapterContent(bookID, sourceID, nextIndex, chapter.Content)
			}
		}
	}()
}

// IsLoading 检查指定章节是否正在加载
func (p *ChapterPreloader) IsLoading(bookID string, sourceID int, chapterIndex int) bool {
	key := fmt.Sprintf("%s:%d:%d", bookID, sourceID, chapterIndex)
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.loading[key]
}

// GetLoadingCount 获取当前正在加载的章节数
func (p *ChapterPreloader) GetLoadingCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.loading)
}

// SetPreloadSize 设置预加载数量
func (p *ChapterPreloader) SetPreloadSize(size int) {
	if size > 0 {
		p.preloadSize = size
	}
}
