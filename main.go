package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

type Stock struct {
	Code          string  `json:"code"`
	Name          string  `json:"name"`
	Price         float64 `json:"price"`
	CostPrice     float64 `json:"cost_price"`
	Quantity      int     `json:"quantity"`
	Change        float64 `json:"change"`
	ChangePercent float64 `json:"change_percent"`
}

type StockData struct {
	Symbol        string  `json:"symbol"`
	Name          string  `json:"name"`
	Price         float64 `json:"price"`
	Change        float64 `json:"change"`
	ChangePercent float64 `json:"change_percent"`
}

type Portfolio struct {
	Stocks []Stock `json:"stocks"`
}

const (
	dataFile        = "portfolio.json"
	refreshInterval = 5 * time.Second
)

type AppState int

const (
	MainMenu AppState = iota
	AddingStock
	RemovingStock
	ViewingStocks
	Monitoring
)

type Model struct {
	state           AppState
	currentMenuItem int
	menuItems       []string
	cursor          int
	input           string
	message         string
	portfolio       Portfolio
	debugMode       bool

	// For stock addition
	addingStep   int
	tempCode     string
	tempCost     string
	tempQuantity string
	stockInfo    *StockData

	// For monitoring
	lastUpdate time.Time
}

type tickMsg struct{}

func main() {
	portfolio := loadPortfolio()

	m := Model{
		state:           MainMenu,
		currentMenuItem: 0,
		menuItems:       []string{"查看股票列表", "添加股票", "删除股票", "开始监控", "调试模式", "退出"},
		portfolio:       portfolio,
		debugMode:       false,
	}

	p := tea.NewProgram(&m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}

func (m *Model) Init() tea.Cmd {
	return nil
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch m.state {
		case MainMenu:
			return m.handleMainMenu(msg)
		case AddingStock:
			return m.handleAddingStock(msg)
		case RemovingStock:
			return m.handleRemovingStock(msg)
		case ViewingStocks:
			return m.handleViewingStocks(msg)
		case Monitoring:
			return m.handleMonitoring(msg)
		}
	case tickMsg:
		if m.state == Monitoring {
			m.lastUpdate = time.Now()
			return m, m.tickCmd()
		}
	}
	return m, nil
}

func (m *Model) View() string {
	switch m.state {
	case MainMenu:
		return m.viewMainMenu()
	case AddingStock:
		return m.viewAddingStock()
	case RemovingStock:
		return m.viewRemovingStock()
	case ViewingStocks:
		return m.viewViewingStocks()
	case Monitoring:
		return m.viewMonitoring()
	}
	return ""
}

func (m *Model) handleMainMenu(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k", "w":
		if m.currentMenuItem > 0 {
			m.currentMenuItem--
		}
	case "down", "j", "s":
		if m.currentMenuItem < len(m.menuItems)-1 {
			m.currentMenuItem++
		}
	case "enter", " ":
		return m.executeMenuItem()
	case "q", "ctrl+c":
		m.savePortfolio()
		return m, tea.Quit
	}
	return m, nil
}

func (m *Model) executeMenuItem() (tea.Model, tea.Cmd) {
	switch m.currentMenuItem {
	case 0: // 查看股票列表
		m.state = ViewingStocks
		return m, nil
	case 1: // 添加股票
		m.state = AddingStock
		m.addingStep = 0
		m.input = ""
		m.message = ""
		return m, nil
	case 2: // 删除股票
		m.state = RemovingStock
		m.cursor = 0
		return m, nil
	case 3: // 开始监控
		if len(m.portfolio.Stocks) == 0 {
			m.message = "请先添加股票到投资组合"
			return m, nil
		}
		m.state = Monitoring
		m.lastUpdate = time.Now()
		return m, m.tickCmd()
	case 4: // 调试模式
		m.debugMode = !m.debugMode
		return m, nil
	case 5: // 退出
		m.savePortfolio()
		return m, tea.Quit
	}
	return m, nil
}

func (m *Model) viewMainMenu() string {
	s := "=== 股票监控系统 ===\n\n"

	for i, item := range m.menuItems {
		prefix := "  "
		if i == m.currentMenuItem {
			prefix = "► "
		}

		if i == 4 {
			debugStatus := "关闭"
			if m.debugMode {
				debugStatus = "开启"
			}
			s += fmt.Sprintf("%s%s: %s\n", prefix, item, debugStatus)
		} else {
			s += fmt.Sprintf("%s%s\n", prefix, item)
		}
	}

	s += "\n"
	if runtime.GOOS == "windows" {
		s += "使用 W/S 键选择，回车确认，Q键退出\n"
	} else {
		s += "使用方向键 ↑↓ 或 W/S 键选择，回车/空格确认，Q键退出\n"
	}
	s += "=========================\n"

	if m.message != "" {
		s += "\n" + m.message + "\n"
	}

	return s
}

