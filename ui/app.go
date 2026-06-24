package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"go-novel-reader/config"
	"go-novel-reader/httpclient"
	"go-novel-reader/model"
	"go-novel-reader/parser"
	"go-novel-reader/service"
	"go-novel-reader/source"
	"go-novel-reader/storage"
)

type AppState int

const (
	StateMainMenu AppState = iota
	StateBookShelf
	StateSearch
	StateSearchResult
	StateToc
	StateReader
	StateSourceSwitch
)

type sourcePanelMode int

const (
	sourcePanelProgress sourcePanelMode = iota
	sourcePanelSources
)

type statusLevel int

const (
	statusInfo statusLevel = iota
	statusSuccess
	statusWarning
	statusError
)

const (
	pageMainMenu     = "main-menu"
	pageBookShelf    = "bookshelf"
	pageSearch       = "search"
	pageSearchResult = "search-result"
	pageToc          = "toc"
	pageReader       = "reader"
	pageSourceSwitch = "source-switch"
	pageLoading      = "loading"
)

var (
	colorPrimary = tcell.NewHexColor(0x7C3AED)
	colorAccent  = tcell.NewHexColor(0x10B981)
	colorWarning = tcell.NewHexColor(0xF59E0B)
	colorError   = tcell.NewHexColor(0xEF4444)
	colorMuted   = tcell.NewHexColor(0x94A3B8)
)

type sourceSearchDone struct {
	searchID   int
	keyword    string
	sourceID   int
	sourceName string
	results    []*model.SearchResultWithSource
	duration   time.Duration
	err        error
}

type tocLoadResult struct {
	chapters  []*model.Chapter
	fromCache bool
	err       error
}

type chapterLoadResult struct {
	chapter   *model.Chapter
	fromCache bool
	err       error
}

type updateCheckResult struct {
	bookID        string
	hasUpdate     bool
	newChapters   int
	latestChapter string
	err           error
}

type App struct {
	app          *tview.Application
	root         *tview.Flex
	headerFlex   *tview.Flex
	contentPages *tview.Pages

	titleView    *tview.TextView
	subtitleView *tview.TextView
	statusView   *tview.TextView
	helpView     *tview.TextView
	loadingView  *tview.TextView

	mainMenuList     *tview.List
	bookshelfList    *tview.List
	searchInput      *tview.InputField
	searchLayout     *tview.Flex
	searchResultList *tview.List
	searchSourceList *tview.List
	searchPreview    *tview.TextView
	tocList          *tview.List
	readerLayout     *tview.Flex
	readerContent    *tview.TextView
	debugView        *tview.TextView
	sourceSwitchList *tview.List

	state            AppState
	width            int
	height           int
	showingLoading   bool
	busy             bool
	loadingTitle     string
	loadingSubtitle  string
	readerFullscreen bool
	showDebugLog     bool

	config        *config.AppConfig
	store         storage.Store
	sourceManager *source.Manager
	rules         []*model.Rule
	currentRule   *model.Rule
	httpClient    *httpclient.Client
	preloader     *service.ChapterPreloader

	bookshelf      *model.BookShelf
	bookshelfIndex int
	fromBookshelf  bool

	keyword               string
	aggregatedResults     *model.MultiSourceSearchResult
	resultIndex           int
	sourceIndex           int
	searchID              int
	searching             bool
	searchStartedAt       time.Time
	searchProgress        map[int]*model.SourceSearchStat
	searchResultsBySource map[int][]*model.SearchResultWithSource
	sourcePanelMode       sourcePanelMode

	currentBook  *model.BookRecord
	selectedBook *model.SearchResult

	chapters     []*model.Chapter
	chapterIndex int

	availableSources []*model.BookSource
	switchIndex      int

	currentChapter *model.Chapter

	debugLogs    []string
	maxDebugLogs int

	statusMsg   string
	statusLevel statusLevel
	err         error

	refreshingBookshelf    bool
	refreshingResultList   bool
	refreshingSourceList   bool
	refreshingToc          bool
	refreshingSourceSwitch bool
}

func NewApp(cfg *config.AppConfig) *App {
	if cfg == nil {
		cfg = config.DefaultConfig()
	}

	sqliteStore, err := storage.NewDefaultSQLiteStore()
	if err != nil {
		fmt.Printf("警告: 无法创建持久化存储: %v\n", err)
	}

	var store storage.Store = sqliteStore
	httpClient := httpclient.NewClient(cfg)

	var preloader *service.ChapterPreloader
	if sqliteStore != nil {
		preloader = service.NewChapterPreloader(sqliteStore, httpClient, 3)
	}

	app := &App{
		app:                   tview.NewApplication(),
		config:                cfg,
		store:                 store,
		sourceManager:         source.NewManagerWithConfig(cfg),
		httpClient:            httpClient,
		preloader:             preloader,
		bookshelf:             model.NewBookShelf(),
		searchProgress:        make(map[int]*model.SourceSearchStat),
		searchResultsBySource: make(map[int][]*model.SearchResultWithSource),
		debugLogs:             make([]string, 0, 32),
		maxDebugLogs:          100,
		statusLevel:           statusInfo,
	}

	app.loadInitialData()
	app.buildUI()
	app.refreshAll()
	app.switchState(StateMainMenu)
	return app
}

