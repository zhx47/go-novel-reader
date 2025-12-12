package model

import (
	"sort"
	"time"
)

// SearchStatus 搜索状态
type SearchStatus int

const (
	SearchStatusPending SearchStatus = iota
	SearchStatusRunning
	SearchStatusSuccess
	SearchStatusFailed
	SearchStatusTimeout
)

// String 返回状态的字符串表示
func (s SearchStatus) String() string {
	switch s {
	case SearchStatusPending:
		return "等待中"
	case SearchStatusRunning:
		return "搜索中"
	case SearchStatusSuccess:
		return "成功"
	case SearchStatusFailed:
		return "失败"
	case SearchStatusTimeout:
		return "超时"
	default:
		return "未知"
	}
}

// SearchResultWithSource 带书源信息的搜索结果
type SearchResultWithSource struct {
	*SearchResult
	SourceName string `json:"source_name"`
}

// AggregatedSearchResult 聚合后的搜索结果
type AggregatedSearchResult struct {
	// 标准化信息
	NormalizedKey string `json:"normalized_key"`

	// 展示信息
	BookName       string `json:"book_name"`
	Author         string `json:"author"`
	Intro          string `json:"intro"`
	Category       string `json:"category"`
	LatestChapter  string `json:"latest_chapter"`
	LastUpdateTime string `json:"last_update_time"`
	Status         string `json:"status"`
	CoverURL       string `json:"cover_url"`
	WordCount      string `json:"word_count"`

	// 来自各书源的搜索结果
	Sources []*SearchResultWithSource `json:"sources"`

	// 统计信息
	SourceCount int `json:"source_count"`
}

// SourceSearchStat 单书源搜索统计
type SourceSearchStat struct {
	SourceID    int          `json:"source_id"`
	SourceName  string       `json:"source_name"`
	ResultCount int          `json:"result_count"`
	Duration    int64        `json:"duration_ms"`
	Error       string       `json:"error,omitempty"`
	Status      SearchStatus `json:"status"`
}

// MultiSourceSearchResult 多书源搜索结果
type MultiSourceSearchResult struct {
	Keyword     string                    `json:"keyword"`
	Results     []*AggregatedSearchResult `json:"results"`
	SourceStats map[int]*SourceSearchStat `json:"source_stats"`
	TotalCount  int                       `json:"total_count"`
	StartTime   time.Time                 `json:"start_time"`
	EndTime     time.Time                 `json:"end_time"`
}

// NewMultiSourceSearchResult 创建多源搜索结果
func NewMultiSourceSearchResult(keyword string) *MultiSourceSearchResult {
	return &MultiSourceSearchResult{
		Keyword:     keyword,
		Results:     make([]*AggregatedSearchResult, 0),
		SourceStats: make(map[int]*SourceSearchStat),
		StartTime:   time.Now(),
	}
}

// AggregateSearchResults 聚合多个书源的搜索结果
func AggregateSearchResults(keyword string, resultsBySource map[int][]*SearchResultWithSource) *MultiSourceSearchResult {
	aggregated := NewMultiSourceSearchResult(keyword)

	// 用于聚合的map
	aggregateMap := make(map[string]*AggregatedSearchResult)

	for _, results := range resultsBySource {
		for _, r := range results {
			key := NormalizeBookKey(r.BookName, r.Author)

			if existing, ok := aggregateMap[key]; ok {
				// 合并到已有结果
				existing.Sources = append(existing.Sources, r)
				existing.SourceCount++

				// 更新展示信息（取更完整的）
				if existing.Intro == "" && r.Intro != "" {
					existing.Intro = r.Intro
				}
				if existing.LatestChapter == "" && r.LatestChapter != "" {
					existing.LatestChapter = r.LatestChapter
				}
				if existing.LastUpdateTime == "" && r.LastUpdateTime != "" {
					existing.LastUpdateTime = r.LastUpdateTime
				}
				if existing.Category == "" && r.Category != "" {
					existing.Category = r.Category
				}
				if existing.Status == "" && r.Status != "" {
					existing.Status = r.Status
				}
				if existing.WordCount == "" && r.WordCount != "" {
					existing.WordCount = r.WordCount
				}
			} else {
				// 创建新的聚合结果
				agg := &AggregatedSearchResult{
					NormalizedKey:  key,
					BookName:       r.BookName,
					Author:         r.Author,
					Intro:          r.Intro,
					Category:       r.Category,
					LatestChapter:  r.LatestChapter,
					LastUpdateTime: r.LastUpdateTime,
					Status:         r.Status,
					WordCount:      r.WordCount,
					Sources:        []*SearchResultWithSource{r},
					SourceCount:    1,
				}
				aggregateMap[key] = agg
			}
		}
	}

	// 转为切片
	for _, agg := range aggregateMap {
		aggregated.Results = append(aggregated.Results, agg)
	}

	// 排序：优先按来源数量降序，其次按书名
	sort.Slice(aggregated.Results, func(i, j int) bool {
		if aggregated.Results[i].SourceCount != aggregated.Results[j].SourceCount {
			return aggregated.Results[i].SourceCount > aggregated.Results[j].SourceCount
		}
		return aggregated.Results[i].BookName < aggregated.Results[j].BookName
	})

	aggregated.TotalCount = len(aggregated.Results)
	aggregated.EndTime = time.Now()
	return aggregated
}

// GetFirstSource 获取第一个书源（用于默认选择）
func (asr *AggregatedSearchResult) GetFirstSource() *SearchResultWithSource {
	if len(asr.Sources) > 0 {
		return asr.Sources[0]
	}
	return nil
}

// GetSourceByID 根据书源ID获取搜索结果
func (asr *AggregatedSearchResult) GetSourceByID(sourceID int) *SearchResultWithSource {
	for _, s := range asr.Sources {
		if s.SourceID == sourceID {
			return s
		}
	}
	return nil
}

// ToBookRecord 转换为书籍记录
func (asr *AggregatedSearchResult) ToBookRecord() *BookRecord {
	firstSource := asr.GetFirstSource()
	if firstSource == nil {
		return nil
	}

	record := NewBookRecord(firstSource.SearchResult, firstSource.SourceName)

	// 添加其他书源
	for _, src := range asr.Sources[1:] {
		record.AddSource(&BookSource{
			SourceID:      src.SourceID,
			SourceName:    src.SourceName,
			BookURL:       src.URL,
			LatestChapter: src.LatestChapter,
			LastUpdated:   src.LastUpdateTime,
			IsAvailable:   true,
		})
	}

	// 使用聚合后的更完整信息
	if asr.Intro != "" {
		record.Intro = asr.Intro
	}
	if asr.Category != "" {
		record.Category = asr.Category
	}

	return record
}

// GetSuccessfulSourceCount 获取搜索成功的书源数量
func (msr *MultiSourceSearchResult) GetSuccessfulSourceCount() int {
	count := 0
	for _, stat := range msr.SourceStats {
		if stat.Status == SearchStatusSuccess {
			count++
		}
	}
	return count
}

// GetTotalSourceCount 获取总书源数量
func (msr *MultiSourceSearchResult) GetTotalSourceCount() int {
	return len(msr.SourceStats)
}

// GetDuration 获取搜索总耗时
func (msr *MultiSourceSearchResult) GetDuration() time.Duration {
	return msr.EndTime.Sub(msr.StartTime)
}
