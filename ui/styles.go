package ui

import (
	"github.com/charmbracelet/lipgloss"
)

// 样式定义
var (
	// 颜色
	primaryColor   = lipgloss.Color("#7C3AED")
	secondaryColor = lipgloss.Color("#10B981")
	warningColor   = lipgloss.Color("#F59E0B")
	errorColor     = lipgloss.Color("#EF4444")
	mutedColor     = lipgloss.Color("#6B7280")

	// 标题样式
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(primaryColor).
			MarginBottom(1)

	// 副标题样式
	subtitleStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			MarginBottom(1)

	// 列表项样式
	itemStyle = lipgloss.NewStyle().
			PaddingLeft(2)

	// 选中项样式
	selectedItemStyle = lipgloss.NewStyle().
				PaddingLeft(2).
				Foreground(primaryColor).
				Bold(true)

	// 高亮样式
	highlightStyle = lipgloss.NewStyle().
			Foreground(secondaryColor)

	// 错误样式
	errorStyle = lipgloss.NewStyle().
			Foreground(errorColor)

	// 帮助信息样式
	helpStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			MarginTop(1)

	// 内容区样式
	contentStyle = lipgloss.NewStyle().
			Padding(1, 2)

	// 章节标题样式
	chapterTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(primaryColor).
				MarginBottom(1).
				Align(lipgloss.Center)

	// 加载中样式
	loadingStyle = lipgloss.NewStyle().
			Foreground(warningColor)
)

// 格式化函数
func formatTitle(s string) string {
	return titleStyle.Render(s)
}

func formatSubtitle(s string) string {
	return subtitleStyle.Render(s)
}

func formatItem(s string) string {
	return itemStyle.Render(s)
}

func formatSelectedItem(s string) string {
	return selectedItemStyle.Render("▸ " + s)
}

func formatHighlight(s string) string {
	return highlightStyle.Render(s)
}

func formatError(s string) string {
	return errorStyle.Render(s)
}

func formatHelp(s string) string {
	return helpStyle.Render(s)
}

func formatLoading(s string) string {
	return loadingStyle.Render(s)
}

func formatChapterTitle(s string) string {
	return chapterTitleStyle.Render(s)
}
