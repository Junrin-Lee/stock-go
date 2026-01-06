package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/NimbleMarkets/ntcharts/canvas"
	"github.com/NimbleMarkets/ntcharts/linechart"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ============================================================================
// 分时数据采集控制
// ============================================================================

// startIntradayDataCollection 开始采集分时数据
func (m *Model) startIntradayDataCollection() {
	if m.intradayManager == nil {
		m.intradayManager = newIntradayManager(m)
	}

	// 收集当前页面的股票（支持所有市场）
	stocksToTrack := make(map[string]string) // code -> name

	if m.state == Monitoring {
		for _, stock := range m.portfolio.Stocks {
			stocksToTrack[stock.Code] = stock.Name
		}
	} else if m.state == WatchlistViewing {
		for _, stock := range m.watchlist.Stocks {
			stocksToTrack[stock.Code] = stock.Name
		}
	}

	logDebug("log.intraday.trackStart", len(stocksToTrack))

	// 为每只股票启动智能 worker
	for code, name := range stocksToTrack {
		if err := m.intradayManager.StartCollection(code, name); err != nil {
			logWarnDirect("Failed to start collection for %s: %v", code, err)
		}
	}
}

// stopIntradayDataCollection 停止采集分时数据
func (m *Model) stopIntradayDataCollection() {
	if m.intradayManager != nil {
		close(m.intradayManager.cancelChan)
		m.intradayManager = nil
		logDebug("log.intraday.trackStop")
	}
}

// ============================================================================
// 分时数据加载和解析
// ============================================================================

// fetchPrevCloseForStock 获取股票的昨日收盘价
// 优先级：1) 缓存 2) API调用 3) 降级到 0.0
func (m *Model) fetchPrevCloseForStock(code string) float64 {
	// 尝试从缓存获取
	m.stockPriceMutex.RLock()
	if entry, exists := m.stockPriceCache[code]; exists && entry.Data != nil {
		prevClose := entry.Data.PrevClose
		m.stockPriceMutex.RUnlock()
		if prevClose > 0 {
			logDebug("log.chart.prevCloseFromCache", code, prevClose)
			return prevClose
		}
	} else {
		m.stockPriceMutex.RUnlock()
	}

	// 缓存未命中 - 从API获取
	logDebug("log.chart.fetchingPrevClose", code)
	stockData := getStockPrice(code)
	if stockData != nil && stockData.PrevClose > 0 {
		logDebug("log.chart.prevCloseFromAPI", code, stockData.PrevClose)
		return stockData.PrevClose
	}

	logDebug("log.chart.prevCloseUnavailable", code)
	return 0.0 // 降级方案
}

// loadIntradayDataForDate 从磁盘加载特定股票和日期的分时数据
func (m *Model) loadIntradayDataForDate(code, name, date string) (*IntradayData, error) {
	// Use getIntradayFilePath for backward compatibility (tries new structure, falls back to old)
	filePath := getIntradayFilePath(code, date)

	fileData, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("file not found: %w", err)
	}

	var data IntradayData
	if err := json.Unmarshal(fileData, &data); err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}

	// 向后兼容：如果 Market 为空，自动识别
	if data.Market == "" {
		data.Market = getMarketType(code)
		logDebug("log.chart.marketAutoDetect", code, data.Market)
	}

	// 验证数据
	if len(data.Datapoints) == 0 {
		return nil, fmt.Errorf("no datapoints in file")
	}

	// 检查格式错误的数据
	for i, dp := range data.Datapoints {
		if dp.Time == "" || dp.Price == 0 {
			return nil, fmt.Errorf("invalid datapoint at index %d", i)
		}
	}

	// NEW: 如果文件缺失 PrevClose，从缓存/API获取
	if data.PrevClose == 0 {
		logDebug("log.chart.prevCloseMissing", code)
		data.PrevClose = m.fetchPrevCloseForStock(code)

		// 可选：异步保存更新后的数据（非阻塞，忽略错误）
		if data.PrevClose > 0 {
			go saveIntradayData(filePath, &data)
		}
	} else {
		logDebug("log.chart.prevCloseExists", code, data.PrevClose)
	}

	return &data, nil
}

// parseIntradayTime 解析分时时间字符串 ("09:31") + 日期 ("20251130") → time.Time
// location: 市场时区（用于正确解析时间）
func parseIntradayTime(date string, timeStr string, location *time.Location) time.Time {
	// date = "20251130", timeStr = "09:31"
	year, _ := strconv.Atoi(date[:4])
	month, _ := strconv.Atoi(date[4:6])
	day, _ := strconv.Atoi(date[6:8])

	parts := strings.Split(timeStr, ":")
	hour, _ := strconv.Atoi(parts[0])
	minute, _ := strconv.Atoi(parts[1])

	if location == nil {
		location = time.Local // 降级为本地时区
	}

	return time.Date(year, time.Month(month), day, hour, minute, 0, 0, location)
}

// ============================================================================
// 图表计算辅助函数
// ============================================================================