func (a *App) Run() error {
	a.app.SetTitle("go-novel-reader")
	a.app.SetRoot(a.root, true)
	a.app.EnableMouse(true)
	a.app.SetInputCapture(a.captureGlobalInput)
	a.app.SetBeforeDrawFunc(func(screen tcell.Screen) bool {
		width, height := screen.Size()
		if width != a.width || height != a.height {
			a.width = width
			a.height = height
			a.handleResize()
		}
		return false
	})
	return a.app.Run()
}

func (a *App) loadInitialData() {
	rules, err := a.sourceManager.GetSearchableRules(a.config.ActiveRules)
	if err != nil {
		a.setStatus(fmt.Sprintf("加载书源失败: %v", err), statusError)
		a.addDebugLog("加载书源失败: %v", err)
	} else {
		a.rules = rules
		if len(rules) > 0 {
			a.currentRule = rules[0]
		}
	}

	if a.store == nil {
		return
	}

	shelf, err := a.store.LoadBookShelf()
	if err != nil {
		a.setStatus(fmt.Sprintf("加载书架失败: %v", err), statusError)
		a.addDebugLog("加载书架失败: %v", err)
		return
	}
	if shelf != nil {
		a.bookshelf = shelf
	}
}

func (a *App) captureGlobalInput(event *tcell.EventKey) *tcell.EventKey {
	if event == nil {
		return nil
	}

	if event.Key() == tcell.KeyCtrlC {
		a.app.Stop()
		return nil
	}

	if a.busy {
		return nil
	}

	focus := a.app.GetFocus()
	if focus == a.searchInput {
		return event
	}

	if a.state == StateReader && a.readerFullscreen {
		if event.Key() == tcell.KeyEsc {
			a.readerFullscreen = false
			a.refreshReaderLayout()
			a.refreshChrome()
			return nil
		}
		if event.Key() == tcell.KeyF11 {
			a.toggleReaderFullscreen()
			return nil
		}
	}

	switch event.Key() {
	case tcell.KeyEsc:
		a.handleBack()
		return nil
	case tcell.KeyF11:
		if a.state == StateReader {
			a.toggleReaderFullscreen()
			return nil
		}
	}

	if event.Key() == tcell.KeyRune {
		switch event.Rune() {
		case 'q':
			if a.state == StateMainMenu {
				a.app.Stop()
			} else {
				a.handleBack()
			}
			return nil
		case '1':
			if a.state == StateMainMenu {
				a.openBookshelfPage()
				return nil
			}
		case '2':
			if a.state == StateMainMenu {
				a.openSearchPage()
				return nil
			}
		}
	}

	switch a.state {
	case StateBookShelf:
		return a.handleBookshelfKeys(event)
	case StateSearchResult:
		return a.handleSearchResultKeys(event)
	case StateToc:
		return a.handleTocKeys(event)
	case StateReader:
		return a.handleReaderKeys(event)
	}

	return event
}

func (a *App) handleBookshelfKeys(event *tcell.EventKey) *tcell.EventKey {
	if event.Key() != tcell.KeyRune {
		return event
	}

	switch event.Rune() {
	case 'd':
		a.removeSelectedBookshelfBook()
		return nil
	case 'u':
		a.checkSelectedBookUpdate()
		return nil
	case 's':
		a.openSearchPage()
		return nil
	}
	return event
}

func (a *App) handleSearchResultKeys(event *tcell.EventKey) *tcell.EventKey {
	if event.Key() == tcell.KeyTAB || event.Key() == tcell.KeyRight {
		if a.sourcePanelMode == sourcePanelSources && a.searchSourceList.GetItemCount() > 0 {
			a.app.SetFocus(a.searchSourceList)
			return nil
		}
		return event
	}
	if event.Key() == tcell.KeyLeft || event.Key() == tcell.KeyBacktab {
		a.app.SetFocus(a.searchResultList)
		return nil
	}
	if event.Key() != tcell.KeyRune {
		return event
	}

	switch event.Rune() {
	case 'h':
		a.app.SetFocus(a.searchResultList)
		return nil
	case 'l':
		if a.sourcePanelMode == sourcePanelSources && a.searchSourceList.GetItemCount() > 0 {
			a.app.SetFocus(a.searchSourceList)
			return nil
		}
	case 'a':
		a.addCurrentSelectionToBookshelf()
		return nil
	}
	return event
}

func (a *App) handleTocKeys(event *tcell.EventKey) *tcell.EventKey {
	if event.Key() != tcell.KeyRune {
		return event
	}

	switch event.Rune() {
	case 'a':
		a.addCurrentSelectionToBookshelf()
		return nil
	case 'c':
		a.openSourceSwitch()
		return nil
	}
	return event
}

func (a *App) handleReaderKeys(event *tcell.EventKey) *tcell.EventKey {
	if event.Key() == tcell.KeyRune && event.Rune() == ' ' {
		a.scheduleReaderChromeRefresh()
		return event
	}

	switch event.Key() {
	case tcell.KeyUp, tcell.KeyDown, tcell.KeyPgUp, tcell.KeyPgDn, tcell.KeyHome, tcell.KeyEnd:
		a.scheduleReaderChromeRefresh()
		return event
	}

	if event.Key() != tcell.KeyRune {
		return event
	}

	switch event.Rune() {
	case 'j', 'k', 'g', 'G':
		a.scheduleReaderChromeRefresh()
		return event
	case 'n':
		a.loadRelativeChapter(1)
		return nil
	case 'p':
		a.loadRelativeChapter(-1)
		return nil
	case 't':
		a.saveCurrentProgress()
		a.switchState(StateToc)
		return nil
	case 'c':
		a.saveCurrentProgress()
		a.openSourceSwitch()
		return nil
	case 'a':
		a.addCurrentSelectionToBookshelf()
		return nil
	case '/':
		a.showDebugLog = !a.showDebugLog
		a.refreshReaderLayout()
		a.refreshChrome()
		return nil
	}
	return event
}

