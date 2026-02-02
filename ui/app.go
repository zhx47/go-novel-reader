package ui

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"go-novel-reader/config"
	"go-novel-reader/httpclient"
	"go-novel-reader/model"
	"go-novel-reader/parser"
	"go-novel-reader/service"
	"go-novel-reader/source"
	"go-novel-reader/storage"
)

// 应用状态
type AppState int

const (
	StateMainMenu     AppState = iota // 主菜单
	StateBookShelf                    // 书架
	StateSearch                       // 搜索
	StateSearchResult                 // 搜索结果
	StateToc                          // 目录
	StateReader                       // 阅读
	StateSourceSwitch                 // 换源
)

// Model 应用模型
type Model struct {
	state  AppState
	width  int
	height int

	// 配置
	config *config.AppConfig

	// 存储
	store storage.Store

	// 书源管理
	sourceManager *source.Manager
	rules         []*model.Rule
	currentRule   *model.Rule

	// HTTP客户端
	httpClient *httpclient.Client

	// 章节预加载器
	preloader *service.ChapterPreloader

	// === 主菜单 ===
	menuIndex int

	// === 书架 ===
	bookshelf      *model.BookShelf
	bookshelfIndex int
	fromBookshelf  bool // 标记是否从书架进入

	// === 搜索 ===
	searchInput textinput.Model
	keyword     string

	// === 多源搜索 ===
	aggregatedResults *model.MultiSourceSearchResult
	resultIndex       int
	searchProgress    map[int]*model.SourceSearchStat

	// === 单源搜索（兼容旧模式） ===
	searchResults []*model.SearchResult

	// === 当前书籍 ===
	currentBook  *model.BookRecord   // 书架中的书籍记录
	selectedBook *model.SearchResult // 搜索选中的书籍

	// === 目录 ===
	chapters     []*model.Chapter
	chapterIndex int
	tocOffset    int

	// === 阅读 ===
	currentChapter *model.Chapter
	contentLines   []string
	lineOffset     int
	linesPerPage   int

	// === 换源 ===
	availableSources []*model.BookSource
	switchIndex      int

	// === 调试日志 ===
	showDebugLog bool
	debugLogs    []string
	maxDebugLogs int

	// 状态
	loading   bool
	err       error
	statusMsg string
}

// 消息类型
type (
	rulesLoadedMsg          struct{ rules []*model.Rule }
	bookshelfLoadedMsg      struct{ shelf *model.BookShelf }
	searchResultMsg         struct{ results []*model.SearchResult }
	multiSearchResultMsg    struct{ result *model.MultiSourceSearchResult }
	searchProgressMsg       struct {
		sourceID   int
		sourceName string
		status     model.SearchStatus
		count      int
		err        error
	}
	tocLoadedMsg struct {
		chapters  []*model.Chapter
		fromCache bool
	}
	chapterLoadedMsg struct {
		chapter   *model.Chapter
		fromCache bool
	}
	progressSavedMsg struct{}
	bookAddedMsg     struct{ book *model.BookRecord }
	updateCheckMsg   struct {
		bookID        string
		hasUpdate     bool
		newChapters   int
		latestChapter string
		err           error
	}
	errorMsg      struct{ err error }
)

// 主菜单选项
var mainMenuItems = []string{
	"📚 书架",
	"🔍 搜索小说",
}

// NewModel 创建应用模型
func NewModel(cfg *config.AppConfig) Model {
	ti := textinput.New()
	ti.Placeholder = "输入书名或作者..."
	ti.CharLimit = 100
	ti.Width = 40

	// 创建存储（使用SQLite）
	sqliteStore, err := storage.NewDefaultSQLiteStore()
	if err != nil {
		// 如果创建失败，使用空存储（打印警告但不阻止启动）
		fmt.Printf("警告: 无法创建持久化存储: %v\n", err)
	}

	// 使用 Store 接口类型
	var store storage.Store = sqliteStore

	// 创建HTTP客户端
	httpClient := httpclient.NewClient(cfg)

	// 创建预加载器（SQLiteStore 实现了 CacheStore 接口）
	var preloader *service.ChapterPreloader
	if sqliteStore != nil {
		preloader = service.NewChapterPreloader(sqliteStore, httpClient, 3)
	}

	return Model{
		state:          StateMainMenu,
		config:         cfg,
		store:          store,
		sourceManager:  source.NewManagerWithConfig(cfg),
		httpClient:     httpClient,
		preloader:      preloader,
		searchInput:    ti,
		linesPerPage:   20,
		searchProgress: make(map[int]*model.SourceSearchStat),
		maxDebugLogs:   100,
	}
}

// Init 初始化
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.loadRules(),
		m.loadBookshelf(),
		tea.WindowSize(),
	)
}

