package main

import (
	"fmt"
	"os"

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
	app := ui.NewApp(cfg)

	// 启动程序
	if err := app.Run(); err != nil {
		fmt.Printf("启动失败: %v\n", err)
		os.Exit(1)
	}
}
