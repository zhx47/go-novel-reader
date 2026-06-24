package ui

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func (a *App) buildUI() {
	a.titleView = tview.NewTextView().
		SetTextColor(colorPrimary).
		SetTextStyle(tcell.StyleDefault.Foreground(colorPrimary).Bold(true))
	a.subtitleView = tview.NewTextView().
		SetTextColor(colorMuted)
	a.statusView = tview.NewTextView().
		SetDynamicColors(true)
	a.helpView = tview.NewTextView().
		SetTextColor(colorMuted)
	a.loadingView = tview.NewTextView().
		SetTextAlign(tview.AlignCenter).
		SetTextColor(colorWarning)

	a.headerFlex = tview.NewFlex().SetDirection(tview.FlexRow)
	a.headerFlex.AddItem(a.titleView, 1, 0, false)
	a.headerFlex.AddItem(a.subtitleView, 1, 0, false)

	a.buildMainMenuPage()
	a.buildBookshelfPage()
	a.buildSearchPage()
	a.buildSearchResultPage()
	a.buildTocPage()
	a.buildReaderPage()
	a.buildSourceSwitchPage()
	a.buildLoadingPage()

	a.contentPages = tview.NewPages()
	a.contentPages.AddPage(pageMainMenu, a.mainMenuList, true, true)
	a.contentPages.AddPage(pageBookShelf, a.bookshelfList, true, false)
	a.contentPages.AddPage(pageSearch, wrapPage(a.searchInput, "输入书名或作者后回车"), true, false)
	a.contentPages.AddPage(pageSearchResult, a.searchLayout, true, false)
	a.contentPages.AddPage(pageToc, a.tocList, true, false)
	a.contentPages.AddPage(pageReader, a.readerLayout, true, false)
	a.contentPages.AddPage(pageSourceSwitch, a.sourceSwitchList, true, false)
	a.contentPages.AddPage(pageLoading, wrapPage(a.loadingView, ""), true, false)

	a.root = tview.NewFlex().SetDirection(tview.FlexRow)
	a.root.AddItem(a.headerFlex, 2, 0, false)
	a.root.AddItem(a.contentPages, 0, 1, true)
	a.root.AddItem(a.statusView, 1, 0, false)
	a.root.AddItem(a.helpView, 1, 0, false)
}

func (a *App) buildMainMenuPage() {
	a.mainMenuList = newList("主菜单", false)
	a.mainMenuList.AddItem("书架", "继续阅读或管理已收藏书籍", 0, nil)
	a.mainMenuList.AddItem("搜索小说", "并发搜索全部已启用书源", 0, nil)
	a.mainMenuList.SetSelectedFunc(func(index int, _, _ string, _ rune) {
		switch index {
		case 0:
			a.openBookshelfPage()
		case 1:
			a.openSearchPage()
		}
	})
}

func (a *App) buildBookshelfPage() {
	a.bookshelfList = newList("书架", true)
	a.bookshelfList.SetChangedFunc(func(index int, _, _ string, _ rune) {
		if a.refreshingBookshelf {
			return
		}
		a.bookshelfIndex = index
	})
	a.bookshelfList.SetSelectedFunc(func(index int, _, _ string, _ rune) {
		a.bookshelfIndex = index
		a.openSelectedBookshelfBook()
	})
}

func (a *App) buildSearchPage() {
	a.searchInput = tview.NewInputField().
		SetLabel("关键词: ").
		SetPlaceholder("输入书名或作者...").
		SetFieldWidth(0)
	a.searchInput.SetDoneFunc(func(key tcell.Key) {
		switch key {
		case tcell.KeyEnter:
			a.startSearch()
		case tcell.KeyEsc:
			a.handleBack()
		}
	})
}

