# Go-Novel-Reader 扩展任务清单（SQLite 重构版）

> 基于 Legado 分析，为 go-novel-reader 制定的 SQLite 存储方案和缓存功能扩展任务。
>
> **核心目标**：将现有的 JSON 存储迁移到 SQLite，并实现章节内容缓存。

## 任务概览

```
优先级说明:
P0 - 必须实现，核心功能（SQLite 迁移）
P1 - 建议实现，提升体验（缓存系统）
P2 - 可选实现，性能优化
```

---

## P0 - 核心功能（SQLite 存储迁移）

### Task 1: SQLite 存储后端实现

**目标**: 将现有的 JSON 文件存储迁移到 SQLite 数据库，提升查询效率和数据一致性

#### 1.1 选择 SQLite 库

**推荐库**: `modernc.org/sqlite`

**理由**:
- ✅ 纯 Go 实现，无需 CGO
- ✅ 跨平台编译简单
- ✅ 性能接近 go-sqlite3
- ✅ 零依赖，部署方便

**替代方案**: `github.com/mattn/go-sqlite3` (需要 CGO，性能更好，但编译复杂)

#### 1.2 数据库表设计

```sql
-- 书架表（对应 bookshelf.json）
CREATE TABLE IF NOT EXISTS books (
    id           TEXT PRIMARY KEY,           -- UUID
    name         TEXT NOT NULL,              -- 书名
    author       TEXT NOT NULL,              -- 作者
    active_source INTEGER NOT NULL DEFAULT 0, -- 当前书源索引
    added_at     INTEGER NOT NULL,           -- 添加时间（Unix时间戳）
    last_read_at INTEGER NOT NULL,           -- 最后阅读时间
    UNIQUE(name, author)
);

-- 书源表（多书源支持）
CREATE TABLE IF NOT EXISTS book_sources (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    book_id    TEXT NOT NULL,              -- 关联书籍ID
    source_id  INTEGER NOT NULL,           -- 书源ID
    book_url   TEXT NOT NULL,              -- 书籍URL
    toc_url    TEXT,                       -- 目录URL
    FOREIGN KEY (book_id) REFERENCES books(id) ON DELETE CASCADE,
    UNIQUE(book_id, source_id)
);

-- 阅读进度表（对应 progress.json）
CREATE TABLE IF NOT EXISTS reading_progress (
    book_id        TEXT PRIMARY KEY,        -- 关联书籍ID
    chapter_index  INTEGER NOT NULL,        -- 章节索引
    chapter_title  TEXT NOT NULL,           -- 章节标题
    scroll_offset  INTEGER NOT NULL,        -- 滚动偏移
    total_chapters INTEGER NOT NULL,        -- 总章节数
    last_read_at   INTEGER NOT NULL,        -- 最后阅读时间
    FOREIGN KEY (book_id) REFERENCES books(id) ON DELETE CASCADE
);

-- 目录缓存表
CREATE TABLE IF NOT EXISTS toc_cache (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    book_id       TEXT NOT NULL,           -- 关联书籍ID
    chapter_index INTEGER NOT NULL,        -- 章节索引
    chapter_title TEXT NOT NULL,           -- 章节标题
    chapter_url   TEXT NOT NULL,           -- 章节URL
    cached_at     INTEGER NOT NULL,        -- 缓存时间
    FOREIGN KEY (book_id) REFERENCES books(id) ON DELETE CASCADE,
    UNIQUE(book_id, chapter_index)
);

-- 章节内容缓存表
CREATE TABLE IF NOT EXISTS chapter_cache (
    book_id       TEXT NOT NULL,           -- 关联书籍ID
    chapter_index INTEGER NOT NULL,        -- 章节索引
    content       TEXT NOT NULL,           -- 章节内容
    cached_at     INTEGER NOT NULL,        -- 缓存时间
    PRIMARY KEY (book_id, chapter_index),
    FOREIGN KEY (book_id) REFERENCES books(id) ON DELETE CASCADE
);

-- 索引
CREATE INDEX IF NOT EXISTS idx_books_last_read ON books(last_read_at DESC);
CREATE INDEX IF NOT EXISTS idx_book_sources_book_id ON book_sources(book_id);
CREATE INDEX IF NOT EXISTS idx_toc_cache_book_id ON toc_cache(book_id, chapter_index);
```

