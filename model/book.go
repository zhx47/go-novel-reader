package model

// Book 书籍信息
type Book struct {
	URL            string // 详情页链接
	BookName       string // 书名
	Author         string // 作者
	Intro          string // 简介
	Category       string // 分类
	CoverURL       string // 封面URL
	LatestChapter  string // 最新章节
	LastUpdateTime string // 最后更新时间
	Status         string // 完结状态
	WordCount      string // 总字数
}
