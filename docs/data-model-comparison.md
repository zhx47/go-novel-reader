# 数据模型对比：Legado vs Go-Novel-Reader（SQLite 重构版）

> 本文档对比 Legado 和 go-novel-reader 的数据存储方案，为 SQLite 迁移提供参考。

## 1. 整体架构对比

| 特性 | Legado | go-novel-reader (当前) | go-novel-reader (目标) |
|------|--------|----------------------|----------------------|
| 技术栈 | Kotlin + Room ORM | Go | Go |
| 存储方式 | SQLite 数据库 | JSON 文件 | SQLite 数据库 |
| 并发控制 | Room 内置 | sync.RWMutex | database/sql 内置 |
| 响应式 | Flow/LiveData | 无 | 无（不需要） |
| ORM | Room | 无 | 原生 SQL |

---

## 2. 书架/书籍数据对比

### 2.1 Legado Book 模型（核心字段）

```kotlin
data class Book(
    var bookUrl: String,                // 主键
    var name: String,                   // 书名
    var author: String,                 // 作者
    var origin: String,                 // 书源URL
    var originName: String,             // 书源名称
    var coverUrl: String?,              // 封面

    // 内嵌阅读进度
    var durChapterIndex: Int,           // 当前章节
    var durChapterTitle: String?,       // 当前章节标题
    var durChapterPos: Int,             // 章节内位置
    var durChapterTime: Long,           // 最近阅读时间

    // 章节统计
    var totalChapterNum: Int,           // 总章节数
    var latestChapterTitle: String?,    // 最新章节
)
```

### 2.2 go-novel-reader 当前模型

```go
// model/bookshelf.go - 书架记录（多源支持）
type BookRecord struct {
    ID           string        `json:"id"`            // UUID
    Name         string        `json:"name"`          // 书名
    Author       string        `json:"author"`        // 作者
    Sources      []BookSource  `json:"sources"`       // 多书源列表
    ActiveSource int           `json:"activeSource"`  // 当前书源索引
    AddedAt      time.Time     `json:"addedAt"`       // 添加时间
    LastReadAt   time.Time     `json:"lastReadAt"`    // 最近阅读
}

type BookSource struct {
    SourceID int    `json:"sourceId"`         // 书源ID
    BookURL  string `json:"bookUrl"`          // 书籍URL
    TocURL   string `json:"tocUrl,omitempty"` // 目录URL
}
```

### 2.3 SQLite 表设计

```sql
-- 书架表
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

CREATE INDEX IF NOT EXISTS idx_books_last_read ON books(last_read_at DESC);
CREATE INDEX IF NOT EXISTS idx_book_sources_book_id ON book_sources(book_id);
```

### 2.4 对比分析

| 功能 | Legado | go-novel-reader (JSON) | go-novel-reader (SQLite) |
|------|--------|----------------------|-------------------------|
| 书籍基础信息 | ✅ 单表 | ✅ 嵌套JSON | ✅ 规范化（两表） |
| 多书源支持 | ❌ 单源 | ✅ Sources数组 | ✅ 独立表（更规范） |
| 查询效率 | O(log n) | O(n) 全扫描 | O(log n) 索引 |
| 数据一致性 | ✅ ACID | ⚠️ 文件锁 | ✅ ACID |
| 并发写入 | ✅ 事务 | ❌ 锁全文件 | ✅ 事务 |

**设计优化**：
- 采用规范化设计，书源独立表
- 外键级联删除，保证数据一致性
- 索引优化常用查询（按最近阅读排序）

---

## 3. 阅读进度对比

### 3.1 Legado 进度管理

```kotlin
// 进度内嵌在 Book 中
data class Book(
    var durChapterIndex: Int = 0,       // 当前章节索引
    var durChapterTitle: String?,       // 当前章节标题
    var durChapterPos: Int = 0,         // 章节内字符位置
    var durChapterTime: Long = 0L       // 阅读时间戳
)
```

### 3.2 go-novel-reader 当前模型

```go
// model/progress.go
type ReadingProgress struct {
    BookID           string    `json:"bookId"`           // 书籍ID
    ChapterIndex     int       `json:"chapterIndex"`     // 章节索引
    ChapterTitle     string    `json:"chapterTitle"`     // 章节标题
    ScrollOffset     int       `json:"scrollOffset"`     // 滚动偏移
    TotalChapters    int       `json:"totalChapters"`    // 总章节数
    LastReadAt       time.Time `json:"lastReadAt"`       // 最后阅读时间
}

type ProgressStore struct {
    Progresses map[string]*ReadingProgress `json:"progresses"` // bookId -> progress
}
```

### 3.3 SQLite 表设计

```sql
-- 阅读进度表
CREATE TABLE IF NOT EXISTS reading_progress (
    book_id        TEXT PRIMARY KEY,        -- 关联书籍ID
    chapter_index  INTEGER NOT NULL,        -- 章节索引
    chapter_title  TEXT NOT NULL,           -- 章节标题
    scroll_offset  INTEGER NOT NULL,        -- 滚动偏移
    total_chapters INTEGER NOT NULL,        -- 总章节数
    last_read_at   INTEGER NOT NULL,        -- 最后阅读时间
    FOREIGN KEY (book_id) REFERENCES books(id) ON DELETE CASCADE
);
```