#### 1.3 代码结构

**涉及文件**:
```
go-novel-reader/
├── storage/
│   ├── storage.go          # 存储接口（保持不变）
│   ├── json_store.go       # JSON实现（保留，用于迁移）
│   ├── sqlite_store.go     # 新增：SQLite实现
│   ├── migrate.go          # 新增：数据迁移工具
│   └── paths.go            # 路径管理
└── cmd/
    └── migrate/            # 新增：迁移命令行工具
        └── main.go
```

#### 1.4 SQLite Store 实现

```go
// storage/sqlite_store.go
package storage

import (
    "database/sql"
    "encoding/json"
    "time"

    _ "modernc.org/sqlite"
    "go-novel-reader/model"
)

type SQLiteStore struct {
    db     *sql.DB
    dbPath string
}

func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
    db, err := sql.Open("sqlite", dbPath)
    if err != nil {
        return nil, err
    }

    store := &SQLiteStore{
        db:     db,
        dbPath: dbPath,
    }

    if err := store.initSchema(); err != nil {
        return nil, err
    }

    return store, nil
}

func (s *SQLiteStore) initSchema() error {
    schema := `
    CREATE TABLE IF NOT EXISTS books (
        id           TEXT PRIMARY KEY,
        name         TEXT NOT NULL,
        author       TEXT NOT NULL,
        active_source INTEGER NOT NULL DEFAULT 0,
        added_at     INTEGER NOT NULL,
        last_read_at INTEGER NOT NULL,
        UNIQUE(name, author)
    );

    CREATE TABLE IF NOT EXISTS book_sources (
        id         INTEGER PRIMARY KEY AUTOINCREMENT,
        book_id    TEXT NOT NULL,
        source_id  INTEGER NOT NULL,
        book_url   TEXT NOT NULL,
        toc_url    TEXT,
        FOREIGN KEY (book_id) REFERENCES books(id) ON DELETE CASCADE,
        UNIQUE(book_id, source_id)
    );

    CREATE TABLE IF NOT EXISTS reading_progress (
        book_id        TEXT PRIMARY KEY,
        chapter_index  INTEGER NOT NULL,
        chapter_title  TEXT NOT NULL,
        scroll_offset  INTEGER NOT NULL,
        total_chapters INTEGER NOT NULL,
        last_read_at   INTEGER NOT NULL,
        FOREIGN KEY (book_id) REFERENCES books(id) ON DELETE CASCADE
    );

    CREATE INDEX IF NOT EXISTS idx_books_last_read ON books(last_read_at DESC);
    CREATE INDEX IF NOT EXISTS idx_book_sources_book_id ON book_sources(book_id);
    `

    _, err := s.db.Exec(schema)
    return err
}

// 实现 Store 接口
func (s *SQLiteStore) LoadBookshelf() (*model.Bookshelf, error) {
    bookshelf := &model.Bookshelf{
        Books: []model.BookRecord{},
    }

    rows, err := s.db.Query(`
        SELECT id, name, author, active_source, added_at, last_read_at
        FROM books
        ORDER BY last_read_at DESC
    `)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    for rows.Next() {
        var book model.BookRecord
        var addedAtUnix, lastReadAtUnix int64

        err := rows.Scan(
            &book.ID,
            &book.Name,
            &book.Author,
            &book.ActiveSource,
            &addedAtUnix,
            &lastReadAtUnix,
        )
        if err != nil {
            return nil, err
        }

        book.AddedAt = time.Unix(addedAtUnix, 0)
        book.LastReadAt = time.Unix(lastReadAtUnix, 0)

        // 加载书源
        sources, err := s.loadBookSources(book.ID)
        if err != nil {
            return nil, err
        }
        book.Sources = sources

        bookshelf.Books = append(bookshelf.Books, book)
    }

    return bookshelf, nil
}

func (s *SQLiteStore) loadBookSources(bookID string) ([]model.BookSource, error) {
    rows, err := s.db.Query(`
        SELECT source_id, book_url, toc_url
        FROM book_sources
        WHERE book_id = ?
        ORDER BY id
    `, bookID)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var sources []model.BookSource
    for rows.Next() {
        var src model.BookSource
        var tocURL sql.NullString

        err := rows.Scan(&src.SourceID, &src.BookURL, &tocURL)
        if err != nil {
            return nil, err
        }

        if tocURL.Valid {
            src.TocURL = tocURL.String
        }

        sources = append(sources, src)
    }

    return sources, nil
}

func (s *SQLiteStore) SaveBookshelf(bookshelf *model.Bookshelf) error {
    tx, err := s.db.Begin()
    if err != nil {
        return err
    }
    defer tx.Rollback()

    for _, book := range bookshelf.Books {
        _, err := tx.Exec(`
            INSERT OR REPLACE INTO books (id, name, author, active_source, added_at, last_read_at)
            VALUES (?, ?, ?, ?, ?, ?)
        `, book.ID, book.Name, book.Author, book.ActiveSource,
           book.AddedAt.Unix(), book.LastReadAt.Unix())
        if err != nil {
            return err
        }

        // 删除旧书源
        _, err = tx.Exec(`DELETE FROM book_sources WHERE book_id = ?`, book.ID)
        if err != nil {
            return err
        }

        // 插入新书源
        for _, src := range book.Sources {
            _, err = tx.Exec(`
                INSERT INTO book_sources (book_id, source_id, book_url, toc_url)
                VALUES (?, ?, ?, ?)
            `, book.ID, src.SourceID, src.BookURL, src.TocURL)
            if err != nil {
                return err
            }
        }
    }

    return tx.Commit()
}

func (s *SQLiteStore) LoadProgress() (*model.ProgressStore, error) {
    store := &model.ProgressStore{
        Progresses: make(map[string]*model.ReadingProgress),
    }

    rows, err := s.db.Query(`
        SELECT book_id, chapter_index, chapter_title, scroll_offset,
               total_chapters, last_read_at
        FROM reading_progress
    `)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    for rows.Next() {
        var p model.ReadingProgress
        var lastReadAtUnix int64

        err := rows.Scan(
            &p.BookID,
            &p.ChapterIndex,
            &p.ChapterTitle,
            &p.ScrollOffset,
            &p.TotalChapters,
            &lastReadAtUnix,
        )
        if err != nil {
            return nil, err
        }

        p.LastReadAt = time.Unix(lastReadAtUnix, 0)
        store.Progresses[p.BookID] = &p
    }

    return store, nil
}

func (s *SQLiteStore) SaveProgress(store *model.ProgressStore) error {
    tx, err := s.db.Begin()
    if err != nil {
        return err
    }
    defer tx.Rollback()

    for _, p := range store.Progresses {
        _, err := tx.Exec(`
            INSERT OR REPLACE INTO reading_progress
            (book_id, chapter_index, chapter_title, scroll_offset, total_chapters, last_read_at)
            VALUES (?, ?, ?, ?, ?, ?)
        `, p.BookID, p.ChapterIndex, p.ChapterTitle, p.ScrollOffset,
           p.TotalChapters, p.LastReadAt.Unix())
        if err != nil {
            return err
        }
    }

    return tx.Commit()
}

func (s *SQLiteStore) UpdateProgress(bookID string, progress *model.ReadingProgress) error {
    _, err := s.db.Exec(`
        INSERT OR REPLACE INTO reading_progress
        (book_id, chapter_index, chapter_title, scroll_offset, total_chapters, last_read_at)
        VALUES (?, ?, ?, ?, ?, ?)
    `, bookID, progress.ChapterIndex, progress.ChapterTitle,
       progress.ScrollOffset, progress.TotalChapters, progress.LastReadAt.Unix())

    // 同时更新书籍的最后阅读时间
    _, _ = s.db.Exec(`
        UPDATE books SET last_read_at = ? WHERE id = ?
    `, progress.LastReadAt.Unix(), bookID)

    return err
}

func (s *SQLiteStore) Close() error {
    return s.db.Close()
}
```