func (a *App) buildSearchResultPage() {
	a.searchResultList = newList("小说", true)
	a.searchResultList.SetChangedFunc(func(index int, _, _ string, _ rune) {
		if a.refreshingResultList {
			return
		}
		a.resultIndex = index
		a.sourceIndex = 0
		a.refreshSearchSourcePanel()
		a.refreshSearchPreview()
		a.refreshChrome()
	})
	a.searchResultList.SetSelectedFunc(func(index int, _, _ string, _ rune) {
		a.resultIndex = index
		a.openSelectedSearchResult()
	})

	a.searchSourceList = newList("书源", true)
	a.searchSourceList.SetChangedFunc(func(index int, _, _ string, _ rune) {
		if a.refreshingSourceList {
			return
		}
		a.sourceIndex = index
		a.refreshSearchPreview()
	})
	a.searchSourceList.SetSelectedFunc(func(index int, _, _ string, _ rune) {
		a.sourceIndex = index
		if a.sourcePanelMode == sourcePanelSources {
			a.openSelectedSearchResult()
		}
	})

	a.searchPreview = tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(true).
		SetWordWrap(true)

	right := tview.NewFlex().SetDirection(tview.FlexRow)
	right.AddItem(a.searchSourceList, 0, 3, false)
	right.AddItem(a.searchPreview, 7, 0, false)

	a.searchLayout = tview.NewFlex().SetDirection(tview.FlexColumn)
	a.searchLayout.AddItem(a.searchResultList, 0, 3, true)
	a.searchLayout.AddItem(right, 0, 2, false)
}

func (a *App) buildTocPage() {
	a.tocList = newList("目录", false)
	a.tocList.SetChangedFunc(func(index int, _, _ string, _ rune) {
		if a.refreshingToc {
			return
		}
		a.chapterIndex = index
		a.refreshChrome()
	})
	a.tocList.SetSelectedFunc(func(index int, _, _ string, _ rune) {
		a.chapterIndex = index
		a.openSelectedChapter()
	})
}

func (a *App) buildReaderPage() {
	a.readerContent = tview.NewTextView().
		SetWrap(true).
		SetWordWrap(true).
		SetScrollable(true)
	a.readerContent.SetMouseCapture(func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		a.scheduleReaderChromeRefresh()
		return action, event
	})

	a.debugView = tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(true).
		SetWordWrap(true).
		SetScrollable(true)

	a.readerLayout = tview.NewFlex().SetDirection(tview.FlexRow)
	a.readerLayout.AddItem(a.readerContent, 0, 1, true)
}

func (a *App) buildSourceSwitchPage() {
	a.sourceSwitchList = newList("切换书源", true)
	a.sourceSwitchList.SetChangedFunc(func(index int, _, _ string, _ rune) {
		if a.refreshingSourceSwitch {
			return
		}
		a.switchIndex = index
		a.refreshChrome()
	})
	a.sourceSwitchList.SetSelectedFunc(func(index int, _, _ string, _ rune) {
		a.switchIndex = index
		a.switchCurrentSource()
	})
}

func (a *App) buildLoadingPage() {
}

func (a *App) refreshAll() {
	a.refreshBookshelfList()
	a.refreshSearchResults()
	a.refreshTocList()
	a.refreshReaderLayout()
	a.refreshSourceSwitchList()
	a.refreshLoadingView()
	a.refreshChrome()
}

func (a *App) refreshChrome() {
	title, subtitle, help := a.pageChrome()
	a.titleView.SetText(title)
	a.subtitleView.SetText(subtitle)
	a.helpView.SetText(help)
	a.statusView.SetText(a.statusMsg)
	a.statusView.SetTextColor(statusColor(a.statusLevel))

	if a.state == StateReader && a.readerFullscreen && !a.showingLoading {
		a.root.ResizeItem(a.headerFlex, 0, 0)
		a.root.ResizeItem(a.statusView, 0, 0)
		a.root.ResizeItem(a.helpView, 0, 0)
		return
	}

	a.root.ResizeItem(a.headerFlex, 2, 0)
	a.root.ResizeItem(a.statusView, 1, 0)
	a.root.ResizeItem(a.helpView, 1, 0)
}

