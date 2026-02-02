package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"go-novel-reader/config"
	"go-novel-reader/ui"
)

func main() {
	// 加载配置
	loader := config.NewLoader(config.GetConfigPath())
	cfg, err := loader.Load()
	if err != nil {
		fmt.Printf("加载配置失败: %v\n", err)
		cfg = config.DefaultConfig()
	}

	// 创建应用
	model := ui.NewModel(cfg)

	// 启动程序
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseAllMotion())
	if _, err := p.Run(); err != nil {
		fmt.Printf("启动失败: %v\n", err)
		os.Exit(1)
	}
}