// Update 更新
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.linesPerPage = m.height - 8
		if m.linesPerPage < 5 {
			m.linesPerPage = 5
		}
		return m, nil

	case tea.KeyMsg:
		// 全局快捷键
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "q":
			if m.state == StateMainMenu {
				return m, tea.Quit
			}
			return m.handleBack()
		case "esc":
			return m.handleBack()
		}

		// 状态特定处理
		switch m.state {
		case StateMainMenu:
			return m.updateMainMenu(msg)
		case StateBookShelf:
			return m.updateBookShelf(msg)
		case StateSearch:
			return m.updateSearch(msg)
		case StateSearchResult:
			return m.updateSearchResult(msg)
		case StateToc:
			return m.updateToc(msg)
		case StateReader:
			return m.updateReader(msg)
		case StateSourceSwitch:
			return m.updateSourceSwitch(msg)
		}

	case tea.MouseMsg:
		return m.handleMouse(msg)

	case rulesLoadedMsg:
		m.rules = msg.rules
		m.loading = false
		if len(m.rules) > 0 {
			m.currentRule = m.rules[0]
		}
		return m, nil

	case bookshelfLoadedMsg:
		m.bookshelf = msg.shelf
		return m, nil

	case searchResultMsg:
		m.searchResults = msg.results
		m.resultIndex = 0
		m.loading = false
		m.state = StateSearchResult
		if len(m.searchResults) == 0 {
			m.statusMsg = "未找到相关书籍"
		} else {
			m.statusMsg = fmt.Sprintf("找到 %d 本书籍", len(m.searchResults))
		}
		return m, nil

	case multiSearchResultMsg:
		m.aggregatedResults = msg.result
		m.resultIndex = 0
		m.loading = false
		m.state = StateSearchResult
		if msg.result.TotalCount == 0 {
			m.statusMsg = "未找到相关书籍"
		} else {
			m.statusMsg = fmt.Sprintf("找到 %d 本书籍 (来自 %d 个书源)",
				msg.result.TotalCount, msg.result.GetSuccessfulSourceCount())
		}
		return m, nil

	case searchProgressMsg:
		m.searchProgress[msg.sourceID] = &model.SourceSearchStat{
			SourceID:    msg.sourceID,
			SourceName:  msg.sourceName,
			Status:      msg.status,
			ResultCount: msg.count,
		}
		if msg.err != nil {
			m.searchProgress[msg.sourceID].Error = msg.err.Error()
		}
		return m, nil

	case tocLoadedMsg:
		m.chapters = msg.chapters
		m.loading = false
		m.state = StateToc

		// 确保 currentRule 与当前书源匹配
		if m.currentBook != nil {
			src := m.currentBook.GetCurrentSource()
			if src != nil {
				for _, r := range m.rules {
					if r.ID == src.SourceID {
						m.currentRule = r
						break
					}
				}
			}
		}

		// 添加调试日志
		var bookURL string
		if m.currentBook != nil {
			src := m.currentBook.GetCurrentSource()
			if src != nil {
				bookURL = src.BookURL
			}
		} else if m.selectedBook != nil {
			bookURL = m.selectedBook.URL
		}
		if msg.fromCache {
			m.addDebugLog("目录加载成功 (缓存): %d 章", len(msg.chapters))
		} else {
			m.addDebugLog("目录加载成功: %d 章", len(msg.chapters))
		}
		m.addDebugLog("书籍URL: %s", bookURL)
		if m.currentRule != nil {
			m.addDebugLog("当前规则: %s (%s)", m.currentRule.Name, m.currentRule.URL)
		}

		// 如果从书架进入，恢复阅读进度
		if m.fromBookshelf && m.currentBook != nil && m.store != nil {
			progress, _ := m.store.GetReadingProgress(m.currentBook.ID)
			if progress != nil {
				m.chapterIndex = progress.ChapterIndex
				if m.chapterIndex >= len(m.chapters) {
					m.chapterIndex = len(m.chapters) - 1
				}
				// 计算 tocOffset
				visibleItems := m.height - 10
				if visibleItems < 5 {
					visibleItems = 5
				}
				m.tocOffset = m.chapterIndex - visibleItems/2
				if m.tocOffset < 0 {
					m.tocOffset = 0
				}
			} else {
				m.chapterIndex = 0
				m.tocOffset = 0
			}
		} else {
			m.chapterIndex = 0
			m.tocOffset = 0
		}

		m.statusMsg = fmt.Sprintf("共 %d 章", len(m.chapters))

		// 更新书籍的章节数
		if m.currentBook != nil {
			m.currentBook.TotalChapters = len(m.chapters)
			if len(m.chapters) > 0 {
				m.currentBook.LatestChapter = m.chapters[len(m.chapters)-1].Title
			}
		}

		return m, nil

	case chapterLoadedMsg:
		m.currentChapter = msg.chapter
		m.contentLines = m.wrapContent(msg.chapter.Content)
		m.loading = false
		m.state = StateReader

		// 添加调试日志
		if msg.fromCache {
			m.addDebugLog("章节加载成功 (缓存): %s", msg.chapter.Title)
		} else {
			m.addDebugLog("章节加载成功: %s", msg.chapter.Title)
		}
		m.addDebugLog("章节URL: %s", msg.chapter.URL)
		if len(msg.chapter.Content) == 0 {
			m.addDebugLog("警告: 章节内容为空")
		} else {
			m.addDebugLog("内容长度: %d 字符, %d 行", len(msg.chapter.Content), len(m.contentLines))
		}

		// 触发章节预加载（只对书架中的书籍预加载）
		if m.preloader != nil && m.currentBook != nil && m.currentRule != nil {
			m.preloader.PreloadAhead(
				m.currentBook.ID,
				m.currentBook.CurrentSourceID,
				m.chapterIndex,
				m.chapters,
				m.currentRule,
			)
			m.addDebugLog("触发预加载: 后续 3 章")
		}

		// 恢复阅读位置
		if m.fromBookshelf && m.currentBook != nil && m.store != nil {
			progress, _ := m.store.GetReadingProgress(m.currentBook.ID)
			if progress != nil && progress.ChapterIndex == m.chapterIndex {
				m.lineOffset = progress.LineOffset
				if m.lineOffset >= len(m.contentLines) {
					m.lineOffset = 0
				}
			} else {
				m.lineOffset = 0
			}
		} else {
			m.lineOffset = 0
		}

		m.statusMsg = ""
		return m, nil

	case progressSavedMsg:
		return m, nil

	case bookAddedMsg:
		if m.bookshelf != nil {
			m.bookshelf.AddBook(msg.book)
			if m.store != nil {
				m.store.SaveBookShelf(m.bookshelf)
			}
		}
		m.statusMsg = fmt.Sprintf("已添加到书架: %s", msg.book.BookName)
		return m, nil

	case updateCheckMsg:
		if m.bookshelf != nil {
			book := m.bookshelf.FindBookByID(msg.bookID)
			if book != nil {
				if msg.hasUpdate {
					book.MarkUpdate(msg.newChapters, msg.latestChapter)
					m.statusMsg = fmt.Sprintf("%s 有 %d 章更新", book.BookName, msg.newChapters)
				} else {
					m.statusMsg = fmt.Sprintf("%s 暂无更新", book.BookName)
				}
				if m.store != nil {
					m.store.SaveBookShelf(m.bookshelf)
				}
			}
		}
		m.loading = false
		return m, nil

	case errorMsg:
		m.err = msg.err
		m.loading = false
		m.statusMsg = fmt.Sprintf("错误: %v", msg.err)
		// 添加调试日志
		m.addDebugLog("错误: %v", msg.err)
		return m, nil
	}

	return m, nil
}

// View 渲染
func (m Model) View() string {
	if m.loading {
		return m.renderLoading()
	}

	switch m.state {
	case StateMainMenu:
		return m.renderMainMenu()
	case StateBookShelf:
		return m.renderBookShelf()
	case StateSearch:
		return m.renderSearch()
	case StateSearchResult:
		return m.renderSearchResult()
	case StateToc:
		return m.renderToc()
	case StateReader:
		return m.renderReader()
	case StateSourceSwitch:
		return m.renderSourceSwitch()
	}

	return ""
}

// handleBack 返回上一级
func (m Model) handleBack() (tea.Model, tea.Cmd) {
	switch m.state {
	case StateBookShelf, StateSearch:
		m.state = StateMainMenu
		m.fromBookshelf = false
	case StateSearchResult:
		m.state = StateSearch
		m.searchInput.Focus()
	case StateToc:
		if m.fromBookshelf {
			m.state = StateBookShelf
		} else {
			m.state = StateSearchResult
		}
	case StateReader:
		// 先改变状态，再保存进度（异步）
		m.state = StateToc
		cmd := m.saveProgress()
		return m, cmd
	case StateSourceSwitch:
		m.state = StateToc
	case StateMainMenu:
		return m, tea.Quit
	}
	return m, nil
}

// === 命令函数 ===

// loadRules 加载规则
func (m Model) loadRules() tea.Cmd {
	return func() tea.Msg {
		rules, err := m.sourceManager.GetSearchableRules(m.config.ActiveRules)
		if err != nil {
			return errorMsg{err}
		}
		return rulesLoadedMsg{rules}
	}
}

