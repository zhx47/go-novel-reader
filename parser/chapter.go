package parser

import (
	"math/rand"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"

	"go-novel-reader/httpclient"
	"go-novel-reader/model"
	"go-novel-reader/selector"
)

// ChapterParser 章节解析器
type ChapterParser struct {
	rule        *model.Rule
	client      *httpclient.Client
	minInterval int
	maxInterval int
}

// NewChapterParser 创建章节解析器
func NewChapterParser(rule *model.Rule, client *httpclient.Client) *ChapterParser {
	minInterval := 200
	maxInterval := 500

	if rule.Crawl != nil {
		if rule.Crawl.MinInterval > 0 {
			minInterval = rule.Crawl.MinInterval
		}
		if rule.Crawl.MaxInterval > 0 {
			maxInterval = rule.Crawl.MaxInterval
		}
	}

	return &ChapterParser{
		rule:        rule,
		client:      client,
		minInterval: minInterval,
		maxInterval: maxInterval,
	}
}

// Parse 解析章节内容
func (p *ChapterParser) Parse(chapter *model.Chapter) error {
	r := p.rule.Chapter
	if r == nil {
		return nil
	}

	// 初始化调试信息
	chapter.Debug = &model.ChapterDebug{
		SelectorUsed: r.Content,
	}

	// 随机延迟
	p.randomSleep()

	var content string
	var err error

	if r.Pagination {
		content, err = p.fetchPaginatedContent(chapter.URL, chapter.Debug)
	} else {
		content, err = p.fetchSinglePageContent(chapter.URL, chapter.Debug)
	}

	if err != nil {
		chapter.Debug.ErrorMsg = err.Error()
		return err
	}

	// 记录选择器匹配到的原始HTML
	chapter.Debug.SelectedHTML = truncateString(content, 2000)

	// 过滤和格式化内容
	chapter.Content = p.filterAndFormat(content, chapter.Title)

	// 记录过滤后的文本
	chapter.Debug.FilteredText = truncateString(chapter.Content, 1000)

	return nil
}

// fetchSinglePageContent 获取单页章节内容
func (p *ChapterParser) fetchSinglePageContent(url string, debug *model.ChapterDebug) (string, error) {
	r := p.rule.Chapter
	baseURI := r.BaseURI
	if baseURI == "" {
		baseURI = p.rule.URL
	}

	resp, err := p.client.Get(url, r.Timeout)
	if err != nil {
		return "", err
	}

	// 记录响应码
	if debug != nil {
		debug.ResponseCode = resp.StatusCode
	}

	body, err := httpclient.ReadBody(resp)
	if err != nil {
		return "", err
	}

	// 记录原始HTML
	if debug != nil {
		debug.ContentLength = len(body)
		debug.RawHTML = truncateString(string(body), 3000)
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return "", err
	}

	return selector.SelectAndGetContent(doc.Selection, r.Content, selector.HTML, baseURI), nil
}

// fetchPaginatedContent 获取分页章节内容
func (p *ChapterParser) fetchPaginatedContent(startURL string, debug *model.ChapterDebug) (string, error) {
	r := p.rule.Chapter
	baseURI := r.BaseURI
	if baseURI == "" {
		baseURI = p.rule.URL
	}

	var contentBuilder strings.Builder
	nextURL := startURL
	visitedURLs := make(map[string]bool)
	isFirstPage := true

	for {
		// 防止无限循环
		if visitedURLs[nextURL] {
			break
		}
		visitedURLs[nextURL] = true

		resp, err := p.client.Get(nextURL, r.Timeout)
		if err != nil {
			break
		}

		// 记录第一页的响应码
		if isFirstPage && debug != nil {
			debug.ResponseCode = resp.StatusCode
		}

		body, err := httpclient.ReadBody(resp)
		if err != nil {
			break
		}

		// 记录第一页的原始HTML
		if isFirstPage && debug != nil {
			debug.ContentLength = len(body)
			debug.RawHTML = truncateString(string(body), 3000)
			isFirstPage = false
		}

		doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
		if err != nil {
			break
		}

		// 追加内容
		content := selector.SelectAndGetContent(doc.Selection, r.Content, selector.HTML, baseURI)
		contentBuilder.WriteString(content)

		// 获取下一页URL
		if r.NextPage == "" {
			break
		}

		nextSel := selector.SelectFirst(doc.Selection, r.NextPage)
		if nextSel.Length() == 0 {
			break
		}

		candidateNext, exists := nextSel.Attr("href")
		if !exists || candidateNext == "" {
			break
		}

		// 转换为绝对路径
		candidateNext = resolveURLHelper(baseURI, candidateNext)

		// 判断是否为最后一页
		if p.isLastPage(candidateNext, nextSel.Text()) {
			break
		}

		nextURL = candidateNext
		p.randomSleep()
	}

	return contentBuilder.String(), nil
}