func (a *App) handleBack() {
	switch a.state {
	case StateMainMenu:
		a.app.Stop()
	case StateBookShelf, StateSearch:
		a.fromBookshelf = false
		a.switchState(StateMainMenu)
	case StateSearchResult:
		a.switchState(StateSearch)
		a.app.SetFocus(a.searchInput)
	case StateToc:
		if a.fromBookshelf {
			a.switchState(StateBookShelf)
			return
		}
		a.switchState(StateSearchResult)
	case StateReader:
		a.readerFullscreen = false
		a.saveCurrentProgress()
		a.switchState(StateToc)
	case StateSourceSwitch:
		a.switchState(StateToc)
	}
}

func (a *App) switchState(state AppState) {
	a.state = state
	if !a.showingLoading {
		a.contentPages.SwitchToPage(a.pageName(state))
	}
	a.refreshChrome()
	a.focusCurrentPage()
}

func (a *App) pageName(state AppState) string {
	switch state {
	case StateMainMenu:
		return pageMainMenu
	case StateBookShelf:
		return pageBookShelf
	case StateSearch:
		return pageSearch
	case StateSearchResult:
		return pageSearchResult
	case StateToc:
		return pageToc
	case StateReader:
		return pageReader
	case StateSourceSwitch:
		return pageSourceSwitch
	default:
		return pageMainMenu
	}
}

func (a *App) focusCurrentPage() {
	if a.showingLoading {
		a.app.SetFocus(a.loadingView)
		return
	}

	switch a.state {
	case StateMainMenu:
		a.app.SetFocus(a.mainMenuList)
	case StateBookShelf:
		a.app.SetFocus(a.bookshelfList)
	case StateSearch:
		a.app.SetFocus(a.searchInput)
	case StateSearchResult:
		if a.app.GetFocus() == a.searchSourceList && a.sourcePanelMode == sourcePanelSources && a.searchSourceList.GetItemCount() > 0 {
			return
		}
		a.app.SetFocus(a.searchResultList)
	case StateToc:
		a.app.SetFocus(a.tocList)
	case StateReader:
		a.app.SetFocus(a.readerContent)
	case StateSourceSwitch:
		a.app.SetFocus(a.sourceSwitchList)
	}
}

func (a *App) openBookshelfPage() {
	a.refreshBookshelfList()
	a.switchState(StateBookShelf)
}

func (a *App) openSearchPage() {
	a.switchState(StateSearch)
	a.app.SetFocus(a.searchInput)
}

func (a *App) startSearch() {
	keyword := strings.TrimSpace(a.searchInput.GetText())
	if keyword == "" {
		a.setStatus("请输入书名或作者", statusWarning)
		a.refreshChrome()
		return
	}
	if len(a.rules) == 0 {
		a.setStatus("未找到可搜索书源", statusError)
		a.refreshChrome()
		return
	}

	a.keyword = keyword
	a.searchID++
	a.searching = true
	a.searchStartedAt = time.Now()
	a.resultIndex = 0
	a.sourceIndex = 0
	a.currentBook = nil
	a.selectedBook = nil
	a.fromBookshelf = false
	a.searchProgress = make(map[int]*model.SourceSearchStat, len(a.rules))
	a.searchResultsBySource = make(map[int][]*model.SearchResultWithSource)
	a.aggregatedResults = model.NewMultiSourceSearchResult(keyword)
	for _, rule := range a.rules {
		a.searchProgress[rule.ID] = &model.SourceSearchStat{
			SourceID:   rule.ID,
			SourceName: rule.Name,
			Status:     model.SearchStatusRunning,
		}
	}
	a.aggregatedResults.SourceStats = a.copySearchProgress()
	a.updateSearchStatus()
	a.refreshSearchResults()
	a.switchState(StateSearchResult)

	for _, rule := range a.rules {
		r := rule
		go a.searchSource(a.searchID, keyword, r)
	}
}

func (a *App) searchSource(searchID int, keyword string, rule *model.Rule) {
	startedAt := time.Now()
	searchParser := parser.NewSearchParser(rule, a.httpClient)
	results, err := searchParser.Parse(keyword)

	msg := sourceSearchDone{
		searchID:   searchID,
		keyword:    keyword,
		sourceID:   rule.ID,
		sourceName: rule.Name,
		duration:   time.Since(startedAt),
		err:        err,
	}

	if err == nil {
		msg.results = make([]*model.SearchResultWithSource, 0, len(results))
		for _, result := range results {
			result.SourceID = rule.ID
			msg.results = append(msg.results, &model.SearchResultWithSource{
				SearchResult: result,
				SourceName:   rule.Name,
			})
		}
	}

	a.app.QueueUpdateDraw(func() {
		a.handleSourceSearchDone(msg)
	})
}