// loadBookshelf 加载书架
func (m Model) loadBookshelf() tea.Cmd {
	return func() tea.Msg {
		if m.store == nil {
			return bookshelfLoadedMsg{model.NewBookShelf()}
		}
		shelf, err := m.store.LoadBookShelf()
		if err != nil {
			return bookshelfLoadedMsg{model.NewBookShelf()}
		}
		return bookshelfLoadedMsg{shelf}
	}
}

// doMultiSearch 执行多源搜索
func (m Model) doMultiSearch() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		resultsBySource := make(map[int][]*model.SearchResultWithSource)
		var mu sync.Mutex
		var wg sync.WaitGroup

		for _, rule := range m.rules {
			wg.Add(1)
			go func(r *model.Rule) {
				defer wg.Done()

				searchParser := parser.NewSearchParser(r, m.httpClient)
				results, err := searchParser.Parse(m.keyword)

				mu.Lock()
				defer mu.Unlock()

				if err != nil {
					return
				}

				for _, result := range results {
					result.SourceID = r.ID
					resultsBySource[r.ID] = append(resultsBySource[r.ID], &model.SearchResultWithSource{
						SearchResult: result,
						SourceName:   r.Name,
					})
				}
			}(rule)
		}

		// 等待所有搜索完成或超时
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-done:
		case <-ctx.Done():
		}

		return multiSearchResultMsg{model.AggregateSearchResults(m.keyword, resultsBySource)}
	}
}

// loadToc 加载目录
func (m Model) loadToc() tea.Cmd {
	return func() tea.Msg {
		var bookURL string
		var bookID string
		var sourceID int
		var rule *model.Rule

		if m.currentBook != nil {
			src := m.currentBook.GetCurrentSource()
			if src == nil {
				return errorMsg{fmt.Errorf("未找到可用书源")}
			}
			bookURL = src.BookURL
			bookID = m.currentBook.ID
			sourceID = src.SourceID
			// 获取对应的规则
			for _, r := range m.rules {
				if r.ID == src.SourceID {
					rule = r
					break
				}
			}
		} else if m.selectedBook != nil {
			bookURL = m.selectedBook.URL
			rule = m.currentRule
			sourceID = m.selectedBook.SourceID
		} else {
			return errorMsg{fmt.Errorf("未选择书籍")}
		}

		if rule == nil {
			return errorMsg{fmt.Errorf("未找到对应书源规则")}
		}

		// 尝试从缓存加载目录（只有书架中的书才使用缓存）
		if bookID != "" && m.store != nil {
			if cacheStore, ok := m.store.(storage.CacheStore); ok {
				cachedToc, exists, err := cacheStore.GetTocCache(bookID, sourceID)
				if err == nil && exists && len(cachedToc) > 0 {
					// 从缓存构建章节列表
					chapters := make([]*model.Chapter, len(cachedToc))
					for i, item := range cachedToc {
						chapters[i] = &model.Chapter{
							Title: item.Title,
							URL:   item.URL,
							Order: item.Index + 1,
						}
					}
					return tocLoadedMsg{chapters: chapters, fromCache: true}
				}
			}
		}

		// 从网络获取目录
		tocParser := parser.NewTocParser(rule, m.httpClient)
		chapters, err := tocParser.Parse(bookURL)
		if err != nil {
			return errorMsg{err}
		}

		// 保存到缓存（只有书架中的书才保存缓存）
		if bookID != "" && m.store != nil {
			if cacheStore, ok := m.store.(storage.CacheStore); ok {
				tocItems := make([]storage.TocCacheItem, len(chapters))
				for i, ch := range chapters {
					tocItems[i] = storage.TocCacheItem{
						Index: i,
						Title: ch.Title,
						URL:   ch.URL,
					}
				}
				cacheStore.SaveTocCache(bookID, sourceID, tocItems)
			}
		}

		return tocLoadedMsg{chapters: chapters, fromCache: false}
	}
}

// loadChapter 加载章节
func (m Model) loadChapter(index int) tea.Cmd {
	return func() tea.Msg {
		if index < 0 || index >= len(m.chapters) {
			return errorMsg{fmt.Errorf("章节索引无效")}
		}
		chapter := m.chapters[index]

		// 获取书籍ID和书源ID用于缓存
		var bookID string
		var sourceID int
		if m.currentBook != nil {
			bookID = m.currentBook.ID
			sourceID = m.currentBook.CurrentSourceID
		}

		// 尝试从缓存加载章节内容（只有书架中的书才使用缓存）
		if bookID != "" && m.store != nil {
			if cacheStore, ok := m.store.(storage.CacheStore); ok {
				content, exists, err := cacheStore.GetChapterContent(bookID, sourceID, index)
				if err == nil && exists && content != "" {
					// 使用缓存的内容
					cachedChapter := &model.Chapter{
						Title:   chapter.Title,
						URL:     chapter.URL,
						Order:   chapter.Order,
						Content: content,
					}
					return chapterLoadedMsg{chapter: cachedChapter, fromCache: true}
				}
			}
		}

		// 从网络获取章节内容
		chapterParser := parser.NewChapterParser(m.currentRule, m.httpClient)
		err := chapterParser.Parse(chapter)
		if err != nil {
			return errorMsg{err}
		}

		// 保存到缓存（只有书架中的书且内容非空时才保存）
		if bookID != "" && chapter.Content != "" && m.store != nil {
			if cacheStore, ok := m.store.(storage.CacheStore); ok {
				cacheStore.SaveChapterContent(bookID, sourceID, index, chapter.Content)
			}
		}

		return chapterLoadedMsg{chapter: chapter, fromCache: false}
	}
}

// saveProgress 保存阅读进度
func (m Model) saveProgress() tea.Cmd {
	if m.store == nil || m.currentBook == nil {
		return nil
	}

	return func() tea.Msg {
		progress := model.NewReadingProgress(
			m.currentBook.ID,
			m.currentBook.CurrentSourceID,
			m.chapterIndex,
			m.currentChapter.Title,
		)
		progress.ChapterURL = m.currentChapter.URL
		progress.UpdatePosition(m.lineOffset, len(m.contentLines))

		m.store.SaveReadingProgress(progress)

		// 更新书籍的最后阅读时间
		m.currentBook.UpdateLastRead()
		m.currentBook.ClearUpdateMark()
		if m.bookshelf != nil {
			m.store.SaveBookShelf(m.bookshelf)
		}

		return progressSavedMsg{}
	}
}

// addToBookshelf 添加到书架
func (m Model) addToBookshelf() tea.Cmd {
	return func() tea.Msg {
		var book *model.BookRecord

		if m.aggregatedResults != nil && m.resultIndex < len(m.aggregatedResults.Results) {
			agg := m.aggregatedResults.Results[m.resultIndex]
			book = agg.ToBookRecord()
		} else if m.selectedBook != nil {
			sourceName := ""
			if m.currentRule != nil {
				sourceName = m.currentRule.Name
			}
			book = model.NewBookRecord(m.selectedBook, sourceName)
		}

		if book == nil {
			return errorMsg{fmt.Errorf("无法添加到书架")}
		}

		return bookAddedMsg{book}
	}
}

