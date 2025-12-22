# 分时图表可视化功能 - 实现计划

## 概述

为股票监控应用添加全面的分时图表可视化功能,包括:
- **全屏时间序列图表** - 用于详细分析(通过 'v' 键访问)
- **内联迷你走势图** - 嵌入在持仓/自选列表表格行中
- **历史日期导航** - 浏览以往日期的分时数据
- **自动触发数据采集** - 对于缺少分时数据的股票
- **最小化 UI 改动** - 遵循现有设计模式

**预计实现时间**: 总计 10-12 小时
- 阶段 1 (全屏图表): 6-7 小时
- 阶段 2 (内联走势图): 4-5 小时

---

## 架构设计

### 1. 状态机集成

**在 `consts.go` 中添加新状态:**
```go
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
    WatchlistTagging
    WatchlistTagSelect
    WatchlistTagManage
    WatchlistTagRemoveSelect
    WatchlistTagEdit
    WatchlistGroupSelect
    PortfolioSorting
    WatchlistSorting
    IntradayChartViewing  // ← 新增状态
)
```

**状态转换流程:**
```
Monitoring (按 'v') → IntradayChartViewing (按 ESC) → Monitoring
WatchlistViewing (按 'v') → IntradayChartViewing (按 ESC) → WatchlistViewing
```

**关键设计考量:** 使用 'v' 键(view chart 的首字母)避免按键冲突且便于记忆。该状态使用代码库中已建立的 `previousState` 模式,实现无缝返回导航。

---

### 2. 数据流架构

```
┌─────────────────────────────────────────────────────────────┐
│ 用户在 Monitoring/WatchlistViewing 中对股票按 'v'          │
└──────────────────┬──────────────────────────────────────────┘
                   │
                   ▼
┌─────────────────────────────────────────────────────────────┐
│ 检查: 今日分时数据是否存在?                                │
│ 路径: data/intraday/{CODE}/{YYYYMMDD}.json                 │
└──────────────┬──────────────────────────────────────────────┘
               │
        ┌──────┴──────┐
        │             │
      是│             │否
        │             │
        ▼             ▼
┌───────────────┐  ┌────────────────────────────────────┐
│ 加载 JSON     │  │ 自动触发数据采集:                  │
│ 解析数据      │  │ 1. 启动 intradayManager worker    │
│ 创建图表      │  │ 2. 显示"采集中..."界面             │
│ 渲染          │  │ 3. 轮询直到数据可用                │
└───────────────┘  │ 4. 切换到图表视图                  │
                   └────────────────────────────────────┘

                   ▼
┌─────────────────────────────────────────────────────────────┐
│ 图表视图状态:                                               │
│ - 全屏 ntcharts 时间序列图表                               │
│ - 头部: 股票名称、日期、价格区间、涨跌幅                   │
│ - 底部: 操作提示 ([←/→] 日期 | [ESC/Q] 返回)              │
│ - 颜色: 绿色(上涨)、红色(下跌)、白色(平盘)                 │
└─────────────────────────────────────────────────────────────┘
```

**数据加载策略:**
- **延迟加载**: 仅在用户按 'v' 时加载图表数据
- **缓存渲染**: 图表模型在 IntradayChartViewing 状态期间持久化
- **日期导航**: 左右箭头重新加载不同日期的文件
- **自动刷新**: 未实现 - 保持快照式方法(更简单,无竞态条件)

---

### 3. ntcharts 集成

**添加依赖 (go.mod):**
```go
require (
    github.com/NimbleMarkets/ntcharts v0.3.1
    github.com/lrstanley/bubblezone v0.0.0-20240524042110-c9cfeaa85de2
)
```

**图表组件:**

**A. 全屏图表 (timeserieslinechart)**
```go
import (
    "github.com/NimbleMarkets/ntcharts/linechart/timeserieslinechart"
    "github.com/charmbracelet/lipgloss"
)

// 创建图表 (在 Model 中)
tslc := timeserieslinechart.New(
    termWidth - 4,    // 留出边框空间
    termHeight - 10,  // 留出头部/底部空间
    timeserieslinechart.WithStyles(chartStyles),
    timeserieslinechart.WithXYSteps(12, 5), // 网格分割
)

// 推送数据点
for _, dp := range intradayData.Datapoints {
    t := parseIntradayTime(intradayData.Date, dp.Time) // 返回 time.Time
    tslc.Push(timeserieslinechart.TimePoint{Time: t, Value: dp.Price})
}

// 绘制和渲染
tslc.DrawBraille() // Braille 渲染实现平滑线条
chartView := tslc.View()
```

**B. 内联迷你走势图 (sparkline)**
```go
import "github.com/NimbleMarkets/ntcharts/sparkline"

// 为表格行创建迷你走势图
sl := sparkline.New(15, 3) // 15 字符宽, 3 行高
sl.SetStyle(miniChartStyle)

// 加载今日数据
intradayData := loadIntradayDataForStock(code, today)
for _, dp := range intradayData.Datapoints {
    sl.Push(dp.Price)
}
sl.Draw()

// 插入表格单元格
sparklineView := sl.View() // 返回紧凑字符串如 " ▁▂▃▅▇ "
```

