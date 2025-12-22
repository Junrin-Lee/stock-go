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
	Code   string     `json:"code"`
	Name   string     `json:"name"`
	Tag    string     `json:"tag,omitempty"`    // 旧格式的单个标签
	Tags   []string   `json:"tags,omitempty"`   // 新格式的多个标签
	Market MarketType `json:"market,omitempty"` // 市场类型（用于兼容已有market字段的数据）
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

		// 处理市场字段的兼容性
		if legacyStock.Market == "" {
			// 自动识别市场类型
			newStock.Market = getMarketType(legacyStock.Code)
		} else {
			newStock.Market = legacyStock.Market
		}

		// 处理标签字段的兼容性并清理市场标签
		var userTags []string
		if len(legacyStock.Tags) > 0 {
			// 新格式：过滤掉市场标签，只保留用户自定义标签
			for _, tag := range legacyStock.Tags {
				if tag != "" && tag != "-" && !isMarketTag(tag) {
					userTags = append(userTags, tag)
				}
			}
			newStock.Tags = userTags
		} else if legacyStock.Tag != "" {
			// 旧格式：将单个 Tag 转换为 Tags 数组（如果不是市场标签）
			if !isMarketTag(legacyStock.Tag) && legacyStock.Tag != "-" {
				newStock.Tags = []string{legacyStock.Tag}
			} else {
				newStock.Tags = []string{}
			}
		} else {
			// 没有标签：使用空数组
			newStock.Tags = []string{}
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
			// 持股列表默认显示所有列（按当前顺序）
			PortfolioColumns: []string{
				"cursor", "code", "name", "prev_close", "open", "high",
				"low", "price", "cost", "quantity", "today_change",
				"position_profit", "profit_rate", "market_value",
			},
			// 自选列表默认显示所有列（按当前顺序）
			WatchlistColumns: []string{
				"cursor", "tag", "code", "name", "price", "prev_close",
				"open", "high", "low", "today_change", "turnover", "volume",
			},
		},
		Update: UpdateConfig{
			RefreshInterval: 5,    // 5秒刷新间隔
			AutoUpdate:      true, // 自动更新开启
		},
		Markets: defaultMarketsConfig(), // 市场配置
		IntradayCollection: IntradayCollectionConfig{
			EnableAutoStop:        true, // 启用自动停止
			CompletenessThreshold: 90.0, // 90% 完整性阈值
			MaxConsecutiveErrors:  5,    // 最大连续错误5次
			MinDatapoints:         20,   // 最小数据点20个
		},
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

	// 向后兼容：如果列配置为空，使用默认值
	if len(config.Display.PortfolioColumns) == 0 {
		config.Display.PortfolioColumns = getDefaultConfig().Display.PortfolioColumns
		debugPrint("debug.config.defaultPortfolioColumns", "使用默认持股列表列配置")
	}
	if len(config.Display.WatchlistColumns) == 0 {
		config.Display.WatchlistColumns = getDefaultConfig().Display.WatchlistColumns
		debugPrint("debug.config.defaultWatchlistColumns", "使用默认自选列表列配置")
	}

	// 验证列配置
	config.Display.PortfolioColumns = validatePortfolioColumns(config.Display.PortfolioColumns)
	config.Display.WatchlistColumns = validateWatchlistColumns(config.Display.WatchlistColumns)

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

// validatePortfolioColumns - 验证Portfolio列配置
func validatePortfolioColumns(configured []string) []string {
	required := []string{"cursor", "code", "name", "price"}
	valid := map[string]bool{
		"cursor": true, "code": true, "name": true, "prev_close": true,
		"open": true, "high": true, "low": true, "price": true,
		"cost": true, "quantity": true, "today_change": true,
		"position_profit": true, "profit_rate": true, "market_value": true,
	}

	return smartMergeRequiredColumns(configured, required, valid)
}

// validateWatchlistColumns - 验证Watchlist列配置
func validateWatchlistColumns(configured []string) []string {
	required := []string{"cursor", "tag", "code", "name", "price"}
	valid := map[string]bool{
		"cursor": true, "tag": true, "code": true, "name": true,
		"price": true, "prev_close": true, "open": true, "high": true,
		"low": true, "today_change": true, "turnover": true, "volume": true,
	}

	return smartMergeRequiredColumns(configured, required, valid)
}

// smartMergeRequiredColumns - 智能合并必须列
// 算法：在保留用户配置顺序的同时，智能插入缺失的必须列
func smartMergeRequiredColumns(userConfig []string, required []string, valid map[string]bool) []string {
	result := []string{}
	inserted := make(map[string]bool)

	// 第一步：添加用户配置的有效列
	for _, col := range userConfig {
		if valid[col] {
			result = append(result, col)
			inserted[col] = true
		} else {
			// 静默忽略无效列ID，在debug模式下记录
			debugPrint("debug.config.invalidColumn", col)
		}
	}

	// 第二步：收集缺失的必须列
	missingRequired := []string{}
	for _, req := range required {
		if !inserted[req] {
			missingRequired = append(missingRequired, req)
		}
	}

	// 第三步：智能插入缺失的必须列
	// 策略：在第一个用户配置列之后插入，如果用户配置为空则放在最前面
	if len(missingRequired) > 0 {
		insertPosition := 0
		if len(result) > 0 {
			insertPosition = 1 // 在第一列之后插入
		}
		result = insertAt(result, insertPosition, missingRequired...)
	}

	// 如果结果为空（用户配置为空且无必须列），返回必须列
	if len(result) == 0 {
		return required
	}

	return result
}

// insertAt - 在指定位置插入元素
func insertAt(slice []string, index int, values ...string) []string {
	// 确保index在有效范围内
	if index < 0 {
		index = 0
	}
	if index > len(slice) {
		index = len(slice)
	}

	// 创建新切片
	result := make([]string, 0, len(slice)+len(values))
	result = append(result, slice[:index]...)
	result = append(result, values...)
	result = append(result, slice[index:]...)
	return result
}

// contains - 检查字符串切片是否包含指定字符串
func contains(slice []string, str string) bool {
	for _, s := range slice {
		if s == str {
			return true
		}
	}
	return false
}