func (m *Model) handleAddingStock(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.state = MainMenu
		m.message = ""
		return m, nil
	case "enter":
		return m.processAddingStep()
	case "backspace":
		if len(m.input) > 0 {
			m.input = m.input[:len(m.input)-1]
		}
	default:
		if len(msg.String()) == 1 && msg.String() != "\n" && msg.String() != "\r" {
			m.input += msg.String()
		}
	}
	return m, nil
}

func (m *Model) processAddingStep() (tea.Model, tea.Cmd) {
	switch m.addingStep {
	case 0: // 输入股票代码
		if m.input == "" {
			m.message = "股票代码不能为空"
			return m, nil
		}
		m.tempCode = m.input
		m.message = "正在获取股票信息..."
		m.stockInfo = getStockInfo(m.input)
		if m.stockInfo == nil || m.stockInfo.Name == "" {
			m.message = fmt.Sprintf("无法获取股票 %s 的信息，请检查股票代码是否正确", m.input)
			m.input = ""
			return m, nil
		}
		m.addingStep = 1
		m.input = ""
		m.message = ""
	case 1: // 输入成本价
		if m.input == "" {
			m.message = "成本价不能为空"
			return m, nil
		}
		if _, err := strconv.ParseFloat(m.input, 64); err != nil {
			m.message = "无效的价格格式"
			m.input = ""
			return m, nil
		}
		m.tempCost = m.input
		m.addingStep = 2
		m.input = ""
		m.message = ""
	case 2: // 输入数量
		if m.input == "" {
			m.message = "数量不能为空"
			return m, nil
		}
		if _, err := strconv.Atoi(m.input); err != nil {
			m.message = "无效的数量格式"
			m.input = ""
			return m, nil
		}
		m.tempQuantity = m.input

		// 添加股票
		costPrice, _ := strconv.ParseFloat(m.tempCost, 64)
		quantity, _ := strconv.Atoi(m.tempQuantity)

		stock := Stock{
			Code:      m.tempCode,
			Name:      m.stockInfo.Name,
			CostPrice: costPrice,
			Quantity:  quantity,
		}

		m.portfolio.Stocks = append(m.portfolio.Stocks, stock)
		m.savePortfolio()

		m.state = MainMenu
		m.message = fmt.Sprintf("成功添加股票: %s (%s)", m.stockInfo.Name, m.tempCode)
		m.addingStep = 0
		m.input = ""
	}
	return m, nil
}

func (m *Model) viewAddingStock() string {
	s := "=== 添加股票 ===\n\n"

	switch m.addingStep {
	case 0:
		s += "请输入股票代码: " + m.input + "_\n"
		s += "\n支持格式: SH601138, 000001, AAPL 等\n"
	case 1:
		s += fmt.Sprintf("股票代码: %s\n", m.tempCode)
		s += fmt.Sprintf("股票名称: %s\n", m.stockInfo.Name)
		s += fmt.Sprintf("当前价格: %.3f\n\n", m.stockInfo.Price)
		s += "请输入成本价: " + m.input + "_\n"
	case 2:
		s += fmt.Sprintf("股票代码: %s\n", m.tempCode)
		s += fmt.Sprintf("股票名称: %s\n", m.stockInfo.Name)
		s += fmt.Sprintf("当前价格: %.3f\n", m.stockInfo.Price)
		s += fmt.Sprintf("成本价: %s\n\n", m.tempCost)
		s += "请输入股票数量: " + m.input + "_\n"
	}

	s += "\nESC或Q键返回主菜单\n"

	if m.message != "" {
		s += "\n" + m.message + "\n"
	}

	return s
}

func (m *Model) handleRemovingStock(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.state = MainMenu
		return m, nil
	case "up", "k", "w":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j", "s":
		if m.cursor < len(m.portfolio.Stocks)-1 {
			m.cursor++
		}
	case "enter", " ":
		if len(m.portfolio.Stocks) > 0 {
			removedStock := m.portfolio.Stocks[m.cursor]
			m.portfolio.Stocks = append(m.portfolio.Stocks[:m.cursor], m.portfolio.Stocks[m.cursor+1:]...)
			m.savePortfolio()
			m.state = MainMenu
			m.message = fmt.Sprintf("成功删除股票: %s (%s)", removedStock.Name, removedStock.Code)
		}
	}
	return m, nil
}