**渲染模式:**
- Braille 模式 (`DrawBraille()`) 用于全屏: 平滑、高分辨率线条
- 线条模式 (`DrawLineColumnAndDot()`) 用于迷你图: 在紧凑空间中清晰

---

## 实现细节

### 阶段 1: 全屏时间序列图表 (6-7 小时)

#### 步骤 1.1: 添加依赖 (30 分钟)
**文件:** `go.mod`
```bash
go get github.com/NimbleMarkets/ntcharts@v0.3.1
go get github.com/lrstanley/bubblezone@latest
go mod tidy
```

#### 步骤 1.2: 扩展 Model 结构 (30 分钟)
**文件:** `main.go` (Model 结构体, 约第 100-232 行)

添加新字段:
```go
type Model struct {
    // ... 现有字段 ...

    // 用于分时图表查看
    chartViewStock      string                           // 正在查看的股票代码
    chartViewStockName  string                           // 股票名称
    chartViewDate       string                           // 正在查看的日期 (YYYYMMDD)
    chartData           *IntradayData                    // 加载的分时数据
    chartModel          *timeserieslinechart.TimeSeriesLineChart // ntcharts 模型
    chartLoadError      error                            // 加载错误(如有)
    chartIsCollecting   bool                             // 是否正在自动采集数据
    chartCollectStartTime time.Time                      // 开始采集的时间
    previousState       AppState                         // 返回目的地
}
```

#### 步骤 1.3: 创建图表加载逻辑 (1.5 小时)
**文件:** `main.go` (新增函数)

```go
// 从磁盘加载特定股票和日期的分时数据
func (m *Model) loadIntradayDataForDate(code, name, date string) (*IntradayData, error) {
    filePath := filepath.Join("data", "intraday", code, date+".json")

    fileData, err := os.ReadFile(filePath)
    if err != nil {
        return nil, fmt.Errorf("file not found: %w", err)
    }

    var data IntradayData
    if err := json.Unmarshal(fileData, &data); err != nil {
        return nil, fmt.Errorf("parse error: %w", err)
    }

    // 验证数据
    if len(data.Datapoints) == 0 {
        return nil, fmt.Errorf("no datapoints in file")
    }

    return &data, nil
}

// 解析分时时间字符串 ("09:31") + 日期 ("20251130") → time.Time
func parseIntradayTime(date string, timeStr string) time.Time {
    // date = "20251130", timeStr = "09:31"
    year, _ := strconv.Atoi(date[:4])
    month, _ := strconv.Atoi(date[4:6])
    day, _ := strconv.Atoi(date[6:8])

    parts := strings.Split(timeStr, ":")
    hour, _ := strconv.Atoi(parts[0])
    minute, _ := strconv.Atoi(parts[1])

    return time.Date(year, time.Month(month), day, hour, minute, 0, 0, time.Local)
}

// 从分时数据创建 ntcharts 时间序列图表
func (m *Model) createIntradayChart() *timeserieslinechart.TimeSeriesLineChart {
    if m.chartData == nil || len(m.chartData.Datapoints) == 0 {
        return nil
    }

    // 获取终端尺寸
    termWidth := 120  // 默认值,可动态检测
    termHeight := 30  // 默认值

    // 创建图表
    chartWidth := termWidth - 4
    chartHeight := termHeight - 10

    tslc := timeserieslinechart.New(chartWidth, chartHeight)

    // 设置样式 (涨为绿色,跌为红色)
    firstPrice := m.chartData.Datapoints[0].Price
    lastPrice := m.chartData.Datapoints[len(m.chartData.Datapoints)-1].Price

    var lineStyle lipgloss.Style
    if lastPrice > firstPrice {
        lineStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10")) // 绿色
    } else if lastPrice < firstPrice {
        lineStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))  // 红色
    } else {
        lineStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("15")) // 白色
    }

    tslc.SetStyle(lineStyle)
    tslc.SetXYSteps(12, 5) // 12 个时间分割, 5 个价格分割

    // 推送所有数据点
    for _, dp := range m.chartData.Datapoints {
        t := parseIntradayTime(m.chartData.Date, dp.Time)
        tslc.Push(timeserieslinechart.TimePoint{Time: t, Value: dp.Price})
    }

    // 自动计算 Y 轴范围
    tslc.AutoAdjustYRange()

    return tslc
}

// 如果数据不存在则触发自动采集
func (m *Model) triggerIntradayDataCollection(code, name, date string) tea.Cmd {
    m.chartIsCollecting = true
    m.chartCollectStartTime = time.Now()

    // 确保 intradayManager 存在
    if m.intradayManager == nil {
        m.intradayManager = newIntradayManager()
    }

    // 为此特定股票启动 worker
    m.intradayManager.startWorker(code, name, m)

    // 返回命令每 2 秒检查数据可用性
    return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
        return checkDataAvailabilityMsg{code: code, date: date}
    })
}

// 数据可用性检查自定义消息
type checkDataAvailabilityMsg struct {
    code string
    date string
}
```

