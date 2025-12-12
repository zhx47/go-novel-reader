package model

// Chapter 章节信息
type Chapter struct {
	URL     string // 章节链接
	Title   string // 章节标题
	Content string // 章节内容（阅读时填充）
	Order   int    // 章节顺序

	// 调试信息
	Debug *ChapterDebug
}

// ChapterDebug 章节调试信息
type ChapterDebug struct {
	RawHTML       string // 原始HTML响应（截取）
	SelectorUsed  string // 使用的选择器
	SelectedHTML  string // 选择器匹配到的HTML
	FilteredText  string // 过滤后的文本（截取）
	ErrorMsg      string // 错误信息
	ResponseCode  int    // HTTP响应码
	ContentLength int    // 原始内容长度
}