#### 1.5 数据迁移工具

```go
// storage/migrate.go
package storage

import (
    "fmt"
    "log"
    "os"
    "path/filepath"
)

type Migrator struct {
    jsonStore   *JSONStore
    sqliteStore *SQLiteStore
}

func NewMigrator(dataDir string) (*Migrator, error) {
    jsonStore := NewJSONStore(dataDir)

    dbPath := filepath.Join(dataDir, "novel.db")
    sqliteStore, err := NewSQLiteStore(dbPath)
    if err != nil {
        return nil, err
    }

    return &Migrator{
        jsonStore:   jsonStore,
        sqliteStore: sqliteStore,
    }, nil
}

func (m *Migrator) Migrate() error {
    log.Println("开始迁移数据...")

    // 1. 迁移书架数据
    log.Println("迁移书架数据...")
    bookshelf, err := m.jsonStore.LoadBookshelf()
    if err != nil {
        return fmt.Errorf("加载书架失败: %w", err)
    }

    if err := m.sqliteStore.SaveBookshelf(bookshelf); err != nil {
        return fmt.Errorf("保存书架失败: %w", err)
    }
    log.Printf("✓ 已迁移 %d 本书籍\n", len(bookshelf.Books))

    // 2. 迁移阅读进度
    log.Println("迁移阅读进度...")
    progress, err := m.jsonStore.LoadProgress()
    if err != nil {
        return fmt.Errorf("加载进度失败: %w", err)
    }

    if err := m.sqliteStore.SaveProgress(progress); err != nil {
        return fmt.Errorf("保存进度失败: %w", err)
    }
    log.Printf("✓ 已迁移 %d 条阅读进度\n", len(progress.Progresses))

    log.Println("数据迁移完成！")
    return nil
}

func (m *Migrator) Backup() error {
    // 备份 JSON 文件
    files := []string{"bookshelf.json", "progress.json"}
    backupDir := filepath.Join(filepath.Dir(m.jsonStore.bookshelfPath), "backup")

    if err := os.MkdirAll(backupDir, 0755); err != nil {
        return err
    }

    for _, file := range files {
        src := filepath.Join(filepath.Dir(m.jsonStore.bookshelfPath), file)
        dst := filepath.Join(backupDir, file)

        data, err := os.ReadFile(src)
        if err != nil {
            if os.IsNotExist(err) {
                continue
            }
            return err
        }

        if err := os.WriteFile(dst, data, 0644); err != nil {
            return err
        }
        log.Printf("✓ 已备份 %s\n", file)
    }

    return nil
}

func (m *Migrator) Close() {
    m.sqliteStore.Close()
}
```

