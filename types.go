package main

import (
	"sync"
	"time"
)

// Stock 持仓股票数据结构
type Stock struct {
	Code          string  `json:"code"`
	Name          string  `json:"name"`
	Price         float64 `json:"price"`
	CostPrice     float64 `json:"cost_price"`
	Quantity      int     `json:"quantity"`
	Change        float64 `json:"change"`
	ChangePercent float64 `json:"change_percent"`
	StartPrice    float64 `json:"start_price"`
	MaxPrice      float64 `json:"max_price"`
	MinPrice      float64 `json:"min_price"`
	PrevClose     float64 `json:"prev_close"`
}

// StockData 股票市场数据（来自API）
type StockData struct {
	Symbol        string  `json:"symbol"`
	Name          string  `json:"name"`
	Price         float64 `json:"price"`
	Change        float64 `json:"change"`
	ChangePercent float64 `json:"change_percent"`
	StartPrice    float64 `json:"start_price"`
	MaxPrice      float64 `json:"max_price"`
	MinPrice      float64 `json:"min_price"`
	PrevClose     float64 `json:"prev_close"` // 昨日收盘价
	TurnoverRate  float64 `json:"turnover_rate"`
	Volume        int64   `json:"volume"`
}

// Portfolio 持仓组合
type Portfolio struct {
	Stocks []Stock `json:"stocks"`
}

// StockPriceCacheEntry 股价缓存条目结构
type StockPriceCacheEntry struct {
	Data       *StockData `json:"data"`        // 股价数据
	UpdateTime time.Time  `json:"update_time"` // 数据更新时间
	IsUpdating bool       `json:"is_updating"` // 是否正在更新中
}

// WatchlistStock 自选股票数据结构
type WatchlistStock struct {
	Code string   `json:"code"`
	Name string   `json:"name"`
	Tags []string `json:"tags"` // 标签字段，支持多个标签
}

// Watchlist 自选股票列表
type Watchlist struct {
	Stocks []WatchlistStock `json:"stocks"`
}

// MarketType 市场类型枚举
type MarketType string

const (
	MarketChina    MarketType = "china"
	MarketUS       MarketType = "us"
	MarketHongKong MarketType = "hongkong"
)

// TradingSession 交易时段
type TradingSession struct {
	StartTime string `yaml:"start_time"` // "09:30"
	EndTime   string `yaml:"end_time"`   // "11:30"
}

// MarketConfig 市场配置
type MarketConfig struct {
	Timezone        string           `yaml:"timezone"`         // "Asia/Shanghai"
	TradingSessions []TradingSession `yaml:"trading_sessions"` // 交易时段列表
	Weekdays        []int            `yaml:"weekdays"`         // [1,2,3,4,5] (周一到周五)
}

// MarketsConfig 所有市场配置
type MarketsConfig struct {
	China    MarketConfig `yaml:"china"`
	US       MarketConfig `yaml:"us"`
	HongKong MarketConfig `yaml:"hongkong"`
}

// Config 系统配置结构
type Config struct {
	System             SystemConfig             `yaml:"system"`              // 系统设置
	Display            DisplayConfig            `yaml:"display"`             // 显示设置
	Update             UpdateConfig             `yaml:"update"`              // 更新设置
	Markets            MarketsConfig            `yaml:"markets"`             // 市场配置
	IntradayCollection IntradayCollectionConfig `yaml:"intraday_collection"` // 分时数据采集配置
}

// SystemConfig 系统设置
type SystemConfig struct {
	Language      string `yaml:"language"`       // 默认语言 "zh" 或 "en"
	AutoStart     bool   `yaml:"auto_start"`     // 有数据时自动进入监控模式
	StartupModule string `yaml:"startup_module"` // 启动模块 "portfolio"(持股) 或 "watchlist"(自选)
	DebugMode     bool   `yaml:"debug_mode"`     // 调试模式开关
}

