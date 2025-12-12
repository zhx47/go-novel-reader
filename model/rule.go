package model

// Rule 书源规则
type Rule struct {
	ID        int     `json:"-"`         // 自动生成的ID
	URL       string  `json:"url"`       // 源站点URL
	Name      string  `json:"name"`      // 源名称
	Comment   string  `json:"comment"`   // 备注
	Language  string  `json:"language"`  // 内容语言 (zh_CN/zh_TW/zh_Hant)
	NeedProxy bool    `json:"needProxy"` // 是否需要代理
	Disabled  bool    `json:"disabled"`  // 是否禁用

	Search  *SearchRule  `json:"search"`  // 搜索规则
	Book    *BookRule    `json:"book"`    // 详情页规则
	Toc     *TocRule     `json:"toc"`     // 目录规则
	Chapter *ChapterRule `json:"chapter"` // 章节规则
	Crawl   *CrawlConfig `json:"crawl"`   // 爬虫配置
}

// SearchRule 搜索规则
type SearchRule struct {
	Disabled       bool   `json:"disabled"`       // 是否禁用搜索
	BaseURI        string `json:"baseUri"`        // 基础URI
	Timeout        int    `json:"timeout"`        // 超时时间（秒）
	URL            string `json:"url"`            // 搜索链接，支持 %s 占位符
	Method         string `json:"method"`         // 请求方法 (GET/POST)
	Data           string `json:"data"`           // POST表单数据 (JSON格式)
	Cookies        string `json:"cookies"`        // 请求Cookie
	Result         string `json:"result"`         // 结果容器选择器
	BookName       string `json:"bookName"`       // 书名选择器
	Author         string `json:"author"`         // 作者选择器
	Category       string `json:"category"`       // 分类选择器
	LatestChapter  string `json:"latestChapter"`  // 最新章节选择器
	LastUpdateTime string `json:"lastUpdateTime"` // 更新时间选择器
	Status         string `json:"status"`         // 状态选择器
	WordCount      string `json:"wordCount"`      // 字数选择器
	Pagination     bool   `json:"pagination"`     // 是否支持分页
	NextPage       string `json:"nextPage"`       // 下一页选择器
}

// BookRule 详情页规则
type BookRule struct {
	BaseURI        string `json:"baseUri"`        // 基础URI
	Timeout        int    `json:"timeout"`        // 超时时间
	URL            string `json:"url"`            // 用于提取书籍ID的正则
	BookName       string `json:"bookName"`       // 书名选择器
	Author         string `json:"author"`         // 作者选择器
	Intro          string `json:"intro"`          // 简介选择器
	Category       string `json:"category"`       // 分类选择器
	CoverURL       string `json:"coverUrl"`       // 封面URL选择器
	LatestChapter  string `json:"latestChapter"`  // 最新章节选择器
	LastUpdateTime string `json:"lastUpdateTime"` // 更新时间选择器
	Status         string `json:"status"`         // 状态选择器
	WordCount      string `json:"wordCount"`      // 字数选择器
}

// TocRule 目录规则
type TocRule struct {
	BaseURI    string `json:"baseUri"`    // 基础URI
	Timeout    int    `json:"timeout"`    // 超时时间
	URL        string `json:"url"`        // 目录页链接（可选）
	List       string `json:"list"`       // 目录列表容器
	Item       string `json:"item"`       // 章节项选择器（必填）
	IsDesc     bool   `json:"isDesc"`     // 是否倒序
	Pagination bool   `json:"pagination"` // 是否分页
	NextPage   string `json:"nextPage"`   // 下一页选择器
}

// ChapterRule 章节内容规则
type ChapterRule struct {
	BaseURI            string `json:"baseUri"`            // 基础URI
	Timeout            int    `json:"timeout"`            // 超时时间
	Title              string `json:"title"`              // 章节标题选择器
	Content            string `json:"content"`            // 正文选择器（必填）
	ParagraphTagClosed bool   `json:"paragraphTagClosed"` // 段落标签是否闭合
	ParagraphTag       string `json:"paragraphTag"`       // 段落分隔符
	FilterTxt          string `json:"filterTxt"`          // 广告过滤正则
	FilterTag          string `json:"filterTag"`          // 过滤HTML标签
	Pagination         bool   `json:"pagination"`         // 是否分页
	NextPage           string `json:"nextPage"`           // 下一页选择器
	NextPageInJs       string `json:"nextPageInJs"`       // JS中的下一页链接
	NextChapterLink    string `json:"nextChapterLink"`    // 下一章链接正则
}

// CrawlConfig 爬虫配置
type CrawlConfig struct {
	Concurrency      int `json:"concurrency"`      // 并发数
	MinInterval      int `json:"minInterval"`      // 最小间隔(ms)
	MaxInterval      int `json:"maxInterval"`      // 最大间隔(ms)
	MaxAttempts      int `json:"maxAttempts"`      // 最大重试次数
	RetryMinInterval int `json:"retryMinInterval"` // 重试最小间隔
	RetryMaxInterval int `json:"retryMaxInterval"` // 重试最大间隔
}