#### 1.6 迁移命令行工具

```go
// cmd/migrate/main.go
package main

import (
    "flag"
    "log"
    "os"
    "path/filepath"

    "go-novel-reader/storage"
)

func main() {
    var dataDir string
    var noBackup bool

    flag.StringVar(&dataDir, "data", "", "数据目录路径")
    flag.BoolVar(&noBackup, "no-backup", false, "跳过备份")
    flag.Parse()

    if dataDir == "" {
        homeDir, _ := os.UserHomeDir()
        dataDir = filepath.Join(homeDir, ".go-novel-reader")
    }

    log.Printf("数据目录: %s\n", dataDir)

    migrator, err := storage.NewMigrator(dataDir)
    if err != nil {
        log.Fatalf("创建迁移器失败: %v", err)
    }
    defer migrator.Close()

    // 备份
    if !noBackup {
        log.Println("备份原始数据...")
        if err := migrator.Backup(); err != nil {
            log.Fatalf("备份失败: %v", err)
        }
    }

    // 迁移
    if err := migrator.Migrate(); err != nil {
        log.Fatalf("迁移失败: %v", err)
    }

    log.Println("✓ 迁移完成！")
}
```

#### 1.7 配置切换

```go
// main.go 中添加配置选项
package main

import (
    "flag"
    "go-novel-reader/storage"
)

var useSQL = flag.Bool("sql", false, "使用 SQLite 存储（默认使用 JSON）")

func main() {
    flag.Parse()

    var store storage.Store
    var err error

    if *useSQL {
        dbPath := filepath.Join(dataDir, "novel.db")
        store, err = storage.NewSQLiteStore(dbPath)
    } else {
        store = storage.NewJSONStore(dataDir)
    }

    if err != nil {
        log.Fatal(err)
    }
    defer store.Close()

    // ... 其余代码
}
```

