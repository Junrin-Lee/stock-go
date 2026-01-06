# 搜索模块分时图表集成实现方案

**版本**: v2.0  
**日期**: 2025-12-22  
**状态**: Plan (Under Review)  
**优先级**: P1 - 高优先级

---

## 目录

- [需求概述](#需求概述)
- [问题分析](#问题分析)
- [设计方案](#设计方案)
  - [架构设计](#架构设计)
  - [数据流程](#数据流程)
  - [状态管理](#状态管理)
- [实现步骤](#实现步骤)
- [技术细节](#技术细节)
- [测试计划](#测试计划)
- [风险评估](#风险评估)
- [后续优化](#后续优化)

---

## 需求概述

### 业务需求

用户在搜索股票时，希望能够：

1. **即时查看图表**：搜索成功后，**自动在结果页展示分时图表**，无需额外操作
2. **实时更新**：图表数据每5秒自动刷新，展示动态变化趋势
3. **无负担退出**：离开搜索模块时自动释放临时数据，不污染持久化存储

### 用户体验优化

**现有流程**（不友好）：
```
搜索股票 → 查看数据 → 添加到自选 → 进入自选列表 → 按V查看图表
```

**期望流程**（友好）：
```
搜索股票 → 查看数据 + 实时分时图（自动展示）→ 图表每5秒自动刷新
```

**关键改进**：
- ❌ **移除** "按V键才展示图表"的交互
- ✅ **自动** 在搜索结果页同时展示基础数据和分时图
- ✅ **实时** 每5秒刷新图表数据，类似股票软件的实时行情

---

## 问题分析

### 现有架构限制

#### 1. **搜索结果页缺少图表区域**
```go
// 当前逻辑: 只显示表格数据，无图表渲染
func (m *Model) viewSearchResultWithActions() string {
    // 显示股票代码、名称、价格、涨跌幅等
    t := table.NewWriter()
    // ...
    return s // 只有表格，无图表
}
```

**问题**: UI布局未预留图表展示区域

#### 2. **分时数据采集时机不合理**
```go
// 当前逻辑: 只在 Monitoring/WatchlistViewing 状态启动 worker
func (m *Model) startIntradayDataCollection() {
    if m.state == Monitoring {
        // 采集持股列表所有股票
    } else if m.state == WatchlistViewing {
        // 采集自选列表所有股票
    }
    // 搜索状态未处理
}
```

**问题**: 
- 搜索状态 (`SearchResultWithActions`) 不在采集范围内
- 采集间隔1分钟，无法满足5秒实时刷新需求

#### 3. **数据持久化默认行为**
```go
// intraday.go - 所有采集的数据都会保存到磁盘
func (im *IntradayManager) fetchAndSaveIntradayData(...) error {
    // ... 采集数据
    return saveIntradayData(filePath, existingData) // ← 总是保存
}
```

**问题**: 搜索产生的临时数据也会被持久化

---

## 设计方案

### 架构设计

#### 核心策略：**自动渲染 + 高频刷新 + 内存存储**

```
┌─────────────────────────────────────────────────────────────────┐
│              搜索模块分时图表实时集成架构                          │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  用户搜索 "600000"                                               │
│       ↓                                                         │
│  ┌────────────────────────────────────────────────┐             │
│  │ SearchResultWithActions                        │             │
│  │ ┌──────────────┐  ┌─────────────────────────┐ │             │
│  │ │ 基础数据表格  │  │   分时图表（自动展示）   │ │             │
│  │ │ - 股票代码   │  │   ┌─────────────────┐  │ │             │
│  │ │ - 现价      │  │   │   📈 实时曲线    │  │ │             │
│  │ │ - 涨跌幅    │  │   │   (5秒刷新)     │  │ │             │
│  │ │ - 换手率    │  │   └─────────────────┘  │ │             │
│  │ └──────────────┘  └─────────────────────────┘ │             │
│  └────────────────────────────────────────────────┘             │
│                         ↑                                       │
│                         │ 5秒刷新                                │
│                         │                                       │
│  ┌──────────────────────────────────────────┐                  │
│  │  临时 Worker (5秒间隔)                    │                  │
│  │  - API获取最新分时数据                     │                  │
│  │  - 更新内存: m.searchIntradayData         │                  │
│  │  - 触发UI重渲染                           │                  │
│  │  - 退出时立即停止 + 清理                   │                  │
│  └──────────────────────────────────────────┘                  │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

#### 关键设计点

1. **自动展示**: 搜索成功后立即在结果页右侧/底部展示分时图
2. **高频刷新**: Worker每5秒调用一次API，刷新内存数据
3. **内存存储**: 数据仅保存在 `m.searchIntradayData`，不写磁盘
4. **自动清理**: 离开搜索结果页时停止worker并释放内存

---

### 数据流程

#### 1. **搜索 → 自动展示图表流程**

```
用户操作:  输入 "600000" → 按 Enter 搜索
           ↓
Step 1:   调用 getStockInfo("600000") 获取基础数据
           - 返回: StockData (现价、涨跌幅、成交量等)
           ↓
Step 2:   进入 SearchResultWithActions 状态
           - 渲染基础数据表格（左侧）
           - 同时启动临时 Worker（自动）
           ↓
Step 3:   临时 Worker 首次执行（立即）
           - 调用 fetchIntradayDataFromAPI("600000")
           - 获取今日分时数据（9:30 至当前时间）
           - 存储到 m.searchIntradayData (内存)
           - 触发 UI 重渲染
           ↓
Step 4:   UI 渲染分时图表（右侧/底部）
           - 从 m.searchIntradayData 读取数据
           - 调用 createIntradayChart() 生成图表
           - 显示在搜索结果页
           ↓
Step 5:   Worker 定时循环（每5秒）
           - 再次调用 API 获取最新数据
           - 更新 m.searchIntradayData
           - 自动触发 UI 重渲染
           - 图表实时更新（类似股票软件）
           ↓
Step 6:   用户查看实时变化的图表
           - 图表曲线每5秒延伸
           - 数据点不断增加
```

**关键点**：
- ✅ **无需按键**：搜索成功后自动展示图表
- ✅ **首次立即**：Worker启动后立即获取数据，不等待5秒
- ✅ **高频更新**：5秒间隔，接近实时行情体验
- ✅ **纯内存**：整个过程不涉及磁盘I/O

#### 2. **退出清理流程**

```
用户操作:  在搜索结果页按 ESC 或切换到其他模块
           ↓
Step 1:   检测状态切换
           - 从 SearchResultWithActions 离开
           ↓
Step 2:   停止临时 Worker
           - 发送停止信号: close(m.searchIntradayWorker)
           - Worker goroutine 退出
           ↓
Step 3:   清理内存数据
           - m.searchIntradayData = nil
           - m.isSearchMode = false
           - m.searchIntradayWorker = nil
           ↓
Step 4:   状态切换完成
           - 返回 MainMenu 或进入其他状态
```

---

### 状态管理

#### 新增 Model 字段

```go
// types.go - Model 结构体新增字段

type Model struct {
    // ... 现有字段 ...
    
    // For search mode intraday (搜索模式临时分时数据)
    isSearchMode         bool          // 是否处于搜索模式（用于区分数据来源）
    searchIntradayData   *IntradayData // 搜索模式的临时分时数据(仅内存)
    searchIntradayWorker chan struct{} // 临时 worker 停止信号
    searchChartWidth     int           // 搜索图表宽度（响应式布局）
    searchChartHeight    int           // 搜索图表高度
}
```

#### 状态转换图

```
                    ┌───────────────────┐
                    │   MainMenu        │
                    └─────────┬─────────┘
                              │ 选择搜索
                              ↓
                    ┌───────────────────┐
                    │ SearchingStock    │
                    │ (输入股票代码)     │
                    └─────────┬─────────┘
                              │ Enter 搜索
                              ↓
                    ┌───────────────────────┐
                    │ SearchResultWithActions│
                    │ ┌───────────────────┐ │
                    │ │ 基础数据 + 分时图  │ │
                    │ │ (自动展示)        │ │
                    │ │ Worker: 5秒刷新   │ │
                    │ └───────────────────┘ │
                    └─────────┬───────┬─────┘
                              │       │
                 ┌────────────┘       └────────┐
                 │ ESC 返回主菜单             │ 按 1 添加到自选
                 ↓                            ↓
    ┌────────────────────┐      ┌────────────────────┐
    │ MainMenu           │      │ WatchlistViewing   │
    │ (Worker已停止)      │      │ (Worker已停止)      │
    │ (数据已清理)        │      │ (数据已清理)        │
    └────────────────────┘      └────────────────────┘
```

**关键变化**：
- ✅ 进入 `SearchResultWithActions` 时立即启动 Worker
- ✅ 离开 `SearchResultWithActions` 时立即停止 Worker
- ✅ 整个过程无需进入 `IntradayChartViewing` 状态（图表嵌入结果页）

---

## 实现步骤

### Step 1: 扩展数据结构 (types.go)

**文件**: `types.go`  
**修改位置**: Model 结构体

```go
type Model struct {
    // ... 现有字段 ...
    
    // For intraday chart viewing - 分时图表查看
    chartViewStock        string        // 正在查看的股票代码
    chartViewStockName    string        // 股票名称
    chartViewDate         string        // 正在查看的日期 (YYYYMMDD)
    chartData             *IntradayData // 加载的分时数据
    chartLoadError        error         // 加载错误(如有)
    chartIsCollecting     bool          // 是否正在自动采集数据
    chartCollectStartTime time.Time     // 开始采集的时间
    
    // NEW: For search mode intraday (搜索模式临时分时数据)
    isSearchMode         bool          // 是否处于搜索模式
    searchIntradayData   *IntradayData // 搜索模式的临时分时数据(仅内存)
    searchIntradayWorker chan struct{} // 临时 worker 停止信号
    searchChartWidth     int           // 搜索图表宽度（自适应终端）
    searchChartHeight    int           // 搜索图表高度
}
```

**代码变更量**: ~7 行

---

### Step 2: 搜索成功后自动启动 Worker (main.go)

**文件**: `main.go`  
**修改位置**: `handleSearchingStock()` 函数

```go
func (m *Model) handleSearchingStock(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
    switch msg.String() {
    case "esc":
        // ... 现有逻辑 ...
        
    case "enter":
        if m.searchInput == "" {
            m.message = m.getText("enterSearch")[:len(m.getText("enterSearch"))-2]
            return m, nil
        }
        
        m.logUserAction(fmt.Sprintf("搜索股票: %s", m.searchInput))
        m.message = m.getText("searching")
        m.searchResult = getStockInfo(m.searchInput)
        
        if m.searchResult == nil || m.searchResult.Name == "" {
            m.logUserAction(fmt.Sprintf("搜索失败: %s", m.searchInput))
            m.message = fmt.Sprintf(m.getText("searchNotFound"), m.searchInput)
            return m, nil
        }
        
        m.logUserAction(fmt.Sprintf("搜索成功: %s (%s)", m.searchResult.Name, m.searchResult.Symbol))
        
        // NEW: 标记为搜索模式
        m.isSearchMode = true
        
        // NEW: 获取智能日期（当日或最近交易日）
        actualDate, _, err := GetTradingDayForCollection(m.searchResult.Symbol, m)
        if err != nil {
            // 降级为简单逻辑
            actualDate = getSmartChartDate()
        }
        
        // NEW: 设置图表参数
        m.chartViewStock = m.searchResult.Symbol
        m.chartViewStockName = m.searchResult.Name
        m.chartViewDate = actualDate
        
        // 根据来源决定下一个状态
        if m.searchFromWatchlist {
            m.state = WatchlistSearchConfirm
        } else {
            m.state = SearchResultWithActions
            
            // NEW: 自动启动临时 Worker
            return m, m.startSearchIntradayWorker(
                m.searchResult.Symbol,
                m.searchResult.Name,
                actualDate,
            )
        }
        
        m.searchInput = ""
        m.searchInputCursor = 0
        m.message = ""
        return m, nil
        
    // ... 其他按键处理 ...
    }
    return m, nil
}
```

**代码变更量**: ~25 行

---

### Step 3: 实现搜索模式的高频 Worker (intraday_chart.go)

**文件**: `intraday_chart.go`  
**新增函数**: 高频临时 Worker

```go
// startSearchIntradayWorker 为搜索模式启动高频临时数据采集
// 特点：
// 1. 5秒刷新间隔（高频）
// 2. 只采集单只股票
// 3. 数据存储在内存 (m.searchIntradayData)
// 4. 不写入磁盘
// 5. 首次立即执行
func (m *Model) startSearchIntradayWorker(code, name, date string) tea.Cmd {
    // 创建停止信号
    m.searchIntradayWorker = make(chan struct{})
    
    debugPrint("debug.search.workerStart", code, date)
    
    // 启动临时 goroutine
    go m.runSearchIntradayWorker(code, name, date)
    
    // 立即返回，不阻塞 UI
    return nil
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
                debugPrint("debug.search.workerAutoStop", code)
                return
            }
            
            // 检查市场是否开市（闭市时降低频率）
            if !isMarketOpen(code, m) {
                debugPrint("debug.search.marketClosed", code)
                // 市场关闭时仍然执行一次获取（获取当日完整数据）
                // 然后停止 worker
                m.fetchAndStoreSearchIntradayData(code, name, date)
                return
            }
            
            // 采集数据并更新内存
            m.fetchAndStoreSearchIntradayData(code, name, date)
            
        case <-m.searchIntradayWorker:
            // 收到停止信号
            debugPrint("debug.search.workerStop", code)
            return
        }
    }
}

// fetchAndStoreSearchIntradayData 获取并存储搜索模式的分时数据（仅内存）
func (m *Model) fetchAndStoreSearchIntradayData(code, name, date string) {
    // 从 API 获取最新数据
    datapoints, err := fetchIntradayDataFromAPI(code)
    if err != nil {
        debugPrint("debug.search.fetchFail", code, err)
        // 不返回错误，继续下次尝试
        return
    }
    
    if len(datapoints) == 0 {
        debugPrint("debug.search.noData", code)
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
    
    debugPrint("debug.search.dataUpdated", code, len(datapoints), time.Now().Format("15:04:05"))
}

// stopSearchIntradayWorker 停止搜索模式的临时 worker
func (m *Model) stopSearchIntradayWorker() {
    if m.searchIntradayWorker != nil {
        close(m.searchIntradayWorker)
        m.searchIntradayWorker = nil
        debugPrint("debug.search.workerClosed")
    }
    
    // 清理内存数据
    m.searchIntradayData = nil
    m.isSearchMode = false
    
    debugPrint("debug.search.cleanupComplete")
}
```

**代码变更量**: ~95 行

**关键改进**：
- ✅ **5秒间隔**：从1分钟改为5秒，实现准实时更新
- ✅ **首次立即执行**：不等待第一个tick，搜索后立即显示图表
- ✅ **直接替换数据**：API返回的是完整数据，无需合并去重

---

### Step 4: 搜索结果页嵌入图表渲染 (main.go)

**文件**: `main.go`  
**修改位置**: `viewSearchResultWithActions()`

```go
func (m *Model) viewSearchResultWithActions() string {
    s := m.getText("detailTitle") + "\n\n"
    
    if m.searchResult == nil {
        s += m.getText("noInfo") + "\n"
        s += "\n" + m.getText("actionHelp") + "\n"
        return s
    }
    
    // === 左侧：基础数据表格 ===
    t := table.NewWriter()
    t.SetStyle(table.StyleLight)
    
    // 构建表头和数据行（现有逻辑）
    var headers []interface{}
    var values []interface{}
    
    // 基本信息
    if m.language == Chinese {
        headers = append(headers, "股票代码", "股票名称", "现价")
    } else {
        headers = append(headers, "Code", "Name", "Price")
    }
    values = append(values, m.searchResult.Symbol, m.searchResult.Name, 
        m.formatPriceWithColorLang(m.searchResult.Price, m.searchResult.PrevClose))
    
    // ... 其他字段（昨收价、开盘价、最高价、最低价、涨跌额、涨跌幅、换手率、成交量）...
    
    // 添加表头和数据行
    t.AppendHeader(table.Row(headers))
    t.AppendRow(table.Row(values))
    
    s += t.Render() + "\n\n"
    
    // === NEW: 右侧/底部：分时图表（自动展示） ===
    if m.isSearchMode {
        // 计算图表尺寸（根据终端大小自适应）
        termWidth := 120  // 可以从环境变量或配置获取
        termHeight := 30
        
        // 预留基础数据区域（表格高度约10行）
        chartHeight := termHeight - 15 // 给图表留15行
        if chartHeight < 10 {
            chartHeight = 10
        }
        
        // 渲染图表区域分隔线
        s += strings.Repeat("─", 80) + "\n"
        if m.language == Chinese {
            s += "📈 实时分时图表 (每5秒自动刷新)\n\n"
        } else {
            s += "📈 Real-time Intraday Chart (Auto-refresh every 5s)\n\n"
        }
        
        // 渲染图表
        if m.searchIntradayData != nil && len(m.searchIntradayData.Datapoints) > 0 {
            // 创建图表
            chartModel := m.createSearchIntradayChart(termWidth, chartHeight)
            if chartModel != nil {
                s += chartModel.View() + "\n"
            } else {
                // 图表创建失败（终端太小）
                if m.language == Chinese {
                    s += "终端尺寸过小，无法显示图表\n"
                } else {
                    s += "Terminal size too small to display chart\n"
                }
            }
        } else {
            // 数据尚未加载
            if m.language == Chinese {
                s += "正在获取分时数据...\n"
            } else {
                s += "Loading intraday data...\n"
            }
        }
        
        s += "\n"
    }
    
    // === 操作提示 ===
    if m.language == Chinese {
        s += "操作:\n"
        s += "  [1] 添加到自选列表\n"
        s += "  [2] 添加到持股列表\n"
        s += "  [R] 重新搜索\n"
        s += "  [ESC] 返回主菜单\n"
    } else {
        s += "Actions:\n"
        s += "  [1] Add to watchlist\n"
        s += "  [2] Add to portfolio\n"
        s += "  [R] Search again\n"
        s += "  [ESC] Back to main menu\n"
    }
    
    if m.message != "" {
        s += "\n" + m.message + "\n"
    }
    
    return s
}
```

**代码变更量**: ~50 行

**关键变化**：
- ✅ **移除V键**: 不再需要按V键，图表自动展示
- ✅ **嵌入式布局**: 表格在上，图表在下，一屏展示
- ✅ **加载状态**: 数据未到时显示"正在获取..."

---

### Step 5: 创建搜索专用图表渲染函数 (intraday_chart.go)

**文件**: `intraday_chart.go`  
**新增函数**: `createSearchIntradayChart`

```go
// createSearchIntradayChart 为搜索模式创建分时图表
// 与 createIntradayChart 的区别:
// 1. 数据源: m.searchIntradayData (内存) vs m.chartData (磁盘/内存)
// 2. 尺寸: 较小的嵌入式图表 vs 全屏图表
// 3. 时间轴: 简化的时间标签 vs 完整时间标签
func (m *Model) createSearchIntradayChart(termWidth, termHeight int) *linechart.Model {
    debugPrint("debug.search.chartCreate", termWidth, termHeight)
    
    if m.searchIntradayData == nil {
        debugPrint("debug.search.chartDataNil")
        return nil
    }
    
    if len(m.searchIntradayData.Datapoints) == 0 {
        debugPrint("debug.search.chartDataEmpty")
        return nil
    }
    
    debugPrint("debug.search.chartDataPoints", len(m.searchIntradayData.Datapoints))
    
    // 最小大小检查
    minWidth := 40
    minHeight := 8  // 搜索模式使用更小的最小高度
    
    if termWidth < minWidth || termHeight < minHeight {
        return nil
    }
    
    // 计算可用空间（搜索模式使用更紧凑的布局）
    chartWidth := termWidth - 4
    if chartWidth < minWidth {
        chartWidth = minWidth
    }
    chartHeight := termHeight - 6  // 减少padding
    if chartHeight < minHeight {
        chartHeight = minHeight
    }
    
    // === 创建完整时间框架（根据市场配置动态生成） ===
    timeFramework := m.createFixedTimeRange(
        m.searchIntradayData.Date, 
        m.searchIntradayData.Market,
    )
    
    if len(timeFramework) == 0 {
        debugPrint("debug.search.chartNoTimeFramework")
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
    
    debugPrint("debug.search.chartPriceRange", minPrice, maxPrice, margin)
    
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
        
        // 只显示开盘(9:30)和收盘(15:00)
        if abs(totalMinutes-570) <= 5 { // 9:30 ± 5分钟
            return "09:30"
        } else if abs(totalMinutes-900) <= 10 { // 15:00 ± 10分钟
            return "15:00"
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
    
    debugPrint("debug.search.chartSuccess")
    return &lc
}

// abs 返回整数的绝对值
func abs(x int) int {
    if x < 0 {
        return -x
    }
    return x
}
```

**代码变更量**: ~150 行

**关键特性**：
- ✅ **紧凑布局**: 更小的尺寸，适合嵌入结果页
- ✅ **简化时间轴**: 只显示开盘和收盘时间点
- ✅ **独立渲染**: 不影响现有的全屏图表功能

---

### Step 6: 退出时自动清理 (main.go)

**文件**: `main.go`  
**修改位置**: `handleSearchResultWithActions()` 函数

```go
func (m *Model) handleSearchResultWithActions(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
    switch msg.String() {
    case "esc":
        // NEW: 停止搜索 worker 并清理数据
        if m.isSearchMode {
            m.stopSearchIntradayWorker()
        }
        
        m.state = MainMenu
        m.message = ""
        return m, nil
        
    case "r":
        // NEW: 重新搜索时也要清理旧数据
        if m.isSearchMode {
            m.stopSearchIntradayWorker()
        }
        
        m.state = SearchingStock
        m.searchFromWatchlist = false
        m.message = ""
        return m, nil
        
    case "1":
        // 添加到自选列表并跳转
        if m.searchResult != nil {
            if m.addToWatchlist(m.searchResult.Symbol, m.searchResult.Name) {
                m.message = fmt.Sprintf(m.getText("addWatchSuccess"), 
                    m.searchResult.Name, m.searchResult.Symbol)
            } else {
                m.message = fmt.Sprintf(m.getText("alreadyInWatch"), 
                    m.searchResult.Symbol)
            }
            
            // NEW: 停止搜索 worker
            if m.isSearchMode {
                m.stopSearchIntradayWorker()
            }
            
            m.state = WatchlistViewing
            m.resetWatchlistCursor()
            m.cursor = 0
            m.lastUpdate = time.Now()
            
            // 启动自选列表的分时数据采集
            m.startIntradayDataCollection()
        }
        return m, m.tickCmd()
        
    case "2":
        // 添加到持股列表（进入添加流程）
        if m.searchResult != nil {
            // NEW: 停止搜索 worker
            if m.isSearchMode {
                m.stopSearchIntradayWorker()
            }
            
            m.state = AddingStock
            m.addingStep = 1
            m.tempCode = m.searchResult.Symbol
            m.stockInfo = &StockData{
                Symbol: m.searchResult.Symbol,
                Name:   m.searchResult.Name,
                Price:  m.searchResult.Price,
            }
            m.input = ""
            m.message = ""
            m.fromSearch = true
        }
        return m, nil
    }
    return m, nil
}
```

**代码变更量**: ~30 行

**关键改进**：
- ✅ **所有退出路径**: ESC、R、添加到自选/持股都会清理数据
- ✅ **自动切换**: 添加到自选后自动启动列表模式的 worker

---

### Step 7: 添加 i18n 文本 (i18n/zh.json, i18n/en.json)

**文件**: `i18n/zh.json`

```json
{
  "searchModeChart": "实时分时图表 (每5秒自动刷新)",
  "loadingIntradayData": "正在获取分时数据...",
  "terminalTooSmallForChart": "终端尺寸过小，无法显示图表",
  
  // Debug 相关
  "debug.search.workerStart": "[搜索] Worker 启动: %s, 日期: %s",
  "debug.search.workerStop": "[搜索] Worker 停止: %s",
  "debug.search.workerAutoStop": "[搜索] Worker 自动停止: %s",
  "debug.search.workerClosed": "[搜索] Worker 信号关闭",
  "debug.search.marketClosed": "[搜索] 市场已关闭: %s",
  "debug.search.fetchFail": "[搜索] 获取数据失败: %s, 错误: %v",
  "debug.search.noData": "[搜索] 无数据: %s",
  "debug.search.dataUpdated": "[搜索] 数据已更新: %s, 数据点: %d, 时间: %s",
  "debug.search.cleanupComplete": "[搜索] 清理完成",
  "debug.search.chartCreate": "[搜索] 创建图表: 宽度=%d, 高度=%d",
  "debug.search.chartDataNil": "[搜索] 图表数据为空",
  "debug.search.chartDataEmpty": "[搜索] 图表数据点为0",
  "debug.search.chartDataPoints": "[搜索] 图表数据点数量: %d",
  "debug.search.chartNoTimeFramework": "[搜索] 时间框架为空",
  "debug.search.chartPriceRange": "[搜索] 价格范围: min=%.3f, max=%.3f, margin=%.3f",
  "debug.search.chartSuccess": "[搜索] 图表创建成功"
}
```

**文件**: `i18n/en.json`

```json
{
  "searchModeChart": "Real-time Intraday Chart (Auto-refresh every 5s)",
  "loadingIntradayData": "Loading intraday data...",
  "terminalTooSmallForChart": "Terminal size too small to display chart",
  
  // Debug messages
  "debug.search.workerStart": "[Search] Worker started: %s, date: %s",
  "debug.search.workerStop": "[Search] Worker stopped: %s",
  "debug.search.workerAutoStop": "[Search] Worker auto-stopped: %s",
  "debug.search.workerClosed": "[Search] Worker signal closed",
  "debug.search.marketClosed": "[Search] Market closed: %s",
  "debug.search.fetchFail": "[Search] Fetch failed: %s, error: %v",
  "debug.search.noData": "[Search] No data: %s",
  "debug.search.dataUpdated": "[Search] Data updated: %s, datapoints: %d, time: %s",
  "debug.search.cleanupComplete": "[Search] Cleanup complete",
  "debug.search.chartCreate": "[Search] Create chart: width=%d, height=%d",
  "debug.search.chartDataNil": "[Search] Chart data is nil",
  "debug.search.chartDataEmpty": "[Search] Chart data is empty",
  "debug.search.chartDataPoints": "[Search] Chart datapoints: %d",
  "debug.search.chartNoTimeFramework": "[Search] Time framework is empty",
  "debug.search.chartPriceRange": "[Search] Price range: min=%.3f, max=%.3f, margin=%.3f",
  "debug.search.chartSuccess": "[Search] Chart created successfully"
}
```

**代码变更量**: ~20 行

---

## 技术细节

### 1. 高频刷新机制

#### 5秒 vs 1分钟对比

| 特性 | 搜索模式 (5秒) | 列表模式 (1分钟) |
|------|---------------|-----------------|
| **刷新间隔** | 5秒 | 60秒 |
| **数据延迟** | 准实时（<10秒） | 较大延迟（<2分钟） |
| **API调用频率** | 720次/小时 | 60次/小时 |
| **适用场景** | 短期关注单只股票 | 长期监控多只股票 |
| **内存占用** | 单只股票（约10KB） | 多只股票（10KB × N） |

#### 实时性保证

```go
// 启动后立即执行首次获取
go func() {
    m.fetchAndStoreSearchIntradayData(code, name, date) // ← 立即执行
    
    ticker := time.NewTicker(5 * time.Second)
    defer ticker.Stop()
    
    for {
        select {
        case <-ticker.C:
            m.fetchAndStoreSearchIntradayData(code, name, date) // ← 5秒后执行
        }
    }
}()
```

**效果**：
- 搜索后 0-5 秒内首次显示图表
- 此后每 5 秒图表自动延伸

---

### 2. 数据完整性处理

#### API返回数据结构

```json
// 腾讯/新浪 API 返回格式
{
  "data": [
    "0930 8.52 12000",  // 09:30 的价格
    "0931 8.53 12500",  // 09:31 的价格
    "0932 8.51 11800",  // 09:32 的价格
    // ... 当前时间为止的所有数据点
    "1045 8.55 13200"   // 10:45 的价格（最新）
  ]
}
```

**关键点**：
- ✅ API每次返回的是**完整数据**（从开盘到当前时间）
- ✅ 无需手动合并去重，直接替换即可
- ✅ 每次调用都获取最新数据点

```go
// 直接替换，无需复杂合并逻辑
m.searchIntradayData = &IntradayData{
    Code:       code,
    Name:       name,
    Date:       date,
    Datapoints: datapoints, // ← API返回的完整数据
    UpdatedAt:  time.Now().Format("2006-01-02 15:04:05"),
}
```

---

### 3. 内存管理策略

#### 生命周期

```
搜索股票（Enter）
    ↓
isSearchMode = true
searchIntradayData = nil
searchIntradayWorker = make(chan)
    ↓
Worker 启动 → 每5秒更新 searchIntradayData
    ↓
内存占用: ~10KB (单只股票，约240个数据点)
    ↓
用户退出（ESC/R/添加到列表）
    ↓
Worker 停止 → close(searchIntradayWorker)
searchIntradayData = nil
isSearchMode = false
    ↓
内存释放：~10KB
```

#### 内存占用计算

```
单个数据点: Time(string) + Price(float64) ≈ 24 bytes
完整交易日: 240 数据点 × 24 bytes ≈ 5.7 KB
加上元数据: IntradayData 结构体 ≈ 2 KB
总计: ~10 KB per stock
```

**优点**：
- ✅ 内存占用极小（10KB vs 持久化数据可能数MB）
- ✅ GC友好（结构简单，无循环引用）
- ✅ 退出时立即释放

---

### 4. UI布局设计

#### 响应式布局方案

```
┌────────────────────────────────────────────────────────────────┐
│ 股票搜索结果                                                     │
├────────────────────────────────────────────────────────────────┤
│                                                                │
│ 基础数据表格 (高度: 8-10行)                                       │
│ ┌──────────┬──────────┬─────────┬──────────┬──────────┐        │
│ │ 股票代码  │ 股票名称  │  现价   │  涨跌幅  │  成交量  │           │
│ ├──────────┼──────────┼─────────┼──────────┼──────────┤        │
│ │ SH600000 │ 浦发银行  │  8.55   │ +1.42%  │  123万手 │          │
│ └──────────┴──────────┴─────────┴──────────┴──────────┘        │
│                                                                │
├────────────────────────────────────────────────────────────────┤
│ 📈 实时分时图表 (每5秒自动刷新)                                    │
│                                                                │
│   8.60 ┤                                                       │
│        │         ⠠⠊⠑⠢⡀                                        │
│   8.55 ┤     ⠔⠁      ⠑⢄⠠⠔⠊⠢⡀                                  │
│        │  ⡠⠊            ⠑⢄   ⠑⠢⡀                              │
│   8.50 ┼⠔⠁                  ⠑⠤⣀⠔⠊⠑⠢⣀                         │
│        └────────────────────────────────────────────          │
│         09:30                    12:00           15:00        │
│                                                                │
│ 最后更新: 10:45:32  |  数据点: 75                                │
│                                                                │
├────────────────────────────────────────────────────────────────┤
│ 操作:                                                           │
│   [1] 添加到自选列表                                             │
│   [2] 添加到持股列表                                             │
│   [R] 重新搜索                                                  │
│   [ESC] 返回主菜单                                              │
└────────────────────────────────────────────────────────────────┘
```

#### 自适应策略

```go
// 根据终端大小动态调整布局
func (m *Model) viewSearchResultWithActions() string {
    // 获取终端尺寸
    termWidth := getTerminalWidth()   // 例如: 120
    termHeight := getTerminalHeight() // 例如: 40
    
    // 计算各区域高度
    headerHeight := 3       // 标题 + 分隔线
    tableHeight := 10       // 基础数据表格
    separatorHeight := 3    // 图表标题 + 分隔线
    footerHeight := 8       // 操作提示
    
    chartHeight := termHeight - headerHeight - tableHeight - 
                   separatorHeight - footerHeight
    
    // 最小高度保护
    if chartHeight < 10 {
        // 终端太小，不显示图表
        return renderTableOnly()
    }
    
    // 渲染完整布局
    return renderTableAndChart(chartHeight)
}
```

---

## 测试计划

### 单元测试

#### 1. 高频 Worker 测试

**文件**: `intraday_chart_test.go`

```go
func TestSearchHighFrequencyWorker(t *testing.T) {
    m := &Model{
        config: getDefaultConfig(),
    }
    
    // 启动 5秒 worker
    go m.runSearchIntradayWorker("SH600000", "浦发银行", "20251222")
    
    // 首次数据应立即获取（不等待5秒）
    time.Sleep(1 * time.Second)
    if m.searchIntradayData == nil {
        t.Fatal("First fetch should be immediate")
    }
    
    firstDataCount := len(m.searchIntradayData.Datapoints)
    
    // 等待第二次更新（5秒后）
    time.Sleep(6 * time.Second)
    
    // 验证数据已更新（数据点数量可能增加）
    if m.searchIntradayData == nil {
        t.Fatal("Data should still exist after second fetch")
    }
    
    secondDataCount := len(m.searchIntradayData.Datapoints)
    
    // 如果市场开市，数据点应该增加
    // 注意：测试时可能市场关闭，所以用 >= 而非 >
    if secondDataCount < firstDataCount {
        t.Errorf("Expected data to remain or grow, got %d -> %d", 
                 firstDataCount, secondDataCount)
    }
    
    // 停止 worker
    m.stopSearchIntradayWorker()
    
    // 验证清理
    if m.searchIntradayData != nil {
        t.Error("Data should be cleared after stop")
    }
}
```

#### 2. 数据替换测试

```go
func TestSearchDataDirectReplacement(t *testing.T) {
    m := &Model{
        config: getDefaultConfig(),
    }
    
    // 模拟第一次获取（10个数据点）
    m.fetchAndStoreSearchIntradayData("SH600000", "浦发银行", "20251222")
    firstCount := len(m.searchIntradayData.Datapoints)
    
    // 模拟第二次获取（15个数据点，因为时间推进了）
    time.Sleep(1 * time.Second)
    m.fetchAndStoreSearchIntradayData("SH600000", "浦发银行", "20251222")
    secondCount := len(m.searchIntradayData.Datapoints)
    
    // 验证：数据直接替换，不是累加
    // 第二次的数据点数应该接近实际API返回的数量（可能多于第一次）
    if secondCount == firstCount*2 {
        t.Error("Data should be replaced, not merged/accumulated")
    }
}
```

#### 3. 自动停止测试

```go
func TestSearchWorkerAutoStopWhenMarketClosed(t *testing.T) {
    m := &Model{
        config: getDefaultConfig(),
    }
    
    // 使用闭市时间测试（例如周末或晚上）
    // 注意：这个测试需要在特定时间运行，或者 mock isMarketOpen
    
    // 启动 worker
    go m.runSearchIntradayWorker("SH600000", "浦发银行", "20251222")
    
    // 如果市场关闭，worker 应该在首次获取后自动停止
    time.Sleep(2 * time.Second)
    
    // 验证数据已获取
    if m.searchIntradayData == nil {
        t.Fatal("Should have fetched data once before stopping")
    }
    
    // 再等待 10 秒，验证 worker 没有继续更新
    initialUpdateTime := m.searchIntradayData.UpdatedAt
    time.Sleep(10 * time.Second)
    
    // 如果市场关闭，更新时间应该不变（worker已停止）
    if isMarketOpen("SH600000", m) == false {
        if m.searchIntradayData.UpdatedAt != initialUpdateTime {
            t.Error("Worker should stop when market is closed")
        }
    }
}
```

---

### 集成测试

#### 测试用例列表

| ID | 场景 | 前置条件 | 操作步骤 | 预期结果 |
|----|------|---------|---------|---------|
| TC-01 | 搜索后自动展示图表 | 无 | 1. 搜索"600000"<br>2. Enter | 结果页自动显示表格+图表 |
| TC-02 | 图表实时更新 | 市场开市 | 1. 搜索股票<br>2. 观察30秒 | 图表每5秒延伸一次 |
| TC-03 | 首次立即展示 | 市场开市 | 1. 搜索股票<br>2. 计时 | 5秒内出现图表 |
| TC-04 | 退出清理数据 | 在搜索结果页 | 1. 按ESC<br>2. 检查内存 | 返回主菜单，数据已清理 |
| TC-05 | 重新搜索清理旧数据 | 在搜索结果页 | 1. 按R<br>2. 搜索新股票 | 旧图表消失，新图表出现 |
| TC-06 | 添加到自选后清理 | 在搜索结果页 | 1. 按1添加<br>2. 检查内存 | 跳转到自选，搜索数据已清理 |
| TC-07 | 闭市时获取完整数据 | 市场关闭 | 1. 搜索股票 | 显示当日完整分时图 |
| TC-08 | 终端尺寸过小 | 终端高度<20行 | 1. 搜索股票 | 显示表格，提示无法显示图表 |

---

### 手工测试清单

#### 基本功能测试

- [ ] 搜索 A 股股票 (SH600000)，验证图表自动展示
- [ ] 搜索美股股票 (AAPL)，验证图表自动展示
- [ ] 搜索港股股票 (HK00700)，验证图表自动展示
- [ ] 观察图表 30 秒，验证每 5 秒自动更新
- [ ] 使用计时器验证首次展示时间 < 5 秒

#### 数据清理测试

- [ ] 在搜索结果页按 ESC，再搜索同一股票，验证图表重新加载
- [ ] 在搜索结果页按 R，搜索新股票，验证旧图表消失
- [ ] 添加到自选后，检查 `data/intraday/` 目录无新文件
- [ ] 使用调试模式查看 `m.searchIntradayData`，退出后应为 nil

#### 实时更新测试

- [ ] 开市时间搜索股票，观察 1 分钟，验证至少 12 次更新（60s / 5s）
- [ ] 记录首次图表显示时的数据点数量（例如 75 个）
- [ ] 10 秒后刷新，验证数据点数量增加（例如 77 个）

#### 边界条件测试

- [ ] 搜索不存在的股票（应显示提示，无图表）
- [ ] 闭市时间搜索股票（应显示当日完整图表，worker自动停止）
- [ ] 调整终端大小到极小（<80列 或 <20行），验证降级处理
- [ ] 快速连续搜索 5 只股票，验证内存无泄漏

#### 并发测试

- [ ] 快速切换：搜索 → 查看 5 秒 → ESC → 搜索另一股票
- [ ] 验证每次 worker 正确停止和重启
- [ ] 使用 `ps` 或 `top` 检查 goroutine 数量稳定

---

## 风险评估

### 技术风险

| 风险 | 等级 | 影响 | 缓解措施 |
|------|------|------|---------|
| **API频率限制** | 🟡 中 | 5秒间隔可能触发API限流 | 1. 监控API响应<br>2. 出现429错误时自动降频到15秒<br>3. 记录限流日志 |
| **内存泄漏** | 🟡 中 | Worker未正确停止导致goroutine累积 | 1. 所有退出路径都调用清理函数<br>2. 添加goroutine计数器监控<br>3. 单元测试覆盖 |
| **并发冲突** | 🟢 低 | 搜索和列表worker同时运行 | 1. 独立的数据存储<br>2. 独立的worker机制<br>3. 无共享状态 |
| **UI渲染阻塞** | 🟢 低 | 图表创建耗时导致卡顿 | 1. 图表创建在子函数<br>2. 数据不足时快速返回<br>3. 使用Braille字符（轻量） |

---

### 用户体验风险

| 风险 | 等级 | 影响 | 缓解措施 |
|------|------|------|---------|
| **首次加载延迟** | 🟡 中 | 搜索后需等待数据采集 | 1. Worker首次立即执行<br>2. 显示"正在获取..."加载状态<br>3. 大多数情况3秒内出图 |
| **图表过小** | 🟡 中 | 小终端下图表难以阅读 | 1. 最小尺寸检查（40×8）<br>2. 不满足时隐藏图表，提示调整终端 |
| **操作混淆** | 🟢 低 | 用户可能不理解"自动展示" | 1. UI明确标注"实时分时图表"<br>2. 显示更新时间和刷新频率<br>3. 帮助文档说明 |

---

## 后续优化

### 短期优化 (1-2周)

#### 1. **智能降频**
```go
// 根据API响应自动调整刷新频率
type AdaptiveWorker struct {
    interval      time.Duration // 当前间隔
    errorCount    int           // 连续错误次数
    maxInterval   time.Duration // 最大间隔（60秒）
    minInterval   time.Duration // 最小间隔（5秒）
}

func (w *AdaptiveWorker) adjustInterval(err error) {
    if err != nil {
        w.errorCount++
        if w.errorCount > 3 {
            // 连续错误，降频
            w.interval = min(w.interval*2, w.maxInterval)
        }
    } else {
        w.errorCount = 0
        // 成功，恢复频率
        w.interval = w.minInterval
    }
}
```

**收益**: 避免API限流，提升稳定性

#### 2. **渐进式加载**
```go
// 首次只获取最近 10 分钟数据（快速显示）
// 后台继续获取完整数据（填充历史）
func (m *Model) fetchSearchIntradayDataProgressive(code, name, date string) {
    // Phase 1: 快速获取最近10分钟（延迟 < 1秒）
    recentData := fetchRecentData(code, 10)
    m.searchIntradayData = &IntradayData{
        Datapoints: recentData,
        // ...
    }
    
    // Phase 2: 后台获取完整数据
    go func() {
        fullData := fetchFullDayData(code)
        m.searchIntradayData.Datapoints = fullData
    }()
}
```

**收益**: 首次展示延迟从 3 秒降至 1 秒

---

### 中期优化 (1-2月)

#### 3. **持久化选项**
```go
// 在结果页添加"保存图表数据"功能
case "s":
    if m.isSearchMode && m.searchIntradayData != nil {
        // 将搜索数据保存到磁盘
        filePath := getIntradayFilePath(m.searchResult.Symbol, m.chartViewDate)
        saveIntradayData(filePath, m.searchIntradayData)
        m.message = m.getText("dataSaved")
    }
```

**收益**: 用户可以选择性保存感兴趣的数据

#### 4. **缓存复用**
```go
// 如果搜索的股票在持股/自选列表中，复用已采集的数据
func (m *Model) tryLoadFromExistingCache(code, date string) *IntradayData {
    // 1. 检查磁盘缓存
    filePath := getIntradayFilePath(code, date)
    if fileExists(filePath) {
        return loadIntradayDataForDate(code, date)
    }
    
    // 2. 检查列表模式的内存缓存
    // ...
    
    return nil
}
```

**收益**: 避免重复采集，减少API调用

---

### 长期优化 (3-6月)

#### 5. **分屏对比**
```go
// 支持同时查看多只股票的分时图（分屏显示）
type MultiStockView struct {
    stocks []string
    charts []*linechart.Model
}

// 用户可以添加多只股票到对比列表
func (m *Model) addToComparison(code string) {
    m.comparisonStocks = append(m.comparisonStocks, code)
    // 启动多个 worker...
}
```

**收益**: 适合快速比较行业内多只股票

#### 6. **历史回放**
```go
// 支持播放历史分时数据（类似视频回放）
type HistoryPlayer struct {
    data        *IntradayData
    currentIdx  int
    playSpeed   time.Duration // 播放速度（例如 100ms）
}

func (p *HistoryPlayer) play() {
    // 逐个数据点播放，模拟实时行情
}
```

**收益**: 用于复盘分析，查看历史走势

---

## 附录

### A. 相关文件清单

| 文件 | 修改类型 | 变更行数 | 说明 |
|------|---------|---------|------|
| `types.go` | 修改 | ~7 | 添加 Model 字段 |
| `main.go` | 修改 | ~105 | 搜索处理 + 结果页渲染 |
| `intraday_chart.go` | 新增 | ~245 | 高频 worker + 图表渲染 |
| `i18n/zh.json` | 修改 | ~20 | 中文文本 |
| `i18n/en.json` | 修改 | ~20 | 英文文本 |
| **总计** | - | **~397** | - |

---

### B. 参考文档

- [分时图表实现方案](./INTRADAY_CHART_IMPLEMENTATION_PLAN.md)
- [分时数据采集功能](./INTRADAY_FEATURE.md)

---

### C. 术语表

| 术语 | 说明 |
|------|------|
| **搜索模式** | 用户从搜索结果页查看分时图的模式（嵌入式） |
| **列表模式** | 用户从持股/自选列表查看分时图的模式（全屏） |
| **高频 Worker** | 5秒刷新间隔的数据采集 goroutine |
| **临时数据** | 存储在 `m.searchIntradayData` 的非持久化数据 |
| **嵌入式图表** | 显示在搜索结果页内的紧凑型图表 |
| **全屏图表** | `IntradayChartViewing` 状态下的完整图表 |

---

## 实施时间表

| 阶段 | 任务 | 预计耗时 | 负责人 |
|------|------|---------|-------|
| **Day 1** | Step 1-2: 数据结构 + 搜索触发 | 2h | - |
| **Day 2** | Step 3: 高频 Worker 实现 | 4h | - |
| **Day 3** | Step 4-5: 结果页嵌入 + 图表渲染 | 4h | - |
| **Day 4** | Step 6-7: 清理逻辑 + i18n | 2h | - |
| **Day 5** | 单元测试 + 调试 | 4h | - |
| **Day 6** | 集成测试 + 手工测试 | 4h | - |
| **Day 7** | 性能优化 + 文档更新 | 2h | - |
| **总计** | - | **~22小时** | - |

---

## 版本历史

| 版本 | 日期 | 修改内容 | 作者 |
|------|------|---------|------|
| v1.0 | 2025-12-22 | 初始版本（V键触发） | AI Assistant |
| v2.0 | 2025-12-22 | 重大调整：自动展示+5秒刷新 | AI Assistant |

---

**文档状态**: ⏳ Awaiting Review  
**最后更新**: 2025-12-22 17:20:00