func (a *App) handleSourceSearchDone(msg sourceSearchDone) {
	if msg.searchID != a.searchID || msg.keyword != a.keyword {
		return
	}

	selectedKey := a.selectedAggregatedKey()
	selectedSourceID := a.selectedAggregatedSourceID()

	status := model.SearchStatusSuccess
	if msg.err != nil {
		status = model.SearchStatusFailed
		delete(a.searchResultsBySource, msg.sourceID)
		a.addDebugLog("搜索失败: %s - %v", msg.sourceName, msg.err)
	} else {
		a.searchResultsBySource[msg.sourceID] = msg.results
		a.addDebugLog("搜索完成: %s - %d 条结果", msg.sourceName, len(msg.results))
	}

	a.searchProgress[msg.sourceID] = &model.SourceSearchStat{
		SourceID:    msg.sourceID,
		SourceName:  msg.sourceName,
		Status:      status,
		ResultCount: len(msg.results),
		Duration:    msg.duration.Milliseconds(),
	}
	if msg.err != nil {
		a.searchProgress[msg.sourceID].Error = msg.err.Error()
	}

	a.aggregatedResults = model.AggregateSearchResults(a.keyword, a.searchResultsBySource)
	a.aggregatedResults.SourceStats = a.copySearchProgress()
	a.aggregatedResults.StartTime = a.searchStartedAt
	a.aggregatedResults.EndTime = time.Now()
	a.searching = !a.isSearchComplete()
	a.restoreSearchSelection(selectedKey, selectedSourceID)
	a.updateSearchStatus()
	a.refreshSearchResults()
	a.refreshChrome()
}

func (a *App) openSelectedSearchResult() {
	selected := a.selectedAggregatedResult()
	if selected == nil {
		return
	}

	selectedSource := a.selectedAggregatedSource()
	if selectedSource == nil {
		return
	}

	book := selected.ToBookRecordWithSource(selectedSource.SourceID)
	if book == nil {
		a.setStatus("无法创建书籍记录", statusError)
		a.refreshChrome()
		return
	}

	if a.bookshelf != nil {
		if existing := a.bookshelf.FindBook(book.BookName, book.Author); existing != nil {
			for _, src := range book.Sources {
				existing.AddSource(src)
			}
			existing.SwitchSource(book.CurrentSourceID)
			book = existing
		}
	}

	a.currentBook = book
	a.selectedBook = selectedSource.SearchResult
	a.currentRule = a.findRuleBySourceID(selectedSource.SourceID)
	a.fromBookshelf = false
	a.beginLoadToc()
}

func (a *App) openSelectedBookshelfBook() {
	books := a.getBookshelfBooks()
	if len(books) == 0 {
		return
	}

	index := clampIndex(a.bookshelfIndex, len(books))
	a.currentBook = books[index]
	a.selectedBook = nil
	a.fromBookshelf = true
	a.beginLoadToc()
}

func (a *App) beginLoadToc() {
	bookURL, bookID, sourceID, rule, err := a.resolveTocContext()
	if err != nil {
		a.setStatus(err.Error(), statusError)
		a.refreshChrome()
		return
	}

	a.showLoading("加载目录...", "正在读取章节列表")
	go func(bookURL, bookID string, sourceID int, rule *model.Rule) {
		result := a.loadToc(bookURL, bookID, sourceID, rule)
		a.app.QueueUpdateDraw(func() {
			a.handleTocLoaded(result)
		})
	}(bookURL, bookID, sourceID, rule)
}

func (a *App) resolveTocContext() (bookURL string, bookID string, sourceID int, rule *model.Rule, err error) {
	if a.currentBook != nil {
		src := a.currentBook.GetCurrentSource()
		if src == nil {
			return "", "", 0, nil, fmt.Errorf("未找到可用书源")
		}
		bookURL = src.BookURL
		bookID = a.persistedBookID(a.currentBook)
		sourceID = src.SourceID
		rule = a.findRuleBySourceID(src.SourceID)
	} else if a.selectedBook != nil {
		bookURL = a.selectedBook.URL
		sourceID = a.selectedBook.SourceID
		rule = a.findRuleBySourceID(a.selectedBook.SourceID)
	} else {
		return "", "", 0, nil, fmt.Errorf("未选择书籍")
	}

	if rule == nil {
		return "", "", 0, nil, fmt.Errorf("未找到对应书源规则")
	}
	return bookURL, bookID, sourceID, rule, nil
}

func (a *App) loadToc(bookURL, bookID string, sourceID int, rule *model.Rule) tocLoadResult {
	if bookID != "" && a.store != nil {
		if cacheStore, ok := a.store.(storage.CacheStore); ok {
			cachedToc, exists, err := cacheStore.GetTocCache(bookID, sourceID)
			if err == nil && exists && len(cachedToc) > 0 {
				chapters := make([]*model.Chapter, len(cachedToc))
				for i, item := range cachedToc {
					chapters[i] = &model.Chapter{
						Title: item.Title,
						URL:   item.URL,
						Order: item.Index + 1,
					}
				}
				return tocLoadResult{chapters: chapters, fromCache: true}
			}
		}
	}

	tocParser := parser.NewTocParser(rule, a.httpClient)
	chapters, err := tocParser.Parse(bookURL)
	if err != nil {
		return tocLoadResult{err: err}
	}

	if bookID != "" && a.store != nil {
		if cacheStore, ok := a.store.(storage.CacheStore); ok {
			tocItems := make([]storage.TocCacheItem, len(chapters))
			for i, chapter := range chapters {
				tocItems[i] = storage.TocCacheItem{
					Index: i,
					Title: chapter.Title,
					URL:   chapter.URL,
				}
			}
			_ = cacheStore.SaveTocCache(bookID, sourceID, tocItems)
		}
	}

	return tocLoadResult{chapters: chapters}
}

