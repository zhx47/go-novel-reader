# Legado 小说阅读器技术分析文档

> 本文档详细分析了 Legado (开源阅读) 应用的数据持久化方案，为 go-novel-reader 扩展提供参考。

## 1. 项目概述

**Legado** 是一个功能强大的 Android 小说阅读器，使用 Kotlin + Room ORM + SQLite 架构。

| 项目属性 | 值 |
|----------|-----|
| 技术栈 | Kotlin + Android + Room ORM |
| 数据库版本 | v75 |
| 数据库引擎 | SQLite |
| 最低 SDK | 21 |

---

## 2. 数据库架构

### 2.1 核心表结构

Legado 使用 **21 个数据表** 存储各类数据：

| 表名 | 用途 | 主键 |
|------|------|------|
| `books` | 书籍信息/书架 | bookUrl |
| `book_groups` | 书籍分组 | groupId |
| `book_sources` | 书源配置 | bookSourceUrl |
| `chapters` | 章节目录 | (url, bookUrl) |
| `bookmarks` | 用户书签 | time |
| `searchBooks` | 搜索结果缓存 | bookUrl |
| `search_keywords` | 搜索关键词历史 | word |
| `replace_rules` | 替换净化规则 | id |
| `readRecord` | 阅读时间统计 | (deviceId, bookName) |
| `cookies` | HTTP Cookie 缓存 | url |
| `caches` | 通用缓存 | key |

### 2.2 数据库配置

```kotlin
@Database(
    version = 75,
    exportSchema = true,
    entities = [Book::class, BookGroup::class, BookSource::class, BookChapter::class, ...],
    autoMigrations = [/* 43个自动迁移 */]
)
abstract class AppDatabase : RoomDatabase()
```

---

## 3. 核心数据模型详解

### 3.1 书籍（Book） - 书架核心

**表名**: `books`

```kotlin
@Entity(
    tableName = "books",
    indices = [Index(value = ["name", "author"], unique = true)]
)
data class Book(
    @PrimaryKey
    override var bookUrl: String = "",          // 主键，书籍唯一标识

    // === 基础信息 ===
    override var name: String = "",             // 书名
    override var author: String = "",           // 作者
    override var origin: String = "",           // 书源URL
    override var originName: String = "",       // 书源名称
    override var type: Int = 0,                 // 类型：0-文本 1-音频 2-图片

    // === 封面和简介 ===
    override var coverUrl: String? = null,      // 封面URL
    override var customCoverUrl: String? = null,// 自定义封面
    override var intro: String? = null,         // 简介
    override var customIntro: String? = null,   // 自定义简介

    // === 分类标签 ===
    override var kind: String? = null,          // 分类/标签
    override var customTag: String? = null,     // 自定义标签

    // === 分组管理（位字段设计）===
    var group: Long = 0L,                       // 分组ID，使用位运算

    // === 阅读进度（内嵌在Book中）===
    var durChapterIndex: Int = 0,               // 当前章节索引
    var durChapterTitle: String? = null,        // 当前章节标题
    var durChapterPos: Int = 0,                 // 当前章节内位置
    var durChapterTime: Long = 0L,              // 最近阅读时间

    // === 章节信息 ===
    var totalChapterNum: Int = 0,               // 总章节数
    override var latestChapterTitle: String? = null,  // 最新章节
    var latestChapterTime: Long = 0L,           // 最新更新时间

    // === 更新检查 ===
    var lastCheckTime: Long = 0L,               // 最后检查时间
    var lastCheckCount: Int = 0,                // 发现新章节数
    var canUpdate: Boolean = true,              // 是否允许更新

    // === 排序和同步 ===
    var order: Int = 0,                         // 用户排序
    var originOrder: Int = 0,                   // 书源排序
    var syncTime: Long = 0L,                    // 同步时间戳

    // === 扩展配置（JSON序列化）===
    var variable: String? = null,               // 自定义变量
    var readConfig: ReadConfig? = null,         // 阅读配置

    // === 本地书籍专用 ===
    override var tocUrl: String = "",           // 目录URL
    override var wordCount: String? = null,     // 字数
    var charset: String? = null                 // 字符编码
)
```

