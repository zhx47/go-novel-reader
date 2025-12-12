package config

import (
	"os"
	"path/filepath"

	"gopkg.in/ini.v1"
)

// Loader 配置加载器
type Loader struct {
	configPath string
}

// NewLoader 创建配置加载器
func NewLoader(configPath string) *Loader {
	return &Loader{configPath: configPath}
}

// Load 加载配置文件
func (l *Loader) Load() (*AppConfig, error) {
	cfg := DefaultConfig()

	// 如果配置文件不存在，返回默认配置
	if _, err := os.Stat(l.configPath); os.IsNotExist(err) {
		return cfg, nil
	}

	iniCfg, err := ini.Load(l.configPath)
	if err != nil {
		return cfg, err
	}

	// [source] section
	if section, err := iniCfg.GetSection("source"); err == nil {
		if key, err := section.GetKey("activeRules"); err == nil {
			cfg.ActiveRules = key.String()
		}
	}

	// [crawl] section
	if section, err := iniCfg.GetSection("crawl"); err == nil {
		if key, err := section.GetKey("minInterval"); err == nil {
			cfg.MinInterval, _ = key.Int()
		}
		if key, err := section.GetKey("maxInterval"); err == nil {
			cfg.MaxInterval, _ = key.Int()
		}
		if key, err := section.GetKey("retryMinInterval"); err == nil {
			cfg.RetryMinInterval, _ = key.Int()
		}
		if key, err := section.GetKey("retryMaxInterval"); err == nil {
			cfg.RetryMaxInterval, _ = key.Int()
		}
	}

	// [proxy] section
	if section, err := iniCfg.GetSection("proxy"); err == nil {
		if key, err := section.GetKey("enabled"); err == nil {
			cfg.ProxyEnabled, _ = key.Int()
		}
		if key, err := section.GetKey("host"); err == nil {
			cfg.ProxyHost = key.String()
		}
		if key, err := section.GetKey("port"); err == nil {
			cfg.ProxyPort, _ = key.Int()
		}
	}

	return cfg, nil
}

// GetConfigPath 获取配置文件路径
func GetConfigPath() string {
	// 优先使用当前目录下的config.ini
	if _, err := os.Stat("config.ini"); err == nil {
		return "config.ini"
	}

	// 使用可执行文件所在目录
	exe, err := os.Executable()
	if err != nil {
		return "config.ini"
	}

	return filepath.Join(filepath.Dir(exe), "config.ini")
}

// GetRulesDir 获取规则文件目录
func GetRulesDir() string {
	// 优先使用当前目录下的rules目录
	if _, err := os.Stat("rules"); err == nil {
		return "rules"
	}

	// 使用可执行文件所在目录
	exe, err := os.Executable()
	if err != nil {
		return "rules"
	}

	return filepath.Join(filepath.Dir(exe), "rules")
}