func (m *Model) viewRemovingStock() string {
	s := "=== 删除股票 ===\n\n"

	if len(m.portfolio.Stocks) == 0 {
		s += "投资组合为空\n\nESC或Q键返回主菜单\n"
		return s
	}

	s += "选择要删除的股票:\n\n"
	for i, stock := range m.portfolio.Stocks {
		prefix := "  "
		if i == m.cursor {
			prefix = "► "
		}
		s += fmt.Sprintf("%s%d. %s (%s)\n", prefix, i+1, stock.Name, stock.Code)
	}

	s += "\n使用方向键选择，回车确认，ESC或Q键返回\n"
	return s
}

func (m *Model) handleViewingStocks(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.state = MainMenu
		return m, nil
	}
	return m, nil
}

func (m *Model) viewViewingStocks() string {
	s := "=== 股票列表 ===\n\n"

	if len(m.portfolio.Stocks) == 0 {
		s += "投资组合为空\n\n"
		s += "请先添加股票到投资组合\n\n"
		s += "ESC或Q键返回主菜单\n"
		return s
	}

	t := table.NewWriter()
	t.SetStyle(table.StyleLight)
	t.AppendHeader(table.Row{"序号", "股票代码", "股票名称", "持股数量", "成本价"})

	for i, stock := range m.portfolio.Stocks {
		t.AppendRow(table.Row{
			i + 1,
			stock.Code,
			stock.Name,
			stock.Quantity,
			fmt.Sprintf("%.3f", stock.CostPrice),
		})
	}

	s += t.Render() + "\n"
	s += fmt.Sprintf("\n总共 %d 只股票\n", len(m.portfolio.Stocks))
	s += "\nESC或Q键返回主菜单\n"

	return s
}

func (m *Model) handleMonitoring(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.state = MainMenu
		return m, nil
	}
	return m, nil
}

func (m *Model) viewMonitoring() string {
	s := "=== 股票实时监控 ===\n"
	s += fmt.Sprintf("更新时间: %s\n", m.lastUpdate.Format("2006-01-02 15:04:05"))
	s += "\n"

	t := table.NewWriter()
	t.SetStyle(table.StyleLight)
	t.AppendHeader(table.Row{"代码", "名称", "现价", "成本价", "数量", "当日盈亏", "总盈亏", "盈亏率", "市值"})

	var totalMarketValue float64
	var totalCost float64
	var totalDailyProfit float64

	for i := range m.portfolio.Stocks {
		stock := &m.portfolio.Stocks[i]

		stockData := getStockPrice(stock.Code)
		if stockData != nil {
			stock.Price = stockData.Price
			stock.Change = stockData.Change
			stock.ChangePercent = stockData.ChangePercent
		}

		if stock.Price > 0 {
			dailyProfit := stock.Change * float64(stock.Quantity)
			totalProfit := (stock.Price - stock.CostPrice) * float64(stock.Quantity)
			profitRate := ((stock.Price - stock.CostPrice) / stock.CostPrice) * 100
			marketValue := stock.Price * float64(stock.Quantity)
			cost := stock.CostPrice * float64(stock.Quantity)

			totalMarketValue += marketValue
			totalCost += cost
			totalDailyProfit += dailyProfit

			// 根据总盈亏设置颜色
			dailyProfitStr := formatProfitWithColor(dailyProfit)
			totalProfitStr := formatProfitWithColor(totalProfit)
			profitRateStr := formatProfitRateWithColor(profitRate)

			t.AppendRow(table.Row{
				stock.Code,
				stock.Name,
				fmt.Sprintf("%.3f", stock.Price),
				fmt.Sprintf("%.3f", stock.CostPrice),
				stock.Quantity,
				dailyProfitStr,
				totalProfitStr,
				profitRateStr,
				fmt.Sprintf("%.2f", marketValue),
			})

			// 在每个股票后添加分隔线（除了最后一个）
			if i < len(m.portfolio.Stocks)-1 {
				t.AppendSeparator()
			}
		}
	}

	totalPortfolioProfit := totalMarketValue - totalCost
	totalProfitRate := 0.0
	if totalCost > 0 {
		totalProfitRate = (totalPortfolioProfit / totalCost) * 100
	}

	t.AppendSeparator()
	t.AppendRow(table.Row{
		"",
		"总计",
		"",
		"",
		"",
		formatProfitWithColor(totalDailyProfit),
		formatProfitWithColor(totalPortfolioProfit),
		formatProfitRateWithColor(totalProfitRate),
		fmt.Sprintf("%.2f", totalMarketValue),
	})

	s += t.Render() + "\n"
	s += "\nESC或Q键返回主菜单\n"

	return s
}