#### 步骤 1.4: 创建图表处理器 (1.5 小时)
**文件:** `main.go` (新增处理函数)

```go
func (m *Model) handleIntradayChartViewing(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
    switch msg.String() {
    case "esc", "q":
        // 返回上一个状态
        m.state = m.previousState
        m.chartModel = nil // 释放内存
        m.chartData = nil
        return m, nil

    case "left":
        // 导航到前一天
        if m.chartData != nil {
            currentDate, _ := time.Parse("20060102", m.chartViewDate)
            previousDate := currentDate.AddDate(0, 0, -1)
            newDateStr := previousDate.Format("20060102")

            // 尝试加载前一天的数据
            data, err := m.loadIntradayDataForDate(m.chartViewStock, m.chartViewStockName, newDateStr)
            if err != nil {
                // 前一天无数据 - 可显示错误或不做任何操作
                m.chartLoadError = err
                return m, nil
            }

            // 更新到前一天
            m.chartViewDate = newDateStr
            m.chartData = data
            m.chartModel = m.createIntradayChart()
            m.chartLoadError = nil
        }
        return m, nil

    case "right":
        // 导航到下一天 (最多到今天)
        if m.chartData != nil {
            currentDate, _ := time.Parse("20060102", m.chartViewDate)
            nextDate := currentDate.AddDate(0, 0, 1)
            today := time.Now()

            // 不能超过今天
            if nextDate.After(today) {
                return m, nil
            }

            newDateStr := nextDate.Format("20060102")

            // 尝试加载下一天的数据
            data, err := m.loadIntradayDataForDate(m.chartViewStock, m.chartViewStockName, newDateStr)
            if err != nil {
                m.chartLoadError = err
                return m, nil
            }

            // 更新到下一天
            m.chartViewDate = newDateStr
            m.chartData = data
            m.chartModel = m.createIntradayChart()
            m.chartLoadError = nil
        }
        return m, nil
    }

    return m, nil
}
```

#### 步骤 1.5: 创建图表视图渲染器 (1.5 小时)
**文件:** `main.go` (新增视图函数)

```go
func (m *Model) viewIntradayChart() string {
    var b strings.Builder

    // 股票信息头部
    b.WriteString(lipgloss.NewStyle().
        Bold(true).
        Foreground(lipgloss.Color("14")). // 青色
        Render(fmt.Sprintf("📈 %s - %s (%s) - %s",
            m.getText("intradayChart"),
            m.chartViewStock,
            m.chartViewStockName,
            formatDate(m.chartViewDate))))
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

    if m.chartModel == nil || m.chartData == nil {
        b.WriteString(m.getText("noChartData"))
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

    openPrice := prices[0]
    closePrice := prices[len(prices)-1]
    change := closePrice - openPrice
    changePercent := (change / openPrice) * 100

    // 统计信息行
    statsStyle := lipgloss.NewStyle()
    if change > 0 {
        statsStyle = statsStyle.Foreground(lipgloss.Color("10")) // 绿色
    } else if change < 0 {
        statsStyle = statsStyle.Foreground(lipgloss.Color("9"))  // 红色
    }

    b.WriteString(statsStyle.Render(fmt.Sprintf(
        "%s: %.2f  %s: %.2f  %s: %.2f  %s: %.2f  %s: %+.2f (%.2f%%)",
        m.getText("open"), openPrice,
        m.getText("close"), closePrice,
        m.getText("high"), maxPrice,
        m.getText("low"), minPrice,
        m.getText("change"), change, changePercent,
    )))
    b.WriteString("\n\n")

    // 渲染图表
    m.chartModel.DrawBraille()
    b.WriteString(m.chartModel.View())
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

// 辅助函数: 格式化 YYYYMMDD → 可读日期
func formatDate(dateStr string) string {
    t, err := time.Parse("20060102", dateStr)
    if err != nil {
        return dateStr
    }
    return t.Format("2006-01-02")
}
```

#### 步骤 1.6: 连接到 Update() 和 View() (30 分钟)
**文件:** `main.go`