// checkBookUpdate 检查书籍更新
func (m Model) checkBookUpdate(book *model.BookRecord) tea.Cmd {
	return func() tea.Msg {
		src := book.GetCurrentSource()
		if src == nil {
			return updateCheckMsg{bookID: book.ID, err: fmt.Errorf("无可用书源")}
		}

		var rule *model.Rule
		for _, r := range m.rules {
			if r.ID == src.SourceID {
				rule = r
				break
			}
		}
		if rule == nil {
			return updateCheckMsg{bookID: book.ID, err: fmt.Errorf("书源规则不存在")}
		}

		tocParser := parser.NewTocParser(rule, m.httpClient)
		chapters, err := tocParser.Parse(src.BookURL)
		if err != nil {
			return updateCheckMsg{bookID: book.ID, err: err}
		}

		newCount := len(chapters)
		oldCount := book.TotalChapters
		latestChapter := ""
		if len(chapters) > 0 {
			latestChapter = chapters[len(chapters)-1].Title
		}

		return updateCheckMsg{
			bookID:        book.ID,
			hasUpdate:     newCount > oldCount,
			newChapters:   newCount - oldCount,
			latestChapter: latestChapter,
		}
	}
}

// wrapContent 自动换行处理
func (m Model) wrapContent(content string) []string {
	if m.width == 0 {
		m.width = 80
	}

	maxWidth := m.width - 6
	if maxWidth < 20 {
		maxWidth = 20
	}

	var lines []string
	for _, paragraph := range strings.Split(content, "\n") {
		if paragraph == "" {
			lines = append(lines, "")
			continue
		}

		runes := []rune(paragraph)
		for len(runes) > 0 {
			// 按实际显示宽度计算换行位置
			lineWidth := 0
			end := 0
			for i, r := range runes {
				w := runewidth.RuneWidth(r)
				if lineWidth+w > maxWidth {
					break
				}
				lineWidth += w
				end = i + 1
			}
			if end == 0 && len(runes) > 0 {
				// 单个字符超宽，至少取一个
				end = 1
			}
			lines = append(lines, string(runes[:end]))
			runes = runes[end:]
		}
	}

	return lines
}

// === 状态更新函数 ===

func (m Model) updateMainMenu(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.menuIndex > 0 {
			m.menuIndex--
		}
	case "down", "j":
		if m.menuIndex < len(mainMenuItems)-1 {
			m.menuIndex++
		}
	case "enter":
		switch m.menuIndex {
		case 0: // 书架
			m.state = StateBookShelf
			m.bookshelfIndex = 0
		case 1: // 搜索
			m.state = StateSearch
			m.searchInput.Focus()
			return m, textinput.Blink
		}
	case "1":
		m.state = StateBookShelf
		m.bookshelfIndex = 0
	case "2":
		m.state = StateSearch
		m.searchInput.Focus()
		return m, textinput.Blink
	}
	return m, nil
}

func (m Model) updateBookShelf(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	books := m.getBookshelfBooks()

	switch msg.String() {
	case "up", "k":
		if m.bookshelfIndex > 0 {
			m.bookshelfIndex--
		}
	case "down", "j":
		if m.bookshelfIndex < len(books)-1 {
			m.bookshelfIndex++
		}
	case "enter":
		if len(books) > 0 {
			m.currentBook = books[m.bookshelfIndex]
			m.selectedBook = nil
			m.fromBookshelf = true
			m.loading = true
			m.statusMsg = "加载目录..."
			return m, m.loadToc()
		}
	case "d":
		// 删除书籍
		if len(books) > 0 && m.bookshelf != nil {
			book := books[m.bookshelfIndex]
			m.bookshelf.RemoveBook(book.ID)
			if m.store != nil {
				m.store.SaveBookShelf(m.bookshelf)
				// 同时删除阅读进度
				progressStore, _ := m.store.LoadProgress()
				if progressStore != nil {
					progressStore.RemoveProgress(book.ID)
					m.store.SaveProgress(progressStore)
				}
			}
			m.statusMsg = fmt.Sprintf("已从书架移除: %s", book.BookName)
			if m.bookshelfIndex >= len(books)-1 && m.bookshelfIndex > 0 {
				m.bookshelfIndex--
			}
		}
	case "u":
		// 检查更新
		if len(books) > 0 {
			book := books[m.bookshelfIndex]
			m.loading = true
			m.statusMsg = "检查更新..."
			return m, m.checkBookUpdate(book)
		}
	case "s":
		// 快速搜索
		m.state = StateSearch
		m.searchInput.Focus()
		return m, textinput.Blink
	}
	return m, nil
}

func (m Model) updateSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		m.keyword = m.searchInput.Value()
		if m.keyword != "" {
			m.loading = true
			m.statusMsg = "搜索中..."
			m.searchProgress = make(map[int]*model.SourceSearchStat)
			// 使用多源搜索
			return m, m.doMultiSearch()
		}
	default:
		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) updateSearchResult(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var resultCount int
	if m.aggregatedResults != nil {
		resultCount = len(m.aggregatedResults.Results)
	} else {
		resultCount = len(m.searchResults)
	}

	switch msg.String() {
	case "up", "k":
		if m.resultIndex > 0 {
			m.resultIndex--
		}
	case "down", "j":
		if m.resultIndex < resultCount-1 {
			m.resultIndex++
		}
	case "enter":
		if resultCount > 0 {
			if m.aggregatedResults != nil {
				agg := m.aggregatedResults.Results[m.resultIndex]
				firstSource := agg.GetFirstSource()
				if firstSource != nil {
					// 创建书籍记录
					m.currentBook = agg.ToBookRecord()
					m.selectedBook = firstSource.SearchResult
					// 设置当前规则
					for _, r := range m.rules {
						if r.ID == firstSource.SourceID {
							m.currentRule = r
							break
						}
					}
				}
			} else {
				m.selectedBook = m.searchResults[m.resultIndex]
				m.currentBook = nil
			}
			m.fromBookshelf = false
			m.loading = true
			m.statusMsg = "加载目录..."
			return m, m.loadToc()
		}
	case "a":
		// 加入书架
		if resultCount > 0 {
			return m, m.addToBookshelf()
		}
	}
	return m, nil
}