func (a *App) pageChrome() (title string, subtitle string, help string) {
	if a.showingLoading {
		return a.loadingTitle, a.loadingSubtitle, ""
	}

	switch a.state {
	case StateMainMenu:
		title = "小说阅读器"
		subtitle = fmt.Sprintf("已启用 %d 个书源 | 书架 %d 本书", len(a.rules), len(a.getBookshelfBooks()))
		help = "↑/↓ 选择  Enter 确认  1/2 快捷键  q 退出"
	case StateBookShelf:
		title = "我的书架"
		subtitle = fmt.Sprintf("按最后阅读时间排序 | 共 %d 本书", len(a.getBookshelfBooks()))
		help = "↑/↓ 选择  Enter 继续阅读  d 删除  u 检查更新  s 搜索  Esc 返回"
	case StateSearch:
		title = "搜索小说"
		subtitle = fmt.Sprintf("将并发搜索 %d 个书源", len(a.rules))
		help = "输入关键词后 Enter 搜索  Esc 返回"
	case StateSearchResult:
		title = fmt.Sprintf("搜索结果: %s", a.keyword)
		subtitle = fmt.Sprintf("聚合结果 %d 本", a.searchResultTotal())
		help = "↑/↓ 选择小说  Tab/←/→ 切换焦点  Enter 查看目录  a 加入书架  Esc 返回"
	case StateToc:
		title = fmt.Sprintf("目录: %s", a.currentBookTitle())
		subtitle = fmt.Sprintf("当前书源: %s | 共 %d 章", a.currentSourceName(), len(a.chapters))
		help = "↑/↓ 选择  PgUp/PgDn 翻页  Enter 阅读  a 加入书架  c 换源  Esc 返回"
	case StateReader:
		title = a.currentChapterTitle()
		subtitle = fmt.Sprintf("%s | %s", a.currentBookTitle(), a.currentSourceName())
		help = a.readerHelpText()
	case StateSourceSwitch:
		title = fmt.Sprintf("切换书源: %s", a.currentBookTitle())
		subtitle = "选择一个书源继续阅读"
		help = "↑/↓ 选择  Enter 确认切换  Esc 返回"
	}
	return
}

func (a *App) refreshBookshelfList() {
	a.refreshingBookshelf = true
	defer func() {
		a.refreshingBookshelf = false
	}()

	books := a.getBookshelfBooks()
	a.bookshelfList.Clear()
	for _, book := range books {
		main := fmt.Sprintf("%s - %s", sanitizeSingleLine(book.BookName), sanitizeSingleLine(book.Author))
		if book.HasUpdate && book.NewChapters > 0 {
			main += fmt.Sprintf(" [+%d]", book.NewChapters)
		}

		secondary := []string{}
		if len(book.Sources) > 1 {
			secondary = append(secondary, fmt.Sprintf("%d 个书源", len(book.Sources)))
		}
		if book.LatestChapter != "" {
			secondary = append(secondary, truncateText(sanitizeSingleLine(book.LatestChapter), 60))
		}
		if book.LastReadAt.IsZero() {
			secondary = append(secondary, "未开始阅读")
		} else {
			secondary = append(secondary, "最近阅读 "+book.LastReadAt.Format("2006-01-02 15:04"))
		}
		a.bookshelfList.AddItem(main, strings.Join(secondary, " | "), 0, nil)
	}

	if len(books) == 0 {
		a.bookshelfIndex = 0
		return
	}
	a.bookshelfIndex = clampIndex(a.bookshelfIndex, len(books))
	a.bookshelfList.SetCurrentItem(a.bookshelfIndex)
}

func (a *App) refreshSearchResults() {
	a.refreshingResultList = true
	a.searchResultList.Clear()
	if a.aggregatedResults != nil {
		for _, result := range a.aggregatedResults.Results {
			main := fmt.Sprintf("%s - %s", sanitizeSingleLine(result.BookName), sanitizeSingleLine(result.Author))
			if result.SourceCount > 1 {
				main += fmt.Sprintf(" [%d源]", result.SourceCount)
			}

			meta := make([]string, 0, 4)
			if result.Category != "" {
				meta = append(meta, sanitizeSingleLine(result.Category))
			}
			if result.Status != "" {
				meta = append(meta, sanitizeSingleLine(result.Status))
			}
			if result.LatestChapter != "" {
				meta = append(meta, truncateText(sanitizeSingleLine(result.LatestChapter), 60))
			}
			a.searchResultList.AddItem(main, strings.Join(meta, " | "), 0, nil)
		}
	}
	a.refreshingResultList = false

	if a.aggregatedResults == nil || len(a.aggregatedResults.Results) == 0 {
		a.resultIndex = 0
		a.refreshSearchSourcePanel()
		a.refreshSearchPreview()
		return
	}

	a.resultIndex = clampIndex(a.resultIndex, len(a.aggregatedResults.Results))
	a.searchResultList.SetCurrentItem(a.resultIndex)
	a.refreshSearchSourcePanel()
	a.refreshSearchPreview()
}