// calculateAdaptiveMargin 根据价格波动幅度智能计算Y轴margin
// 返回: minPrice, maxPrice, margin
func calculateAdaptiveMargin(prices []float64) (float64, float64, float64) {
	if len(prices) == 0 {
		return 0, 0, 0
	}

	minPrice := prices[0]
	maxPrice := prices[0]
	for _, p := range prices {
		if p < minPrice {
			minPrice = p
		}
		if p > maxPrice {
			maxPrice = p
		}
	}

	priceRange := maxPrice - minPrice

	// 处理无波动情况
	if priceRange < 0.0001 {
		// 价格基本无变化，使用固定的0.5%视觉空间
		margin := minPrice * 0.005
		return minPrice, maxPrice, margin
	}

	// 计算波动率
	volatility := (priceRange / minPrice) * 100

	var marginRatio float64
	if volatility < 1.0 {
		marginRatio = 0.5 // 50% margin for low volatility (<1%)
	} else if volatility < 3.0 {
		marginRatio = 0.2 // 20% margin for medium volatility (1-3%)
	} else {
		marginRatio = 0.1 // 10% margin for high volatility (>3%)
	}

	margin := priceRange * marginRatio

	// 确保最小margin（至少0.3%的价格）
	minMargin := minPrice * 0.003
	if margin < minMargin {
		margin = minMargin
	}

	return minPrice, maxPrice, margin
}

// ============================================================================
// 交易日计算
// ============================================================================

// getSmartChartDate 根据当前时间智能选择图表日期
// 开盘前（< 9:30）：返回上一个交易日
// 盘中（9:30-15:00）或收盘后（≥ 15:00）：返回今天
func getSmartChartDate() string {
	now := time.Now()
	hour := now.Hour()
	minute := now.Minute()

	// 判断是否在开盘前（9:30之前）
	if hour < 9 || (hour == 9 && minute < 30) {
		// 开盘前，查找上一个交易日
		return findPreviousTradingDayFromDate(now.Format("20060102"))
	}

	// 盘中或收盘后，使用今天
	return now.Format("20060102")
}

// findPreviousTradingDayFromDate 从指定日期查找上一个交易日（跳过周末）
func findPreviousTradingDayFromDate(dateStr string) string {
	// 解析日期
	currentDate, err := time.Parse("20060102", dateStr)
	if err != nil {
		return dateStr
	}

	// 最多尝试10天，找到上一个交易日
	for i := 1; i <= 10; i++ {
		prevDate := currentDate.AddDate(0, 0, -i)
		weekday := prevDate.Weekday()

		// 跳过周末（周六=6，周日=0）
		if weekday != time.Saturday && weekday != time.Sunday {
			return prevDate.Format("20060102")
		}
	}

	// 如果10天内都找不到，返回原日期
	return dateStr
}

// isWeekend 判断是否为周末
func isWeekend(t time.Time) bool {
	weekday := t.Weekday()
	return weekday == time.Saturday || weekday == time.Sunday
}

// Note: findPreviousTradingDay 已移至 intraday.go 并增强为支持多市场

// findNextTradingDay 查找下一个交易日（跳过周末）
// 最多往后查找7天，避免无限循环
func findNextTradingDay(currentDateStr string, maxDate time.Time) (string, error) {
	currentDate, err := time.Parse("20060102", currentDateStr)
	if err != nil {
		return "", err
	}

	// 最多往后查找7天
	for i := 1; i <= 7; i++ {
		nextDate := currentDate.AddDate(0, 0, i)

		// 不能超过最大日期（通常是今天）
		if nextDate.After(maxDate) {
			return "", fmt.Errorf("已到达最新日期")
		}

		if !isWeekend(nextDate) {
			return nextDate.Format("20060102"), nil
		}
	}

	// 如果7天内都是周末（理论上不可能），返回错误
	return "", fmt.Errorf("无法找到下一个交易日")
}

// formatDate 辅助函数: 格式化 YYYYMMDD → 可读日期
func formatDate(dateStr string) string {
	t, err := time.Parse("20060102", dateStr)
	if err != nil {
		return dateStr
	}
	return t.Format("2006-01-02")
}

// ============================================================================
// 时间框架创建
// ============================================================================

// createFixedTimeRange 创建固定的时间范围框架（9:30-15:00，共331个分钟点，包含午休）
// 创建完整连续的时间轴，午休时段（11:30-13:00）也包含在内，用于正确的时间映射
func (m *Model) createFixedTimeRange(date string, market MarketType) []TimePoint {
	var marketConfig MarketConfig
	switch market {
	case MarketChina:
		marketConfig = m.config.Markets.China
	case MarketUS:
		marketConfig = m.config.Markets.US
	case MarketHongKong:
		marketConfig = m.config.Markets.HongKong
	default:
		logDebug("log.chart.unknownMarket", market)
		return nil
	}

	points := make([]TimePoint, 0)

	// 遍历所有交易时段
	for _, session := range marketConfig.TradingSessions {
		startTime, err := parseTimeInMarket(date, session.StartTime, marketConfig)
		if err != nil {
			logDebug("log.chart.parseStartFail", session.StartTime, err)
			continue
		}

		endTime, err := parseTimeInMarket(date, session.EndTime, marketConfig)
		if err != nil {
			logDebug("log.chart.parseEndFail", session.EndTime, err)
			continue
		}

		// 生成该时段的所有分钟点
		totalMinutes := int(endTime.Sub(startTime).Minutes()) + 1
		for i := 0; i < totalMinutes; i++ {
			t := startTime.Add(time.Duration(i) * time.Minute)
			// 保持市场时区（不转换为本地时区，确保与存储数据的时区一致）
			points = append(points, TimePoint{
				Time:  t,
				Value: 0,
			})
		}
	}

	return points
}

