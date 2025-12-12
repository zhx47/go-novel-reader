package source

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"go-novel-reader/config"
	"go-novel-reader/model"
)

// Manager 书源管理器
type Manager struct {
	rulesDir    string
	cachedRules []*model.Rule
}

// NewManager 创建书源管理器
func NewManager(rulesDir string) *Manager {
	return &Manager{
		rulesDir: rulesDir,
	}
}

// GetAllRules 获取所有规则
func (m *Manager) GetAllRules(filename string) ([]*model.Rule, error) {
	if m.cachedRules != nil {
		return m.cachedRules, nil
	}

	rules, err := m.loadRulesFromFile(filename)
	if err != nil {
		return nil, err
	}

	// 填充自增ID（从1开始）
	for i := range rules {
		rules[i].ID = i + 1
	}

	m.cachedRules = rules
	return rules, nil
}

// GetRule 根据ID获取规则
func (m *Manager) GetRule(sourceID int, filename string) (*model.Rule, error) {
	rules, err := m.GetAllRules(filename)
	if err != nil {
		return nil, err
	}

	for _, r := range rules {
		if r.ID == sourceID {
			return m.applyDefaultValues(r), nil
		}
	}

	return nil, fmt.Errorf("书源 ID %d 不存在", sourceID)
}

// GetSearchableRules 获取可搜索的规则列表
func (m *Manager) GetSearchableRules(filename string) ([]*model.Rule, error) {
	rules, err := m.GetAllRules(filename)
	if err != nil {
		return nil, err
	}

	var searchable []*model.Rule
	for _, r := range rules {
		if !r.Disabled && r.Search != nil && !r.Search.Disabled {
			searchable = append(searchable, r)
		}
	}

	return searchable, nil
}

// loadRulesFromFile 从嵌入数据或文件加载规则
func (m *Manager) loadRulesFromFile(filename string) ([]*model.Rule, error) {
	var data []byte
	var err error

	// 优先使用嵌入的规则数据
	data, err = GetEmbeddedRules(filename)
	if err != nil {
		// 嵌入数据不存在，回退到文件读取
		filePath := filepath.Join(m.rulesDir, filename)
		data, err = os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("读取规则文件失败: %w", err)
		}
	}

	var rules []*model.Rule
	if err := json.Unmarshal(data, &rules); err != nil {
		return nil, fmt.Errorf("解析规则文件失败: %w", err)
	}

	return rules, nil
}

// applyDefaultValues 应用默认值
func (m *Manager) applyDefaultValues(rule *model.Rule) *model.Rule {
	// Search默认值
	if rule.Search != nil {
		if rule.Search.BaseURI == "" {
			rule.Search.BaseURI = rule.URL
		}
		if rule.Search.Timeout == 0 {
			rule.Search.Timeout = 15
		}
		if rule.Search.Method == "" {
			rule.Search.Method = "GET"
		}
	}

	// Book默认值
	if rule.Book != nil {
		if rule.Book.BaseURI == "" {
			rule.Book.BaseURI = rule.URL
		}
		if rule.Book.Timeout == 0 {
			rule.Book.Timeout = 15
		}
	}

	// Toc默认值
	if rule.Toc != nil {
		if rule.Toc.BaseURI == "" {
			rule.Toc.BaseURI = rule.URL
		}
		if rule.Toc.Timeout == 0 {
			rule.Toc.Timeout = 15
		}
	}

	// Chapter默认值
	if rule.Chapter != nil {
		if rule.Chapter.BaseURI == "" {
			rule.Chapter.BaseURI = rule.URL
		}
		if rule.Chapter.Timeout == 0 {
			rule.Chapter.Timeout = 15
		}
		if rule.Chapter.ParagraphTag == "" && !rule.Chapter.ParagraphTagClosed {
			rule.Chapter.ParagraphTag = "<br>+"
		}
	}

	// Crawl默认值
	if rule.Crawl == nil {
		rule.Crawl = &model.CrawlConfig{
			MinInterval:      200,
			MaxInterval:      500,
			MaxAttempts:      3,
			RetryMinInterval: 2000,
			RetryMaxInterval: 4000,
		}
	} else {
		if rule.Crawl.MinInterval == 0 {
			rule.Crawl.MinInterval = 200
		}
		if rule.Crawl.MaxInterval == 0 {
			rule.Crawl.MaxInterval = 500
		}
		if rule.Crawl.MaxAttempts == 0 {
			rule.Crawl.MaxAttempts = 3
		}
	}

	return rule
}

// NewManagerWithConfig 使用配置创建书源管理器
func NewManagerWithConfig(cfg *config.AppConfig) *Manager {
	return NewManager(config.GetRulesDir())
}
