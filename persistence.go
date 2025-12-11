package main

import (
	"encoding/json"
	"os"

	"gopkg.in/yaml.v3"
)

// ============================================================================
// Portfolio 持仓数据持久化
// ============================================================================

// savePortfolio 保存持仓数据到文件
func (m *Model) savePortfolio() {
	data, err := json.MarshalIndent(m.portfolio, "", "  ")
	if err != nil {
		return
	}
	os.WriteFile(dataFile, data, 0644)
}

// loadPortfolio 从文件加载持仓数据
func loadPortfolio() Portfolio {
	data, err := os.ReadFile(dataFile)
	if err != nil {
		return Portfolio{Stocks: []Stock{}}
	}

	var portfolio Portfolio
	err = json.Unmarshal(data, &portfolio)
	if err != nil {
		return Portfolio{Stocks: []Stock{}}
	}
	return portfolio
}

// ============================================================================
// Watchlist 自选股数据持久化
// ============================================================================

// WatchlistStockLegacy 旧版自选股数据结构（用于迁移兼容）
type WatchlistStockLegacy struct {
	Code string   `json:"code"`
	Name string   `json:"name"`
	Tag  string   `json:"tag,omitempty"`  // 旧格式的单个标签
	Tags []string `json:"tags,omitempty"` // 新格式的多个标签
}

// WatchlistLegacy 旧版自选股列表（用于迁移兼容）
type WatchlistLegacy struct {
	Stocks []WatchlistStockLegacy `json:"stocks"`
}

// loadWatchlist 加载自选股票列表（支持旧格式迁移）
func loadWatchlist() Watchlist {
	data, err := os.ReadFile(watchlistFile)
	if err != nil {
		return Watchlist{Stocks: []WatchlistStock{}}
	}

	// 先尝试用兼容性结构体加载数据
	var legacyWatchlist WatchlistLegacy
	err = json.Unmarshal(data, &legacyWatchlist)
	if err != nil {
		return Watchlist{Stocks: []WatchlistStock{}}
	}

	// 转换为新格式
	var watchlist Watchlist
	for _, legacyStock := range legacyWatchlist.Stocks {
		newStock := WatchlistStock{
			Code: legacyStock.Code,
			Name: legacyStock.Name,
		}

		// 处理标签字段的兼容性
		if len(legacyStock.Tags) > 0 {
			// 新格式：直接使用 Tags 数组
			newStock.Tags = legacyStock.Tags
		} else if legacyStock.Tag != "" {
			// 旧格式：将单个 Tag 转换为 Tags 数组
			newStock.Tags = []string{legacyStock.Tag}
		} else {
			// 没有标签：使用默认标签
			newStock.Tags = []string{"-"}
		}

		watchlist.Stocks = append(watchlist.Stocks, newStock)
	}

	return watchlist
}

// saveWatchlist 保存自选股票列表
func (m *Model) saveWatchlist() {
	data, err := json.MarshalIndent(m.watchlist, "", "  ")
	if err != nil {
		return
	}
	os.WriteFile(watchlistFile, data, 0644)
}

// ============================================================================
// Config 配置文件持久化
// ============================================================================

// defaultMarketsConfig 获取默认的市场配置
func defaultMarketsConfig() MarketsConfig {
	return MarketsConfig{
		China: MarketConfig{
			Timezone: "Asia/Shanghai",
			TradingSessions: []TradingSession{
				{StartTime: "09:30", EndTime: "11:30"},
				{StartTime: "13:00", EndTime: "15:00"},
			},
			Weekdays: []int{1, 2, 3, 4, 5},
		},
		US: MarketConfig{
			Timezone: "America/New_York",
			TradingSessions: []TradingSession{
				{StartTime: "09:30", EndTime: "16:00"},
			},
			Weekdays: []int{1, 2, 3, 4, 5},
		},
		HongKong: MarketConfig{
			Timezone: "Asia/Hong_Kong",
			TradingSessions: []TradingSession{
				{StartTime: "09:30", EndTime: "12:00"},
				{StartTime: "13:00", EndTime: "16:00"},
			},
			Weekdays: []int{1, 2, 3, 4, 5},
		},
	}
}

// getDefaultConfig 获取默认配置
func getDefaultConfig() Config {
	return Config{
		System: SystemConfig{
			Language:      "en",        // 默认英文
			AutoStart:     true,        // 有数据时自动进入监控模式
			StartupModule: "portfolio", // 默认启动持股模块
			DebugMode:     false,       // 调试模式关闭
		},
		Display: DisplayConfig{
			ColorScheme:        "professional", // 专业配色方案
			DecimalPlaces:      3,              // 3位小数
			TableStyle:         "light",        // 轻量表格样式
			MaxLines:           10,             // 默认每页显示10行
			PortfolioHighlight: "yellow",       // 默认黄色
		},
		Update: UpdateConfig{
			RefreshInterval: 5,    // 5秒刷新间隔
			AutoUpdate:      true, // 自动更新开启
		},
		Markets: defaultMarketsConfig(), // 市场配置
	}
}

// loadConfig 加载配置文件
func loadConfig() Config {
	data, err := os.ReadFile(configFile)
	if err != nil {
		// 如果配置文件不存在，创建默认配置文件
		config := getDefaultConfig()
		saveConfig(config)
		return config
	}

	var config Config
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		// 如果配置文件格式错误，使用默认配置
		return getDefaultConfig()
	}

	// 验证配置的合理性
	if config.Display.MaxLines <= 0 || config.Display.MaxLines > 50 {
		config.Display.MaxLines = 10 // 默认值
	}

	// 验证并设置高亮颜色的默认值
	if config.Display.PortfolioHighlight == "" {
		config.Display.PortfolioHighlight = "yellow" // 默认黄色背景
		debugPrint("debug.config.defaultHighlight", config.Display.PortfolioHighlight)
	} else {
		debugPrint("debug.config.loadedHighlight", config.Display.PortfolioHighlight)
	}

	// 如果 Markets 为空，填充默认值（向后兼容）
	if config.Markets.China.Timezone == "" {
		config.Markets = defaultMarketsConfig()
		debugPrint("debug.config.defaultMarkets", "填充默认市场配置")
	}

	return config
}

// saveConfig 保存配置文件
func saveConfig(config Config) error {
	data, err := yaml.Marshal(config)
	if err != nil {
		return err
	}
	return os.WriteFile(configFile, data, 0644)
}