**阅读配置（嵌套对象）**:

```kotlin
data class ReadConfig(
    var reverseToc: Boolean = false,           // 目录倒序
    var pageAnim: Int? = null,                 // 翻页动画
    var reSegment: Boolean = false,            // 重新分段
    var imageStyle: String? = null,            // 图片样式
    var useReplaceRule: Boolean? = null,       // 使用替换规则
    var delTag: Long = 0L,                     // 删除标签
    var ttsEngine: String? = null,             // TTS引擎
    var splitLongChapter: Boolean = true,      // 分割长章节
    var dailyChapters: Int = 3                 // 每日章节限制
)
```

**关键设计点**:
1. **阅读进度内嵌**: `durChapterIndex`, `durChapterPos`, `durChapterTime` 直接存储在书籍表中
2. **位字段分组**: 使用 Long 类型的位运算实现多分组
3. **JSON扩展**: 复杂配置使用 JSON 序列化

---

### 3.2 章节目录（BookChapter）

**表名**: `chapters`

```kotlin
@Entity(
    tableName = "chapters",
    primaryKeys = ["url", "bookUrl"],
    indices = [
        Index(value = ["bookUrl"], unique = false),
        Index(value = ["bookUrl", "index"], unique = true)
    ],
    foreignKeys = [ForeignKey(
        entity = Book::class,
        parentColumns = ["bookUrl"],
        childColumns = ["bookUrl"],
        onDelete = ForeignKey.CASCADE
    )]
)
data class BookChapter(
    var url: String = "",                       // 章节URL（主键1）
    var bookUrl: String = "",                   // 书籍URL（主键2，外键）

    // === 基础信息 ===
    var title: String = "",                     // 章节标题
    var index: Int = 0,                         // 章节序号（0开始）
    var baseUrl: String = "",                   // 基础URL

    // === 章节属性 ===
    var isVolume: Boolean = false,              // 是否为卷名
    var isVip: Boolean = false,                 // 是否VIP
    var isPay: Boolean = false,                 // 是否已购买
    var tag: String? = null,                    // 更新时间等标签
    var wordCount: String? = null,              // 本章字数

    // === 文件定位（本地书籍用）===
    var start: Long? = null,                    // 起始字节偏移
    var end: Long? = null,                      // 结束字节偏移

    // === EPUB 定位 ===
    var startFragmentId: String? = null,        // EPUB起始fragment
    var endFragmentId: String? = null,          // EPUB结束fragment

    // === 音频资源 ===
    var resourceUrl: String? = null,            // 音频真实URL

    // === 扩展 ===
    var variable: String? = null                // 自定义变量
)
```

**关键设计点**:
1. **复合主键**: `(url, bookUrl)` 保证唯一性
2. **外键级联删除**: 删除书籍时自动删除章节
3. **文件偏移**: `start`/`end` 用于快速定位本地文件内容
4. **多格式支持**: 同时支持网络、本地TXT、EPUB、音频

---

### 3.3 书籍分组（BookGroup）

**表名**: `book_groups`

```kotlin
@Entity(tableName = "book_groups")
data class BookGroup(
    @PrimaryKey
    val groupId: Long = 0L,                     // 分组ID（位字段值）
    var groupName: String,                      // 分组名称
    var order: Int = 0,                         // 显示顺序
    var cover: String? = null,                  // 分组封面
    var enableRefresh: Boolean = true,          // 是否启用刷新
    var show: Boolean = true,                   // 是否显示
    var bookSort: Int = 0                       // 书籍排序方式
) {
    companion object {
        // 预定义系统分组
        const val IdRoot = -100L                // 根分组
        const val IdAll = -1L                   // 全部
        const val IdLocal = -2L                 // 本地
        const val IdAudio = -3L                 // 音频
        const val IdNetNone = -4L               // 网络未分组
        const val IdLocalNone = -5L             // 本地未分组
        const val IdError = -11L                // 更新失败
    }
}
```

