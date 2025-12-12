package model

// SearchResult 搜索结果
type SearchResult struct {
	SourceID       int    // 书源ID
	URL            string // 详情页链接
	BookName       string // 书名
	Author         string // 作者
	Intro          string // 简介
	Category       string // 分类
	LatestChapter  string // 最新章节
	LastUpdateTime string // 更新时间
	Status         string // 状态
	WordCount      string // 字数
}