// ============================================================================
// 图表创建
// ============================================================================

// createIntradayChart 从分时数据创建图表（使用普通 linechart 以精确控制数据点）
func (m *Model) createIntradayChart(termWidth, termHeight int) *linechart.Model {
	logDebug("log.chart.creating", termWidth, termHeight)

	if m.chartData == nil {
		logDebug("log.chart.dataNil")
		return nil
	}

	if len(m.chartData.Datapoints) == 0 {
		logDebug("log.chart.dataEmpty")
		return nil
	}

	logDebug("log.chart.dataPoints", len(m.chartData.Datapoints))

	// 最小大小检查
	minWidth := 40
	minHeight := 15

	if termWidth < minWidth || termHeight < minHeight {
		return nil
	}

	// 计算可用空间
	chartWidth := termWidth - 4
	if chartWidth < minWidth {
		chartWidth = minWidth
	}
	chartHeight := termHeight - 10
	if chartHeight < minHeight {
		chartHeight = minHeight
	}

	// === 创建完整时间框架（根据市场配置动态生成） ===
	timeFramework := m.createFixedTimeRange(m.chartData.Date, m.chartData.Market)

	// === 将实际数据填充到时间框架中 ===
	dataMap := make(map[string]float64)
	for _, dp := range m.chartData.Datapoints {
		dataMap[dp.Time] = dp.Price
	}

	// 填充价格值（缺失数据用最后已知价格）
	var lastKnownPrice float64
	if len(m.chartData.Datapoints) > 0 {
		lastKnownPrice = m.chartData.Datapoints[0].Price
	}

	// 准备数据点数组：索引 -> 价格
	dataPoints := make([]float64, len(timeFramework))
	timeLabels := make([]string, len(timeFramework)) // 索引 -> 时间标签

	for i, tp := range timeFramework {
		timeKey := tp.Time.Format("15:04")
		timeLabels[i] = timeKey

		if price, exists := dataMap[timeKey]; exists {
			dataPoints[i] = price
			lastKnownPrice = price
		} else {
			dataPoints[i] = lastKnownPrice
		}
	}

	// === 智能计算Y轴范围 ===
	actualPrices := make([]float64, len(m.chartData.Datapoints))
	for i, dp := range m.chartData.Datapoints {
		actualPrices[i] = dp.Price
	}

	minPrice, maxPrice, margin := calculateAdaptiveMargin(actualPrices)

	logDebug("log.chart.priceRange", minPrice, maxPrice, (maxPrice-minPrice)/minPrice*100, margin)

	// 设置样式：A股红涨绿跌，非A股绿涨红跌
	lastPrice := m.chartData.Datapoints[len(m.chartData.Datapoints)-1].Price
	prevClose := m.chartData.PrevClose // 使用昨日收盘价

	// 降级方案：如果 prevClose 不可用，回退到开盘价（保持现有行为）
	comparisonBase := prevClose
	if comparisonBase == 0 {
		comparisonBase = m.chartData.Datapoints[0].Price // 降级到开盘价
		logDebug("log.chart.colorFallback", m.chartData.Code)
	}

	// 判断是否为A股（SH/SZ开头）
	isAShare := strings.HasPrefix(m.chartData.Code, "SH") || strings.HasPrefix(m.chartData.Code, "SZ")

	var chartStyle lipgloss.Style
	if lastPrice > comparisonBase {
		// 上涨：A股红色，非A股绿色
		if isAShare {
			chartStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("9")) // 红色
		} else {
			chartStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10")) // 绿色
		}
	} else if lastPrice < comparisonBase {
		// 下跌：A股绿色，非A股红色
		if isAShare {
			chartStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10")) // 绿色
		} else {
			chartStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("9")) // 红色
		}
	} else {
		chartStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("15")) // 白色
	}

	// === 创建自定义 Y 轴标签格式化器 ===
	// 根据价格量级动态选择精度
	yLabelFormatter := func(index int, value float64) string {
		if value >= 100 {
			return fmt.Sprintf("%.1f", value) // 100+ → 150.5
		} else if value >= 10 {
			return fmt.Sprintf("%.2f", value) // 10-100 → 35.25
		} else if value >= 1 {
			return fmt.Sprintf("%.3f", value) // 1-10 → 5.745
		} else {
			return fmt.Sprintf("%.4f", value) // <1 → 0.7452
		}
	}

	// === 创建自定义 X 轴标签格式化器 ===
	// 只在4个关键时间点显示标签：开盘、午休、午盘、收盘
	// 使用时间容差匹配，因为刻度位置可能不恰好落在关键时间点
	xLabelFormatter := func(index int, value float64) string {
		idx := int(math.Round(value))
		if idx < 0 || idx >= len(timeLabels) {
			return ""
		}

		timeLabel := timeLabels[idx]

		// 解析时间为分钟数
		parts := strings.Split(timeLabel, ":")
		if len(parts) != 2 {
			return ""
		}
		hour, _ := strconv.Atoi(parts[0])
		minute, _ := strconv.Atoi(parts[1])
		totalMinutes := hour*60 + minute

		// 关键时间点（以分钟表示）及容差
		// 09:30 = 570, 11:30 = 690, 13:00 = 780, 15:00 = 900
		keyPoints := []struct {
			minutes   int
			label     string
			tolerance int
		}{
			{570, "09:30", 10}, // 09:30 ± 10分钟
			{690, "11:30", 10}, // 11:30 ± 10分钟
			{780, "13:00", 10}, // 13:00 ± 10分钟
			{900, "15:00", 20}, // 15:00 ± 10分钟
		}

		// 找到最接近的关键时间点
		for _, kp := range keyPoints {
			diff := totalMinutes - kp.minutes
			if diff < 0 {
				diff = -diff
			}
			if diff <= kp.tolerance {
				return kp.label
			}
		}

		return ""
	}

	// === 创建图表 ===
	logDebug("log.chart.dimensions", chartWidth, chartHeight, len(dataPoints), minPrice-margin, maxPrice+margin)

	lc := linechart.New(chartWidth, chartHeight,
		0, float64(len(dataPoints)-1), // X 轴范围：0 到数据点数量-1
		minPrice-margin, maxPrice+margin, // Y 轴范围
		linechart.WithXYSteps(8, 5), // X轴8个刻度, Y轴5个刻度
		linechart.WithXLabelFormatter(xLabelFormatter),
		linechart.WithYLabelFormatter(yLabelFormatter), // Y轴标签格式化器
		linechart.WithStyles(lipgloss.Style{}, lipgloss.Style{}, chartStyle),
	)

	// === 使用 Braille 字符绘制数据点 ===
	for i := 0; i < len(dataPoints)-1; i++ {
		p1 := canvas.Float64Point{X: float64(i), Y: dataPoints[i]}
		p2 := canvas.Float64Point{X: float64(i + 1), Y: dataPoints[i+1]}
		lc.DrawBrailleLineWithStyle(p1, p2, chartStyle)
	}

	lc.DrawXYAxisAndLabel()

	logDebug("log.chart.success")
	return &lc
}