**分组使用方式**:
```kotlin
// 书籍添加到多个分组（位运算）
book.group = book.group or groupId

// 检查书籍是否属于某分组
if ((book.group and groupId) > 0) {
    // 属于该分组
}

// 从分组移除
book.group = book.group and groupId.inv()
```

---

### 3.4 阅读记录（ReadRecord）

**表名**: `readRecord`

```kotlin
@Entity(
    tableName = "readRecord",
    primaryKeys = ["deviceId", "bookName"]
)
data class ReadRecord(
    var deviceId: String = "",                  // 设备ID
    var bookName: String = "",                  // 书名
    var readTime: Long = 0L,                    // 累计阅读时间（毫秒）
    var lastRead: Long = System.currentTimeMillis()  // 最后阅读时间
)
```

**阅读记录展示（视图类）**:
```kotlin
data class ReadRecordShow(
    var bookName: String = "",
    var readTime: Long = 0L,
    var lastRead: Long = 0L
)
```

**关键设计点**:
1. **多设备支持**: 按设备ID区分阅读记录
2. **累计时间**: 记录总阅读时长
3. **独立于书架**: 即使书籍从书架删除，阅读记录仍保留

---

### 3.5 书签（Bookmark）

**表名**: `bookmarks`

```kotlin
@Entity(
    tableName = "bookmarks",
    indices = [Index(value = ["bookName", "bookAuthor"])]
)
data class Bookmark(
    @PrimaryKey
    var time: Long = 0L,                        // 主键，创建时间戳
    var bookName: String = "",                  // 书名
    var bookAuthor: String = "",                // 作者
    var chapterIndex: Int = 0,                  // 章节索引
    var chapterPos: Int = 0,                    // 章节内位置
    var chapterName: String = "",               // 章节名称
    var bookText: String = "",                  // 书签位置文本片段
    var content: String = ""                    // 书签备注
)
```

---

### 3.6 书源（BookSource） - 仅供参考

**表名**: `book_sources`

```kotlin
@Entity(
    tableName = "book_sources",
    indices = [Index(value = ["bookSourceUrl"])]
)
data class BookSource(
    @PrimaryKey
    var bookSourceUrl: String = "",             // 主键
    var bookSourceName: String = "",            // 名称
    var bookSourceGroup: String? = null,        // 分组（逗号分隔）
    var bookSourceType: Int = 0,                // 类型：0-文本 1-音频 2-图片 3-文件
    var bookUrlPattern: String? = null,         // 详情页URL正则
    var customOrder: Int = 0,                   // 排序
    var enabled: Boolean = true,                // 是否启用
    var enabledExplore: Boolean = true,         // 是否启用发现
    var enabledCookieJar: Boolean = false,      // 是否启用Cookie
    var concurrentRate: String? = null,         // 并发速率
    var header: String? = null,                 // 请求头（JSON）
    var loginUrl: String? = null,               // 登录地址
    var loginUi: String? = null,                // 登录UI（JSON）
    var loginCheckJs: String? = null,           // 登录检测JS
    var coverDecodeJs: String? = null,          // 封面解密JS
    var bookSourceComment: String? = null,      // 注释
    var lastUpdateTime: Long = 0L,              // 更新时间
    var respondTime: Long = 0L,                 // 响应时间
    var weight: Int = 0,                        // 权重

    // === 规则（JSON序列化）===
    var exploreUrl: String? = null,             // 发现URL
    var ruleExplore: ExploreRule? = null,       // 发现规则
    var searchUrl: String? = null,              // 搜索URL
    var ruleSearch: SearchRule? = null,         // 搜索规则
    var ruleBookInfo: BookInfoRule? = null,     // 书籍信息规则
    var ruleToc: TocRule? = null,               // 目录规则
    var ruleContent: ContentRule? = null        // 正文规则
)
```

