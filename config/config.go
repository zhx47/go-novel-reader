package config

// AppConfig 应用配置
type AppConfig struct {
	// [source] 书源配置
	ActiveRules  string // 激活规则文件

	// [crawl] 爬虫配置
	MinInterval      int // 最小间隔(ms)
	MaxInterval      int // 最大间隔(ms)
	RetryMinInterval int // 重试最小间隔
	RetryMaxInterval int // 重试最大间隔

	// [proxy] 代理配置
	ProxyEnabled int    // 是否启用代理
	ProxyHost    string // 代理主机
	ProxyPort    int    // 代理端口
}

// DefaultConfig 返回默认配置
func DefaultConfig() *AppConfig {
	return &AppConfig{
		ActiveRules:      "main-rules.json",
		MinInterval:      200,
		MaxInterval:      500,
		RetryMinInterval: 2000,
		RetryMaxInterval: 4000,
		ProxyEnabled:     0,
	}
}