func (m Model) updateToc(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	visibleItems := m.height - 10 // 与 renderToc 保持一致
	if visibleItems < 5 {
		visibleItems = 5
	}

	switch msg.String() {
	case "up", "k":
		if m.chapterIndex > 0 {
			m.chapterIndex--
			if m.chapterIndex < m.tocOffset {
				m.tocOffset = m.chapterIndex
			}
		}
	case "down", "j":
		if m.chapterIndex < len(m.chapters)-1 {
			m.chapterIndex++
			if m.chapterIndex >= m.tocOffset+visibleItems {
				m.tocOffset = m.chapterIndex - visibleItems + 1
			}
		}
	case "pgup":
		// 向上翻页：光标和页面都向上移动一页
		m.tocOffset -= visibleItems
		if m.tocOffset < 0 {
			m.tocOffset = 0
		}
		m.chapterIndex = m.tocOffset // 光标在页面第一项
	case "pgdown":
		// 向下翻页：光标和页面都向下移动一页
		m.tocOffset += visibleItems
		maxOffset := len(m.chapters) - visibleItems
		if maxOffset < 0 {
			maxOffset = 0
		}
		if m.tocOffset > maxOffset {
			m.tocOffset = maxOffset
		}
		m.chapterIndex = m.tocOffset // 光标在页面第一项
		// 确保光标不超出范围
		if m.chapterIndex >= len(m.chapters) {
			m.chapterIndex = len(m.chapters) - 1
		}
	case "home", "g":
		m.chapterIndex = 0
		m.tocOffset = 0
	case "end", "G":
		m.chapterIndex = len(m.chapters) - 1
		m.tocOffset = len(m.chapters) - visibleItems
		if m.tocOffset < 0 {
			m.tocOffset = 0
		}
	case "enter":
		if len(m.chapters) > 0 {
			m.loading = true
			m.statusMsg = "加载章节内容..."
			return m, m.loadChapter(m.chapterIndex)
		}
	case "c":
		// 切换书源
		if m.currentBook != nil && len(m.currentBook.Sources) > 1 {
			m.availableSources = m.currentBook.Sources
			m.switchIndex = 0
			// 找到当前源的索引
			for i, s := range m.availableSources {
				if s.SourceID == m.currentBook.CurrentSourceID {
					m.switchIndex = i
					break
				}
			}
			m.state = StateSourceSwitch
		} else {
			m.statusMsg = "只有一个书源，无法切换"
		}
	case "a":
		// 加入书架
		if m.currentBook != nil && m.bookshelf != nil {
			existing := m.bookshelf.FindBook(m.currentBook.BookName, m.currentBook.Author)
			if existing == nil {
				return m, m.addToBookshelf()
			} else {
				m.statusMsg = "该书已在书架中"
			}
		} else if m.selectedBook != nil {
			return m, m.addToBookshelf()
		}
	}
	return m, nil
}

func (m Model) updateReader(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.lineOffset > 0 {
			m.lineOffset--
		}
	case "down", "j", " ":
		if m.lineOffset < len(m.contentLines)-m.linesPerPage {
			m.lineOffset++
		}
	case "pgup":
		m.lineOffset -= m.linesPerPage
		if m.lineOffset < 0 {
			m.lineOffset = 0
		}
	case "pgdown":
		m.lineOffset += m.linesPerPage
		maxOffset := len(m.contentLines) - m.linesPerPage
		if maxOffset < 0 {
			maxOffset = 0
		}
		if m.lineOffset > maxOffset {
			m.lineOffset = maxOffset
		}
	case "home", "g":
		m.lineOffset = 0
	case "end", "G":
		m.lineOffset = len(m.contentLines) - m.linesPerPage
		if m.lineOffset < 0 {
			m.lineOffset = 0
		}
	case "n", "right", "l":
		// 下一章
		if m.chapterIndex < len(m.chapters)-1 {
			m.chapterIndex++
			m.loading = true
			m.statusMsg = "加载下一章..."
			// 同时保存进度和加载下一章
			return m, tea.Batch(m.saveProgress(), m.loadChapter(m.chapterIndex))
		}
	case "p", "left", "h":
		// 上一章
		if m.chapterIndex > 0 {
			m.chapterIndex--
			m.loading = true
			m.statusMsg = "加载上一章..."
			// 同时保存进度和加载上一章
			return m, tea.Batch(m.saveProgress(), m.loadChapter(m.chapterIndex))
		}
	case "t":
		// 返回目录
		m.state = StateToc
		return m, m.saveProgress()
	case "c":
		// 切换书源
		if m.currentBook != nil && len(m.currentBook.Sources) > 1 {
			m.availableSources = m.currentBook.Sources
			m.switchIndex = 0
			for i, s := range m.availableSources {
				if s.SourceID == m.currentBook.CurrentSourceID {
					m.switchIndex = i
					break
				}
			}
			m.state = StateSourceSwitch
			return m, m.saveProgress()
		} else {
			m.statusMsg = "只有一个书源，无法切换"
		}
	case "a":
		// 加入书架
		if m.currentBook != nil && m.bookshelf != nil {
			existing := m.bookshelf.FindBook(m.currentBook.BookName, m.currentBook.Author)
			if existing == nil {
				return m, m.addToBookshelf()
			} else {
				m.statusMsg = "该书已在书架中"
			}
		}
	case "/":
		// 切换调试日志
		m.showDebugLog = !m.showDebugLog
	}
	return m, nil
}

func (m Model) updateSourceSwitch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.switchIndex > 0 {
			m.switchIndex--
		}
	case "down", "j":
		if m.switchIndex < len(m.availableSources)-1 {
			m.switchIndex++
		}
	case "enter":
		if len(m.availableSources) > 0 && m.currentBook != nil {
			newSource := m.availableSources[m.switchIndex]
			if newSource.SourceID != m.currentBook.CurrentSourceID {
				// 切换书源
				m.currentBook.SwitchSource(newSource.SourceID)

				// 更新当前规则
				for _, r := range m.rules {
					if r.ID == newSource.SourceID {
						m.currentRule = r
						break
					}
				}

				// 重新加载目录
				m.loading = true
				m.statusMsg = fmt.Sprintf("切换到 %s，重新加载目录...", newSource.SourceName)
				return m, m.loadToc()
			} else {
				m.statusMsg = "已是当前书源"
				m.state = StateToc
			}
		}
	}
	return m, nil
}

// handleMouse 处理鼠标事件
func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		return m.handleScrollUp()
	case tea.MouseButtonWheelDown:
		return m.handleScrollDown()
	case tea.MouseButtonLeft:
		if msg.Action == tea.MouseActionRelease {
			return m.handleClick(msg.X, msg.Y)
		}
	}
	return m, nil
}

// handleScrollUp 处理向上滚动
func (m Model) handleScrollUp() (tea.Model, tea.Cmd) {
	switch m.state {
	case StateMainMenu:
		if m.menuIndex > 0 {
			m.menuIndex--
		}
	case StateBookShelf:
		if m.bookshelfIndex > 0 {
			m.bookshelfIndex--
		}
	case StateSearchResult:
		resultCount := 0
		if m.aggregatedResults != nil {
			resultCount = len(m.aggregatedResults.Results)
		} else {
			resultCount = len(m.searchResults)
		}
		if m.resultIndex > 0 && resultCount > 0 {
			m.resultIndex--
		}
	case StateToc:
		if m.chapterIndex > 0 {
			m.chapterIndex--
			if m.chapterIndex < m.tocOffset {
				m.tocOffset = m.chapterIndex
			}
		}
	case StateReader:
		if m.lineOffset > 0 {
			m.lineOffset -= 3
			if m.lineOffset < 0 {
				m.lineOffset = 0
			}
		}
	case StateSourceSwitch:
		if m.switchIndex > 0 {
			m.switchIndex--
		}
	}
	return m, nil
}