**在 `Update()` 方法中:**
```go
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.KeyMsg:
        switch m.state {
        // ... 现有 case ...

        case IntradayChartViewing:
            return m.handleIntradayChartViewing(msg)

        case Monitoring:
            // 添加 'v' 键绑定
            switch msg.String() {
            case "v":
                if len(m.portfolio.Stocks) > 0 {
                    selectedStock := m.portfolio.Stocks[m.portfolioCursor]
                    m.chartViewStock = selectedStock.Code
                    m.chartViewStockName = selectedStock.Name
                    m.chartViewDate = time.Now().Format("20060102")
                    m.previousState = Monitoring

                    // 尝试加载数据
                    data, err := m.loadIntradayDataForDate(
                        selectedStock.Code,
                        selectedStock.Name,
                        m.chartViewDate,
                    )

                    if err != nil {
                        // 无数据 - 触发采集
                        m.chartData = nil
                        m.chartModel = nil
                        m.chartLoadError = nil
                        m.state = IntradayChartViewing
                        return &m, m.triggerIntradayDataCollection(
                            selectedStock.Code,
                            selectedStock.Name,
                            m.chartViewDate,
                        )
                    }

                    // 数据存在 - 创建图表
                    m.chartData = data
                    m.chartModel = m.createIntradayChart()
                    m.chartLoadError = nil
                    m.chartIsCollecting = false
                    m.state = IntradayChartViewing
                }
                return &m, nil
            // ... Monitoring 的其余处理器 ...
            }

        case WatchlistViewing:
            // 添加 'v' 键绑定 (类似逻辑)
            switch msg.String() {
            case "v":
                // 类似 Monitoring, 但用于自选列表
                // ... 实现 ...
            }
        }

    case checkDataAvailabilityMsg:
        // 在自动采集期间处理数据可用性检查
        if m.state == IntradayChartViewing && m.chartIsCollecting {
            data, err := m.loadIntradayDataForDate(msg.code, m.chartViewStockName, msg.date)
            if err == nil {
                // 数据现在可用!
                m.chartData = data
                m.chartModel = m.createIntradayChart()
                m.chartIsCollecting = false
                m.chartLoadError = nil
                return &m, nil
            }

            // 仍在等待 - 2 秒后再次检查 (最多 30 秒超时)
            if time.Since(m.chartCollectStartTime) < 30*time.Second {
                return &m, tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
                    return checkDataAvailabilityMsg{code: msg.code, date: msg.date}
                })
            } else {
                // 超时 - 显示错误
                m.chartLoadError = fmt.Errorf("data collection timeout")
                m.chartIsCollecting = false
                return &m, nil
            }
        }
    }

    // ... Update 的其余逻辑 ...
}
```

**在 `View()` 方法中:**
```go
func (m Model) View() string {
    switch m.state {
    // ... 现有 case ...

    case IntradayChartViewing:
        return m.viewIntradayChart()

    // ... 其余 ...
    }
}
```

#### 步骤 1.7: 添加国际化翻译 (30 分钟)
**文件:** `i18n/zh.json`, `i18n/en.json`

**中文 (`i18n/zh.json`):**
```json
{
  "intradayChart": "分时图表",
  "collectingData": "正在采集数据",
  "pleaseWait": "请稍候,首次采集可能需要1-2分钟...",
  "loadError": "加载失败",
  "noDataAvailable": "暂无分时数据",
  "noChartData": "无图表数据",
  "open": "开盘",
  "close": "收盘",
  "high": "最高",
  "low": "最低",
  "change": "涨跌",
  "changeDate": "切换日期",
  "back": "返回"
}
```

**英文 (`i18n/en.json`):**
```json
{
  "intradayChart": "Intraday Chart",
  "collectingData": "Collecting data",
  "pleaseWait": "Please wait, initial collection may take 1-2 minutes...",
  "loadError": "Load Error",
  "noDataAvailable": "No intraday data available",
  "noChartData": "No chart data",
  "open": "Open",
  "close": "Close",
  "high": "High",
  "low": "Low",
  "change": "Change",
  "changeDate": "Change Date",
  "back": "Back"
}
```

#### 步骤 1.8: 测试与优化 (1 小时)
- 测试有数据的股票
- 测试缺少数据的股票(自动采集流程)
- 测试日期导航(左右箭头)
- 测试返回上一状态
- 验证颜色是否匹配涨跌
- 检查终端调整大小处理

---

### 阶段 2: 内联迷你走势图 (4-5 小时)

#### 步骤 2.1: 添加走势图辅助函数 (1 小时)
**文件:** `main.go` (新增函数)

```go
// 为表格显示创建迷你走势图
func (m *Model) createSparklineForStock(code string) string {
    // 加载今日数据
    today := time.Now().Format("20060102")
    data, err := m.loadIntradayDataForDate(code, "", today)
    if err != nil || len(data.Datapoints) == 0 {
        // 无数据 - 返回占位符
        return strings.Repeat("─", 12) // "────────────"
    }

    // 创建走势图
    sl := sparkline.New(12, 1) // 12 字符宽, 1 行高

    // 根据涨跌确定颜色
    firstPrice := data.Datapoints[0].Price
    lastPrice := data.Datapoints[len(data.Datapoints)-1].Price

    var sparkStyle lipgloss.Style
    if lastPrice > firstPrice {
        sparkStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10")) // 绿色
    } else if lastPrice < firstPrice {
        sparkStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))  // 红色
    } else {
        sparkStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("15")) // 白色
    }

    sl.SetStyle(sparkStyle)

    // 如果数据点过多则采样(限制为约 24 个点以保证可见性)
    step := len(data.Datapoints) / 24
    if step < 1 {
        step = 1
    }

    for i := 0; i < len(data.Datapoints); i += step {
        sl.Push(data.Datapoints[i].Price)
    }

    sl.Draw()
    return sl.View()
}
```