// ============================================================================
// 数据采集触发
// ============================================================================

// triggerIntradayDataCollection 如果数据不存在则触发自动采集
func (m *Model) triggerIntradayDataCollection(code, name, date string) tea.Cmd {
	m.chartIsCollecting = true
	m.chartCollectStartTime = time.Now()

	// 确保 intradayManager 存在
	if m.intradayManager == nil {
		m.intradayManager = newIntradayManager(m)
	}

	// 为此特定股票启动智能 worker
	if err := m.intradayManager.StartCollection(code, name); err != nil {
		logWarnDirect("Failed to trigger collection for %s: %v", code, err)
	}

	// 返回命令每 2 秒检查数据可用性
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return checkDataAvailabilityMsg{code: code, date: date}
	})
}

// ============================================================================
// 分时图表状态处理器
// ============================================================================

// handleIntradayChartViewing 处理分时图表查看状态的键盘事件
func (m *Model) handleIntradayChartViewing(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		// 返回上一个状态
		m.state = m.previousState
		m.chartData = nil

		// 如果返回到 Monitoring 或 WatchlistViewing，需要重启定时器和数据更新
		if m.previousState == Monitoring || m.previousState == WatchlistViewing {
			m.lastUpdate = time.Now()
			// 返回定时器命令以恢复自动刷新
			var cmds []tea.Cmd
			cmds = append(cmds, m.tickCmd())
			// 立即触发一次股价更新
			if stockPriceCmd := m.startStockPriceUpdates(); stockPriceCmd != nil {
				cmds = append(cmds, stockPriceCmd)
			}
			return m, tea.Batch(cmds...)
		}

		return m, nil

	case "left":
		// 导航到前一个交易日（跳过周末）
		if m.chartData != nil {
			newDateStr := findPreviousTradingDay(m.chartViewStock, m.chartViewDate, m)

			// 尝试加载前一个交易日的数据
			data, err := m.loadIntradayDataForDate(m.chartViewStock, m.chartViewStockName, newDateStr)
			if err != nil {
				// 前一个交易日无数据，继续往前查找
				// 最多再往前尝试10个交易日
				found := false
				for attempt := 0; attempt < 10; attempt++ {
					newDateStr = findPreviousTradingDay(m.chartViewStock, newDateStr, m)
					data, err = m.loadIntradayDataForDate(m.chartViewStock, m.chartViewStockName, newDateStr)
					if err == nil {
						found = true
						break
					}
				}

				if !found {
					m.chartLoadError = fmt.Errorf("未找到更早的交易日数据")
					return m, nil
				}
			}

			// 更新到找到的交易日
			m.chartViewDate = newDateStr
			m.chartData = data
			m.chartLoadError = nil
		}
		return m, nil

	case "right":
		// 导航到下一个交易日（跳过周末，最多到今天）
		if m.chartData != nil {
			today := time.Now()
			newDateStr, err := findNextTradingDay(m.chartViewDate, today)
			if err != nil {
				// 已经是最新日期或无法找到下一个交易日
				m.chartLoadError = err
				return m, nil
			}

			// 尝试加载下一个交易日的数据
			data, err := m.loadIntradayDataForDate(m.chartViewStock, m.chartViewStockName, newDateStr)
			if err != nil {
				// 下一个交易日无数据，继续往后查找
				// 最多再往后尝试10个交易日（但不超过今天）
				found := false
				for attempt := 0; attempt < 10; attempt++ {
					newDateStr, err = findNextTradingDay(newDateStr, today)
					if err != nil {
						break
					}
					data, err = m.loadIntradayDataForDate(m.chartViewStock, m.chartViewStockName, newDateStr)
					if err == nil {
						found = true
						break
					}
				}

				if !found {
					m.chartLoadError = fmt.Errorf("未找到更新的交易日数据")
					return m, nil
				}
			}

			// 更新到找到的交易日
			m.chartViewDate = newDateStr
			m.chartData = data
			m.chartLoadError = nil
		}
		return m, nil
	}

	return m, nil
}