// handleScrollDown 处理向下滚动
func (m Model) handleScrollDown() (tea.Model, tea.Cmd) {
	switch m.state {
	case StateMainMenu:
		if m.menuIndex < len(mainMenuItems)-1 {
			m.menuIndex++
		}
	case StateBookShelf:
		books := m.getBookshelfBooks()
		if m.bookshelfIndex < len(books)-1 {
			m.bookshelfIndex++
		}
	case StateSearchResult:
		resultCount := 0
		if m.aggregatedResults != nil {
			resultCount = len(m.aggregatedResults.Results)
		} else {
			resultCount = len(m.searchResults)
		}
		if m.resultIndex < resultCount-1 {
			m.resultIndex++
		}
	case StateToc:
		visibleItems := m.height - 10
		if visibleItems < 5 {
			visibleItems = 5
		}
		if m.chapterIndex < len(m.chapters)-1 {
			m.chapterIndex++
			if m.chapterIndex >= m.tocOffset+visibleItems {
				m.tocOffset = m.chapterIndex - visibleItems + 1
			}
		}
	case StateReader:
		maxOffset := len(m.contentLines) - m.linesPerPage
		if maxOffset < 0 {
			maxOffset = 0
		}
		if m.lineOffset < maxOffset {
			m.lineOffset += 3
			if m.lineOffset > maxOffset {
				m.lineOffset = maxOffset
			}
		}
	case StateSourceSwitch:
		if m.switchIndex < len(m.availableSources)-1 {
			m.switchIndex++
		}
	}
	return m, nil
}

// handleClick 处理鼠标点击
func (m Model) handleClick(x, y int) (tea.Model, tea.Cmd) {
	// contentStyle 有 padding(1, 2)，所以内容区域从 y=1 开始
	// 标题占用约 2-3 行
	contentStartY := 4 // 标题 + 空行后的内容起始行

	switch m.state {
	case StateMainMenu:
		// 主菜单项从第4行开始（标题2行 + 空行1行 + padding 1行）
		clickedIndex := y - contentStartY
		if clickedIndex >= 0 && clickedIndex < len(mainMenuItems) {
			m.menuIndex = clickedIndex
			// 双击效果：如果点击的是当前选中项，则进入
			return m.updateMainMenu(tea.KeyMsg{Type: tea.KeyEnter})
		}

	case StateBookShelf:
		books := m.getBookshelfBooks()
		if len(books) == 0 {
			return m, nil
		}
		visibleItems := m.height - 10
		if visibleItems < 5 {
			visibleItems = 5
		}
		start := 0
		if m.bookshelfIndex >= visibleItems {
			start = m.bookshelfIndex - visibleItems + 1
		}
		clickedIndex := y - contentStartY + start
		if clickedIndex >= 0 && clickedIndex < len(books) {
			if m.bookshelfIndex == clickedIndex {
				// 点击已选中项，进入
				return m.updateBookShelf(tea.KeyMsg{Type: tea.KeyEnter})
			}
			m.bookshelfIndex = clickedIndex
		}

	case StateSearchResult:
		resultCount := 0
		if m.aggregatedResults != nil {
			resultCount = len(m.aggregatedResults.Results)
		} else {
			resultCount = len(m.searchResults)
		}
		if resultCount == 0 {
			return m, nil
		}
		visibleItems := m.height - 14
		if visibleItems < 3 {
			visibleItems = 3
		}
		start := 0
		if m.resultIndex >= visibleItems {
			start = m.resultIndex - visibleItems + 1
		}
		clickedIndex := y - contentStartY + start
		if clickedIndex >= 0 && clickedIndex < resultCount {
			if m.resultIndex == clickedIndex {
				// 点击已选中项，进入
				return m.updateSearchResult(tea.KeyMsg{Type: tea.KeyEnter})
			}
			m.resultIndex = clickedIndex
		}

	case StateToc:
		if len(m.chapters) == 0 {
			return m, nil
		}
		clickedIndex := y - contentStartY + m.tocOffset
		if clickedIndex >= 0 && clickedIndex < len(m.chapters) {
			if m.chapterIndex == clickedIndex {
				// 点击已选中项，进入阅读
				return m.updateToc(tea.KeyMsg{Type: tea.KeyEnter})
			}
			m.chapterIndex = clickedIndex
		}

	case StateSourceSwitch:
		if len(m.availableSources) == 0 {
			return m, nil
		}
		clickedIndex := y - contentStartY
		if clickedIndex >= 0 && clickedIndex < len(m.availableSources) {
			if m.switchIndex == clickedIndex {
				// 点击已选中项，确认切换
				return m.updateSourceSwitch(tea.KeyMsg{Type: tea.KeyEnter})
			}
			m.switchIndex = clickedIndex
		}
	}
	return m, nil
}

// === 渲染函数 ===

func (m Model) renderLoading() string {
	msg := m.statusMsg
	if msg == "" {
		msg = "加载中..."
	}
	return lipgloss.Place(
		m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		formatLoading("⏳ "+msg),
	)
}

func (m Model) renderMainMenu() string {
	var b strings.Builder

	b.WriteString(formatTitle("📚 小说阅读器"))
	b.WriteString("\n\n")

	for i, item := range mainMenuItems {
		if i == m.menuIndex {
			b.WriteString(formatSelectedItem(item))
		} else {
			b.WriteString(formatItem(" " + item))
		}
		b.WriteString("\n")
	}

	// 显示书架统计
	if m.bookshelf != nil && len(m.bookshelf.Books) > 0 {
		b.WriteString("\n")
		b.WriteString(formatSubtitle(fmt.Sprintf("书架: %d 本书", len(m.bookshelf.Books))))
	}

	b.WriteString("\n")
	if m.statusMsg != "" {
		b.WriteString(formatHighlight(m.statusMsg))
		b.WriteString("\n")
	}
	b.WriteString(formatHelp("↑/↓: 选择  Enter: 确认  1/2: 快捷键  q: 退出"))

	return contentStyle.Render(b.String())
}

