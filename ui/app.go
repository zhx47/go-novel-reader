package ui

import (
	"fmt"
	"strings"
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
	aggregatedResults     *model.MultiSourceSearchResult
	resultIndex           int
	sourceIndex           int
	searchID              int
	searching             bool
	searchStartedAt       time.Time
	searchProgress        map[int]*model.SourceSearchStat
	searchResultsBySource map[int][]*model.SearchResultWithSource

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
	currentChapter        *model.Chapter
	contentLines          []string
	lineOffset            int
	linesPerPage          int
	readerFullscreen      bool
	resetLineOffsetOnLoad bool

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
	rulesLoadedMsg      struct{ rules []*model.Rule }
	bookshelfLoadedMsg  struct{ shelf *model.BookShelf }
	searchResultMsg     struct{ results []*model.SearchResult }
	sourceSearchDoneMsg struct {
		searchID   int
		keyword    string
		sourceID   int
		sourceName string
		results    []*model.SearchResultWithSource
		duration   time.Duration
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
	errorMsg struct{ err error }
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
		state:                 StateMainMenu,
		config:                cfg,
		store:                 store,
		sourceManager:         source.NewManagerWithConfig(cfg),
		httpClient:            httpClient,
		preloader:             preloader,
		searchInput:           ti,
		linesPerPage:          20,
		searchProgress:        make(map[int]*model.SourceSearchStat),
		searchResultsBySource: make(map[int][]*model.SearchResultWithSource),
		maxDebugLogs:          100,
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
		if m.state == StateReader {
			m.clampReaderOffset()
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
			if m.state == StateReader && m.readerFullscreen {
				m.readerFullscreen = false
				m.clampReaderOffset()
				return m, nil
			}
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
		m.aggregatedResults = nil
		m.resultIndex = 0
		m.sourceIndex = 0
		m.searching = false
		m.loading = false
		m.state = StateSearchResult
		if len(m.searchResults) == 0 {
			m.statusMsg = "未找到相关书籍"
		} else {
			m.statusMsg = fmt.Sprintf("找到 %d 本书籍", len(m.searchResults))
		}
		return m, nil

	case sourceSearchDoneMsg:
		if msg.searchID != m.searchID || msg.keyword != m.keyword {
			return m, nil
		}

		selectedKey := m.selectedAggregatedKey()
		selectedSourceID := m.selectedAggregatedSourceID()
		status := model.SearchStatusSuccess
		if msg.err != nil {
			status = model.SearchStatusFailed
			delete(m.searchResultsBySource, msg.sourceID)
		} else {
			m.searchResultsBySource[msg.sourceID] = msg.results
		}
		m.searchProgress[msg.sourceID] = &model.SourceSearchStat{
			SourceID:    msg.sourceID,
			SourceName:  msg.sourceName,
			Status:      status,
			ResultCount: len(msg.results),
			Duration:    msg.duration.Milliseconds(),
		}
		if msg.err != nil {
			m.searchProgress[msg.sourceID].Error = msg.err.Error()
		}
		m.aggregatedResults = model.AggregateSearchResults(m.keyword, m.searchResultsBySource)
		m.aggregatedResults.SourceStats = m.copySearchProgress()
		m.aggregatedResults.StartTime = m.searchStartedAt
		m.aggregatedResults.EndTime = time.Now()
		m.searching = !m.isSearchComplete()
		m.restoreSearchSelection(selectedKey, selectedSourceID)
		if m.state == StateSearchResult || m.state == StateSearch {
			m.updateSearchStatus()
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
		if m.resetLineOffsetOnLoad {
			m.lineOffset = 0
			m.resetLineOffsetOnLoad = false
		} else if m.fromBookshelf && m.currentBook != nil && m.store != nil {
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
			if saved := m.bookshelf.FindBook(msg.book.BookName, msg.book.Author); saved != nil {
				saved.SwitchSource(msg.book.CurrentSourceID)
			}
			if m.store != nil {
				m.store.SaveBookShelf(m.bookshelf)
			}
		}
		sourceName := ""
		if src := msg.book.GetCurrentSource(); src != nil {
			sourceName = src.SourceName
		}
		if sourceName == "" {
			m.statusMsg = fmt.Sprintf("已添加到书架: %s", msg.book.BookName)
		} else {
			m.statusMsg = fmt.Sprintf("已添加到书架: %s (%s)", msg.book.BookName, sourceName)
		}
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
			m.updateSearchStatus()
		}
	case StateReader:
		// 先改变状态，再保存进度（异步）
		m.readerFullscreen = false
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
func (m Model) doMultiSearch(searchID int, keyword string) tea.Cmd {
	if len(m.rules) == 0 {
		return func() tea.Msg {
			return errorMsg{fmt.Errorf("未找到可搜索书源")}
		}
	}

	cmds := make([]tea.Cmd, 0, len(m.rules))
	for _, rule := range m.rules {
		r := rule
		cmds = append(cmds, m.searchSource(searchID, keyword, r))
	}
	return tea.Batch(cmds...)
}

func (m Model) searchSource(searchID int, keyword string, rule *model.Rule) tea.Cmd {
	return func() tea.Msg {
		startedAt := time.Now()
		searchParser := parser.NewSearchParser(rule, m.httpClient)
		results, err := searchParser.Parse(keyword)
		if err != nil {
			return sourceSearchDoneMsg{
				searchID:   searchID,
				keyword:    keyword,
				sourceID:   rule.ID,
				sourceName: rule.Name,
				duration:   time.Since(startedAt),
				err:        err,
			}
		}

		sourceResults := make([]*model.SearchResultWithSource, 0, len(results))
		for _, result := range results {
			result.SourceID = rule.ID
			sourceResults = append(sourceResults, &model.SearchResultWithSource{
				SearchResult: result,
				SourceName:   rule.Name,
			})
		}

		return sourceSearchDoneMsg{
			searchID:   searchID,
			keyword:    keyword,
			sourceID:   rule.ID,
			sourceName: rule.Name,
			results:    sourceResults,
			duration:   time.Since(startedAt),
		}
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
			if src := m.selectedAggregatedSource(); src != nil {
				book = agg.ToBookRecordWithSource(src.SourceID)
			} else {
				book = agg.ToBookRecord()
			}
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
		m.keyword = strings.TrimSpace(m.searchInput.Value())
		if m.keyword != "" {
			if len(m.rules) == 0 {
				m.statusMsg = "未找到可搜索书源"
				return m, nil
			}
			m.searchID++
			m.searching = true
			m.searchStartedAt = time.Now()
			m.resultIndex = 0
			m.sourceIndex = 0
			m.currentBook = nil
			m.selectedBook = nil
			m.searchResults = nil
			m.searchProgress = make(map[int]*model.SourceSearchStat, len(m.rules))
			m.searchResultsBySource = make(map[int][]*model.SearchResultWithSource)
			for _, r := range m.rules {
				m.searchProgress[r.ID] = &model.SourceSearchStat{
					SourceID:   r.ID,
					SourceName: r.Name,
					Status:     model.SearchStatusRunning,
				}
			}
			m.aggregatedResults = model.NewMultiSourceSearchResult(m.keyword)
			m.aggregatedResults.SourceStats = m.copySearchProgress()
			m.state = StateSearchResult
			m.loading = false
			m.updateSearchStatus()
			// 使用多源搜索
			return m, m.doMultiSearch(m.searchID, m.keyword)
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
			m.sourceIndex = 0
		}
	case "down", "j":
		if m.resultIndex < resultCount-1 {
			m.resultIndex++
			m.sourceIndex = 0
		}
	case "left", "h":
		if m.sourceIndex > 0 {
			m.sourceIndex--
		}
	case "right", "l", "tab":
		if m.sourceIndex < m.selectedAggregatedSourceCount()-1 {
			m.sourceIndex++
		}
	case "enter":
		if resultCount > 0 {
			if m.aggregatedResults != nil {
				agg := m.aggregatedResults.Results[m.resultIndex]
				selectedSource := m.selectedAggregatedSource()
				if selectedSource != nil {
					// 创建书籍记录
					m.currentBook = agg.ToBookRecordWithSource(selectedSource.SourceID)
					m.selectedBook = selectedSource.SearchResult
					// 设置当前规则
					for _, r := range m.rules {
						if r.ID == selectedSource.SourceID {
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
	pageLines := m.readerContentLineCount()
	switch msg.String() {
	case "f11":
		m.readerFullscreen = !m.readerFullscreen
		m.clampReaderOffset()
	case "up", "k":
		if m.lineOffset > 0 {
			m.lineOffset--
		}
	case "down", "j", " ":
		if m.lineOffset < len(m.contentLines)-pageLines {
			m.lineOffset++
		}
	case "pgup":
		m.lineOffset -= pageLines
		if m.lineOffset < 0 {
			m.lineOffset = 0
		}
	case "pgdown":
		m.lineOffset += pageLines
		maxOffset := len(m.contentLines) - pageLines
		if maxOffset < 0 {
			maxOffset = 0
		}
		if m.lineOffset > maxOffset {
			m.lineOffset = maxOffset
		}
	case "home", "g":
		m.lineOffset = 0
	case "end", "G":
		m.lineOffset = len(m.contentLines) - pageLines
		if m.lineOffset < 0 {
			m.lineOffset = 0
		}
	case "n", "right", "l":
		// 下一章
		if m.chapterIndex < len(m.chapters)-1 {
			saveCmd := m.saveProgress()
			m.chapterIndex++
			m.lineOffset = 0
			m.resetLineOffsetOnLoad = true
			m.loading = true
			m.statusMsg = "加载下一章..."
			// 同时保存进度和加载下一章
			return m, tea.Batch(saveCmd, m.loadChapter(m.chapterIndex))
		}
	case "p", "left", "h":
		// 上一章
		if m.chapterIndex > 0 {
			saveCmd := m.saveProgress()
			m.chapterIndex--
			m.lineOffset = 0
			m.resetLineOffsetOnLoad = true
			m.loading = true
			m.statusMsg = "加载上一章..."
			// 同时保存进度和加载上一章
			return m, tea.Batch(saveCmd, m.loadChapter(m.chapterIndex))
		}
	case "t":
		// 返回目录
		m.readerFullscreen = false
		m.state = StateToc
		return m, m.saveProgress()
	case "c":
		// 切换书源
		if m.currentBook != nil && len(m.currentBook.Sources) > 1 {
			m.readerFullscreen = false
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
		maxOffset := len(m.contentLines) - m.readerContentLineCount()
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

		visibleItems := m.searchVisibleItems()
		itemStartY := contentStartY + 1
		leftWidth, _, stacked := m.searchColumnWidths()
		if m.aggregatedResults != nil && !stacked && x >= leftWidth+4 {
			selected := m.selectedAggregatedResult()
			if selected == nil || len(selected.Sources) == 0 {
				return m, nil
			}
			sourceIndex := clampIndex(m.sourceIndex, len(selected.Sources))
			start, _ := visibleWindow(sourceIndex, len(selected.Sources), visibleItems)
			clickedSourceIndex := y - itemStartY + start
			if clickedSourceIndex >= 0 && clickedSourceIndex < len(selected.Sources) {
				m.sourceIndex = clickedSourceIndex
			}
			return m, nil
		}

		start, _ := visibleWindow(m.resultIndex, resultCount, visibleItems)
		clickedIndex := y - itemStartY + start
		if clickedIndex >= 0 && clickedIndex < resultCount {
			if m.resultIndex == clickedIndex {
				// 点击已选中项，进入
				return m.updateSearchResult(tea.KeyMsg{Type: tea.KeyEnter})
			}
			m.resultIndex = clickedIndex
			m.sourceIndex = 0
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

	if m.aggregatedResults != nil {
		b.WriteString(m.renderAggregatedSearchResults())
	} else if len(m.searchResults) == 0 {
		b.WriteString(formatError("未找到相关书籍"))
	} else {
		b.WriteString(m.renderLegacySearchResults())
	}

	b.WriteString("\n")
	b.WriteString(formatHelp("↑/↓: 选择小说  ←/→: 选择书源  Enter: 查看目录  a: 用当前源加入书架  Esc: 返回"))

	return contentStyle.Render(b.String())
}

func (m Model) renderAggregatedSearchResults() string {
	leftWidth, rightWidth, stacked := m.searchColumnWidths()
	visibleItems := m.searchVisibleItems()
	results := m.aggregatedResults.Results

	var left strings.Builder
	left.WriteString(formatSubtitle("小说"))
	left.WriteString("\n")
	if len(results) == 0 {
		if m.searching {
			left.WriteString(formatLoading("搜索中，等待书源返回结果..."))
		} else {
			left.WriteString(formatError("未找到相关书籍"))
		}
		left.WriteString("\n")
	} else {
		start, end := visibleWindow(m.resultIndex, len(results), visibleItems)
		for i := start; i < end; i++ {
			result := results[i]
			line := fmt.Sprintf("%s - %s", result.BookName, result.Author)
			if result.SourceCount > 1 {
				line += fmt.Sprintf(" [%d源]", result.SourceCount)
			}
			if result.LatestChapter != "" {
				line += fmt.Sprintf(" | %s", result.LatestChapter)
			}
			line = truncateDisplay(line, leftWidth-4)
			if i == m.resultIndex {
				left.WriteString(formatSelectedItem(line))
			} else {
				left.WriteString(formatItem(" " + line))
			}
			left.WriteString("\n")
		}
	}

	var right strings.Builder
	selected := m.selectedAggregatedResult()
	if selected == nil {
		right.WriteString(formatSubtitle("书源进度"))
		right.WriteString("\n")
		right.WriteString(m.renderSearchProgressList(visibleItems, rightWidth))
	} else {
		right.WriteString(formatSubtitle(truncateDisplay(fmt.Sprintf("书源: %s", selected.BookName), rightWidth-2)))
		right.WriteString("\n")
		if len(selected.Sources) == 0 {
			right.WriteString(formatError("当前小说暂无可用书源"))
			right.WriteString("\n")
		} else {
			sourceIndex := clampIndex(m.sourceIndex, len(selected.Sources))
			start, end := visibleWindow(sourceIndex, len(selected.Sources), visibleItems)
			for i := start; i < end; i++ {
				src := selected.Sources[i]
				line := src.SourceName
				if src.LatestChapter != "" {
					line += fmt.Sprintf(" | %s", src.LatestChapter)
				}
				if src.LastUpdateTime != "" {
					line += fmt.Sprintf(" (%s)", src.LastUpdateTime)
				}
				line = truncateDisplay(line, rightWidth-4)
				if i == sourceIndex {
					right.WriteString(formatSelectedItem(line))
				} else {
					right.WriteString(formatItem(" " + line))
				}
				right.WriteString("\n")
			}
			if m.searching {
				right.WriteString("\n")
				right.WriteString(formatLoading("搜索中，后续书源会继续合并"))
				right.WriteString("\n")
			}
		}
	}

	if stacked {
		return left.String() + "\n" + right.String()
	}

	leftPane := lipgloss.NewStyle().Width(leftWidth).Render(left.String())
	rightPane := lipgloss.NewStyle().Width(rightWidth).PaddingLeft(2).Render(right.String())
	return lipgloss.JoinHorizontal(lipgloss.Top, leftPane, rightPane)
}

func (m Model) renderLegacySearchResults() string {
	var b strings.Builder

	visibleItems := m.searchVisibleItems()
	start, end := visibleWindow(m.resultIndex, len(m.searchResults), visibleItems)
	for i := start; i < end; i++ {
		result := m.searchResults[i]
		line := fmt.Sprintf("%s - %s", result.BookName, result.Author)
		if result.LatestChapter != "" {
			line += fmt.Sprintf(" | %s", result.LatestChapter)
		}
		line = truncateDisplay(line, m.width-8)
		if i == m.resultIndex {
			b.WriteString(formatSelectedItem(line))
		} else {
			b.WriteString(formatItem(" " + line))
		}
		b.WriteString("\n")
	}

	return b.String()
}

func (m Model) renderSearchProgressList(limit int, width int) string {
	var b strings.Builder
	count := 0
	for _, rule := range m.rules {
		if count >= limit {
			break
		}
		stat := m.searchProgress[rule.ID]
		status := model.SearchStatusRunning
		resultCount := 0
		if stat != nil {
			status = stat.Status
			resultCount = stat.ResultCount
		}
		line := fmt.Sprintf("%s: %s", rule.Name, status.String())
		if resultCount > 0 {
			line += fmt.Sprintf("，%d 条", resultCount)
		}
		line = truncateDisplay(line, width-4)
		b.WriteString(formatItem(" " + line))
		b.WriteString("\n")
		count++
	}
	return b.String()
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

	if !m.readerFullscreen && m.currentChapter != nil {
		b.WriteString(formatChapterTitle(m.currentChapter.Title))
		b.WriteString("\n\n")
	}

	contentLinesAvailable := m.readerContentLineCount()

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

	if m.readerFullscreen {
		return contentStyle.Render(b.String())
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
	progress := 0.0
	if len(m.contentLines) > 0 {
		progress = float64(m.lineOffset+contentLinesAvailable) / float64(len(m.contentLines)) * 100
	}
	if progress > 100 {
		progress = 100
	}
	chapterProgress := fmt.Sprintf("章节 %d/%d", m.chapterIndex+1, len(m.chapters))
	pageProgress := fmt.Sprintf("%.0f%%", progress)

	b.WriteString("\n")

	helpText := "j/k: 滚动  n/p: 上下章  t: 目录  F11: 沉浸"
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

func (m Model) readerContentLineCount() int {
	if m.readerFullscreen {
		lines := m.height - 2
		if lines < 5 {
			lines = m.linesPerPage
		}
		if lines < 5 {
			lines = 5
		}
		return lines
	}

	lines := m.linesPerPage
	if m.showDebugLog {
		lines = m.linesPerPage / 2
		if lines < 5 {
			lines = 5
		}
	}
	return lines
}

func (m Model) readerMaxLineOffset() int {
	maxOffset := len(m.contentLines) - m.readerContentLineCount()
	if maxOffset < 0 {
		return 0
	}
	return maxOffset
}

func (m *Model) clampReaderOffset() {
	maxOffset := m.readerMaxLineOffset()
	if m.lineOffset > maxOffset {
		m.lineOffset = maxOffset
	}
	if m.lineOffset < 0 {
		m.lineOffset = 0
	}
}

func (m Model) selectedAggregatedResult() *model.AggregatedSearchResult {
	if m.aggregatedResults == nil || len(m.aggregatedResults.Results) == 0 {
		return nil
	}
	return m.aggregatedResults.Results[clampIndex(m.resultIndex, len(m.aggregatedResults.Results))]
}

func (m Model) selectedAggregatedSource() *model.SearchResultWithSource {
	selected := m.selectedAggregatedResult()
	if selected == nil || len(selected.Sources) == 0 {
		return nil
	}
	return selected.Sources[clampIndex(m.sourceIndex, len(selected.Sources))]
}

func (m Model) selectedAggregatedSourceCount() int {
	selected := m.selectedAggregatedResult()
	if selected == nil {
		return 0
	}
	return len(selected.Sources)
}

func (m Model) selectedAggregatedKey() string {
	selected := m.selectedAggregatedResult()
	if selected == nil {
		return ""
	}
	return selected.NormalizedKey
}

func (m Model) selectedAggregatedSourceID() int {
	selected := m.selectedAggregatedSource()
	if selected == nil {
		return 0
	}
	return selected.SourceID
}

func (m *Model) restoreSearchSelection(selectedKey string, selectedSourceID int) {
	if m.aggregatedResults == nil || len(m.aggregatedResults.Results) == 0 {
		m.resultIndex = 0
		m.sourceIndex = 0
		return
	}

	if selectedKey != "" {
		for i, result := range m.aggregatedResults.Results {
			if result.NormalizedKey == selectedKey {
				m.resultIndex = i
				break
			}
		}
	}
	m.resultIndex = clampIndex(m.resultIndex, len(m.aggregatedResults.Results))

	selected := m.aggregatedResults.Results[m.resultIndex]
	if selectedSourceID != 0 {
		for i, src := range selected.Sources {
			if src.SourceID == selectedSourceID {
				m.sourceIndex = i
				return
			}
		}
	}
	m.sourceIndex = clampIndex(m.sourceIndex, len(selected.Sources))
}

func (m Model) copySearchProgress() map[int]*model.SourceSearchStat {
	copied := make(map[int]*model.SourceSearchStat, len(m.searchProgress))
	for sourceID, stat := range m.searchProgress {
		if stat == nil {
			continue
		}
		value := *stat
		copied[sourceID] = &value
	}
	return copied
}

func (m Model) isSearchComplete() bool {
	total := m.searchSourceTotal()
	return total > 0 && m.completedSearchSourceCount() >= total
}

func (m Model) searchSourceTotal() int {
	if len(m.searchProgress) > 0 {
		return len(m.searchProgress)
	}
	return len(m.rules)
}

func (m Model) completedSearchSourceCount() int {
	count := 0
	for _, stat := range m.searchProgress {
		if stat != nil && isFinalSearchStatus(stat.Status) {
			count++
		}
	}
	return count
}

func (m Model) resultSourceCount() int {
	count := 0
	for _, results := range m.searchResultsBySource {
		if len(results) > 0 {
			count++
		}
	}
	return count
}

func isFinalSearchStatus(status model.SearchStatus) bool {
	return status == model.SearchStatusSuccess ||
		status == model.SearchStatusFailed ||
		status == model.SearchStatusTimeout
}

func (m *Model) updateSearchStatus() {
	m.statusMsg = m.searchStatusText()
}

func (m Model) searchStatusText() string {
	total := m.searchSourceTotal()
	if total == 0 {
		return "未找到可搜索书源"
	}

	done := m.completedSearchSourceCount()
	found := 0
	if m.aggregatedResults != nil {
		found = m.aggregatedResults.TotalCount
	}

	if m.searching {
		if found == 0 {
			return fmt.Sprintf("搜索中... %d/%d 个书源完成", done, total)
		}
		return fmt.Sprintf("已找到 %d 本，搜索中... %d/%d 个书源完成", found, done, total)
	}

	if found == 0 {
		return fmt.Sprintf("未找到相关书籍 (已搜索 %d 个书源)", done)
	}
	return fmt.Sprintf("找到 %d 本书籍 (来自 %d 个书源)", found, m.resultSourceCount())
}

func (m Model) searchColumnWidths() (int, int, bool) {
	contentWidth := m.width - 4
	if contentWidth <= 0 {
		contentWidth = 80
	}
	if contentWidth < 64 {
		return contentWidth, contentWidth, true
	}

	gap := 2
	leftWidth := contentWidth * 45 / 100
	if leftWidth < 30 {
		leftWidth = 30
	}
	rightWidth := contentWidth - leftWidth - gap
	if rightWidth < 30 {
		rightWidth = 30
		leftWidth = contentWidth - rightWidth - gap
	}
	if leftWidth < 24 {
		return contentWidth, contentWidth, true
	}
	return leftWidth, rightWidth, false
}

func (m Model) searchVisibleItems() int {
	visibleItems := m.height - 12
	if visibleItems < 4 {
		visibleItems = 4
	}
	return visibleItems
}

func visibleWindow(index int, count int, visibleItems int) (int, int) {
	if count <= 0 {
		return 0, 0
	}
	if visibleItems <= 0 || visibleItems > count {
		visibleItems = count
	}

	index = clampIndex(index, count)
	start := 0
	if index >= visibleItems {
		start = index - visibleItems + 1
	}
	if start+visibleItems > count {
		start = count - visibleItems
	}
	if start < 0 {
		start = 0
	}
	return start, start + visibleItems
}

func clampIndex(index int, count int) int {
	if count <= 0 {
		return 0
	}
	if index < 0 {
		return 0
	}
	if index >= count {
		return count - 1
	}
	return index
}

func truncateDisplay(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if runewidth.StringWidth(s) <= width {
		return s
	}
	if width <= 3 {
		return runewidth.Truncate(s, width, "")
	}
	return runewidth.Truncate(s, width, "...")
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
