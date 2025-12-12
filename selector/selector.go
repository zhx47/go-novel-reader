package selector

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/antchfx/htmlquery"
	"golang.org/x/net/html"
)

const jsSeparator = "@js:"

// ContentType 内容类型
type ContentType int

const (
	Text ContentType = iota
	HTML
	AttrHref
	AttrSrc
	AttrContent
	AttrValue
)

// Select 使用CSS或XPath选择元素
func Select(doc *goquery.Selection, query string) *goquery.Selection {
	if query == "" {
		return doc.Find("__not_found__")
	}

	// 去除JS部分
	actualQuery := strings.Split(query, jsSeparator)[0]
	actualQuery = strings.TrimSpace(actualQuery)

	// 判断是否为XPath（以 / 或 // 或 (/ 开头）
	if strings.HasPrefix(actualQuery, "/") || strings.HasPrefix(actualQuery, "//") || strings.HasPrefix(actualQuery, "(/") {
		return selectXPath(doc, actualQuery)
	}

	// CSS Selector查询
	return doc.Find(actualQuery)
}

// selectXPath 使用XPath选择元素
func selectXPath(doc *goquery.Selection, xpath string) *goquery.Selection {
	// 获取HTML节点
	htmlStr, _ := doc.Html()
	if htmlStr == "" {
		return doc.Find("__not_found__")
	}

	node, err := htmlquery.Parse(strings.NewReader("<html><body>" + htmlStr + "</body></html>"))
	if err != nil {
		return doc.Find("__not_found__")
	}

	nodes := htmlquery.Find(node, xpath)
	if len(nodes) == 0 {
		return doc.Find("__not_found__")
	}

	// 将XPath结果转换回goquery Selection
	var htmlParts []string
	for _, n := range nodes {
		htmlParts = append(htmlParts, nodeToHTML(n))
	}

	newDoc, err := goquery.NewDocumentFromReader(strings.NewReader(strings.Join(htmlParts, "")))
	if err != nil {
		return doc.Find("__not_found__")
	}

	return newDoc.Selection
}

// nodeToHTML 将html.Node转换为HTML字符串
func nodeToHTML(n *html.Node) string {
	var b strings.Builder
	html.Render(&b, n)
	return b.String()
}

// SelectAndGetContent 选择元素并获取内容
func SelectAndGetContent(doc *goquery.Selection, query string, contentType ContentType, baseURI string) string {
	if query == "" {
		return ""
	}

	parts := strings.SplitN(query, jsSeparator, 2)
	actualQuery := parts[0]

	sel := Select(doc, actualQuery)
	if sel.Length() == 0 {
		return ""
	}

	var result string
	switch contentType {
	case Text:
		result = strings.TrimSpace(sel.Text())
	case HTML:
		result, _ = sel.Html()
	case AttrHref:
		href, exists := sel.Attr("href")
		if exists {
			result = resolveURL(baseURI, href)
		}
	case AttrSrc:
		src, exists := sel.Attr("src")
		if exists {
			result = resolveURL(baseURI, src)
		}
	case AttrContent:
		result, _ = sel.Attr("content")
	case AttrValue:
		result, _ = sel.Attr("value")
	}

	// 如果有JS处理
	if len(parts) == 2 && result != "" {
		result = invokeJs(parts[1], result)
	}

	return result
}

// SelectFirst 选择第一个匹配元素
func SelectFirst(doc *goquery.Selection, query string) *goquery.Selection {
	return Select(doc, query).First()
}

// SelectAll 选择所有匹配元素
func SelectAll(doc *goquery.Selection, query string) *goquery.Selection {
	return Select(doc, query)
}

// invokeJs 执行JS脚本（简化实现）
func invokeJs(jsCode, input string) string {
	// 处理常见的JS场景
	jsCode = strings.TrimSpace(jsCode)

	// 场景1: r='prefix'+r (添加前缀)
	if strings.HasPrefix(jsCode, "r='") || strings.HasPrefix(jsCode, "r=\"") {
		re := regexp.MustCompile(`r=['"]([^'"]*)['"]\s*\+\s*r`)
		if matches := re.FindStringSubmatch(jsCode); len(matches) > 1 {
			return matches[1] + input
		}
	}

	// 场景2: r=r+'suffix' (添加后缀)
	if strings.Contains(jsCode, "r+") {
		re := regexp.MustCompile(`r\s*\+\s*['"]([^'"]*)['"]`)
		if matches := re.FindStringSubmatch(jsCode); len(matches) > 1 {
			return input + matches[1]
		}
	}

	// 场景3: r=r.replace('old', 'new')
	if strings.Contains(jsCode, ".replace(") {
		re := regexp.MustCompile(`r\.replace\(['"]([^'"]*)['"]\s*,\s*['"]([^'"]*)['"]`)
		if matches := re.FindStringSubmatch(jsCode); len(matches) > 2 {
			return strings.Replace(input, matches[1], matches[2], 1)
		}
	}

	// 场景4: r=r.replaceAll('old', 'new')
	if strings.Contains(jsCode, ".replaceAll(") {
		re := regexp.MustCompile(`r\.replaceAll\(['"]([^'"]*)['"]\s*,\s*['"]([^'"]*)['"]`)
		if matches := re.FindStringSubmatch(jsCode); len(matches) > 2 {
			return strings.ReplaceAll(input, matches[1], matches[2])
		}
	}

	return input
}

// resolveURL 解析相对URL为绝对URL
func resolveURL(baseURI, href string) string {
	if href == "" {
		return ""
	}

	// 已经是绝对路径
	if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") {
		return href
	}

	// 协议相对路径
	if strings.HasPrefix(href, "//") {
		return "http:" + href
	}

	// 解析基础URL
	base, err := url.Parse(baseURI)
	if err != nil {
		return href
	}

	// 解析相对URL
	ref, err := url.Parse(href)
	if err != nil {
		return href
	}

	// 合并URL
	return base.ResolveReference(ref).String()
}

// HasJsCode 判断查询是否包含JS代码
func HasJsCode(query string) bool {
	return strings.Contains(query, jsSeparator)
}

// GetJsCode 获取查询中的JS代码
func GetJsCode(query string) string {
	parts := strings.SplitN(query, jsSeparator, 2)
	if len(parts) == 2 {
		return parts[1]
	}
	return ""
}

// GetQuery 获取去除JS后的查询
func GetQuery(query string) string {
	return strings.Split(query, jsSeparator)[0]
}