**预估工作量**: 高（3-5天）

---

## P1 - 建议实现（缓存系统）

### Task 2: 章节内容缓存

**目标**: 避免重复请求，提升阅读体验

#### 2.1 使用 SQLite 缓存（推荐）

**优点**:
- 与书架数据统一管理
- 支持事务和 ACID
- 易于查询和统计
- 自动关联删除

**实现**:

```go
// storage/sqlite_store.go 添加方法

func (s *SQLiteStore) GetChapterContent(bookID string, chapterIndex int) (string, bool, error) {
    var content string
    err := s.db.QueryRow(`
        SELECT content FROM chapter_cache
        WHERE book_id = ? AND chapter_index = ?
    `, bookID, chapterIndex).Scan(&content)

    if err == sql.ErrNoRows {
        return "", false, nil
    }
    if err != nil {
        return "", false, err
    }

    return content, true, nil
}

func (s *SQLiteStore) SaveChapterContent(bookID string, chapterIndex int, content string) error {
    _, err := s.db.Exec(`
        INSERT OR REPLACE INTO chapter_cache (book_id, chapter_index, content, cached_at)
        VALUES (?, ?, ?, ?)
    `, bookID, chapterIndex, content, time.Now().Unix())

    return err
}

func (s *SQLiteStore) DeleteBookCache(bookID string) error {
    _, err := s.db.Exec(`
        DELETE FROM chapter_cache WHERE book_id = ?
    `, bookID)

    return err
}

func (s *SQLiteStore) GetCacheStats() (CacheStats, error) {
    var stats CacheStats

    err := s.db.QueryRow(`
        SELECT
            COUNT(DISTINCT book_id) as book_count,
            COUNT(*) as chapter_count,
            SUM(LENGTH(content)) as total_size
        FROM chapter_cache
    `).Scan(&stats.BookCount, &stats.ChapterCount, &stats.TotalSize)

    return stats, err
}

type CacheStats struct {
    BookCount    int   // 缓存书籍数
    ChapterCount int   // 缓存章节数
    TotalSize    int64 // 总大小（字节）
}
```

#### 2.2 使用文件缓存（可选方案）

如果不使用 SQLite，可以保持文件缓存：