// ============================================================================
// 分时图表视图渲染
// ============================================================================

// getMarketTradingSessionText 获取市场交易时段文本
func (m *Model) getMarketTradingSessionText(market MarketType) string {
	var marketConfig MarketConfig
	switch market {
	case MarketChina:
		marketConfig = m.config.Markets.China
	case MarketUS:
		marketConfig = m.config.Markets.US
	case MarketHongKong:
		marketConfig = m.config.Markets.HongKong
	default:
		return m.getText("tradingSession")
	}

	if len(marketConfig.TradingSessions) == 0 {
		return m.getText("tradingSession")
	}

	var sessions []string
	for i, session := range marketConfig.TradingSessions {
		if i == 0 {
			// 第一个时段：开盘
			sessions = append(sessions, fmt.Sprintf("%s %s", session.StartTime, m.getText("open")))
		} else {
			// 后续时段：午盘
			sessions = append(sessions, fmt.Sprintf("%s %s", session.StartTime, m.getText("afternoon")))
		}

		if i < len(marketConfig.TradingSessions)-1 {
			// 非最后一个时段：午休
			sessions = append(sessions, fmt.Sprintf("%s %s", session.EndTime, m.getText("lunch")))
		} else {
			// 最后一个时段：收盘
			sessions = append(sessions, fmt.Sprintf("%s %s", session.EndTime, m.getText("close")))
		}
	}

	return "⏰ " + m.getText("tradingSession") + ": " + strings.Join(sessions, " | ")
}