func (a *App) handleTocLoaded(result tocLoadResult) {
	a.hideLoading()
	if result.err != nil {
		a.setStatus(fmt.Sprintf("加载目录失败: %v", result.err), statusError)
		a.addDebugLog("加载目录失败: %v", result.err)
		a.contentPages.SwitchToPage(a.pageName(a.state))
		a.refreshChrome()
		a.focusCurrentPage()
		return
	}

	a.chapters = result.chapters
	a.currentRule = a.matchCurrentRule()
	a.chapterIndex = 0

	var bookURL string
	if a.currentBook != nil {
		if src := a.currentBook.GetCurrentSource(); src != nil {
			bookURL = src.BookURL
		}
	} else if a.selectedBook != nil {
		bookURL = a.selectedBook.URL
	}

	if result.fromCache {
		a.addDebugLog("目录加载成功 (缓存): %d 章", len(result.chapters))
	} else {
		a.addDebugLog("目录加载成功: %d 章", len(result.chapters))
	}
	a.addDebugLog("书籍URL: %s", bookURL)
	if a.currentRule != nil {
		a.addDebugLog("当前规则: %s (%s)", a.currentRule.Name, a.currentRule.URL)
	}

	if a.fromBookshelf && a.currentBook != nil && a.store != nil {
		if progress, err := a.store.GetReadingProgress(a.persistedBookID(a.currentBook)); err == nil && progress != nil {
			a.chapterIndex = clampIndex(progress.ChapterIndex, len(a.chapters))
		}
	}

	if a.currentBook != nil {
		a.currentBook.TotalChapters = len(a.chapters)
		if len(a.chapters) > 0 {
			a.currentBook.LatestChapter = a.chapters[len(a.chapters)-1].Title
		}
	}

	a.setStatus(fmt.Sprintf("共 %d 章", len(a.chapters)), statusInfo)
	a.refreshTocList()
	a.switchState(StateToc)
}

func (a *App) openSelectedChapter() {
	if len(a.chapters) == 0 {
		return
	}

	index := clampIndex(a.chapterIndex, len(a.chapters))
	a.beginLoadChapter(index)
}

func (a *App) beginLoadChapter(index int) {
	if index < 0 || index >= len(a.chapters) {
		a.setStatus("章节索引无效", statusError)
		a.refreshChrome()
		return
	}

	chapter := a.chapters[index]
	rule := a.currentRule
	if rule == nil {
		a.setStatus("未找到书源规则", statusError)
		a.refreshChrome()
		return
	}

	a.chapterIndex = index
	bookID := ""
	sourceID := 0
	if a.currentBook != nil {
		bookID = a.persistedBookID(a.currentBook)
		sourceID = a.currentBook.CurrentSourceID
	}

	a.showLoading("加载章节内容...", sanitizeSingleLine(chapter.Title))
	go func(index int, chapter *model.Chapter, bookID string, sourceID int, rule *model.Rule) {
		result := a.loadChapter(index, chapter, bookID, sourceID, rule)
		a.app.QueueUpdateDraw(func() {
			a.handleChapterLoaded(result)
		})
	}(index, chapter, bookID, sourceID, rule)
}

func (a *App) loadChapter(index int, chapter *model.Chapter, bookID string, sourceID int, rule *model.Rule) chapterLoadResult {
	if bookID != "" && a.store != nil {
		if cacheStore, ok := a.store.(storage.CacheStore); ok {
			content, exists, err := cacheStore.GetChapterContent(bookID, sourceID, index)
			if err == nil && exists && content != "" {
				cachedChapter := &model.Chapter{
					Title:   chapter.Title,
					URL:     chapter.URL,
					Order:   chapter.Order,
					Content: content,
				}
				return chapterLoadResult{chapter: cachedChapter, fromCache: true}
			}
		}
	}

	loaded := &model.Chapter{
		Title: chapter.Title,
		URL:   chapter.URL,
		Order: chapter.Order,
	}

	chapterParser := parser.NewChapterParser(rule, a.httpClient)
	if err := chapterParser.Parse(loaded); err != nil {
		return chapterLoadResult{err: err}
	}

	if bookID != "" && loaded.Content != "" && a.store != nil {
		if cacheStore, ok := a.store.(storage.CacheStore); ok {
			_ = cacheStore.SaveChapterContent(bookID, sourceID, index, loaded.Content)
		}
	}

	return chapterLoadResult{chapter: loaded}
}

func (a *App) handleChapterLoaded(result chapterLoadResult) {
	a.hideLoading()
	if result.err != nil {
		a.setStatus(fmt.Sprintf("加载章节失败: %v", result.err), statusError)
		a.addDebugLog("加载章节失败: %v", result.err)
		a.contentPages.SwitchToPage(a.pageName(StateToc))
		a.refreshChrome()
		a.app.SetFocus(a.tocList)
		return
	}

	a.currentChapter = result.chapter
	if result.fromCache {
		a.addDebugLog("章节加载成功 (缓存): %s", result.chapter.Title)
	} else {
		a.addDebugLog("章节加载成功: %s", result.chapter.Title)
	}
	a.addDebugLog("章节URL: %s", result.chapter.URL)
	if len(result.chapter.Content) == 0 {
		a.addDebugLog("警告: 章节内容为空")
	} else {
		a.addDebugLog("内容长度: %d 字符", len(result.chapter.Content))
	}

	if a.preloader != nil && a.currentBook != nil && a.currentRule != nil {
		bookID := a.persistedBookID(a.currentBook)
		if bookID != "" {
			a.preloader.PreloadAhead(
				bookID,
				a.currentBook.CurrentSourceID,
				a.chapterIndex,
				a.chapters,
				a.currentRule,
			)
			a.addDebugLog("触发预加载: 后续 3 章")
		}
	}

	a.refreshReaderContent()
	a.restoreReaderProgress()
	a.setStatus("", statusInfo)
	a.switchState(StateReader)
}

