package main

import "time"

// 文件路径常量
const (
	dataFile        = "data/portfolio.json"
	watchlistFile   = "data/watchlist.json"
	configFile      = "cmd/conf/config.yml"
	refreshInterval = 5 * time.Second
)

// 语言常量
type Language string

const (
	Chinese Language = "zh"
	English Language = "en"
)

// 应用状态常量
type AppState int

const (
	MainMenu AppState = iota
	AddingStock
	Monitoring
	EditingStock
	SearchingStock
	SearchResult
	LanguageSelection
	WatchlistViewing
	SearchResultWithActions
	WatchlistSearchConfirm
	WatchlistTagging     // 自选股票打标签状态
	WatchlistTagSelect   // 自选股票标签选择状态
	WatchlistGroupSelect // 自选股票分组选择状态
	PortfolioSorting     // 持股列表排序状态
	WatchlistSorting     // 自选列表排序状态
)

// 排序字段枚举
type SortField int

const (
	SortByCode          SortField = iota // 股票代码
	SortByName                           // 股票名称
	SortByPrice                          // 现价
	SortByCostPrice                      // 成本价
	SortByChange                         // 涨跌额
	SortByChangePercent                  // 涨跌幅
	SortByQuantity                       // 持股数量
	SortByTotalProfit                    // 持仓盈亏
	SortByProfitRate                     // 盈亏率
	SortByMarketValue                    // 市值
	SortByTag                            // 标签 (仅自选列表)
	SortByTurnoverRate                   // 换手率 (仅自选列表)
	SortByVolume                         // 成交量 (仅自选列表)
)

// 排序方向枚举
type SortDirection int

const (
	SortAsc  SortDirection = iota // 升序
	SortDesc                      // 降序
)