// viewIntradayChart 渲染分时图表视图
func (m *Model) viewIntradayChart(termWidth, termHeight int) string {
	var b strings.Builder

	// 股票信息头部（带市场标签）
	marketLabel := ""
	if m.chartData != nil {
		switch m.chartData.Market {
		case MarketChina:
			marketLabel = m.getText("marketChina")
		case MarketUS:
			marketLabel = m.getText("marketUS")
		case MarketHongKong:
			marketLabel = m.getText("marketHongKong")
		default:
			marketLabel = m.getText("market")
		}
	}

	b.WriteString(lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("14")). // 青色
		Render(fmt.Sprintf("📈 %s [%s] - %s (%s) - %s",
			m.getText("intradayChart"),
			marketLabel,
			m.chartViewStock,
			m.chartViewStockName,
			formatDate(m.chartViewDate))))
	b.WriteString("\n\n")

	// === 动态交易时段说明 ===
	timeMarkers := ""
	if m.chartData != nil {
		timeMarkers = m.getMarketTradingSessionText(m.chartData.Market)
	} else {
		timeMarkers = m.getText("tradingSession")
	}
	b.WriteString(lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Render(timeMarkers))
	b.WriteString("\n\n")

	// 处理不同状态
	if m.chartIsCollecting {
		// 显示采集状态
		elapsed := time.Since(m.chartCollectStartTime).Seconds()
		b.WriteString(lipgloss.NewStyle().
			Foreground(lipgloss.Color("11")). // 黄色
			Render(fmt.Sprintf("%s... (%.0fs)", m.getText("collectingData"), elapsed)))
		b.WriteString("\n\n")
		b.WriteString(m.getText("pleaseWait"))
		b.WriteString("\n\n")
		b.WriteString(lipgloss.NewStyle().
			Faint(true).
			Render(fmt.Sprintf("[%s] %s", "ESC/Q", m.getText("back"))))
		return b.String()
	}

	if m.chartLoadError != nil {
		// 显示错误消息
		b.WriteString(lipgloss.NewStyle().
			Foreground(lipgloss.Color("9")). // 红色
			Render(fmt.Sprintf("%s: %s", m.getText("loadError"), m.chartLoadError.Error())))
		b.WriteString("\n\n")
		b.WriteString(m.getText("noDataAvailable"))
		b.WriteString("\n\n")
		b.WriteString(lipgloss.NewStyle().
			Faint(true).
			Render(fmt.Sprintf("[%s] %s", "ESC/Q", m.getText("back"))))
		return b.String()
	}

	if m.chartData == nil {
		b.WriteString(m.getText("noChartData"))
		b.WriteString("\n\n")
		b.WriteString(lipgloss.NewStyle().
			Faint(true).
			Render(fmt.Sprintf("[%s] %s", "ESC/Q", m.getText("back"))))
		return b.String()
	}

	// 创建图表
	chartModel := m.createIntradayChart(termWidth, termHeight)
	if chartModel == nil {
		b.WriteString(m.getText("terminalTooSmall"))
		b.WriteString("\n\n")
		b.WriteString(m.getText("pleaseResize"))
		b.WriteString("\n\n")
		b.WriteString(lipgloss.NewStyle().
			Faint(true).
			Render(fmt.Sprintf("[%s] %s", "ESC/Q", m.getText("back"))))
		return b.String()
	}

	// 计算头部统计信息
	prices := make([]float64, len(m.chartData.Datapoints))
	for i, dp := range m.chartData.Datapoints {
		prices[i] = dp.Price
	}
	minPrice := prices[0]
	maxPrice := prices[0]
	for _, p := range prices {
		if p < minPrice {
			minPrice = p
		}
		if p > maxPrice {
			maxPrice = p
		}
	}

	closePrice := prices[len(prices)-1]
	prevClose := m.chartData.PrevClose

	// 降级方案：如果 prevClose 不可用，回退到开盘价（保持现有行为）
	comparisonBase := prevClose
	if comparisonBase == 0 {
		comparisonBase = prices[0] // 降级到开盘价
		logDebug("log.chart.statsFallback", m.chartData.Code)
	}

	change := closePrice - comparisonBase
	changePercent := (change / comparisonBase) * 100

	// 统计信息行：A股红涨绿跌，非A股绿涨红跌
	isAShare := strings.HasPrefix(m.chartData.Code, "SH") || strings.HasPrefix(m.chartData.Code, "SZ")
	statsStyle := lipgloss.NewStyle()
	if change > 0 {
		// 上涨：A股红色，非A股绿色
		if isAShare {
			statsStyle = statsStyle.Foreground(lipgloss.Color("9")) // 红色
		} else {
			statsStyle = statsStyle.Foreground(lipgloss.Color("10")) // 绿色
		}
	} else if change < 0 {
		// 下跌：A股绿色，非A股红色
		if isAShare {
			statsStyle = statsStyle.Foreground(lipgloss.Color("10")) // 绿色
		} else {
			statsStyle = statsStyle.Foreground(lipgloss.Color("9")) // 红色
		}
	}

	b.WriteString(statsStyle.Render(fmt.Sprintf(
		"%s: %.2f  %s: %.2f  %s: %.2f  %s: %.2f  %s: %.2f  %s: %+.2f (%.2f%%)",
		m.getText("prevClose"), prevClose,
		m.getText("open"), prices[0],
		m.getText("close"), closePrice,
		m.getText("high"), maxPrice,
		m.getText("low"), minPrice,
		m.getText("change"), change, changePercent,
	)))
	b.WriteString("\n\n")

	// 渲染图表
	b.WriteString(chartModel.View())
	b.WriteString("\n\n")

	// 底部操作提示
	controls := fmt.Sprintf(
		"[%s/%s] %s | [%s/%s] %s",
		"←", "→", m.getText("changeDate"),
		"ESC", "Q", m.getText("back"),
	)
	b.WriteString(lipgloss.NewStyle().
		Faint(true).
		Render(controls))

	return b.String()
}

// ============================================================================
// 搜索模式分时数据采集（高频临时 Worker）
// ============================================================================

// startSearchIntradayWorker 为搜索模式启动高频临时数据采集
// 特点：
// 1. 5秒刷新间隔（高频）
// 2. 只采集单只股票
// 3. 数据存储在内存 (m.searchIntradayData)
// 4. 不写入磁盘
// 5. 首次立即执行
func (m *Model) startSearchIntradayWorker(code, name, date string) tea.Cmd {
	// 创建停止信号和更新通知 channel
	m.searchIntradayWorker = make(chan struct{})
	m.searchIntradayUpdateCh = make(chan struct{}, 10) // 带缓冲，避免阻塞

	logDebug("log.search.workerStart", code, date)

	// 启动临时 goroutine
	go m.runSearchIntradayWorker(code, name, date)

	// 启动监听更新的 cmd
	return m.waitForSearchIntradayUpdate()
}

// runSearchIntradayWorker 运行搜索模式的高频临时 worker
func (m *Model) runSearchIntradayWorker(code, name, date string) {
	// 使用 5 秒间隔的 ticker（高频刷新）
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	// 首次立即执行数据获取（不等待第一个 tick）
	m.fetchAndStoreSearchIntradayData(code, name, date)

	// 定时采集循环
	for {
		select {
		case <-ticker.C:
			// 检查是否仍在搜索模式
			if !m.isSearchMode || m.state != SearchResultWithActions {
				logDebug("log.search.workerAutoStop", code)
				return
			}

			// 检查市场是否开市（闭市时降低频率）
			if !isMarketOpen(code, m) {
				logDebug("log.search.marketClosed", code)
				// 市场关闭时仍然执行一次获取（获取当日完整数据）
				// 然后停止 worker
				m.fetchAndStoreSearchIntradayData(code, name, date)
				return
			}

			// 采集数据并更新内存
			m.fetchAndStoreSearchIntradayData(code, name, date)

		case <-m.searchIntradayWorker:
			// 收到停止信号
			logDebug("log.search.workerStop", code)
			return
		}
	}
}