---

## 4. 正文内容存储方案

**重要**: Legado 的正文内容 **不存储在数据库中**，而是使用文件缓存。

### 4.1 内容存储策略

```
内容获取流程:
1. 检查本地缓存文件
2. 如无缓存，从网络获取
3. 获取后保存到缓存目录
4. 读取时从缓存文件加载
```

### 4.2 缓存目录结构

```
/data/data/io.legado.app/cache/
└── book_cache/
    └── {bookUrl的MD5}/
        └── {chapterIndex}.txt
```

### 4.3 内容处理类

```kotlin
// BookContent.kt - 章节内容数据
data class BookContent(
    val sameTitleRemoved: Boolean,              // 是否移除重复标题
    val textList: List<String>,                 // 段落列表
    val effectiveReplaceRules: List<ReplaceRule>?  // 应用的替换规则
)

// ContentHelp.kt - 内容处理工具
object ContentHelp {
    // 段落重排
    fun reSegment(content: String, segmentSize: Int): List<String>

    // 标点优化
    fun lightNovelParagraph(content: String): String

    // 格式化处理
    fun format(content: String): String
}
```

---

## 5. DAO 接口设计

### 5.1 BookDao - 书籍操作

```kotlin
@Dao
interface BookDao {
    // === 查询 ===
    @Query("SELECT * FROM books ORDER BY durChapterTime DESC")
    fun flowAll(): Flow<List<Book>>

    @Query("SELECT * FROM books WHERE `group` & :groupId > 0")
    fun flowByGroup(groupId: Long): Flow<List<Book>>

    @Query("SELECT * FROM books WHERE bookUrl = :bookUrl")
    fun getBook(bookUrl: String): Book?

    @Query("SELECT * FROM books WHERE name = :name AND author = :author")
    fun getBook(name: String, author: String): Book?

    // === 修改 ===
    @Insert(onConflict = OnConflictStrategy.REPLACE)
    fun insert(vararg book: Book)

    @Update
    fun update(vararg book: Book)

    @Delete
    fun delete(vararg book: Book)

    // === 进度更新 ===
    @Query("UPDATE books SET durChapterIndex = :index, durChapterPos = :pos, durChapterTime = :time WHERE bookUrl = :bookUrl")
    fun upProgress(bookUrl: String, index: Int, pos: Int, time: Long)

    // === 换源 ===
    @Transaction
    fun changeSource(oldBook: Book, newBook: Book)
}
```

### 5.2 BookChapterDao - 章节操作

```kotlin
@Dao
interface BookChapterDao {
    @Query("SELECT * FROM chapters WHERE bookUrl = :bookUrl ORDER BY `index`")
    fun getChapterList(bookUrl: String): List<BookChapter>

    @Query("SELECT * FROM chapters WHERE bookUrl = :bookUrl AND `index` = :index")
    fun getChapter(bookUrl: String, index: Int): BookChapter?

    @Query("SELECT COUNT(*) FROM chapters WHERE bookUrl = :bookUrl")
    fun getChapterCount(bookUrl: String): Int

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    fun insert(vararg chapter: BookChapter)

    @Query("DELETE FROM chapters WHERE bookUrl = :bookUrl")
    fun delByBook(bookUrl: String)

    // 搜索章节
    @Query("SELECT * FROM chapters WHERE bookUrl = :bookUrl AND title LIKE '%' || :key || '%'")
    fun search(bookUrl: String, key: String): List<BookChapter>
}
```

### 5.3 ReadRecordDao - 阅读记录

