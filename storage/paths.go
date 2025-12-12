package storage

import (
	"os"
	"path/filepath"
)

const (
	appDirName = ".go-novel-reader"
)

// GetDefaultDataDir 获取默认数据目录
func GetDefaultDataDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, appDirName), nil
}

// GetDataDirWithFallback 获取数据目录（带回退）
func GetDataDirWithFallback() string {
	// 1. 优先使用环境变量
	if dir := os.Getenv("GO_NOVEL_READER_DATA"); dir != "" {
		return dir
	}

	// 2. 使用用户主目录
	if dir, err := GetDefaultDataDir(); err == nil {
		return dir
	}

	// 3. 回退到当前目录
	return ".go-novel-reader-data"
}

// NewDefaultJSONStore 使用默认路径创建存储
func NewDefaultJSONStore() (*JSONStore, error) {
	dataDir := GetDataDirWithFallback()
	return NewJSONStore(dataDir)
}

// NewDefaultSQLiteStore 使用默认路径创建SQLite存储
func NewDefaultSQLiteStore() (*SQLiteStore, error) {
	dataDir := GetDataDirWithFallback()
	return NewSQLiteStore(dataDir)
}