```go
// cache/file_cache.go
package cache

import (
    "crypto/md5"
    "encoding/hex"
    "fmt"
    "os"
    "path/filepath"
)

type FileCache struct {
    baseDir string
}

func NewFileCache(baseDir string) *FileCache {
    cacheDir := filepath.Join(baseDir, "cache")
    os.MkdirAll(cacheDir, 0755)
    return &FileCache{baseDir: cacheDir}
}

func (c *FileCache) cachePath(bookID string, chapterIndex int) string {
    hash := md5.Sum([]byte(bookID))
    bookDir := hex.EncodeToString(hash[:8])
    return filepath.Join(c.baseDir, bookDir, fmt.Sprintf("%d.txt", chapterIndex))
}

func (c *FileCache) Get(bookID string, chapterIndex int) (string, bool, error) {
    path := c.cachePath(bookID, chapterIndex)
    data, err := os.ReadFile(path)
    if os.IsNotExist(err) {
        return "", false, nil
    }
    if err != nil {
        return "", false, err
    }
    return string(data), true, nil
}

func (c *FileCache) Put(bookID string, chapterIndex int, content string) error {
    path := c.cachePath(bookID, chapterIndex)
    dir := filepath.Dir(path)
    if err := os.MkdirAll(dir, 0755); err != nil {
        return err
    }
    return os.WriteFile(path, []byte(content), 0644)
}
```

**预估工作量**: 中等（1-2天）

---

### Task 3: 目录缓存

**目标**: 缓存目录列表，避免每次打开书籍都重新获取

**说明**: 目录缓存表已在 Task 1 中定义，这里实现相关方法。

```go
// storage/sqlite_store.go 添加方法

type TocItem struct {
    Index int    `json:"index"`
    Title string `json:"title"`
    URL   string `json:"url"`
}

func (s *SQLiteStore) GetTocCache(bookID string) ([]TocItem, bool, error) {
    rows, err := s.db.Query(`
        SELECT chapter_index, chapter_title, chapter_url
        FROM toc_cache
        WHERE book_id = ?
        ORDER BY chapter_index
    `, bookID)
    if err != nil {
        return nil, false, err
    }
    defer rows.Close()

    var items []TocItem
    for rows.Next() {
        var item TocItem
        if err := rows.Scan(&item.Index, &item.Title, &item.URL); err != nil {
            return nil, false, err
        }
        items = append(items, item)
    }

    if len(items) == 0 {
        return nil, false, nil
    }

    return items, true, nil
}

func (s *SQLiteStore) SaveTocCache(bookID string, items []TocItem) error {
    tx, err := s.db.Begin()
    if err != nil {
        return err
    }
    defer tx.Rollback()

    // 删除旧缓存
    _, err = tx.Exec(`DELETE FROM toc_cache WHERE book_id = ?`, bookID)
    if err != nil {
        return err
    }

    // 插入新缓存
    stmt, err := tx.Prepare(`
        INSERT INTO toc_cache (book_id, chapter_index, chapter_title, chapter_url, cached_at)
        VALUES (?, ?, ?, ?, ?)
    `)
    if err != nil {
        return err
    }
    defer stmt.Close()

    now := time.Now().Unix()
    for _, item := range items {
        _, err = stmt.Exec(bookID, item.Index, item.Title, item.URL, now)
        if err != nil {
            return err
        }
    }

    return tx.Commit()
}
```

**预估工作量**: 低（半天）

---

## P2 - 可选实现（性能优化）

### Task 4: 章节预加载

**目标**: 预加载后续章节，提升翻页体验

**说明**: 在后台预加载接下来 3-5 章的内容到缓存

```go
// service/preloader.go
package service

import (
    "go-novel-reader/storage"
    "sync"
)

type ChapterPreloader struct {
    store       storage.Store
    preloadSize int
    mu          sync.Mutex
    loading     map[string]bool
}

func NewChapterPreloader(store storage.Store, preloadSize int) *ChapterPreloader {
    return &ChapterPreloader{
        store:       store,
        preloadSize: preloadSize,
        loading:     make(map[string]bool),
    }
}

func (p *ChapterPreloader) PreloadAhead(bookID string, currentIndex int, totalChapters int, fetchFunc func(int) (string, error)) {
    go func() {
        for i := 1; i <= p.preloadSize; i++ {
            nextIndex := currentIndex + i
            if nextIndex >= totalChapters {
                break
            }

            // 检查是否已缓存
            if _, exists, _ := p.store.GetChapterContent(bookID, nextIndex); exists {
                continue
            }

            // 检查是否正在加载
            key := fmt.Sprintf("%s:%d", bookID, nextIndex)
            p.mu.Lock()
            if p.loading[key] {
                p.mu.Unlock()
                continue
            }
            p.loading[key] = true
            p.mu.Unlock()

            // 获取并缓存
            content, err := fetchFunc(nextIndex)
            if err == nil {
                p.store.SaveChapterContent(bookID, nextIndex, content)
            }

            p.mu.Lock()
            delete(p.loading, key)
            p.mu.Unlock()
        }
    }()
}
```