func (m Model) renderBookShelf() string {
	var b strings.Builder

	b.WriteString(formatTitle("📚 我的书架"))
	b.WriteString("\n\n")

	books := m.getBookshelfBooks()

	if len(books) == 0 {
		b.WriteString(formatSubtitle("书架空空如也，去搜索添加一些书吧~"))
		b.WriteString("\n")
	} else {
		visibleItems := m.height - 10
		if visibleItems < 5 {
			visibleItems = 5
		}

		start := 0
		if m.bookshelfIndex >= visibleItems {
			start = m.bookshelfIndex - visibleItems + 1
		}
		end := start + visibleItems
		if end > len(books) {
			end = len(books)
		}

		for i := start; i < end; i++ {
			book := books[i]
			line := fmt.Sprintf("%s - %s", book.BookName, book.Author)

			// 显示更新标记
			if book.HasUpdate && book.NewChapters > 0 {
				line += fmt.Sprintf(" [+%d]", book.NewChapters)
			}

			// 显示书源数量
			if len(book.Sources) > 1 {
				line += fmt.Sprintf(" (%d源)", len(book.Sources))
			}

			if i == m.bookshelfIndex {
				b.WriteString(formatSelectedItem(line))
			} else {
				b.WriteString(formatItem(" " + line))
			}
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	if m.statusMsg != "" {
		b.WriteString(formatHighlight(m.statusMsg))
		b.WriteString("\n")
	}
	b.WriteString(formatHelp("↑/↓: 选择  Enter: 继续阅读  d: 删除  u: 检查更新  s: 搜索  Esc: 返回"))

	return contentStyle.Render(b.String())
}

func (m Model) renderSearch() string {
	var b strings.Builder

	b.WriteString(formatTitle("🔍 搜索小说"))
	b.WriteString("\n")
	b.WriteString(formatSubtitle(fmt.Sprintf("将搜索 %d 个书源", len(m.rules))))
	b.WriteString("\n\n")

	b.WriteString(m.searchInput.View())
	b.WriteString("\n\n")

	if m.statusMsg != "" {
		b.WriteString(formatError(m.statusMsg))
		b.WriteString("\n")
	}

	b.WriteString(formatHelp("Enter: 搜索  Esc: 返回"))

	return contentStyle.Render(b.String())
}

func (m Model) renderSearchResult() string {
	var b strings.Builder

	b.WriteString(formatTitle(fmt.Sprintf("📖 搜索结果: %s", m.keyword)))
	b.WriteString("\n")
	b.WriteString(formatSubtitle(m.statusMsg))
	b.WriteString("\n\n")

	// 使用聚合结果
	if m.aggregatedResults != nil {
		if len(m.aggregatedResults.Results) == 0 {
			b.WriteString(formatError("未找到相关书籍"))
		} else {
			// 计算可见行数，需要为选中项的详情预留空间
			visibleItems := m.height - 14
			if visibleItems < 3 {
				visibleItems = 3
			}

			start := 0
			end := len(m.aggregatedResults.Results)
			if end > visibleItems {
				if m.resultIndex >= visibleItems {
					start = m.resultIndex - visibleItems + 1
				}
				end = start + visibleItems
				if end > len(m.aggregatedResults.Results) {
					end = len(m.aggregatedResults.Results)
				}
			}

			for i := start; i < end; i++ {
				result := m.aggregatedResults.Results[i]
				line := fmt.Sprintf("%s - %s", result.BookName, result.Author)
				if result.SourceCount > 1 {
					line += fmt.Sprintf(" [%d源]", result.SourceCount)
				}
				// 显示最新章节（如果有）
				if result.LatestChapter != "" {
					// 截断过长的章节名
					chapter := result.LatestChapter
					if len([]rune(chapter)) > 20 {
						chapter = string([]rune(chapter)[:20]) + "..."
					}
					line += fmt.Sprintf(" | %s", chapter)
				}
				if i == m.resultIndex {
					b.WriteString(formatSelectedItem(line))
				} else {
					b.WriteString(formatItem(" " + line))
				}
				b.WriteString("\n")
			}

			// 显示选中书籍的各书源详情
			if m.resultIndex < len(m.aggregatedResults.Results) {
				selected := m.aggregatedResults.Results[m.resultIndex]
				if len(selected.Sources) > 0 {
					b.WriteString("\n")
					b.WriteString(formatSubtitle("  📚 各书源最新章节:"))
					b.WriteString("\n")
					for _, src := range selected.Sources {
						srcLine := fmt.Sprintf("    • %s", src.SourceName)
						if src.LatestChapter != "" {
							srcLine += fmt.Sprintf(": %s", src.LatestChapter)
						}
						if src.LastUpdateTime != "" {
							srcLine += fmt.Sprintf(" (%s)", src.LastUpdateTime)
						}
						b.WriteString(formatItem(srcLine))
						b.WriteString("\n")
					}
				}
			}
		}
	} else if len(m.searchResults) == 0 {
		b.WriteString(formatError("未找到相关书籍"))
	} else {
		// 兼容单源搜索结果
		visibleItems := m.height - 10
		if visibleItems < 5 {
			visibleItems = 5
		}

		start := 0
		end := len(m.searchResults)
		if end > visibleItems {
			if m.resultIndex >= visibleItems {
				start = m.resultIndex - visibleItems + 1
			}
			end = start + visibleItems
			if end > len(m.searchResults) {
				end = len(m.searchResults)
			}
		}

		for i := start; i < end; i++ {
			result := m.searchResults[i]
			line := fmt.Sprintf("%s - %s", result.BookName, result.Author)
			if result.LatestChapter != "" {
				chapter := result.LatestChapter
				if len([]rune(chapter)) > 20 {
					chapter = string([]rune(chapter)[:20]) + "..."
				}
				line += fmt.Sprintf(" | %s", chapter)
			}
			if i == m.resultIndex {
				b.WriteString(formatSelectedItem(line))
			} else {
				b.WriteString(formatItem(" " + line))
			}
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(formatHelp("↑/↓: 选择  Enter: 查看目录  a: 加入书架  Esc: 返回"))

	return contentStyle.Render(b.String())
}

func (m Model) renderToc() string {
	var b strings.Builder

	bookName := ""
	sourceName := ""
	if m.currentBook != nil {
		bookName = m.currentBook.BookName
		src := m.currentBook.GetCurrentSource()
		if src != nil {
			sourceName = src.SourceName
		}
	} else if m.selectedBook != nil {
		bookName = m.selectedBook.BookName
		if m.currentRule != nil {
			sourceName = m.currentRule.Name
		}
	}

	b.WriteString(formatTitle(fmt.Sprintf("📑 %s", bookName)))
	b.WriteString("\n")
	if sourceName != "" {
		b.WriteString(formatSubtitle(fmt.Sprintf("当前书源: %s | %s", sourceName, m.statusMsg)))
	} else {
		b.WriteString(formatSubtitle(m.statusMsg))
	}
	b.WriteString("\n\n")

	if len(m.chapters) == 0 {
		b.WriteString(formatError("目录为空"))
	} else {
		visibleItems := m.height - 10
		if visibleItems < 5 {
			visibleItems = 5
		}

		end := m.tocOffset + visibleItems
		if end > len(m.chapters) {
			end = len(m.chapters)
		}

		for i := m.tocOffset; i < end; i++ {
			chapter := m.chapters[i]
			line := fmt.Sprintf("%d. %s", chapter.Order, chapter.Title)
			if i == m.chapterIndex {
				b.WriteString(formatSelectedItem(line))
			} else {
				b.WriteString(formatItem(" " + line))
			}
			b.WriteString("\n")
		}

		if m.tocOffset > 0 {
			b.WriteString(formatSubtitle("  ↑ 更多..."))
			b.WriteString("\n")
		}
		if end < len(m.chapters) {
			b.WriteString(formatSubtitle("  ↓ 更多..."))
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	helpText := "↑/↓: 选择  PgUp/PgDn: 翻页  Enter: 阅读  a: 加入书架"
	if m.currentBook != nil && len(m.currentBook.Sources) > 1 {
		helpText += "  c: 换源"
	}
	helpText += "  Esc: 返回"
	b.WriteString(formatHelp(helpText))

	return contentStyle.Render(b.String())
}

func (m Model) renderReader() string {
	var b strings.Builder

	// 章节标题
	if m.currentChapter != nil {
		b.WriteString(formatChapterTitle(m.currentChapter.Title))
		b.WriteString("\n\n")
	}

	// 计算可用于内容的行数
	contentLinesAvailable := m.linesPerPage
	if m.showDebugLog {
		// 调试日志占用一部分空间
		contentLinesAvailable = m.linesPerPage / 2
		if contentLinesAvailable < 5 {
			contentLinesAvailable = 5
		}
	}

	// 章节内容
	if len(m.contentLines) == 0 {
		b.WriteString(formatError("章节内容为空"))
	} else {
		end := m.lineOffset + contentLinesAvailable
		if end > len(m.contentLines) {
			end = len(m.contentLines)
		}

		for i := m.lineOffset; i < end; i++ {
			b.WriteString(m.contentLines[i])
			b.WriteString("\n")
		}
	}

	// 调试日志区域
	if m.showDebugLog {
		b.WriteString("\n")
		b.WriteString(formatSubtitle("─── 调试日志 (按 / 关闭) ───"))
		b.WriteString("\n")

		// 显示当前章节URL
		if m.currentChapter != nil && m.currentChapter.URL != "" {
			b.WriteString(formatItem(fmt.Sprintf("章节URL: %s", m.currentChapter.URL)))
			b.WriteString("\n")
		}

		// 显示书源信息
		if m.currentRule != nil {
			b.WriteString(formatItem(fmt.Sprintf("书源: %s (%s)", m.currentRule.Name, m.currentRule.URL)))
			b.WriteString("\n")
		}

		// 显示章节调试信息
		if m.currentChapter != nil && m.currentChapter.Debug != nil {
			debug := m.currentChapter.Debug
			b.WriteString(formatItem(fmt.Sprintf("HTTP状态码: %d | 响应长度: %d 字节", debug.ResponseCode, debug.ContentLength)))
			b.WriteString("\n")
			b.WriteString(formatItem(fmt.Sprintf("选择器: %s", debug.SelectorUsed)))
			b.WriteString("\n")

			if debug.ErrorMsg != "" {
				b.WriteString(formatError(fmt.Sprintf("错误: %s", debug.ErrorMsg)))
				b.WriteString("\n")
			}

			// 显示选择器匹配到的HTML片段
			if debug.SelectedHTML != "" {
				b.WriteString(formatSubtitle("选择器匹配内容:"))
				b.WriteString("\n")
				// 截取显示
				preview := debug.SelectedHTML
				if len(preview) > 500 {
					preview = preview[:500] + "..."
				}
				b.WriteString(formatItem(preview))
				b.WriteString("\n")

				// 显示过滤后的内容
				if debug.FilteredText != "" {
					b.WriteString(formatSubtitle("过滤后内容:"))
					b.WriteString("\n")
					preview = debug.FilteredText
					if len(preview) > 300 {
						preview = preview[:300] + "..."
					}
					b.WriteString(formatItem(preview))
					b.WriteString("\n")
				} else {
					b.WriteString(formatError("过滤后内容为空! 检查规则的 filterTxt/filterTag/paragraphTag 设置"))
					b.WriteString("\n")
				}
			} else {
				b.WriteString(formatError("选择器未匹配到任何内容!"))
				b.WriteString("\n")
				// 显示原始HTML片段帮助调试
				if debug.RawHTML != "" {
					b.WriteString(formatSubtitle("原始HTML片段:"))
					b.WriteString("\n")
					preview := debug.RawHTML
					if len(preview) > 800 {
						preview = preview[:800] + "..."
					}
					b.WriteString(formatItem(preview))
					b.WriteString("\n")
				}
			}
		}

		// 显示最近的调试日志
		if len(m.debugLogs) > 0 {
			b.WriteString(formatSubtitle("操作日志:"))
			b.WriteString("\n")
			logLines := 5
			startLog := len(m.debugLogs) - logLines
			if startLog < 0 {
				startLog = 0
			}
			for i := startLog; i < len(m.debugLogs); i++ {
				b.WriteString(formatItem(m.debugLogs[i]))
				b.WriteString("\n")
			}
		}
	}

	// 进度信息
	progress := float64(m.lineOffset+contentLinesAvailable) / float64(len(m.contentLines)) * 100
	if progress > 100 {
		progress = 100
	}
	chapterProgress := fmt.Sprintf("章节 %d/%d", m.chapterIndex+1, len(m.chapters))
	pageProgress := fmt.Sprintf("%.0f%%", progress)

	b.WriteString("\n")

	helpText := "j/k: 滚动  n/p: 上下章  t: 目录"
	if m.currentBook != nil && len(m.currentBook.Sources) > 1 {
		helpText += "  c: 换源"
	}
	helpText += fmt.Sprintf("  /: 调试  |  %s  %s", chapterProgress, pageProgress)

	b.WriteString(formatHelp(helpText))

	return contentStyle.Render(b.String())
}

func (m Model) renderSourceSwitch() string {
	var b strings.Builder

	bookName := ""
	if m.currentBook != nil {
		bookName = m.currentBook.BookName
	}

	b.WriteString(formatTitle(fmt.Sprintf("🔄 切换书源: %s", bookName)))
	b.WriteString("\n")
	b.WriteString(formatSubtitle("选择一个书源继续阅读"))
	b.WriteString("\n\n")

	for i, src := range m.availableSources {
		line := src.SourceName
		if src.SourceID == m.currentBook.CurrentSourceID {
			line += " (当前)"
		}
		if src.TotalChapters > 0 {
			line += fmt.Sprintf(" - %d章", src.TotalChapters)
		}
		if src.LatestChapter != "" {
			line += fmt.Sprintf(" - %s", src.LatestChapter)
		}

		if i == m.switchIndex {
			b.WriteString(formatSelectedItem(line))
		} else {
			b.WriteString(formatItem(" " + line))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	if m.statusMsg != "" {
		b.WriteString(formatHighlight(m.statusMsg))
		b.WriteString("\n")
	}
	b.WriteString(formatHelp("↑/↓: 选择  Enter: 确认切换  Esc: 取消"))

	return contentStyle.Render(b.String())
}

// === 辅助函数 ===

func (m Model) getBookshelfBooks() []*model.BookRecord {
	if m.bookshelf == nil {
		return nil
	}
	return m.bookshelf.GetBooksSortedByLastRead()
}

// addDebugLog 添加调试日志
func (m *Model) addDebugLog(format string, args ...any) {
	timestamp := time.Now().Format("15:04:05")
	log := fmt.Sprintf("[%s] %s", timestamp, fmt.Sprintf(format, args...))
	m.debugLogs = append(m.debugLogs, log)
	// 限制日志数量
	if len(m.debugLogs) > m.maxDebugLogs {
		m.debugLogs = m.debugLogs[len(m.debugLogs)-m.maxDebugLogs:]
	}
}