#### 步骤 2.2: 修改持仓列表表格视图 (1.5 小时)
**文件:** `main.go` (`viewMonitoring()` 函数, 约第 967-1165 行)

更新表格以包含走势图列:

```go
func (m *Model) viewMonitoring() string {
    // ... 现有设置 ...

    // 创建带新列的表格
    t := table.NewWriter()
    t.SetStyle(tableStyle)

    // 添加带走势图列的表头
    header := table.Row{
        "",
        m.getText("code"),
        m.getText("name"),
        m.getText("prevClose"),
        m.getText("startPrice"),
        m.getText("maxPrice"),
        m.getText("minPrice"),
        m.getText("price"),
        m.getText("costPrice"),
        m.getText("quantity"),
        m.getText("todayChangePercent"),
        m.getText("totalProfit"),
        m.getText("profitRate"),
        m.getText("marketValue"),
        m.getText("trend"), // ← 新增: 走势图列
    }
    t.AppendHeader(header)

    // 添加带走势图的行
    for i, stock := range displayStocks {
        // ... 现有股票数据获取 ...

        // 生成走势图
        sparklineView := m.createSparklineForStock(stock.Code)

        // 光标指示器
        cursor := " "
        if i == m.portfolioCursor {
            cursor = "▶"
        }

        row := table.Row{
            cursor,
            stock.Code,
            stock.Name,
            formatPrice(prevClose, m.config.Display.DecimalPlaces),
            formatPrice(stockData.StartPrice, m.config.Display.DecimalPlaces),
            formatPrice(stockData.MaxPrice, m.config.Display.DecimalPlaces),
            formatPrice(stockData.MinPrice, m.config.Display.DecimalPlaces),
            formatPrice(stockData.Price, m.config.Display.DecimalPlaces),
            formatPrice(stock.CostPrice, m.config.Display.DecimalPlaces),
            fmt.Sprintf("%d", stock.Quantity),
            formatChangePercent(changePercent),
            formatProfit(totalProfit),
            formatChangePercent(profitRate),
            formatPrice(marketValue, m.config.Display.DecimalPlaces),
            sparklineView, // ← 新增: 走势图单元格
        }

        // 应用行颜色
        t.AppendRow(row, table.RowConfig{
            AutoMerge: false,
        })

        // ... 颜色格式化 ...
    }

    // ... 函数其余部分 ...
}
```

#### 步骤 2.3: 更新自选列表视图 (1 小时)
**文件:** `main.go` (`viewWatchlist()` 函数)

类似修改,为自选列表表格添加走势图列。

#### 步骤 2.4: 添加国际化键 (15 分钟)
**文件:** `i18n/zh.json`, `i18n/en.json`

```json
{
  "trend": "走势" // 中文
}
```

```json
{
  "trend": "Trend" // 英文
}
```

#### 步骤 2.5: 性能优化 (45 分钟)
走势图生成如果每次渲染都执行可能开销较大。实现缓存:

```go
type Model struct {
    // ... 现有字段 ...

    // 走势图缓存
    sparklineCache       map[string]string // code → sparkline view
    sparklineCacheTime   time.Time         // 上次缓存更新时间
    sparklineCacheMutex  sync.RWMutex      // 线程安全
}

func (m *Model) createSparklineForStockCached(code string) string {
    // 检查缓存 (5 秒 TTL)
    m.sparklineCacheMutex.RLock()
    if time.Since(m.sparklineCacheTime) < 5*time.Second {
        if cached, exists := m.sparklineCache[code]; exists {
            m.sparklineCacheMutex.RUnlock()
            return cached
        }
    }
    m.sparklineCacheMutex.RUnlock()

    // 生成新走势图
    sparkline := m.createSparklineForStock(code)

    // 更新缓存
    m.sparklineCacheMutex.Lock()
    if m.sparklineCache == nil {
        m.sparklineCache = make(map[string]string)
    }
    m.sparklineCache[code] = sparkline
    m.sparklineCacheTime = time.Now()
    m.sparklineCacheMutex.Unlock()

    return sparkline
}
```

---

## 边界情况与错误处理

### 1. 缺少分时数据文件
**场景:** 用户按 'v' 但今日没有 JSON 文件。

**解决方案:**
- 在 `loadIntradayDataForDate()` 中检测文件不存在
- 通过 `triggerIntradayDataCollection()` 触发自动采集
- 显示"采集数据中..."界面,显示已用时间
- 每 2 秒轮询文件创建(最多 30 秒超时)
- 如果超时,显示错误消息及说明

**代码位置:** `handleIntradayChartViewing()`, `viewIntradayChart()`

---

### 2. 交易时段间隔 (11:30-13:00 午休)
**场景:** 图表在上午 11:30 到下午 1:00 之间没有数据。

**解决方案:**
- ntcharts 自动处理时间序列数据中的间隔
- 时间序列图表自然显示不连续性(线条断开)
- 无需特殊处理 - 数据结构已排除午休时段