func (a *App) loadRelativeChapter(delta int) {
	next := a.chapterIndex + delta
	if next < 0 || next >= len(a.chapters) {
		return
	}
	a.saveCurrentProgress()
	a.beginLoadChapter(next)
}

func (a *App) restoreReaderProgress() {
	a.readerContent.ScrollToBeginning()
	if !a.fromBookshelf || a.currentBook == nil || a.store == nil {
		a.refreshChrome()
		return
	}

	bookID := a.persistedBookID(a.currentBook)
	if bookID == "" {
		a.refreshChrome()
		return
	}

	progress, err := a.store.GetReadingProgress(bookID)
	if err != nil || progress == nil || progress.ChapterIndex != a.chapterIndex {
		a.refreshChrome()
		return
	}

	a.readerContent.ScrollTo(progress.LineOffset, 0)
	a.refreshChrome()
}

func (a *App) saveCurrentProgress() {
	if a.store == nil || a.currentBook == nil || a.currentChapter == nil {
		return
	}

	bookID := a.persistedBookID(a.currentBook)
	if bookID == "" {
		return
	}

	row, _ := a.readerContent.GetScrollOffset()
	totalLines := a.readerContent.GetWrappedLineCount()
	if totalLines == 0 {
		totalLines = a.readerContent.GetOriginalLineCount()
	}

	progress := model.NewReadingProgress(
		bookID,
		a.currentBook.CurrentSourceID,
		a.chapterIndex,
		a.currentChapter.Title,
	)
	progress.ChapterURL = a.currentChapter.URL
	progress.UpdatePosition(row, totalLines)

	if err := a.store.SaveReadingProgress(progress); err != nil {
		a.addDebugLog("保存进度失败: %v", err)
		return
	}

	a.currentBook.UpdateLastRead()
	a.currentBook.ClearUpdateMark()
	if a.bookshelf != nil {
		_ = a.store.SaveBookShelf(a.bookshelf)
	}
}

func (a *App) addCurrentSelectionToBookshelf() {
	if a.bookshelf == nil {
		a.bookshelf = model.NewBookShelf()
	}

	var book *model.BookRecord
	switch {
	case a.currentBook != nil:
		book = a.currentBook
	case a.aggregatedResults != nil:
		selected := a.selectedAggregatedResult()
		source := a.selectedAggregatedSource()
		if selected != nil && source != nil {
			book = selected.ToBookRecordWithSource(source.SourceID)
		}
	case a.selectedBook != nil:
		sourceName := ""
		if a.currentRule != nil {
			sourceName = a.currentRule.Name
		}
		book = model.NewBookRecord(a.selectedBook, sourceName)
	}

	if book == nil {
		a.setStatus("无法添加到书架", statusError)
		a.refreshChrome()
		return
	}

	a.bookshelf.AddBook(book)
	saved := a.bookshelf.FindBook(book.BookName, book.Author)
	if saved != nil {
		saved.SwitchSource(book.CurrentSourceID)
		a.currentBook = saved
	}

	if a.store != nil {
		if err := a.store.SaveBookShelf(a.bookshelf); err != nil {
			a.setStatus(fmt.Sprintf("保存书架失败: %v", err), statusError)
			a.refreshChrome()
			return
		}
	}

	sourceName := ""
	if saved != nil {
		if src := saved.GetCurrentSource(); src != nil {
			sourceName = src.SourceName
		}
	} else if src := book.GetCurrentSource(); src != nil {
		sourceName = src.SourceName
	}

	if sourceName == "" {
		a.setStatus(fmt.Sprintf("已添加到书架: %s", book.BookName), statusSuccess)
	} else {
		a.setStatus(fmt.Sprintf("已添加到书架: %s (%s)", book.BookName, sourceName), statusSuccess)
	}

	a.refreshBookshelfList()
	a.refreshChrome()
}

func (a *App) removeSelectedBookshelfBook() {
	books := a.getBookshelfBooks()
	if len(books) == 0 || a.bookshelf == nil {
		return
	}

	index := clampIndex(a.bookshelfIndex, len(books))
	book := books[index]
	a.bookshelf.RemoveBook(book.ID)

	if a.store != nil {
		_ = a.store.SaveBookShelf(a.bookshelf)
		progressStore, _ := a.store.LoadProgress()
		if progressStore != nil {
			progressStore.RemoveProgress(book.ID)
			_ = a.store.SaveProgress(progressStore)
		}
	}

	if a.bookshelfIndex >= len(books)-1 && a.bookshelfIndex > 0 {
		a.bookshelfIndex--
	}

	a.setStatus(fmt.Sprintf("已从书架移除: %s", book.BookName), statusSuccess)
	a.refreshBookshelfList()
	a.refreshChrome()
}

func (a *App) checkSelectedBookUpdate() {
	books := a.getBookshelfBooks()
	if len(books) == 0 {
		return
	}

	book := books[clampIndex(a.bookshelfIndex, len(books))]
	a.setStatus("检查更新中...", statusInfo)
	a.refreshChrome()

	go func(book *model.BookRecord) {
		result := a.checkBookUpdate(book)
		a.app.QueueUpdateDraw(func() {
			a.handleUpdateCheckResult(result)
		})
	}(book)
}