### 3.4 对比分析

| 功能 | Legado | go-novel-reader (JSON) | go-novel-reader (SQLite) |
|------|--------|----------------------|-------------------------|
| 存储方式 | 内嵌在Book表 | 独立JSON文件 | 独立表 |
| 数据规范化 | ❌ 反规范化 | ✅ | ✅ |
| 查询进度 | ✅ 无需JOIN | ⚠️ 加载全部 | ✅ 直接查询 |
| 更新进度 | ⚠️ 更新整行 | ⚠️ 保存全部 | ✅ 仅更新一行 |
| 外键约束 | ❌ | ❌ | ✅ 级联删除 |

**设计优化**：
- 独立表存储，逻辑清晰
- 外键约束保证一致性
- 删除书籍时自动删除进度

---

## 4. 目录缓存对比

### 4.1 Legado BookChapter 模型

```kotlin
data class BookChapter(
    var url: String,                    // 章节URL（主键1）
    var bookUrl: String,                // 书籍URL（主键2）
    var title: String,                  // 标题
    var index: Int,                     // 序号
)
```

### 4.2 go-novel-reader 当前状态

**当前没有目录缓存，每次都重新获取**

### 4.3 SQLite 表设计

```sql
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

CREATE INDEX IF NOT EXISTS idx_toc_cache_book_id ON toc_cache(book_id, chapter_index);
```

### 4.4 对比分析

| 功能 | Legado | go-novel-reader (JSON) | go-novel-reader (SQLite) |
|------|--------|----------------------|-------------------------|
| 目录缓存 | ✅ 独立表 | ❌ 无缓存 | ✅ 独立表 |
| 查询速度 | ✅ 快速 | ❌ 每次网络请求 | ✅ 快速 |
| 离线阅读 | ✅ 支持 | ❌ 不支持 | ✅ 支持 |
| 缓存时间 | ✅ 记录 | ❌ | ✅ 记录 |

**设计优化**：
- 独立表缓存目录列表
- 支持快速查询和离线访问
- 记录缓存时间，支持过期策略

---

## 5. 章节内容缓存对比

### 5.1 Legado 缓存方案

```
存储位置: /cache/book_cache/{bookUrl_md5}/{chapterIndex}.txt
特点:
- 文件系统存储，不占用数据库
- 按章节独立缓存
- 支持过期清理
```

### 5.2 go-novel-reader 当前状态

**当前没有正文缓存，每次都重新获取**

### 5.3 SQLite 表设计

```sql
-- 章节内容缓存表
CREATE TABLE IF NOT EXISTS chapter_cache (
    book_id       TEXT NOT NULL,           -- 关联书籍ID
    chapter_index INTEGER NOT NULL,        -- 章节索引
    content       TEXT NOT NULL,           -- 章节内容
    cached_at     INTEGER NOT NULL,        -- 缓存时间
    PRIMARY KEY (book_id, chapter_index),
    FOREIGN KEY (book_id) REFERENCES books(id) ON DELETE CASCADE
);
```

### 5.4 方案对比

| 方案 | 优点 | 缺点 | 适用场景 |
|------|------|------|---------|
| **文件缓存** (Legado) | • 不占数据库空间<br>• 读写快<br>• 易于清理 | • 文件系统碎片<br>• 难以统计<br>• 难以查询 | 内容量大 |
| **SQLite缓存** (推荐) | • 统一管理<br>• 支持事务<br>• 易于统计<br>• 级联删除 | • 数据库文件变大<br>• 需要VACUUM维护 | 内容量中等 |
| **混合方案** | • 灵活<br>• 可选 | • 复杂度高 | 高级需求 |

**推荐方案**：SQLite 缓存
- 统一数据管理
- 简化实现
- 便于统计和维护
- 支持外键级联删除

---

## 6. 存储层架构对比

### 6.1 Legado 存储架构

```
┌─────────────────────────────────┐
│          Room ORM               │
├─────────────────────────────────┤
│  DAO Interface Layer            │
│  - BookDao                      │
│  - BookChapterDao               │
├─────────────────────────────────┤
│       SQLite Database           │
│       legado.db                 │
└─────────────────────────────────┘
         +
┌─────────────────────────────────┐
│      File Cache System          │
│  - Chapter Content              │
└─────────────────────────────────┘
```

### 6.2 go-novel-reader 当前架构

```
┌─────────────────────────────────┐
│       Store Interface           │
│  storage/storage.go             │
├─────────────────────────────────┤
│       JSONStore                 │
│  storage/json_store.go          │
├─────────────────────────────────┤
│       JSON Files                │
│  - bookshelf.json               │
│  - progress.json                │
└─────────────────────────────────┘
```

### 6.3 go-novel-reader 目标架构