// DisplayConfig 显示设置
type DisplayConfig struct {
	ColorScheme        string   `yaml:"color_scheme"`        // 颜色方案 "professional", "simple"
	DecimalPlaces      int      `yaml:"decimal_places"`      // 价格显示小数位数
	TableStyle         string   `yaml:"table_style"`         // 表格样式 "light", "bold", "simple"
	MaxLines           int      `yaml:"max_lines"`           // 列表每页最大显示行数
	PortfolioHighlight string   `yaml:"portfolio_highlight"` // 自选列表中持仓股票的背景高亮颜色
	PortfolioColumns   []string `yaml:"portfolio_columns"`   // 持股列表显示的列（按顺序）
	WatchlistColumns   []string `yaml:"watchlist_columns"`   // 自选列表显示的列（按顺序）
}

// UpdateConfig 更新设置
type UpdateConfig struct {
	RefreshInterval int  `yaml:"refresh_interval"` // 刷新间隔（秒）
	AutoUpdate      bool `yaml:"auto_update"`      // 是否自动更新
}

// TextMap 文本映射结构（用于i18n）
type TextMap map[string]string

// Model 应用程序主模型
type Model struct {
	state           AppState
	currentMenuItem int
	menuItems       []string
	cursor          int
	input           string
	inputCursor     int // 通用输入框光标位置
	message         string
	portfolio       Portfolio
	watchlist       Watchlist // 自选股票列表
	config          Config    // 系统配置
	debugMode       bool
	language        Language
	debugLogs       []string // 调试日志存储
	debugScrollPos  int      // debug日志滚动位置

	// For stock addition
	addingStep         int
	tempCode           string
	tempCodeCursor     int // 股票代码输入光标位置
	tempCost           string
	tempCostCursor     int // 成本价输入光标位置
	tempQuantity       string
	tempQuantityCursor int // 数量输入光标位置
	stockInfo          *StockData
	fromSearch         bool     // 标记是否从搜索结果添加
	previousState      AppState // 记录进入编辑/删除前的状态

	// For stock editing
	editingStep        int
	selectedStockIndex int

	// For stock searching
	searchInput         string
	searchInputCursor   int // 搜索输入光标位置
	searchResult        *StockData
	searchFromWatchlist bool // 标记是否从自选列表进入搜索

	// For language selection
	languageCursor int

	// For monitoring
	lastUpdate time.Time

	// For scrolling
	portfolioScrollPos int // 持股列表滚动位置
	watchlistScrollPos int // 自选列表滚动位置
	portfolioCursor    int // 持股列表当前选中行
	watchlistCursor    int // 自选列表当前选中行

	// For watchlist tagging and grouping
	selectedTag        string   // 当前选择的标签过滤
	availableTags      []string // 所有可用的标签列表
	tagInput           string   // 标签输入框内容
	tagInputCursor     int      // 标签输入光标位置
	tagSelectCursor    int      // 标签选择界面的游标位置
	currentStockTags   []string // 当前选中股票的标签列表（用于删除管理）
	tagManageCursor    int      // 标签管理界面的游标位置
	tagRemoveCursor    int      // 标签删除选择界面的游标位置
	isInRemoveMode     bool     // 是否处于删除模式
	tagToEdit          string   // 要编辑的原标签名称
	tagEditInput       string   // 标签编辑输入框内容
	tagEditInputCursor int      // 标签编辑输入光标位置

	// Performance optimization - cached filtered watchlist
	cachedFilteredWatchlist  []WatchlistStock // 缓存的过滤后自选列表
	cachedFilterTag          string           // 缓存的过滤标签
	isFilteredWatchlistValid bool             // 缓存是否有效

	// For sorting - 持股列表排序状态
	portfolioSortField     SortField     // 持股列表当前排序字段
	portfolioSortDirection SortDirection // 持股列表当前排序方向
	portfolioSortCursor    int           // 持股列表排序菜单光标位置
	portfolioIsSorted      bool          // 持股列表是否已经应用了排序

	// For sorting - 自选列表排序状态
	watchlistSortField     SortField     // 自选列表当前排序字段
	watchlistSortDirection SortDirection // 自选列表当前排序方向
	watchlistSortCursor    int           // 自选列表排序菜单光标位置
	watchlistIsSorted      bool          // 自选列表是否已经应用了排序

	// For stock price async data - 股价异步数据
	stockPriceCache      map[string]*StockPriceCacheEntry // 股价数据缓存
	stockPriceMutex      sync.RWMutex                     // 股价数据读写锁
	stockPriceUpdateTime time.Time                        // 上次更新股价数据的时间

	// For intraday data collection - 分时数据采集
	intradayManager *IntradayManager // 分时数据管理器

	// For intraday chart viewing - 分时图表查看
	chartViewStock        string        // 正在查看的股票代码
	chartViewStockName    string        // 股票名称
	chartViewDate         string        // 正在查看的日期 (YYYYMMDD)
	chartData             *IntradayData // 加载的分时数据
	chartLoadError        error         // 加载错误(如有)
	chartIsCollecting     bool          // 是否正在自动采集数据
	chartCollectStartTime time.Time     // 开始采集的时间
}

