package source

import "embed"

//go:embed rules/main-rules.json
var embeddedRulesFS embed.FS

// GetEmbeddedRules 获取嵌入的规则文件内容
func GetEmbeddedRules(filename string) ([]byte, error) {
	return embeddedRulesFS.ReadFile("rules/" + filename)
}