```
┌─────────────────────────────────┐
│       Store Interface           │
│  storage/storage.go             │
│  (保持不变，向后兼容)             │
├────────────┬────────────────────┤
│ JSONStore  │   SQLiteStore      │
│ (保留)     │   (新增，推荐)      │
├────────────┴────────────────────┤
│       SQLite Database           │
│  - books                        │
│  - book_sources                 │
│  - reading_progress             │
│  - toc_cache                    │
│  - chapter_cache                │
└─────────────────────────────────┘
```

---

## 7. 性能对比分析

### 7.1 书架操作性能

| 操作 | JSON (10本) | JSON (100本) | SQLite (10本) | SQLite (100本) |
|------|------------|-------------|--------------|---------------|
| 加载书架 | ~5ms | ~50ms | ~2ms | ~5ms |
| 添加书籍 | ~10ms | ~10ms | ~1ms | ~1ms |
| 更新进度 | ~10ms | ~10ms | ~1ms | ~1ms |
| 查询书籍 | O(n) | O(n) | O(log n) | O(log n) |

### 7.2 缓存操作性能

| 操作 | 文件缓存 | SQLite缓存 |
|------|---------|-----------|
| 读取章节 | ~1ms | ~1-2ms |
| 写入章节 | ~2ms | ~1-2ms |
| 批量统计 | 需遍历文件 | SQL聚合 ~5ms |
| 删除书籍缓存 | 删除目录 | CASCADE删除 |

### 7.3 并发性能

| 场景 | JSON | SQLite |
|------|------|--------|
| 并发读取 | ✅ 支持 | ✅ 支持（WAL模式） |
| 并发写入 | ❌ 锁全文件 | ✅ 行级锁 |
| 事务支持 | ❌ 无 | ✅ ACID |

---

## 8. 数据迁移策略

### 8.1 迁移流程

```
1. 备份 JSON 文件 → backup/
2. 创建 SQLite 数据库
3. 读取 bookshelf.json → 写入 books + book_sources 表
4. 读取 progress.json → 写入 reading_progress 表
5. 验证数据完整性
6. 切换到 SQLite 模式
7. (可选) 删除旧 JSON 文件
```

### 8.2 回滚策略

```
1. 保留 JSON Store 实现
2. 支持 -json 启动参数
3. 从备份恢复 JSON 文件
```

### 8.3 兼容性保证

- 保持 `Store` 接口不变
- JSONStore 和 SQLiteStore 都实现相同接口
- 通过配置选择存储后端
- 支持双模式运行

---

## 9. SQLite 优化建议

### 9.1 性能优化

```go
// 启用 WAL 模式（支持读写并发）
db.Exec("PRAGMA journal_mode=WAL")

// 设置缓存大小（默认2MB，建议8MB）
db.Exec("PRAGMA cache_size=-8000")

// 同步模式（NORMAL 平衡性能和安全）
db.Exec("PRAGMA synchronous=NORMAL")

// 临时文件存储在内存
db.Exec("PRAGMA temp_store=MEMORY")
```

### 9.2 维护操作

```go
// 定期 VACUUM 清理碎片
db.Exec("VACUUM")

// 分析查询优化
db.Exec("ANALYZE")
```

### 9.3 事务优化

```go
// 批量操作使用事务
tx, _ := db.Begin()
for _, item := range items {
    tx.Exec("INSERT ...")
}
tx.Commit()
```

---

## 10. 实施优先级

| 任务 | 优先级 | 复杂度 | 预估工作量 |
|------|--------|--------|-----------|
| SQLite Store 实现 | P0 | 高 | 3-5天 |
| 数据迁移工具 | P0 | 中 | 1-2天 |
| 章节内容缓存 | P1 | 中 | 1-2天 |
| 目录缓存 | P1 | 低 | 半天 |
| 性能测试和优化 | P1 | 中 | 1天 |

---

## 11. 总结

### 11.1 迁移收益

✅ **性能提升**：查询效率提升 2-10倍
✅ **数据一致性**：ACID 事务保证
✅ **并发支持**：支持读写并发
✅ **扩展性**：易于添加新功能
✅ **维护性**：SQL 标准，易于维护

### 11.2 实施路径

```
Phase 1: 核心迁移（必须）
├── 实现 SQLite Store
├── 实现数据迁移工具
└── 测试和验证

Phase 2: 缓存系统（建议）
├── 实现目录缓存
├── 实现章节内容缓存
└── 性能优化

Phase 3: 清理和维护
├── 删除旧 JSON 文件
└── 文档更新
```

### 11.3 风险控制

1. **数据备份**：迁移前自动备份
2. **兼容性**：保留 JSON Store 支持回退
3. **测试充分**：单元测试 + 集成测试
4. **渐进式迁移**：支持两种模式并存
5. **错误处理**：完善的错误处理和日志

---

## 参考资料

- [SQLite 官方文档](https://sqlite.org/docs.html)
- [modernc.org/sqlite](https://gitlab.com/cznic/sqlite)
- [Go database/sql 教程](https://go.dev/doc/database/querying)
- Legado 源码：`app/src/main/java/io/legado/app/data/`