```kotlin
@Dao
interface ReadRecordDao {
    @Query("SELECT * FROM readRecord")
    fun getAll(): List<ReadRecord>

    @Query("SELECT SUM(readTime) FROM readRecord")
    fun getAllTime(): Long

    @Query("SELECT readTime FROM readRecord WHERE bookName = :bookName")
    fun getReadTime(bookName: String): Long?

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    fun insert(vararg record: ReadRecord)

    @Query("UPDATE readRecord SET readTime = readTime + :time WHERE bookName = :bookName")
    fun addReadTime(bookName: String, time: Long)

    @Query("DELETE FROM readRecord WHERE bookName = :bookName")
    fun delete(bookName: String)
}
```

---

## 6. 数据库初始化流程

### 6.1 数据库创建回调

```kotlin
private val dbCallback = object : Callback() {
    override fun onCreate(db: SupportSQLiteDatabase) {
        // 数据库首次创建
    }

    override fun onOpen(db: SupportSQLiteDatabase) {
        // 每次打开数据库
        scope.launch {
            // 1. 初始化预定义分组
            initBookGroups()

            // 2. 数据清理
            cleanupInvalidData()

            // 3. 初始化辅助数据
            initKeyboardAssists()
        }
    }
}

private suspend fun initBookGroups() {
    val groups = listOf(
        BookGroup(IdAll, "全部"),
        BookGroup(IdLocal, "本地"),
        BookGroup(IdAudio, "音频"),
        BookGroup(IdNetNone, "网络未分组"),
        BookGroup(IdLocalNone, "本地未分组"),
        BookGroup(IdError, "更新失败")
    )
    groups.forEach { group ->
        if (bookGroupDao.getByID(group.groupId) == null) {
            bookGroupDao.insert(group)
        }
    }
}
```

### 6.2 版本迁移策略

```kotlin
Room.databaseBuilder(context, AppDatabase::class.java, "legado.db")
    .fallbackToDestructiveMigrationFrom(1, 2, 3, 4, 5, 6, 7, 8, 9)  // v1-9破坏性迁移
    .addMigrations(*DatabaseMigrations.migrations)                   // v10-42自定义迁移
    // v43+ 使用 AutoMigration
    .build()
```

---

## 7. 设计模式总结

### 7.1 位字段分组

**优点**:
- 一个字段支持多分组
- 查询效率高（位运算）
- 存储空间小

**使用场景**: 书籍分组管理

### 7.2 JSON序列化嵌套对象

**优点**:
- 灵活存储复杂配置
- 易于扩展字段
- 不需要频繁修改表结构

**使用场景**: ReadConfig、书源规则

### 7.3 阅读进度内嵌

**优点**:
- 查询效率高（无需 JOIN）
- 数据一致性好
- 实现简单

**使用场景**: 当前阅读位置存储在 Book 表中

### 7.4 文件缓存正文

**优点**:
- 避免数据库膨胀
- 读写效率高
- 易于清理

**使用场景**: 章节正文内容

---

## 8. 源码路径参考

```
legado/app/src/main/java/io/legado/app/data/
├── AppDatabase.kt                     # 数据库配置
├── DatabaseMigrations.kt              # 迁移脚本
├── dao/
│   ├── BookDao.kt                     # 书籍DAO
│   ├── BookChapterDao.kt              # 章节DAO
│   ├── BookGroupDao.kt                # 分组DAO
│   ├── BookmarkDao.kt                 # 书签DAO
│   ├── ReadRecordDao.kt               # 阅读记录DAO
│   └── ...
└── entities/
    ├── Book.kt                        # 书籍实体
    ├── BookChapter.kt                 # 章节实体
    ├── BookGroup.kt                   # 分组实体
    ├── BookProgress.kt                # 进度实体
    ├── Bookmark.kt                    # 书签实体
    ├── ReadRecord.kt                  # 阅读记录实体
    ├── BaseBook.kt                    # 书籍基类
    └── ...
```