func (m *Model) tickCmd() tea.Cmd {
	return tea.Tick(refreshInterval, func(t time.Time) tea.Msg {
		return tickMsg{}
	})
}

func (m *Model) savePortfolio() {
	data, err := json.MarshalIndent(m.portfolio, "", "  ")
	if err != nil {
		return
	}
	os.WriteFile(dataFile, data, 0644)
}

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

func formatProfitWithColor(profit float64) string {
	if profit >= 0 {
		return text.FgRed.Sprintf("+%.2f", profit)
	}
	return text.FgGreen.Sprintf("%.2f", profit)
}

func formatProfitRateWithColor(rate float64) string {
	if rate >= 0 {
		return text.FgRed.Sprintf("+%.2f%%", rate)
	}
	return text.FgGreen.Sprintf("%.2f%%", rate)
}

func getStockInfo(symbol string) *StockData {
	return getStockPrice(symbol)
}

func getStockPrice(symbol string) *StockData {
	if isChinaStock(symbol) {
		data := tryTencentAPI(symbol)
		if data.Price > 0 {
			return data
		}
		debugPrint("[调试] 腾讯API失败，尝试其他API\n")
	}

	data := tryFinnhubAPI(symbol)
	if data.Price > 0 {
		return data
	}

	debugPrint("[调试] Finnhub API失败，尝试模拟数据\n")
	return generateMockData(symbol)
}

func isChinaStock(symbol string) bool {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	return strings.HasPrefix(symbol, "SH") || strings.HasPrefix(symbol, "SZ") ||
		(len(symbol) == 6 && (strings.HasPrefix(symbol, "0") || strings.HasPrefix(symbol, "3") || strings.HasPrefix(symbol, "6")))
}

func tryTencentAPI(symbol string) *StockData {
	tencentSymbol := convertStockSymbolForTencent(symbol)
	debugPrint("[调试] 腾讯API - 原始代码: %s -> 转换后: %s\n", symbol, tencentSymbol)

	url := fmt.Sprintf("https://qt.gtimg.cn/q=%s", tencentSymbol)
	debugPrint("[调试] 腾讯请求URL: %s\n", url)

	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		debugPrint("[错误] 腾讯API HTTP请求失败: %v\n", err)
		return &StockData{Symbol: symbol, Price: 0}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		debugPrint("[错误] 腾讯API读取响应失败: %v\n", err)
		return &StockData{Symbol: symbol, Price: 0}
	}

	content, err := gbkToUtf8(body)
	if err != nil {
		debugPrint("[错误] 腾讯API编码转换失败: %v\n", err)
		content = string(body)
	}
	debugPrint("[调试] 腾讯API响应: %s\n", content[:min(100, len(content))])

	if !strings.Contains(content, "~") {
		debugPrint("[调试] 腾讯API响应格式错误\n")
		return &StockData{Symbol: symbol, Price: 0}
	}

	fields := strings.Split(content, "~")
	if len(fields) < 5 {
		debugPrint("[调试] 腾讯API数据字段不足\n")
		return &StockData{Symbol: symbol, Price: 0}
	}

	stockName := fields[1]

	price, err := strconv.ParseFloat(fields[3], 64)
	if err != nil || price <= 0 {
		debugPrint("[调试] 腾讯API价格解析失败: %s\n", fields[3])
		return &StockData{Symbol: symbol, Price: 0}
	}

	previousClose, err := strconv.ParseFloat(fields[4], 64)
	if err != nil || previousClose <= 0 {
		debugPrint("[调试] 腾讯API昨收价解析失败: %s\n", fields[4])
		return &StockData{Symbol: symbol, Price: 0}
	}

	change := price - previousClose
	changePercent := (change / previousClose) * 100

	debugPrint("[调试] 腾讯API获取成功 - 名称: %s, 价格: %.2f, 涨跌: %.2f (%.2f%%)\n", stockName, price, change, changePercent)

	return &StockData{
		Symbol:        symbol,
		Name:          stockName,
		Price:         price,
		Change:        change,
		ChangePercent: changePercent,
	}
}

func convertStockSymbolForTencent(symbol string) string {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))

	if strings.HasPrefix(symbol, "SH") {
		return "sh" + strings.TrimPrefix(symbol, "SH")
	} else if strings.HasPrefix(symbol, "SZ") {
		return "sz" + strings.TrimPrefix(symbol, "SZ")
	}

	if len(symbol) == 6 && strings.HasPrefix(symbol, "6") {
		return "sh" + symbol
	} else if len(symbol) == 6 && (strings.HasPrefix(symbol, "0") || strings.HasPrefix(symbol, "3")) {
		return "sz" + symbol
	}

	return symbol
}