func (a *App) checkBookUpdate(book *model.BookRecord) updateCheckResult {
	src := book.GetCurrentSource()
	if src == nil {
		return updateCheckResult{bookID: book.ID, err: fmt.Errorf("无可用书源")}
	}

	rule := a.findRuleBySourceID(src.SourceID)
	if rule == nil {
		return updateCheckResult{bookID: book.ID, err: fmt.Errorf("书源规则不存在")}
	}

	tocParser := parser.NewTocParser(rule, a.httpClient)
	chapters, err := tocParser.Parse(src.BookURL)
	if err != nil {
		return updateCheckResult{bookID: book.ID, err: err}
	}

	newCount := len(chapters)
	oldCount := book.TotalChapters
	latestChapter := ""
	if len(chapters) > 0 {
		latestChapter = chapters[len(chapters)-1].Title
	}

	return updateCheckResult{
		bookID:        book.ID,
		hasUpdate:     newCount > oldCount,
		newChapters:   newCount - oldCount,
		latestChapter: latestChapter,
	}
}

func (a *App) handleUpdateCheckResult(result updateCheckResult) {
	if result.err != nil {
		a.setStatus(fmt.Sprintf("检查更新失败: %v", result.err), statusError)
		a.addDebugLog("检查更新失败: %v", result.err)
		a.refreshChrome()
		return
	}

	if a.bookshelf != nil {
		book := a.bookshelf.FindBookByID(result.bookID)
		if book != nil {
			if result.hasUpdate {
				book.MarkUpdate(result.newChapters, result.latestChapter)
				a.setStatus(fmt.Sprintf("%s 有 %d 章更新", book.BookName, result.newChapters), statusSuccess)
			} else {
				a.setStatus(fmt.Sprintf("%s 暂无更新", book.BookName), statusInfo)
			}
			if a.store != nil {
				_ = a.store.SaveBookShelf(a.bookshelf)
			}
		}
	}

	a.refreshBookshelfList()
	a.refreshChrome()
}

func (a *App) openSourceSwitch() {
	if a.currentBook == nil || len(a.currentBook.Sources) <= 1 {
		a.setStatus("只有一个书源，无法切换", statusWarning)
		a.refreshChrome()
		return
	}

	a.availableSources = a.currentBook.Sources
	a.switchIndex = 0
	for i, src := range a.availableSources {
		if src.SourceID == a.currentBook.CurrentSourceID {
			a.switchIndex = i
			break
		}
	}

	a.refreshSourceSwitchList()
	a.switchState(StateSourceSwitch)
}

func (a *App) switchCurrentSource() {
	if a.currentBook == nil || len(a.availableSources) == 0 {
		return
	}

	selected := a.availableSources[clampIndex(a.switchIndex, len(a.availableSources))]
	if selected.SourceID == a.currentBook.CurrentSourceID {
		a.setStatus("已是当前书源", statusInfo)
		a.switchState(StateToc)
		return
	}

	a.saveCurrentProgress()
	a.currentBook.SwitchSource(selected.SourceID)
	a.currentRule = a.findRuleBySourceID(selected.SourceID)
	a.beginLoadToc()
}

func (a *App) matchCurrentRule() *model.Rule {
	if a.currentBook == nil {
		return a.currentRule
	}
	src := a.currentBook.GetCurrentSource()
	if src == nil {
		return a.currentRule
	}
	if rule := a.findRuleBySourceID(src.SourceID); rule != nil {
		return rule
	}
	return a.currentRule
}

func (a *App) findRuleBySourceID(sourceID int) *model.Rule {
	for _, rule := range a.rules {
		if rule.ID == sourceID {
			return rule
		}
	}
	return nil
}

func (a *App) getBookshelfBooks() []*model.BookRecord {
	if a.bookshelf == nil {
		return nil
	}
	return a.bookshelf.GetBooksSortedByLastRead()
}

func (a *App) persistedBookID(book *model.BookRecord) string {
	if book == nil || a.bookshelf == nil {
		return ""
	}
	if existing := a.bookshelf.FindBookByID(book.ID); existing != nil {
		return existing.ID
	}
	if existing := a.bookshelf.FindBook(book.BookName, book.Author); existing != nil {
		return existing.ID
	}
	return ""
}

func (a *App) showLoading(title, subtitle string) {
	a.busy = true
	a.showingLoading = true
	a.loadingTitle = title
	a.loadingSubtitle = subtitle
	a.refreshLoadingView()
	a.contentPages.SwitchToPage(pageLoading)
	a.refreshChrome()
	a.app.SetFocus(a.loadingView)
}

func (a *App) hideLoading() {
	a.busy = false
	a.showingLoading = false
	a.contentPages.SwitchToPage(a.pageName(a.state))
}

func (a *App) toggleReaderFullscreen() {
	if a.state != StateReader {
		return
	}
	a.readerFullscreen = !a.readerFullscreen
	a.refreshReaderLayout()
	a.refreshChrome()
}

func (a *App) setStatus(message string, level statusLevel) {
	a.statusMsg = message
	a.statusLevel = level
}

func (a *App) updateSearchStatus() {
	a.setStatus(a.searchStatusText(), statusInfo)
}