func (a *App) refreshSearchSourcePanel() {
	a.refreshingSourceList = true
	defer func() {
		a.refreshingSourceList = false
	}()

	a.searchSourceList.Clear()

	selected := a.selectedAggregatedResult()
	if selected == nil {
		a.sourcePanelMode = sourcePanelProgress
		for _, rule := range a.rules {
			stat := a.searchProgress[rule.ID]
			main := rule.Name
			secondary := "等待中"
			if stat != nil {
				secondary = stat.Status.String()
				if stat.ResultCount > 0 {
					secondary += fmt.Sprintf(" | %d 条", stat.ResultCount)
				}
				if stat.Error != "" {
					secondary += " | " + truncateText(sanitizeSingleLine(stat.Error), 40)
				}
			}
			a.searchSourceList.AddItem(main, secondary, 0, nil)
		}
		if len(a.rules) > 0 {
			a.searchSourceList.SetCurrentItem(clampIndex(a.sourceIndex, len(a.rules)))
		}
		return
	}

	a.sourcePanelMode = sourcePanelSources
	for _, src := range selected.Sources {
		main := sanitizeSingleLine(src.SourceName)
		meta := make([]string, 0, 3)
		if src.LatestChapter != "" {
			meta = append(meta, truncateText(sanitizeSingleLine(src.LatestChapter), 50))
		}
		if src.LastUpdateTime != "" {
			meta = append(meta, sanitizeSingleLine(src.LastUpdateTime))
		}
		a.searchSourceList.AddItem(main, strings.Join(meta, " | "), 0, nil)
	}

	if len(selected.Sources) == 0 {
		a.sourceIndex = 0
		return
	}
	a.sourceIndex = clampIndex(a.sourceIndex, len(selected.Sources))
	a.searchSourceList.SetCurrentItem(a.sourceIndex)
}

func (a *App) refreshSearchPreview() {
	var b strings.Builder

	selected := a.selectedAggregatedResult()
	if selected == nil {
		fmt.Fprintf(&b, "关键词：%s\n", a.keyword)
		fmt.Fprintf(&b, "状态：%s\n", a.searchStatusText())
		if a.aggregatedResults != nil {
			fmt.Fprintf(&b, "聚合结果：%d 本\n", a.aggregatedResults.TotalCount)
		}
		a.searchPreview.SetText(strings.TrimSpace(b.String()))
		return
	}

	fmt.Fprintf(&b, "书名：%s\n", sanitizeSingleLine(selected.BookName))
	fmt.Fprintf(&b, "作者：%s\n", sanitizeSingleLine(selected.Author))
	if selected.Category != "" {
		fmt.Fprintf(&b, "分类：%s\n", sanitizeSingleLine(selected.Category))
	}
	if selected.Status != "" {
		fmt.Fprintf(&b, "状态：%s\n", sanitizeSingleLine(selected.Status))
	}
	if selected.WordCount != "" {
		fmt.Fprintf(&b, "字数：%s\n", sanitizeSingleLine(selected.WordCount))
	}
	if selected.LatestChapter != "" {
		fmt.Fprintf(&b, "最新章节：%s\n", truncateText(sanitizeSingleLine(selected.LatestChapter), 80))
	}
	if selected.Intro != "" {
		fmt.Fprintf(&b, "\n简介：%s", truncateText(sanitizeSingleLine(selected.Intro), 240))
	}

	if src := a.selectedAggregatedSource(); src != nil {
		fmt.Fprintf(&b, "\n\n当前书源：%s", sanitizeSingleLine(src.SourceName))
	}

	a.searchPreview.SetText(strings.TrimSpace(b.String()))
}