**视觉效果:**
```
价格
  ^
  |    /\  /\        (此处间隔)       /\  /\
  |   /  \/  \                       /  \/  \
  +----------------------------------------> 时间
    09:30  11:30   13:00            15:00
```

---

### 3. 不同股票市场 (A股 vs 美股 vs 港股)
**场景:** 美股的交易时间与A股不同。

**当前状态:**
- 分时采集当前硬编码为A股交易时间 (09:30-15:00)
- 美股/港股可能没有数据或采集时间不正确

**解决方案 (阶段 1 - 最小化):**
- 显示 JSON 文件中存在的任何数据
- 图表可正确工作,无论市场时间如何
- 如果没有数据,用户会看到相应错误

**未来增强 (不在当前范围内):**
- 从股票代码前缀检测市场类型
- 在 `intraday.go` 的 `isMarketOpen()` 函数中调整采集时间
- 将市场时间添加到配置文件

---

### 4. 空的或损坏的 JSON 文件
**场景:** JSON 文件存在但为空、格式错误或数据点为零。

**解决方案:**
```go
func (m *Model) loadIntradayDataForDate(code, name, date string) (*IntradayData, error) {
    // ... 现有文件读取 ...

    // 解析后验证
    if len(data.Datapoints) == 0 {
        return nil, fmt.Errorf("no datapoints in file")
    }

    // 检查格式错误的数据
    for i, dp := range data.Datapoints {
        if dp.Time == "" || dp.Price == 0 {
            return nil, fmt.Errorf("invalid datapoint at index %d", i)
        }
    }

    return &data, nil
}
```

**用户体验:**
- 在图表视图中显示错误消息
- 可返回上一状态的选项 (ESC/Q)
- 可选择性触发重新采集

---

### 5. 终端大小限制
**场景:** 终端太小无法渲染完整图表。

**解决方案:**
```go
func (m *Model) createIntradayChart() *timeserieslinechart.TimeSeriesLineChart {
    // 获取实际终端大小 (来自 Bubble Tea)
    termWidth, termHeight := m.terminalSize.Width, m.terminalSize.Height

    // 最小大小检查
    minWidth := 40
    minHeight := 15

    if termWidth < minWidth || termHeight < minHeight {
        // 返回 nil - viewIntradayChart() 将显示大小错误
        return nil
    }

    // 计算可用空间
    chartWidth := max(termWidth-4, minWidth)
    chartHeight := max(termHeight-10, minHeight)

    // ... 图表创建的其余部分 ...
}
```

**视图处理:**
```go
func (m *Model) viewIntradayChart() string {
    if m.chartModel == nil {
        return fmt.Sprintf(
            "%s\n\n%s",
            m.getText("terminalTooSmall"),
            m.getText("pleaseResize"),
        )
    }
    // ... 正常渲染 ...
}
```

---

### 6. 大型持仓列表的走势图性能
**场景:** 用户持仓中有 50+ 只股票,每次渲染生成 50 个走势图。

**解决方案:**
- 实现 5 秒 TTL 缓存(见步骤 2.5)
- 仅可见股票需要走势图(分页已限制为 `max_lines`)
- 延迟生成: 仅在实际渲染行时创建走势图

**内存影响:**
- 每个走势图: 约 100 字节
- 50 只股票: 总计约 5 KB(可忽略不计)

---

## 国际化

### 所需的新 i18n 键

**`i18n/zh.json` 和 `i18n/en.json` 的完整列表:**

```json
{
  "intradayChart": "分时图表" / "Intraday Chart",
  "collectingData": "正在采集数据" / "Collecting data",
  "pleaseWait": "请稍候,首次采集可能需要1-2分钟..." / "Please wait, initial collection may take 1-2 minutes...",
  "loadError": "加载失败" / "Load Error",
  "noDataAvailable": "暂无分时数据" / "No intraday data available",
  "noChartData": "无图表数据" / "No chart data",
  "open": "开盘" / "Open",
  "close": "收盘" / "Close",
  "high": "最高" / "High",
  "low": "最低" / "Low",
  "change": "涨跌" / "Change",
  "changeDate": "切换日期" / "Change Date",
  "back": "返回" / "Back",
  "trend": "走势" / "Trend",
  "terminalTooSmall": "终端窗口太小" / "Terminal window too small",
  "pleaseResize": "请调整窗口大小至至少 80x25" / "Please resize to at least 80x25"
}
```

---

## 文件修改摘要

### 需要修改的关键文件

| 文件 | 修改内容 | 新增行数 | 用途 |
|------|---------|---------|------|
| **go.mod** | 添加 ntcharts + bubblezone 依赖 | 约 2 行 | 启用图表功能 |
| **consts.go** | 添加 `IntradayChartViewing` 状态 | 约 1 行 | 状态机 |
| **main.go** | 核心实现 | 约 600 行 | 所有图表逻辑 |
| **i18n/zh.json** | 添加中文翻译 | 约 16 个键 | 本地化 |
| **i18n/en.json** | 添加英文翻译 | 约 16 个键 | 本地化 |

