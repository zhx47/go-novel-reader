package parser

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"

	"go-novel-reader/httpclient"
	"go-novel-reader/model"
	"go-novel-reader/selector"
)

// SearchParser 搜索解析器
type SearchParser struct {
	rule   *model.Rule
	client *httpclient.Client
}

// NewSearchParser 创建搜索解析器
func NewSearchParser(rule *model.Rule, client *httpclient.Client) *SearchParser {
	return &SearchParser{rule: rule, client: client}
}

// Parse 搜索小说
func (p *SearchParser) Parse(keyword string) ([]*model.SearchResult, error) {
	r := p.rule.Search
	if r == nil || r.Disabled {
		return nil, fmt.Errorf("书源 %s 不支持搜索", p.rule.Name)
	}

	// 构建搜索URL
	searchURL := r.URL
	if strings.Contains(searchURL, "%s") {
		searchURL = fmt.Sprintf(searchURL, url.QueryEscape(keyword))
	}

	var resp *[]byte
	var err error

	if strings.ToLower(r.Method) == "post" {
		// POST请求
		formData := buildFormData(r.Data, keyword)
		httpResp, err := p.client.PostWithCookies(searchURL, formData, r.Cookies, r.Timeout)
		if err != nil {
			return nil, fmt.Errorf("搜索请求失败: %w", err)
		}
		body, err := httpclient.ReadBody(httpResp)
		if err != nil {
			return nil, fmt.Errorf("读取响应失败: %w", err)
		}
		resp = &body
	} else {
		// GET请求
		httpResp, err := p.client.GetWithCookies(searchURL, r.Cookies, r.Timeout)
		if err != nil {
			return nil, fmt.Errorf("搜索请求失败: %w", err)
		}
		body, err := httpclient.ReadBody(httpResp)
		if err != nil {
			return nil, fmt.Errorf("读取响应失败: %w", err)
		}
		resp = &body
	}

	// 解析HTML
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(*resp)))
	if err != nil {
		return nil, fmt.Errorf("解析HTML失败: %w", err)
	}

	// 设置baseURI
	baseURI := r.BaseURI
	if baseURI == "" {
		baseURI = p.rule.URL
	}

	// 获取搜索结果
	return p.parseResults(doc, baseURI)
}

// parseResults 解析搜索结果
func (p *SearchParser) parseResults(doc *goquery.Document, baseURI string) ([]*model.SearchResult, error) {
	r := p.rule.Search
	var results []*model.SearchResult

	// 选择搜索结果容器
	resultSel := selector.Select(doc.Selection, r.Result)

	resultSel.Each(func(i int, s *goquery.Selection) {
		// 获取书名
		bookName := selector.SelectAndGetContent(s, r.BookName, selector.Text, baseURI)
		if bookName == "" {
			return
		}

		// 获取详情页链接
		href := selector.SelectAndGetContent(s, r.BookName, selector.AttrHref, baseURI)

		// 获取其他字段
		author := selector.SelectAndGetContent(s, r.Author, selector.Text, baseURI)
		latestChapter := selector.SelectAndGetContent(s, r.LatestChapter, selector.Text, baseURI)
		lastUpdateTime := selector.SelectAndGetContent(s, r.LastUpdateTime, selector.Text, baseURI)
		category := selector.SelectAndGetContent(s, r.Category, selector.Text, baseURI)
		status := selector.SelectAndGetContent(s, r.Status, selector.Text, baseURI)
		wordCount := selector.SelectAndGetContent(s, r.WordCount, selector.Text, baseURI)

		results = append(results, &model.SearchResult{
			SourceID:       p.rule.ID,
			URL:            href,
			BookName:       strings.TrimSpace(bookName),
			Author:         strings.TrimSpace(author),
			LatestChapter:  strings.TrimSpace(latestChapter),
			LastUpdateTime: strings.TrimSpace(lastUpdateTime),
			Category:       strings.TrimSpace(category),
			Status:         strings.TrimSpace(status),
			WordCount:      strings.TrimSpace(wordCount),
		})
	})

	return results, nil
}

// buildFormData 构建POST表单数据
// 输入格式: "{searchkey: %s, type: all}"
func buildFormData(dataTemplate, keyword string) url.Values {
	values := url.Values{}

	if dataTemplate == "" {
		return values
	}

	// 移除花括号并解析
	dataTemplate = strings.TrimSpace(dataTemplate)
	dataTemplate = strings.TrimPrefix(dataTemplate, "{")
	dataTemplate = strings.TrimSuffix(dataTemplate, "}")

	// 尝试作为JSON解析
	jsonStr := "{" + dataTemplate + "}"
	jsonStr = strings.ReplaceAll(jsonStr, "%s", keyword)

	// 简单的JSON解析（处理非标准JSON格式）
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		// 尝试手动解析 key: value 格式
		pairs := strings.Split(dataTemplate, ",")
		for _, pair := range pairs {
			kv := strings.SplitN(pair, ":", 2)
			if len(kv) == 2 {
				key := strings.TrimSpace(kv[0])
				value := strings.TrimSpace(kv[1])
				value = strings.Trim(value, "\"'")
				if value == "%s" {
					value = keyword
				}
				values.Set(key, value)
			}
		}
	} else {
		for k, v := range data {
			values.Set(k, fmt.Sprintf("%v", v))
		}
	}

	return values
}