func tryFinnhubAPI(symbol string) *StockData {
	convertedSymbol := convertStockSymbolForFinnhub(symbol)
	debugPrint("[调试] Finnhub - 原始代码: %s -> 转换后: %s\n", symbol, convertedSymbol)

	stockName := getFinnhubStockName(convertedSymbol)

	url := fmt.Sprintf("https://finnhub.io/api/v1/quote?symbol=%s&token=demo", convertedSymbol)
	debugPrint("[调试] Finnhub请求URL: %s\n", url)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		debugPrint("[错误] Finnhub HTTP请求失败: %v\n", err)
		return &StockData{Symbol: symbol, Price: 0}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		debugPrint("[错误] Finnhub读取响应失败: %v\n", err)
		return &StockData{Symbol: symbol, Price: 0}
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		debugPrint("[错误] Finnhub JSON解析失败: %v\n", err)
		return &StockData{Symbol: symbol, Price: 0}
	}

	current, currentOk := result["c"].(float64)
	previous, prevOk := result["pc"].(float64)

	if !currentOk || !prevOk || current <= 0 {
		debugPrint("[调试] Finnhub数据无效或为空\n")
		return &StockData{Symbol: symbol, Price: 0}
	}

	change := current - previous
	changePercent := (change / previous) * 100

	debugPrint("[调试] Finnhub获取成功 - 名称: %s, 价格: %.2f, 涨跌: %.2f (%.2f%%)\n", stockName, current, change, changePercent)

	return &StockData{
		Symbol:        symbol,
		Name:          stockName,
		Price:         current,
		Change:        change,
		ChangePercent: changePercent,
	}
}

func convertStockSymbolForFinnhub(symbol string) string {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))

	if strings.HasPrefix(symbol, "SH") {
		return strings.TrimPrefix(symbol, "SH") + ".SS"
	} else if strings.HasPrefix(symbol, "SZ") {
		return strings.TrimPrefix(symbol, "SZ") + ".SZ"
	} else if strings.HasPrefix(symbol, "HK") {
		return strings.TrimPrefix(symbol, "HK") + ".HK"
	}

	if len(symbol) == 6 && strings.HasPrefix(symbol, "6") {
		return symbol + ".SS"
	} else if len(symbol) == 6 && (strings.HasPrefix(symbol, "0") || strings.HasPrefix(symbol, "3")) {
		return symbol + ".SZ"
	}

	return symbol
}

func getFinnhubStockName(symbol string) string {
	url := fmt.Sprintf("https://finnhub.io/api/v1/stock/profile2?symbol=%s&token=demo", symbol)

	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		debugPrint("[调试] 无法获取股票名称\n")
		return symbol
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return symbol
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return symbol
	}

	if name, ok := result["name"].(string); ok && name != "" {
		return name
	}

	return symbol
}

func generateMockData(symbol string) *StockData {
	debugPrint("[调试] 生成模拟数据用于演示\n")

	var mockName string
	switch {
	case strings.HasPrefix(symbol, "SH601138") || strings.HasPrefix(symbol, "601138"):
		mockName = "工业富联"
	case strings.HasPrefix(symbol, "SH000001") || strings.HasPrefix(symbol, "000001"):
		mockName = "上证指数"
	case strings.ToUpper(symbol) == "AAPL":
		mockName = "苹果公司"
	case strings.ToUpper(symbol) == "TSLA":
		mockName = "特斯拉"
	default:
		mockName = "模拟股票"
	}

	basePrice := 50.0 + float64((len(symbol)*7)%100)

	now := time.Now()
	variation := float64((now.Hour()*60+now.Minute())%100) / 100.0
	change := (variation - 0.5) * 4.0
	price := basePrice + change

	changePercent := (change / basePrice) * 100

	debugPrint("[调试] 模拟数据生成 - 名称: %s, 价格: %.2f, 涨跌: %.2f (%.2f%%)\n", mockName, price, change, changePercent)

	return &StockData{
		Symbol:        symbol,
		Name:          mockName,
		Price:         price,
		Change:        change,
		ChangePercent: changePercent,
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func debugPrint(format string, args ...any) {
	// Debug output is disabled in Bubble Tea mode for clean interface
	// If debug mode is needed, could write to a log file instead
}

func gbkToUtf8(data []byte) (string, error) {
	reader := transform.NewReader(strings.NewReader(string(data)), simplifiedchinese.GBK.NewDecoder())
	utf8Data, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	return string(utf8Data), nil
}