func (a *App) refreshTocList() {
	a.refreshingToc = true
	defer func() {
		a.refreshingToc = false
	}()

	a.tocList.Clear()
	for _, chapter := range a.chapters {
		a.tocList.AddItem(
			fmt.Sprintf("%d. %s", chapter.Order, sanitizeSingleLine(chapter.Title)),
			"",
			0,
			nil,
		)
	}
	if len(a.chapters) == 0 {
		a.chapterIndex = 0
		return
	}
	a.chapterIndex = clampIndex(a.chapterIndex, len(a.chapters))
	a.tocList.SetCurrentItem(a.chapterIndex)
}

func (a *App) refreshReaderContent() {
	if a.currentChapter == nil {
		a.readerContent.SetText("")
		a.refreshReaderLayout()
		return
	}

	a.readerContent.SetText(a.currentChapter.Content)
	a.readerContent.ScrollToBeginning()
	a.refreshReaderLayout()
}

func (a *App) refreshReaderLayout() {
	a.readerLayout.Clear()
	a.readerLayout.SetDirection(tview.FlexRow)
	a.readerLayout.AddItem(a.readerContent, 0, 1, true)

	if a.readerFullscreen {
		return
	}
	if a.showDebugLog {
		a.refreshReaderDebug()
		a.readerLayout.AddItem(a.debugView, 10, 0, false)
	}
}

func (a *App) refreshReaderDebug() {
	var b strings.Builder

	if a.currentChapter != nil && a.currentChapter.URL != "" {
		fmt.Fprintf(&b, "章节URL: %s\n", a.currentChapter.URL)
	}
	if a.currentRule != nil {
		fmt.Fprintf(&b, "书源: %s (%s)\n", a.currentRule.Name, a.currentRule.URL)
	}
	if a.currentChapter != nil && a.currentChapter.Debug != nil {
		debug := a.currentChapter.Debug
		fmt.Fprintf(&b, "HTTP状态码: %d | 响应长度: %d 字节\n", debug.ResponseCode, debug.ContentLength)
		fmt.Fprintf(&b, "选择器: %s\n", debug.SelectorUsed)
		if debug.ErrorMsg != "" {
			fmt.Fprintf(&b, "错误: %s\n", debug.ErrorMsg)
		}
		if debug.SelectedHTML != "" {
			fmt.Fprintf(&b, "\n选择器匹配内容:\n%s\n", truncateText(debug.SelectedHTML, 500))
		} else if debug.RawHTML != "" {
			fmt.Fprintf(&b, "\n原始HTML片段:\n%s\n", truncateText(debug.RawHTML, 800))
		}
		if debug.FilteredText != "" {
			fmt.Fprintf(&b, "\n过滤后内容:\n%s\n", truncateText(debug.FilteredText, 300))
		}
	}
	if len(a.debugLogs) > 0 {
		fmt.Fprintf(&b, "\n操作日志:\n")
		start := len(a.debugLogs) - 8
		if start < 0 {
			start = 0
		}
		for _, line := range a.debugLogs[start:] {
			fmt.Fprintf(&b, "%s\n", line)
		}
	}

	a.debugView.SetText(strings.TrimSpace(b.String()))
}

func (a *App) refreshSourceSwitchList() {
	a.refreshingSourceSwitch = true
	defer func() {
		a.refreshingSourceSwitch = false
	}()

	a.sourceSwitchList.Clear()
	for _, src := range a.availableSources {
		main := sanitizeSingleLine(src.SourceName)
		if a.currentBook != nil && src.SourceID == a.currentBook.CurrentSourceID {
			main += " (当前)"
		}
		meta := make([]string, 0, 3)
		if src.TotalChapters > 0 {
			meta = append(meta, fmt.Sprintf("%d 章", src.TotalChapters))
		}
		if src.LatestChapter != "" {
			meta = append(meta, truncateText(sanitizeSingleLine(src.LatestChapter), 50))
		}
		a.sourceSwitchList.AddItem(main, strings.Join(meta, " | "), 0, nil)
	}

	if len(a.availableSources) == 0 {
		a.switchIndex = 0
		return
	}
	a.switchIndex = clampIndex(a.switchIndex, len(a.availableSources))
	a.sourceSwitchList.SetCurrentItem(a.switchIndex)
}