// tickMsg 定时刷新消息
type tickMsg struct{}

// stockPriceUpdateMsg 股价数据更新消息
type stockPriceUpdateMsg struct {
	Symbol string
	Data   *StockData
	Error  error
}

// checkDataAvailabilityMsg 数据可用性检查消息
type checkDataAvailabilityMsg struct {
	code string
	date string
}

// TimePoint 图表时间点数据
type TimePoint struct {
	Time  time.Time
	Value float64
}

// CollectionMode 数据采集模式
type CollectionMode int

const (
	CollectionModeHistorical CollectionMode = iota // 采集历史交易日数据
	CollectionModeLive                             // 采集当日实时数据
	CollectionModeComplete                         // 数据已完整，无需采集
)

// String returns the string representation of CollectionMode
func (c CollectionMode) String() string {
	switch c {
	case CollectionModeHistorical:
		return "Historical"
	case CollectionModeLive:
		return "Live"
	case CollectionModeComplete:
		return "Complete"
	default:
		return "Unknown"
	}
}

// TradingState 交易状态
type TradingState int

const (
	TradingStatePreMarket  TradingState = iota // 盘前（开盘前）
	TradingStateLive                           // 交易中
	TradingStatePostMarket                     // 盘后（收盘后，当日）
	TradingStateWeekend                        // 周末
	TradingStateHoliday                        // 假日
)

// String returns the string representation of TradingState
func (t TradingState) String() string {
	switch t {
	case TradingStatePreMarket:
		return "PreMarket"
	case TradingStateLive:
		return "Live"
	case TradingStatePostMarket:
		return "PostMarket"
	case TradingStateWeekend:
		return "Weekend"
	case TradingStateHoliday:
		return "Holiday"
	default:
		return "Unknown"
	}
}

// WorkerMetadata Worker状态元数据
type WorkerMetadata struct {
	StockCode         string         // 股票代码
	TargetDate        string         // 目标日期 (YYYYMMDD)
	Mode              CollectionMode // 采集模式
	StartTime         time.Time      // Worker启动时间
	LastUpdateTime    time.Time      // 最后更新时间
	DatapointCount    int            // 已采集数据点数量
	ConsecutiveErrors int            // 连续错误次数
	IsRunning         bool           // 是否正在运行
}

// IntradayCollectionConfig 分时数据采集配置
type IntradayCollectionConfig struct {
	EnableAutoStop        bool    `yaml:"enable_auto_stop"`        // 启用自动停止
	CompletenessThreshold float64 `yaml:"completeness_threshold"`  // 完整性阈值 (百分比)
	MaxConsecutiveErrors  int     `yaml:"max_consecutive_errors"`  // 最大连续错误次数
	MinDatapoints         int     `yaml:"min_datapoints"`          // 最小数据点数量
}