// fetchAndStoreSearchIntradayData 获取并存储搜索模式的分时数据（仅内存）
func (m *Model) fetchAndStoreSearchIntradayData(code, name, date string) {
	// 从 API 获取最新数据
	datapoints, err := fetchIntradayDataFromAPI(code)
	if err != nil {
		logDebug("log.search.fetchFail", code, err)
		// 不返回错误，继续下次尝试
		return
	}

	if len(datapoints) == 0 {
		logDebug("log.search.noData", code)
		return
	}

	// 获取市场类型
	market := getMarketType(code)

	// 获取昨收价（用于图表颜色判断）
	prevClose := 0.0
	if m.searchResult != nil {
		prevClose = m.searchResult.PrevClose
	}

	// 直接使用新数据替换（不需要合并，每次都是完整数据）
	m.searchIntradayData = &IntradayData{
		Code:       code,
		Name:       name,
		Date:       date,
		Market:     market,
		Datapoints: datapoints, // 直接使用新数据
		UpdatedAt:  time.Now().Format("2006-01-02 15:04:05"),
		PrevClose:  prevClose,
	}

	logDebug("log.search.dataUpdated", code, len(datapoints), time.Now().Format("15:04:05"))

	// 发送更新通知，触发 UI 重新渲染
	if m.searchIntradayUpdateCh != nil {
		select {
		case m.searchIntradayUpdateCh <- struct{}{}:
			// 通知已发送
		default:
			// channel 满了，跳过（不阻塞）
		}
	}
}

// stopSearchIntradayWorker 停止搜索模式的临时 worker
func (m *Model) stopSearchIntradayWorker() {
	if m.searchIntradayWorker != nil {
		close(m.searchIntradayWorker)
		m.searchIntradayWorker = nil
		logDebug("log.search.workerClosed")
	}

	// 关闭更新通知 channel
	if m.searchIntradayUpdateCh != nil {
		close(m.searchIntradayUpdateCh)
		m.searchIntradayUpdateCh = nil
	}

	// 清理内存数据
	m.searchIntradayData = nil
	m.isSearchMode = false

	logDebug("log.search.cleanupComplete")
}