func (a *App) refreshLoadingView() {
	text := a.loadingTitle
	if text == "" {
		text = "加载中..."
	}
	if a.loadingSubtitle != "" {
		text += "\n\n" + a.loadingSubtitle
	}
	a.loadingView.SetText(text)
}

func (a *App) currentBookTitle() string {
	if a.currentBook != nil && a.currentBook.BookName != "" {
		return sanitizeSingleLine(a.currentBook.BookName)
	}
	if a.selectedBook != nil {
		return sanitizeSingleLine(a.selectedBook.BookName)
	}
	return "未选择书籍"
}

func (a *App) currentSourceName() string {
	if a.currentBook != nil {
		if src := a.currentBook.GetCurrentSource(); src != nil && src.SourceName != "" {
			return sanitizeSingleLine(src.SourceName)
		}
	}
	if a.currentRule != nil {
		return sanitizeSingleLine(a.currentRule.Name)
	}
	return "未知书源"
}

func (a *App) currentChapterTitle() string {
	if a.currentChapter == nil {
		return "阅读"
	}
	return sanitizeSingleLine(a.currentChapter.Title)
}

func (a *App) readerHelpText() string {
	progress := a.readerProgressText()
	help := "j/k 滚动  PgUp/PgDn 翻页  n/p 上下章  t 目录  F11 沉浸  / 调试"
	if a.currentBook != nil && len(a.currentBook.Sources) > 1 {
		help += "  c 换源"
	}
	if progress != "" {
		help += "  |  " + progress
	}
	return help
}

func (a *App) readerProgressText() string {
	if a.currentChapter == nil {
		return ""
	}
	totalLines := a.readerContent.GetWrappedLineCount()
	if totalLines == 0 {
		totalLines = a.readerContent.GetOriginalLineCount()
	}
	row, _ := a.readerContent.GetScrollOffset()
	percent := 0.0
	if totalLines > 0 {
		percent = float64(row) / float64(totalLines) * 100
	}
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	return fmt.Sprintf("章节 %d/%d  %.0f%%", a.chapterIndex+1, len(a.chapters), percent)
}

func newList(title string, secondary bool) *tview.List {
	list := tview.NewList()
	list.ShowSecondaryText(secondary)
	list.SetHighlightFullLine(true)
	list.SetSelectedBackgroundColor(colorPrimary)
	list.SetSelectedTextColor(tcell.ColorWhite)
	list.SetMainTextColor(tcell.ColorWhite)
	list.SetSecondaryTextColor(colorMuted)
	list.SetDoneFunc(func() {})
	list.SetMouseCapture(func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		switch action {
		case tview.MouseScrollUp:
			if list.GetCurrentItem() > 0 {
				list.SetCurrentItem(list.GetCurrentItem() - 1)
			}
			return tview.MouseConsumed, nil
		case tview.MouseScrollDown:
			if list.GetCurrentItem() < list.GetItemCount()-1 {
				list.SetCurrentItem(list.GetCurrentItem() + 1)
			}
			return tview.MouseConsumed, nil
		}
		return action, event
	})
	return list
}

func wrapPage(primitive tview.Primitive, title string) tview.Primitive {
	frame := tview.NewFrame(primitive)
	frame.SetBorders(0, 0, 0, 0, 0, 0)
	return frame
}

func statusColor(level statusLevel) tcell.Color {
	switch level {
	case statusSuccess:
		return colorAccent
	case statusWarning:
		return colorWarning
	case statusError:
		return colorError
	default:
		return tcell.ColorWhite
	}
}

func (a *App) searchResultTotal() int {
	if a.aggregatedResults == nil {
		return 0
	}
	return a.aggregatedResults.TotalCount
}