// isLastPage 判断是否为最后一页
func (p *ChapterParser) isLastPage(nextURL, buttonText string) bool {
	if nextURL == "" {
		return true
	}

	r := p.rule.Chapter

	// 方式1: 正则判断是否为下一章链接
	if r.NextChapterLink != "" {
		matched, _ := regexp.MatchString(r.NextChapterLink, nextURL)
		if matched {
			return true
		}
	}

	// 方式2: 通用规则判断
	// URL模式：如果不是 *-2.html, *_2.html 等分页格式
	isPageURL, _ := regexp.MatchString(`.*[-_]\d+\.html`, nextURL)

	// 按钮文本：如果包含"下一章"、"没有了"等
	isEndText, _ := regexp.MatchString(`.*(下一章|没有了|>>|书末页|END).*`, buttonText)

	return !isPageURL && isEndText
}

// filterAndFormat 过滤和格式化内容
func (p *ChapterParser) filterAndFormat(content, title string) string {
	r := p.rule.Chapter

	// 1. 清理不可见字符
	content = cleanInvisibleChars(content)

	// 2. 清理HTML实体
	content = regexp.MustCompile(`&[a-zA-Z]+;`).ReplaceAllString(content, "")
	content = regexp.MustCompile(`&#\d+;`).ReplaceAllString(content, "")

	// 3. 正则过滤广告文本
	if r.FilterTxt != "" {
		re, err := regexp.Compile(r.FilterTxt)
		if err == nil {
			content = re.ReplaceAllString(content, "")
		}
	}

	// 4. 移除指定HTML标签
	if r.FilterTag != "" {
		tags := strings.Fields(r.FilterTag)
		for _, tag := range tags {
			// 移除标签及其内容
			re := regexp.MustCompile(`(?i)<` + tag + `[^>]*>.*?</` + tag + `>`)
			content = re.ReplaceAllString(content, "")
			// 移除自闭合标签
			re = regexp.MustCompile(`(?i)<` + tag + `[^>]*/?>`)
			content = re.ReplaceAllString(content, "")
		}
	}

	// 5. 格式化段落
	content = p.formatParagraphs(content)

	// 6. 清理标题重复（如果正文开头包含标题）
	if title != "" {
		cleanTitle := strings.ReplaceAll(title, " ", "")
		if strings.HasPrefix(content, title) {
			content = strings.TrimPrefix(content, title)
		} else if strings.HasPrefix(content, cleanTitle) {
			content = strings.TrimPrefix(content, cleanTitle)
		}
	}

	return strings.TrimSpace(content)
}

// formatParagraphs 格式化段落
func (p *ChapterParser) formatParagraphs(content string) string {
	r := p.rule.Chapter

	// 先清理所有HTML标签的属性
	content = cleanHTMLAttributes(content)

	if r.ParagraphTagClosed {
		// 标签闭合情况（如 <p>段落</p>）
		re := regexp.MustCompile(`(?i)<p[^>]*>(.*?)</p>`)
		matches := re.FindAllStringSubmatch(content, -1)

		var lines []string
		for _, m := range matches {
			if len(m) > 1 {
				text := strings.TrimSpace(stripHTMLTags(m[1]))
				if text != "" {
					lines = append(lines, "  "+text)
				}
			}
		}

		if len(lines) > 0 {
			return strings.Join(lines, "\n\n")
		}
	}

	// 标签不闭合情况（如 段落1<br><br>段落2）
	if r.ParagraphTag != "" {
		re, err := regexp.Compile(r.ParagraphTag)
		if err == nil {
			parts := re.Split(content, -1)
			var lines []string
			for _, part := range parts {
				text := strings.TrimSpace(stripHTMLTags(part))
				if text != "" {
					lines = append(lines, "  "+text)
				}
			}
			if len(lines) > 0 {
				return strings.Join(lines, "\n\n")
			}
		}
	}

	// 默认处理：按<br>和<p>分割
	content = regexp.MustCompile(`(?i)<br\s*/?>|<p>`).ReplaceAllString(content, "\n")
	content = stripHTMLTags(content)

	var lines []string
	for _, line := range strings.Split(content, "\n") {
		text := strings.TrimSpace(line)
		if text != "" {
			lines = append(lines, "  "+text)
		}
	}

	return strings.Join(lines, "\n\n")
}

// cleanInvisibleChars 清理不可见字符
func cleanInvisibleChars(text string) string {
	// 清理控制字符、私有区字符等
	re := regexp.MustCompile(`[\x00-\x08\x0B\x0C\x0E-\x1F\x7F\xA0]`)
	text = re.ReplaceAllString(text, "")

	// 清理零宽字符
	text = strings.ReplaceAll(text, "\u200B", "") // 零宽空格
	text = strings.ReplaceAll(text, "\uFEFF", "") // BOM

	return text
}

// cleanHTMLAttributes 清理HTML标签的属性
func cleanHTMLAttributes(content string) string {
	re := regexp.MustCompile(`<(\w+)[^>]*>`)
	return re.ReplaceAllString(content, "<$1>")
}

// stripHTMLTags 移除所有HTML标签
func stripHTMLTags(content string) string {
	re := regexp.MustCompile(`<[^>]+>`)
	return re.ReplaceAllString(content, "")
}

// randomSleep 随机延迟
func (p *ChapterParser) randomSleep() {
	if p.maxInterval <= p.minInterval {
		time.Sleep(time.Duration(p.minInterval) * time.Millisecond)
		return
	}
	interval := p.minInterval + rand.Intn(p.maxInterval-p.minInterval)
	time.Sleep(time.Duration(interval) * time.Millisecond)
}

// truncateString 截断字符串
func truncateString(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