**预估工作量**: 中等（1天）

---

## 实现顺序建议

```
Phase 1 (SQLite 迁移):
├── Task 1.1-1.4: 实现 SQLite Store ★★★★★
├── Task 1.5-1.6: 实现数据迁移工具 ★★★★
└── Task 1.7: 配置切换和测试 ★★★

Phase 2 (缓存系统):
├── Task 2: 章节内容缓存 ★★★
└── Task 3: 目录缓存 ★★

Phase 3 (性能优化):
└── Task 4: 章节预加载 ★
```

---

## 文件结构变更

```
go-novel-reader/
├── storage/
│   ├── storage.go           # 存储接口（保持不变）
│   ├── json_store.go        # JSON实现（保留）
│   ├── sqlite_store.go      # 新增：SQLite实现
│   ├── migrate.go           # 新增：数据迁移
│   └── paths.go
├── cache/                    # 可选：文件缓存
│   └── file_cache.go
├── service/                  # 可选：预加载服务
│   └── preloader.go
├── cmd/
│   └── migrate/             # 新增：迁移工具
│       └── main.go
└── go.mod                   # 添加依赖：modernc.org/sqlite
```

---

## 数据存储路径

```
~/.go-novel-reader/
├── novel.db                 # SQLite 数据库（新增）
├── backup/                  # 备份目录（新增）
│   ├── bookshelf.json
│   └── progress.json
├── cache/                   # 可选：文件缓存目录
│   └── {bookId_hash}/
│       ├── 0.txt
│       ├── 1.txt
│       └── ...
└── [旧文件]
    ├── bookshelf.json       # 可在迁移后删除
    └── progress.json        # 可在迁移后删除
```

---

## 迁移步骤

1. **添加依赖**:
```bash
go get modernc.org/sqlite
```

2. **实现 SQLite Store**:
```bash
# 按照 Task 1 的代码实现
```

3. **运行迁移工具**:
```bash
go run cmd/migrate/main.go
# 或者编译后运行
go build -o migrate cmd/migrate/main.go
./migrate
```

4. **测试验证**:
```bash
# 使用 SQLite 模式启动
go run . -sql
```

5. **清理旧文件（可选）**:
```bash
# 确认迁移成功后，可删除旧的 JSON 文件
rm ~/.go-novel-reader/bookshelf.json
rm ~/.go-novel-reader/progress.json
```

---

## 性能对比预估

| 操作 | JSON | SQLite | 提升 |
|------|------|--------|------|
| 加载书架（10本书） | ~5ms | ~2ms | 2.5x |
| 加载书架（100本书） | ~50ms | ~5ms | 10x |
| 更新单本进度 | ~10ms | ~1ms | 10x |
| 查询书籍 | O(n) | O(log n) | 显著 |
| 缓存查询 | O(1) 文件 | O(1) 索引 | 相近 |

---

## 风险和注意事项

1. **并发安全**: SQLite 的 WAL 模式支持读写并发，建议启用
```go
db.Exec("PRAGMA journal_mode=WAL")
```

2. **数据备份**: 迁移前自动备份 JSON 文件

3. **兼容性**: 保留 JSON Store 实现，支持回退

4. **事务管理**: 批量操作使用事务，提升性能

5. **错误处理**: 数据库操作要有完善的错误处理和日志

---

## 下一步

1. 开始实现 Task 1：SQLite 存储后端
2. 编写单元测试
3. 实现数据迁移工具
4. 完成后再进行缓存系统的实现