func (a *App) searchStatusText() string {
	total := a.searchSourceTotal()
	if total == 0 {
		return "未找到可搜索书源"
	}

	done := a.completedSearchSourceCount()
	found := 0
	if a.aggregatedResults != nil {
		found = a.aggregatedResults.TotalCount
	}

	if a.searching {
		if found == 0 {
			return fmt.Sprintf("搜索中... %d/%d 个书源完成", done, total)
		}
		return fmt.Sprintf("已找到 %d 本，搜索中... %d/%d 个书源完成", found, done, total)
	}

	if found == 0 {
		return fmt.Sprintf("未找到相关书籍 (已搜索 %d 个书源)", done)
	}
	return fmt.Sprintf("找到 %d 本书籍 (来自 %d 个书源)", found, a.resultSourceCount())
}

func (a *App) searchSourceTotal() int {
	if len(a.searchProgress) > 0 {
		return len(a.searchProgress)
	}
	return len(a.rules)
}

func (a *App) completedSearchSourceCount() int {
	count := 0
	for _, stat := range a.searchProgress {
		if stat != nil && isFinalSearchStatus(stat.Status) {
			count++
		}
	}
	return count
}

func (a *App) resultSourceCount() int {
	count := 0
	for _, results := range a.searchResultsBySource {
		if len(results) > 0 {
			count++
		}
	}
	return count
}

func (a *App) isSearchComplete() bool {
	total := a.searchSourceTotal()
	return total > 0 && a.completedSearchSourceCount() >= total
}

func (a *App) copySearchProgress() map[int]*model.SourceSearchStat {
	copied := make(map[int]*model.SourceSearchStat, len(a.searchProgress))
	for sourceID, stat := range a.searchProgress {
		if stat == nil {
			continue
		}
		value := *stat
		copied[sourceID] = &value
	}
	return copied
}

func (a *App) selectedAggregatedResult() *model.AggregatedSearchResult {
	if a.aggregatedResults == nil || len(a.aggregatedResults.Results) == 0 {
		return nil
	}
	return a.aggregatedResults.Results[clampIndex(a.resultIndex, len(a.aggregatedResults.Results))]
}

func (a *App) selectedAggregatedSource() *model.SearchResultWithSource {
	selected := a.selectedAggregatedResult()
	if selected == nil || len(selected.Sources) == 0 {
		return nil
	}
	return selected.Sources[clampIndex(a.sourceIndex, len(selected.Sources))]
}

func (a *App) selectedAggregatedKey() string {
	selected := a.selectedAggregatedResult()
	if selected == nil {
		return ""
	}
	return selected.NormalizedKey
}

func (a *App) selectedAggregatedSourceID() int {
	selected := a.selectedAggregatedSource()
	if selected == nil {
		return 0
	}
	return selected.SourceID
}

func (a *App) restoreSearchSelection(selectedKey string, selectedSourceID int) {
	if a.aggregatedResults == nil || len(a.aggregatedResults.Results) == 0 {
		a.resultIndex = 0
		a.sourceIndex = 0
		return
	}

	if selectedKey != "" {
		for i, result := range a.aggregatedResults.Results {
			if result.NormalizedKey == selectedKey {
				a.resultIndex = i
				break
			}
		}
	}
	a.resultIndex = clampIndex(a.resultIndex, len(a.aggregatedResults.Results))

	selected := a.aggregatedResults.Results[a.resultIndex]
	if selectedSourceID != 0 {
		for i, src := range selected.Sources {
			if src.SourceID == selectedSourceID {
				a.sourceIndex = i
				return
			}
		}
	}
	a.sourceIndex = clampIndex(a.sourceIndex, len(selected.Sources))
}

func isFinalSearchStatus(status model.SearchStatus) bool {
	return status == model.SearchStatusSuccess ||
		status == model.SearchStatusFailed ||
		status == model.SearchStatusTimeout
}

func (a *App) addDebugLog(format string, args ...any) {
	entry := fmt.Sprintf("%s %s", time.Now().Format("15:04:05"), fmt.Sprintf(format, args...))
	a.debugLogs = append(a.debugLogs, entry)
	if len(a.debugLogs) > a.maxDebugLogs {
		a.debugLogs = a.debugLogs[len(a.debugLogs)-a.maxDebugLogs:]
	}
	if a.debugView != nil && a.showDebugLog && a.state == StateReader {
		a.refreshReaderDebug()
	}
}

func (a *App) handleResize() {
	if a.searchLayout != nil {
		if a.width > 0 && a.width < 110 {
			a.searchLayout.SetDirection(tview.FlexRow)
		} else {
			a.searchLayout.SetDirection(tview.FlexColumn)
		}
	}
	if a.state == StateReader {
		a.refreshChrome()
	}
}

func (a *App) scheduleReaderChromeRefresh() {
	time.AfterFunc(0, func() {
		a.app.QueueUpdateDraw(func() {
			if a.state == StateReader {
				a.refreshChrome()
			}
		})
	})
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

func sanitizeSingleLine(text string) string {
	text = strings.TrimSpace(text)
	text = strings.ReplaceAll(text, "\r", " ")
	text = strings.ReplaceAll(text, "\n", " ")
	text = strings.ReplaceAll(text, "\t", " ")
	for strings.Contains(text, "  ") {
		text = strings.ReplaceAll(text, "  ", " ")
	}
	return text
}

func truncateText(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	if limit <= 3 {
		return string(runes[:limit])
	}
	return string(runes[:limit-3]) + "..."
}