### 只读文件(无需修改)

| 文件 | 用途 |
|------|------|
| **intraday.go** | 参考数据结构,理解采集流程 |
| **color.go** | 颜色方案一致性 |
| **sort.go** | 表格渲染模式 |

---

## 测试策略

### 手动测试用例

#### 阶段 1 测试(全屏图表)

**测试 1: 正常路径 - 有数据的股票**
1. 启动应用,进入 Monitoring 状态
2. 选择有现存分时数据的股票
3. 按 'v'
4. ✅ 图表正确显示,包含价格范围、颜色、网格
5. 按 ESC
6. ✅ 返回 Monitoring 状态

**测试 2: 自动采集 - 无数据的股票**
1. 删除某股票的分时 JSON 文件
2. 选择该股票并按 'v'
3. ✅ 看到"采集数据中..."界面
4. 等待 30-60 秒
5. ✅ 数据采集后显示图表,或超时后显示错误

**测试 3: 日期导航**
1. 打开有多日数据的股票图表
2. 按 ← (左箭头)
3. ✅ 图表显示前一天的数据
4. 按 → (右箭头)
5. ✅ 图表返回当前日期
6. 多次按 →
7. ✅ 无法超过今天

**测试 4: 颜色编码**
1. 打开有涨幅的股票图表(收盘 > 开盘)
2. ✅ 线条为绿色
3. 打开有跌幅的股票图表(收盘 < 开盘)
4. ✅ 线条为红色

**测试 5: 错误处理**
1. 破坏一个 JSON 文件(无效的 JSON 语法)
2. 对该股票按 'v'
3. ✅ 显示错误消息
4. 按 ESC
5. ✅ 返回上一状态

**测试 6: 终端调整大小**
1. 打开图表
2. 将终端调整为很小 (40x15)
3. ✅ 图表调整或显示大小错误
4. 调整回正常大小
5. ✅ 图表正确渲染

**测试 7: 国际化**
1. 切换语言为英文
2. 打开图表
3. ✅ 所有标签为英文
4. 切换为中文
5. ✅ 所有标签为中文

#### 阶段 2 测试(走势图)

**测试 8: 走势图显示**
1. 进入 Monitoring 状态
2. ✅ 在表格中看到走势图列
3. ✅ 走势图显示迷你趋势
4. ✅ 颜色匹配涨跌(绿色/红色)

**测试 9: 走势图占位符**
1. 添加没有分时数据的新股票
2. ✅ 走势图显示占位符(如 "────────────")

**测试 10: 走势图缓存**
1. 观察表格每 5 秒自动刷新
2. ✅ 走势图平滑更新无延迟
3. 监控内存使用
4. ✅ 随时间推移无内存泄漏

---

### 性能基准

**预期性能:**
- 图表创建: 240 个数据点 < 100ms
- 图表渲染: < 50ms (Braille 模式)
- 走势图生成: 每只股票 < 10ms
- 走势图缓存命中: < 1ms
- 内存占用: 每个图表约 4KB,每个走势图约 100 字节

**可接受阈值:**
- 全屏图表加载时间: < 500ms
- 含 20 个走势图的表格渲染: < 1 秒
- 50 只股票的内存使用: 总计 < 10 MB

---

## 未来可扩展性

### 阶段 3 增强功能(不在当前范围内)

#### 1. 多日历史图表
**概念:** 查看更长的时间范围(周、月、年)。

**实现:**
- 加载多个 JSON 文件并连接数据点
- 添加缩放级别选择器 (1D, 5D, 1M, 3M, 1Y)
- 对更长范围的数据进行降采样(例如,小时级而非分钟级)

**工作量:** 4-6 小时

---

#### 2. 成交量数据集成
**概念:** 在价格图表下方显示成交量柱状图(标准 OHLC+V 图表)。

**当前阻碍:** 分时采集当前不存储成交量。

**实现:**
- 更新 `IntradayDataPoint` 结构: 添加 `Volume int64` 字段
- 修改 `intraday.go` 中的 API 获取以提取成交量
- 在折线图下方使用 ntcharts 柱状图(垂直柱)

**工作量:** 6-8 小时(包括数据采集修改)

---

#### 3. 技术指标
**概念:** 叠加移动平均线、布林带、RSI 等。

**实现:**
- 从数据点计算指标(简单 MA: 滚动平均)
- 在 ntcharts 中创建第二条线系列(叠加在同一图表上)
- 添加切换控制(1-9 键用于不同指标)

**MA(20) 示例:**
```go
ma20Values := calculateMovingAverage(prices, 20)
tslc.PushSeries("MA20", ma20TimePoints, maLineStyle)
```

**工作量:** 8-10 小时(需要指标计算库或自定义实现)

---

#### 4. 导出图表数据到 CSV
**概念:** 将可见图表数据保存到文件以供外部分析。