// createSearchIntradayChart 为搜索模式创建分时图表
// 与 createIntradayChart 的区别:
// 1. 数据源: m.searchIntradayData (内存) vs m.chartData (磁盘/内存)
// 2. 尺寸: 较小的嵌入式图表 vs 全屏图表
// 3. 时间轴: 简化的时间标签 vs 完整时间标签
func (m *Model) createSearchIntradayChart(termWidth, termHeight int) *linechart.Model {
	logDebug("log.search.chartCreate", termWidth, termHeight)

	if m.searchIntradayData == nil {
		logDebug("log.search.chartDataNil")
		return nil
	}

	if len(m.searchIntradayData.Datapoints) == 0 {
		logDebug("log.search.chartDataEmpty")
		return nil
	}

	logDebug("log.search.chartDataPoints", len(m.searchIntradayData.Datapoints))

	// 最小大小检查（搜索模式使用更小的最小尺寸）
	minWidth := 40
	minHeight := 8

	if termWidth < minWidth || termHeight < minHeight {
		return nil
	}

	// 计算可用空间（搜索模式使用更紧凑的布局）
	chartWidth := termWidth - 4
	if chartWidth < minWidth {
		chartWidth = minWidth
	}
	chartHeight := termHeight - 6 // 减少padding
	if chartHeight < minHeight {
		chartHeight = minHeight
	}

	// === 创建完整时间框架（根据市场配置动态生成） ===
	timeFramework := m.createFixedTimeRange(
		m.searchIntradayData.Date,
		m.searchIntradayData.Market,
	)

	if len(timeFramework) == 0 {
		logDebug("log.search.chartNoTimeFramework")
		return nil
	}

	// === 将实际数据填充到时间框架中 ===
	dataMap := make(map[string]float64)
	for _, dp := range m.searchIntradayData.Datapoints {
		dataMap[dp.Time] = dp.Price
	}

	// 填充价格值（使用最后已知价格填充空白）
	var lastKnownPrice float64
	if len(m.searchIntradayData.Datapoints) > 0 {
		lastKnownPrice = m.searchIntradayData.Datapoints[0].Price
	}

	dataPoints := make([]float64, len(timeFramework))
	timeLabels := make([]string, len(timeFramework))

	for i, tp := range timeFramework {
		timeKey := tp.Time.Format("15:04")
		timeLabels[i] = timeKey

		if price, exists := dataMap[timeKey]; exists {
			dataPoints[i] = price
			lastKnownPrice = price
		} else {
			dataPoints[i] = lastKnownPrice
		}
	}

	// === 智能计算Y轴范围 ===
	actualPrices := make([]float64, len(m.searchIntradayData.Datapoints))
	for i, dp := range m.searchIntradayData.Datapoints {
		actualPrices[i] = dp.Price
	}

	minPrice, maxPrice, margin := calculateAdaptiveMargin(actualPrices)

	logDebug("log.search.chartPriceRange", minPrice, maxPrice, margin)

	// === 设置样式：A股红涨绿跌，非A股绿涨红跌 ===
	lastPrice := m.searchIntradayData.Datapoints[len(m.searchIntradayData.Datapoints)-1].Price
	prevClose := m.searchIntradayData.PrevClose

	comparisonBase := prevClose
	if comparisonBase == 0 {
		comparisonBase = m.searchIntradayData.Datapoints[0].Price
	}

	isAShare := strings.HasPrefix(m.searchIntradayData.Code, "SH") ||
		strings.HasPrefix(m.searchIntradayData.Code, "SZ")

	var chartStyle lipgloss.Style
	if lastPrice > comparisonBase {
		if isAShare {
			chartStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("9")) // 红色
		} else {
			chartStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10")) // 绿色
		}
	} else if lastPrice < comparisonBase {
		if isAShare {
			chartStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10")) // 绿色
		} else {
			chartStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("9")) // 红色
		}
	} else {
		chartStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("15")) // 白色
	}

	// === 创建简化的 Y 轴标签格式化器 ===
	yLabelFormatter := func(index int, value float64) string {
		if value >= 100 {
			return fmt.Sprintf("%.1f", value)
		} else if value >= 10 {
			return fmt.Sprintf("%.2f", value)
		} else {
			return fmt.Sprintf("%.3f", value)
		}
	}

	// === 创建简化的 X 轴标签格式化器（搜索模式只显示开盘和收盘）===
	xLabelFormatter := func(index int, value float64) string {
		idx := int(math.Round(value))
		if idx < 0 || idx >= len(timeLabels) {
			return ""
		}

		timeLabel := timeLabels[idx]
		parts := strings.Split(timeLabel, ":")
		if len(parts) != 2 {
			return ""
		}
		hour, _ := strconv.Atoi(parts[0])
		minute, _ := strconv.Atoi(parts[1])
		totalMinutes := hour*60 + minute

		// 根据市场类型显示不同的时间标签
		switch m.searchIntradayData.Market {
		case MarketChina:
			// A股：9:30 和 15:00
			diff1 := totalMinutes - 570
			if diff1 < 0 {
				diff1 = -diff1
			}
			diff2 := totalMinutes - 900
			if diff2 < 0 {
				diff2 = -diff2
			}
			if diff1 <= 5 { // 9:30 ± 5分钟
				return "09:30"
			} else if diff2 <= 10 { // 15:00 ± 10分钟
				return "15:00"
			}
		case MarketUS:
			// 美股：9:30 和 16:00
			diff1 := totalMinutes - 570
			if diff1 < 0 {
				diff1 = -diff1
			}
			diff2 := totalMinutes - 960
			if diff2 < 0 {
				diff2 = -diff2
			}
			if diff1 <= 5 { // 9:30 ± 5分钟
				return "09:30"
			} else if diff2 <= 10 { // 16:00 ± 10分钟
				return "16:00"
			}
		case MarketHongKong:
			// 港股：9:30 和 16:00
			diff1 := totalMinutes - 570
			if diff1 < 0 {
				diff1 = -diff1
			}
			diff2 := totalMinutes - 960
			if diff2 < 0 {
				diff2 = -diff2
			}
			if diff1 <= 5 { // 9:30 ± 5分钟
				return "09:30"
			} else if diff2 <= 10 { // 16:00 ± 10分钟
				return "16:00"
			}
		}

		return ""
	}

	// === 创建图表 ===
	lc := linechart.New(chartWidth, chartHeight,
		0, float64(len(dataPoints)-1),
		minPrice-margin, maxPrice+margin,
		linechart.WithXYSteps(4, 4), // 减少刻度数量
		linechart.WithXLabelFormatter(xLabelFormatter),
		linechart.WithYLabelFormatter(yLabelFormatter),
		linechart.WithStyles(lipgloss.Style{}, lipgloss.Style{}, chartStyle),
	)

	// === 使用 Braille 字符绘制数据点 ===
	for i := 0; i < len(dataPoints)-1; i++ {
		p1 := canvas.Float64Point{X: float64(i), Y: dataPoints[i]}
		p2 := canvas.Float64Point{X: float64(i + 1), Y: dataPoints[i+1]}
		lc.DrawBrailleLineWithStyle(p1, p2, chartStyle)
	}

	lc.DrawXYAxisAndLabel()

	logDebug("log.search.chartSuccess")
	return &lc
}

// waitForSearchIntradayUpdate 监听搜索模式数据更新通知
func (m *Model) waitForSearchIntradayUpdate() tea.Cmd {
	return func() tea.Msg {
		// 阻塞等待 channel 消息
		if m.searchIntradayUpdateCh != nil {
			_, ok := <-m.searchIntradayUpdateCh
			if ok {
				// 收到更新通知，返回消息触发 UI 重新渲染
				return searchIntradayUpdateMsg{}
			}
		}
		// channel 已关闭，返回 nil
		return nil
	}
}
