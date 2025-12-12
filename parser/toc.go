package parser

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"

	"go-novel-reader/httpclient"
	"go-novel-reader/model"
	"go-novel-reader/selector"
)

// TocParser 目录解析器
type TocParser struct {
	rule   *model.Rule
	client *httpclient.Client
}

// NewTocParser 创建目录解析器
func NewTocParser(rule *model.Rule, client *httpclient.Client) *TocParser {
	return &TocParser{rule: rule, client: client}
}

// Parse 解析目录
func (p *TocParser) Parse(bookURL string) ([]*model.Chapter, error) {
	ruleToc := p.rule.Toc
	if ruleToc == nil {
		return nil, fmt.Errorf("书源 %s 没有目录规则", p.rule.Name)
	}

	// 从URL提取书籍ID（如果有book规则）
	var bookID string
	if p.rule.Book != nil && p.rule.Book.URL != "" {
		// 去除JS部分
		pattern := selector.GetQuery(p.rule.Book.URL)
		re, err := regexp.Compile(pattern)
		if err == nil {
			matches := re.FindStringSubmatch(bookURL)
			if len(matches) > 1 {
				bookID = matches[1]
			}
		}
	}

	// 构建目录URL
	tocURL := bookURL
	baseURI := ruleToc.BaseURI
	if baseURI == "" {
		baseURI = p.rule.URL
	}

	// 如果有目录URL模板，使用它
	if ruleToc.URL != "" && bookID != "" {
		tocURL = fmt.Sprintf(ruleToc.URL, bookID)
		baseURI = fmt.Sprintf(baseURI, bookID)
	}

	// 获取所有目录页URL（处理分页）
	tocURLs := []string{tocURL}
	if ruleToc.Pagination {
		paginatedURLs, err := p.extractPaginationURLs(tocURL, baseURI)
		if err == nil && len(paginatedURLs) > 0 {
			tocURLs = paginatedURLs
		}
	}

	// 解析所有目录页
	return p.parseAllTocPages(tocURLs, baseURI)
}

// extractPaginationURLs 提取分页URL
func (p *TocParser) extractPaginationURLs(firstURL, baseURI string) ([]string, error) {
	ruleToc := p.rule.Toc
	urls := []string{firstURL}

	resp, err := p.client.Get(firstURL, ruleToc.Timeout)
	if err != nil {
		return urls, err
	}

	body, err := httpclient.ReadBody(resp)
	if err != nil {
		return urls, err
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return urls, err
	}

	// 查找分页链接
	if ruleToc.NextPage != "" {
		selector.Select(doc.Selection, ruleToc.NextPage).Each(func(i int, s *goquery.Selection) {
			href, exists := s.Attr("href")
			if exists && href != "" {
				fullURL := selector.SelectAndGetContent(s, "self::*", selector.AttrHref, baseURI)
				if fullURL != "" && fullURL != firstURL {
					// 检查是否已存在
					exists := false
					for _, u := range urls {
						if u == fullURL {
							exists = true
							break
						}
					}
					if !exists {
						urls = append(urls, fullURL)
					}
				}
			}
		})
	}

	return urls, nil
}

// parseAllTocPages 解析所有目录页
func (p *TocParser) parseAllTocPages(urls []string, baseURI string) ([]*model.Chapter, error) {
	ruleToc := p.rule.Toc
	var allChapters []*model.Chapter
	order := 1

	for _, tocURL := range urls {
		resp, err := p.client.Get(tocURL, ruleToc.Timeout)
		if err != nil {
			continue
		}

		body, err := httpclient.ReadBody(resp)
		if err != nil {
			continue
		}

		doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
		if err != nil {
			continue
		}

		// 选择章节列表容器（如果有）
		container := doc.Selection
		if ruleToc.List != "" {
			container = selector.Select(doc.Selection, ruleToc.List)
		}

		// 选择章节项
		chapters := p.parseChaptersFromDoc(container, baseURI, &order)
		allChapters = append(allChapters, chapters...)
	}

	// 处理倒序
	if ruleToc.IsDesc {
		reverseChapters(allChapters)
	}

	return allChapters, nil
}

// parseChaptersFromDoc 从文档中解析章节
func (p *TocParser) parseChaptersFromDoc(container *goquery.Selection, baseURI string, order *int) []*model.Chapter {
	ruleToc := p.rule.Toc
	var chapters []*model.Chapter

	selector.Select(container, ruleToc.Item).Each(func(i int, s *goquery.Selection) {
		title := strings.TrimSpace(s.Text())
		if title == "" {
			return
		}

		href, exists := s.Attr("href")
		if !exists || href == "" {
			return
		}

		// 转换为绝对路径
		fullURL := resolveURLHelper(baseURI, href)

		chapters = append(chapters, &model.Chapter{
			Title: title,
			URL:   fullURL,
			Order: *order,
		})
		*order++
	})

	return chapters
}

// reverseChapters 反转章节顺序
func reverseChapters(chapters []*model.Chapter) {
	for i, j := 0, len(chapters)-1; i < j; i, j = i+1, j-1 {
		chapters[i], chapters[j] = chapters[j], chapters[i]
	}
	// 重新设置order
	for i := range chapters {
		chapters[i].Order = i + 1
	}
}

// resolveURLHelper 解析URL
func resolveURLHelper(baseURI, href string) string {
	if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") {
		return href
	}

	if strings.HasPrefix(href, "//") {
		return "http:" + href
	}

	// 简单的URL拼接
	baseURI = strings.TrimSuffix(baseURI, "/")

	if strings.HasPrefix(href, "/") {
		// 绝对路径，需要提取域名
		parts := strings.SplitN(baseURI, "://", 2)
		if len(parts) == 2 {
			domainEnd := strings.Index(parts[1], "/")
			if domainEnd > 0 {
				return parts[0] + "://" + parts[1][:domainEnd] + href
			}
			return baseURI + href
		}
	}

	return baseURI + "/" + href
}