**实现:**
```go
func (m *Model) exportChartToCSV(filename string) error {
    f, _ := os.Create(filename)
    defer f.Close()

    writer := csv.NewWriter(f)
    defer writer.Flush()

    writer.Write([]string{"Time", "Price"})
    for _, dp := range m.chartData.Datapoints {
        writer.Write([]string{dp.Time, fmt.Sprintf("%.2f", dp.Price)})
    }
    return nil
}
```

**UI:**
- 在图表视图中按 'e' 导出
- 保存到 `data/exports/{CODE}_{DATE}.csv`

**工作量:** 2-3 小时

---

#### 5. 对比图表(多只股票)
**概念:** 在同一图表上叠加 2-3 只股票进行对比。

**实现:**
- 允许选择多只股票(复选框模式)
- 加载所有选中股票的分时数据
- 将价格标准化为百分比基准(相对于开盘 = 0%)
- 为每条线使用不同的颜色/样式

**挑战:**
- 不同的价格尺度(解决方案: 标准化)
- UI 复杂性(股票选择 UX)

**工作量:** 10-12 小时

---

## 已知限制与权衡

### 1. 基于快照,非实时
**决策:** 图表显示时间点视图,非实时更新。

**理由:**
- 实现更简单(无 Bubble Tea 命令复杂性)
- 避免图表模型变更的竞态条件
- 用户可使用 'r' 键手动刷新(MVP 中未实现)

**权衡:** UX 稍欠动态,但更可靠。

---

### 2. 数据采集依赖
**决策:** 图表依赖现有分时采集系统。

**理由:**
- 重用已验证的基础设施
- 无重复 API 调用
- 与应用架构一致

**权衡:** 仅当 intraday.go 采集正常工作时图表才能工作。

---

### 3. MVP 中无缩放/平移
**决策:** 阶段 1 不实现缩放/平移控制。

**理由:**
- ntcharts 支持缩放/平移但需要鼠标或复杂的键处理
- 240 个数据点在全屏图表中无需缩放即可良好呈现
- 如需要可在阶段 3 添加

**权衡:** 对于非常密集的数据探索有限。

---

### 4. 仅支持 A 股交易时间
**决策:** 硬编码为 A 股交易时间 (09:30-15:00)。

**理由:**
- 主要用例是 A 股
- 美股/港股交易时间支持需要对 `intraday.go` 进行重大重构
- 超出图表功能范围

**权衡:** 在增强之前对美股/港股的实用性有限。

---

## 总结清单

### 阶段 1: 全屏图表 (MVP)
- [ ] 在 go.mod 中添加 ntcharts 依赖
- [ ] 在 consts.go 中添加 `IntradayChartViewing` 状态
- [ ] 添加图表状态的 Model 字段
- [ ] 实现 `loadIntradayDataForDate()`
- [ ] 实现 `parseIntradayTime()`
- [ ] 实现 `createIntradayChart()`
- [ ] 实现 `triggerIntradayDataCollection()`
- [ ] 实现 `handleIntradayChartViewing()`
- [ ] 实现 `viewIntradayChart()`
- [ ] 在 Monitoring 状态中连接 'v' 键绑定
- [ ] 在 WatchlistViewing 状态中连接 'v' 键绑定
- [ ] 添加 checkDataAvailabilityMsg 处理器
- [ ] 添加 i18n 翻译 (zh.json, en.json)
- [ ] 测试: 有数据的股票
- [ ] 测试: 无数据的股票(自动采集)
- [ ] 测试: 日期导航 (← →)
- [ ] 测试: 颜色编码 (绿色/红色)
- [ ] 测试: 错误处理 (缺少/损坏的文件)

### 阶段 2: 内联走势图
- [ ] 实现 `createSparklineForStock()`
- [ ] 实现走势图缓存
- [ ] 修改 `viewMonitoring()` 添加走势图列
- [ ] 修改 `viewWatchlist()` 添加走势图列
- [ ] 添加 "trend" i18n 键
- [ ] 测试: 表格中的走势图显示
- [ ] 测试: 缺少数据的走势图占位符
- [ ] 测试: 缓存性能
- [ ] 测试: 大型持仓列表的内存使用

---

## 结论

本实现计划为股票监控应用添加专业级分时图表可视化功能提供了完整的路线图。该方法:

✅ **遵循现有模式:** 状态机、Bubble Tea 惯用法、i18n 系统
✅ **最小侵入式变更:** 仅 1 个新状态,main.go 中约 600 行
✅ **利用现有基础设施:** 分时数据采集、颜色工具
✅ **处理边界情况:** 缺少数据、错误、自动采集、市场间隔
✅ **可扩展:** 明确的未来增强路径(成交量、指标、导出)
✅ **用户友好:** 'v' 键访问、日期导航、带反馈的自动采集

**总实现时间:** 10-12 小时
- 阶段 1 (全屏图表): 6-7 小时 ← **优先**
- 阶段 2 (内联走势图): 4-5 小时 ← **可选增强**

分阶段方法允许在保持代码质量和与现有应用用户体验一致性的同时递增地提供价值。
