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
	"gopkg.in/yaml.v3"
)

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

type Portfolio struct {
	Stocks []Stock `json:"stocks"`
}

// 系统配置结构
type Config struct {
	// 系统设置
	System SystemConfig `yaml:"system"`
	// 显示设置
	Display DisplayConfig `yaml:"display"`
	// 更新设置
	Update UpdateConfig `yaml:"update"`
}

type SystemConfig struct {
	Language    string `yaml:"language"`    // 默认语言 "zh" 或 "en"
	AutoStart   bool   `yaml:"auto_start"`  // 有数据时自动进入监控模式
	DebugMode   bool   `yaml:"debug_mode"`  // 调试模式开关
}

type DisplayConfig struct {
	ColorScheme   string `yaml:"color_scheme"`   // 颜色方案 "professional", "simple"
	DecimalPlaces int    `yaml:"decimal_places"` // 价格显示小数位数
	TableStyle    string `yaml:"table_style"`    // 表格样式 "light", "bold", "simple"
}

type UpdateConfig struct {
	RefreshInterval int  `yaml:"refresh_interval"` // 刷新间隔（秒）
	AutoUpdate      bool `yaml:"auto_update"`      // 是否自动更新
}

const (
	dataFile        = "portfolio.json"
	configFile      = "config.yaml"
	refreshInterval = 5 * time.Second
)

type Language string

const (
	Chinese Language = "zh"
	English Language = "en"
)

type AppState int

const (
	MainMenu AppState = iota
	AddingStock
	RemovingStock
	Monitoring
	EditingStock
	SearchingStock
	SearchResult
	LanguageSelection
)

// 文本映射结构
type TextMap map[string]string

// 语言文本映射
var texts = map[Language]TextMap{
	Chinese: {
		"title":             "=== 股票监控系统 ===",
		"stockList":         "股票列表",
		"stockSearch":       "股票搜索",
		"addStock":          "添加股票",
		"editStock":         "修改股票",
		"removeStock":       "删除股票",
		"debugMode":         "调试模式",
		"language":          "语言",
		"exit":              "退出",
		"on":                "开启",
		"off":               "关闭",
		"chinese":           "中文",
		"english":           "English",
		"keyHelp":           "使用方向键 ↑↓ 或 W/S 键选择，回车/空格确认，Q键退出",
		"keyHelpWin":        "使用 W/S 键选择，回车确认，Q键退出",
		"returnToMenu":      "ESC、Q键或M键返回主菜单",
		"returnToMenuShort": "ESC或Q键返回主菜单",
		"monitoringTitle":   "=== 股票实时监控 ===",
		"updateTime":        "更新时间(5s): %s",
		"emptyPortfolio":    "投资组合为空",
		"addStockFirst":     "请先添加股票到投资组合",
		"total":             "总计",
		"addingTitle":       "=== 添加股票 ===",
		"enterCode":         "请输入股票代码: ",
		"enterCost":         "请输入成本价: ",
		"enterQuantity":     "请输入股票数量: ",
		"codeFormat":        "支持格式: SH601138, 000001, AAPL 等",
		"stockCode":         "股票代码: %s",
		"stockName":         "股票名称: %s",
		"currentPrice":      "当前价格: %.3f",
		"costPrice":         "成本价: %s",
		"codeRequired":      "股票代码不能为空",
		"costRequired":      "成本价不能为空",
		"quantityRequired":  "数量不能为空",
		"invalidPrice":      "无效的价格格式",
		"invalidQuantity":   "无效的数量格式",
		"fetchingInfo":      "正在获取股票信息...",
		"stockNotFound":     "无法获取股票 %s 的信息，请检查股票代码是否正确",
		"addSuccess":        "成功添加股票: %s (%s)",
		"removeTitle":       "=== 删除股票 ===",
		"selectToRemove":    "选择要删除的股票:",
		"navHelp":           "使用方向键选择，回车确认，ESC或Q键返回",
		"removeSuccess":     "成功删除股票: %s (%s)",
		"editTitle":         "=== 修改股票 ===",
		"selectToEdit":      "选择要修改的股票:",
		"currentCost":       "当前成本价: %.3f",
		"enterNewCost":      "请输入新的成本价: ",
		"newCost":           "新成本价: %.3f",
		"currentQuantity":   "当前数量: %d",
		"enterNewQuantity":  "请输入新的数量: ",
		"editSuccess":       "成功修改股票 %s 的成本价和数量",
		"searchTitle":       "=== 股票搜索 ===",
		"enterSearch":       "请输入股票代码或名称: ",
		"searchFormats":     "支持格式:\n• 中文名称: 贵州茅台, 苹果, 腾讯, 阿里巴巴 等\n• 中国股票: SH601138, 000001, SZ000002 等\n• 美股: AAPL, TSLA, MSFT 等\n• 港股: HK00700 等",
		"searchHelp":        "回车搜索，ESC或Q键返回主菜单",
		"searching":         "正在搜索股票信息...",
		"searchNotFound":    "无法找到股票 %s 的信息，请检查输入是否正确",
		"detailTitle":       "=== 股票详情信息 ===",
		"noInfo":            "未找到股票信息",
		"detailHelp":        "ESC或Q键返回主菜单，R键重新搜索",
		"emptyCannotEdit":   "投资组合为空，无法修改股票",
		"languageTitle":     "=== 语言选择 ===",
		"selectLanguage":    "请选择您的语言:",
		"languageHelp":      "使用方向键选择，回车确认，ESC或Q键返回主菜单",
	},
	English: {
		"title":             "=== Stock Monitor System ===",
		"stockList":         "Stock List",
		"stockSearch":       "Stock Search",
		"addStock":          "Add Stock",
		"editStock":         "Edit Stock",
		"removeStock":       "Remove Stock",
		"debugMode":         "Debug Mode",
		"language":          "Language",
		"exit":              "Exit",
		"on":                "On",
		"off":               "Off",
		"chinese":           "中文",
		"english":           "English",
		"keyHelp":           "Use arrow keys ↑↓ or W/S to select, Enter/Space to confirm, Q to exit",
		"keyHelpWin":        "Use W/S keys to select, Enter to confirm, Q to exit",
		"returnToMenu":      "ESC, Q or M to return to main menu",
		"returnToMenuShort": "ESC or Q to return to main menu",
		"monitoringTitle":   "=== Real-time Stock Monitor ===",
		"updateTime":        "Update Time(5s): %s",
		"emptyPortfolio":    "Portfolio is empty",
		"addStockFirst":     "Please add stocks to your portfolio first",
		"total":             "Total",
		"addingTitle":       "=== Add Stock ===",
		"enterCode":         "Enter stock code: ",
		"enterCost":         "Enter cost price: ",
		"enterQuantity":     "Enter quantity: ",
		"codeFormat":        "Supported formats: SH601138, 000001, AAPL, etc.",
		"stockCode":         "Stock Code: %s",
		"stockName":         "Stock Name: %s",
		"currentPrice":      "Current Price: %.3f",
		"costPrice":         "Cost Price: %s",
		"codeRequired":      "Stock code cannot be empty",
		"costRequired":      "Cost price cannot be empty",
		"quantityRequired":  "Quantity cannot be empty",
		"invalidPrice":      "Invalid price format",
		"invalidQuantity":   "Invalid quantity format",
		"fetchingInfo":      "Fetching stock information...",
		"stockNotFound":     "Unable to get information for stock %s, please check the code is correct",
		"addSuccess":        "Successfully added stock: %s (%s)",
		"removeTitle":       "=== Remove Stock ===",
		"selectToRemove":    "Select stock to remove:",
		"navHelp":           "Use arrow keys to select, Enter to confirm, ESC or Q to return",
		"removeSuccess":     "Successfully removed stock: %s (%s)",
		"editTitle":         "=== Edit Stock ===",
		"selectToEdit":      "Select stock to edit:",
		"currentCost":       "Current cost price: %.3f",
		"enterNewCost":      "Enter new cost price: ",
		"newCost":           "New cost price: %.3f",
		"currentQuantity":   "Current quantity: %d",
		"enterNewQuantity":  "Enter new quantity: ",
		"editSuccess":       "Successfully edited stock %s cost price and quantity",
		"searchTitle":       "=== Stock Search ===",
		"enterSearch":       "Enter stock code or name: ",
		"searchFormats":     "Supported formats:\n• Chinese names: 贵州茅台, Apple, Tencent, Alibaba, etc.\n• Chinese stocks: SH601138, 000001, SZ000002, etc.\n• US stocks: AAPL, TSLA, MSFT, etc.\n• Hong Kong stocks: HK00700, etc.",
		"searchHelp":        "Press Enter to search, ESC or Q to return to main menu",
		"searching":         "Searching stock information...",
		"searchNotFound":    "Unable to find information for stock %s, please check your input is correct",
		"detailTitle":       "=== Stock Detail Information ===",
		"noInfo":            "No stock information found",
		"detailHelp":        "ESC or Q to return to main menu, R to search again",
		"emptyCannotEdit":   "Portfolio is empty, cannot edit stocks",
		"languageTitle":     "=== Language Selection ===",
		"selectLanguage":    "Please select your language:",
		"languageHelp":      "Use arrow keys to select, Enter to confirm, ESC or Q to return to main menu",
	},
}

type Model struct {
	state           AppState
	currentMenuItem int
	menuItems       []string
	cursor          int
	input           string
	message         string
	portfolio       Portfolio
	config          Config    // 系统配置
	debugMode       bool
	language        Language

	// For stock addition
	addingStep   int
	tempCode     string
	tempCost     string
	tempQuantity string
	stockInfo    *StockData

	// For stock editing
	editingStep        int
	selectedStockIndex int

	// For stock searching
	searchInput  string
	searchResult *StockData

	// For language selection
	languageCursor int

	// For monitoring
	lastUpdate time.Time
}

type tickMsg struct{}

// 获取本地化文本的辅助函数
func (m *Model) getText(key string) string {
	if text, exists := texts[m.language][key]; exists {
		return text
	}
	// 如果找不到文本，返回英文版本作为备用
	if text, exists := texts[English][key]; exists {
		return text
	}
	return key // 最后备用返回key本身
}

// 获取菜单项
func (m *Model) getMenuItems() []string {
	return []string{
		m.getText("stockList"),
		m.getText("stockSearch"),
		m.getText("addStock"),
		m.getText("editStock"),
		m.getText("removeStock"),
		m.getText("debugMode"),
		m.getText("language"),
		m.getText("exit"),
	}
}

func main() {
	// 加载配置文件
	config := loadConfig()
	portfolio := loadPortfolio()

	// 根据配置和是否有股票数据决定初始状态
	initialState := MainMenu
	var lastUpdate time.Time
	if config.System.AutoStart && len(portfolio.Stocks) > 0 {
		initialState = Monitoring
		lastUpdate = time.Now()
	}

	// 根据配置文件设置语言
	language := English // 默认英文
	if config.System.Language == "zh" {
		language = Chinese
	}

	m := Model{
		state:           initialState,
		currentMenuItem: 0,
		portfolio:       portfolio,
		config:          config,
		debugMode:       config.System.DebugMode,
		language:        language,
		lastUpdate:      lastUpdate,
	}

	// 根据语言设置菜单项
	m.menuItems = m.getMenuItems()

	p := tea.NewProgram(&m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}

func (m *Model) Init() tea.Cmd {
	if m.state == Monitoring {
		return m.tickCmd()
	}
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
		case Monitoring:
			return m.handleMonitoring(msg)
		case EditingStock:
			return m.handleEditingStock(msg)
		case SearchingStock:
			return m.handleSearchingStock(msg)
		case SearchResult:
			return m.handleSearchResult(msg)
		case LanguageSelection:
			return m.handleLanguageSelection(msg)
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
	case Monitoring:
		return m.viewMonitoring()
	case EditingStock:
		return m.viewEditingStock()
	case SearchingStock:
		return m.viewSearchingStock()
	case SearchResult:
		return m.viewSearchResult()
	case LanguageSelection:
		return m.viewLanguageSelection()
	}
	return ""
}

func (m *Model) handleMainMenu(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k", "w":
		if m.currentMenuItem > 0 {
			m.currentMenuItem--
		}
		m.message = "" // 清除消息
	case "down", "j", "s":
		if m.currentMenuItem < len(m.menuItems)-1 {
			m.currentMenuItem++
		}
		m.message = "" // 清除消息
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
	case 0: // 股票列表
		m.state = Monitoring
		m.lastUpdate = time.Now()
		return m, m.tickCmd()
	case 1: // 股票搜索
		m.state = SearchingStock
		m.searchInput = ""
		m.searchResult = nil
		m.message = ""
		return m, nil
	case 2: // 添加股票
		m.state = AddingStock
		m.addingStep = 0
		m.input = ""
		m.message = ""
		return m, nil
	case 3: // 修改股票
		if len(m.portfolio.Stocks) == 0 {
			m.message = m.getText("emptyCannotEdit")
			return m, nil
		}
		m.state = EditingStock
		m.editingStep = 0
		m.cursor = 0
		m.input = ""
		m.message = ""
		return m, nil
	case 4: // 删除股票
		m.state = RemovingStock
		m.cursor = 0
		return m, nil
	case 5: // 调试模式
		m.debugMode = !m.debugMode
		m.config.System.DebugMode = m.debugMode
		// 保存配置到文件
		if err := saveConfig(m.config); err != nil && m.debugMode {
			m.message = fmt.Sprintf("Warning: Failed to save config: %v", err)
		}
		return m, nil
	case 6: // 语言选择页面
		m.state = LanguageSelection
		m.languageCursor = 0
		if m.language == English {
			m.languageCursor = 1
		}
		return m, nil
	case 7: // 退出
		m.savePortfolio()
		return m, tea.Quit
	}
	return m, nil
}

func (m *Model) viewMainMenu() string {
	s := m.getText("title") + "\n\n"

	for i, item := range m.menuItems {
		prefix := "  "
		if i == m.currentMenuItem {
			prefix = "► "
		}

		if i == 5 { // 调试模式
			debugStatus := m.getText("off")
			if m.debugMode {
				debugStatus = m.getText("on")
			}
			s += fmt.Sprintf("%s%s: %s\n", prefix, item, debugStatus)
		} else if i == 6 { // 语言选择
			langStatus := m.getText("english")
			if m.language == Chinese {
				langStatus = m.getText("chinese")
			}
			s += fmt.Sprintf("%s%s: %s\n", prefix, item, langStatus)
		} else {
			s += fmt.Sprintf("%s%s\n", prefix, item)
		}
	}

	s += "\n"
	if runtime.GOOS == "windows" {
		s += m.getText("keyHelpWin") + "\n"
	} else {
		s += m.getText("keyHelp") + "\n"
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
			// 正确处理多字节字符（如中文）的删除
			runes := []rune(m.input)
			if len(runes) > 0 {
				m.input = string(runes[:len(runes)-1])
			}
		}
	default:
		// 改进的输入处理：支持多字节字符（如中文）
		str := msg.String()
		if len(str) > 0 && str != "\n" && str != "\r" && !isControlKey(str) {
			m.input += str
		}
	}
	return m, nil
}

func (m *Model) processAddingStep() (tea.Model, tea.Cmd) {
	switch m.addingStep {
	case 0: // 输入股票代码
		if m.input == "" {
			m.message = m.getText("codeRequired")
			return m, nil
		}
		m.tempCode = m.input
		m.message = m.getText("fetchingInfo")
		m.stockInfo = getStockInfo(m.input)
		if m.stockInfo == nil || m.stockInfo.Name == "" {
			m.message = fmt.Sprintf(m.getText("stockNotFound"), m.input)
			m.input = ""
			return m, nil
		}
		m.addingStep = 1
		m.input = ""
		m.message = ""
	case 1: // 输入成本价
		if m.input == "" {
			m.message = m.getText("costRequired")
			return m, nil
		}
		if _, err := strconv.ParseFloat(m.input, 64); err != nil {
			m.message = m.getText("invalidPrice")
			m.input = ""
			return m, nil
		}
		m.tempCost = m.input
		m.addingStep = 2
		m.input = ""
		m.message = ""
	case 2: // 输入数量
		if m.input == "" {
			m.message = m.getText("quantityRequired")
			return m, nil
		}
		if _, err := strconv.Atoi(m.input); err != nil {
			m.message = m.getText("invalidQuantity")
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
		m.message = fmt.Sprintf(m.getText("addSuccess"), m.stockInfo.Name, m.tempCode)
		m.addingStep = 0
		m.input = ""
	}
	return m, nil
}

func (m *Model) viewAddingStock() string {
	s := m.getText("addingTitle") + "\n\n"

	switch m.addingStep {
	case 0:
		s += m.getText("enterCode") + m.input + "_\n"
		s += "\n" + m.getText("codeFormat") + "\n"
	case 1:
		s += fmt.Sprintf(m.getText("stockCode"), m.tempCode) + "\n"
		s += fmt.Sprintf(m.getText("stockName"), m.stockInfo.Name) + "\n"
		s += fmt.Sprintf(m.getText("currentPrice"), m.stockInfo.Price) + "\n\n"
		s += m.getText("enterCost") + m.input + "_\n"
	case 2:
		s += fmt.Sprintf(m.getText("stockCode"), m.tempCode) + "\n"
		s += fmt.Sprintf(m.getText("stockName"), m.stockInfo.Name) + "\n"
		s += fmt.Sprintf(m.getText("currentPrice"), m.stockInfo.Price) + "\n"
		s += fmt.Sprintf(m.getText("costPrice"), m.tempCost) + "\n\n"
		s += m.getText("enterQuantity") + m.input + "_\n"
	}

	s += "\n" + m.getText("returnToMenuShort") + "\n"

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
			m.message = fmt.Sprintf(m.getText("removeSuccess"), removedStock.Name, removedStock.Code)
		}
	}
	return m, nil
}

func (m *Model) viewRemovingStock() string {
	s := m.getText("removeTitle") + "\n\n"

	if len(m.portfolio.Stocks) == 0 {
		s += m.getText("emptyPortfolio") + "\n\n" + m.getText("returnToMenuShort") + "\n"
		return s
	}

	s += m.getText("selectToRemove") + "\n\n"
	for i, stock := range m.portfolio.Stocks {
		prefix := "  "
		if i == m.cursor {
			prefix = "► "
		}
		s += fmt.Sprintf("%s%d. %s (%s)\n", prefix, i+1, stock.Name, stock.Code)
	}

	s += "\n" + m.getText("navHelp") + "\n"
	return s
}

func (m *Model) handleMonitoring(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "m":
		m.state = MainMenu
		return m, nil
	}
	return m, nil
}

func (m *Model) viewMonitoring() string {
	s := m.getText("monitoringTitle") + "\n"
	s += fmt.Sprintf(m.getText("updateTime"), m.lastUpdate.Format("2006-01-02 15:04:05")) + "\n"
	s += "\n"

	if len(m.portfolio.Stocks) == 0 {
		s += m.getText("emptyPortfolio") + "\n\n"
		s += m.getText("addStockFirst") + "\n\n"
		s += m.getText("returnToMenu") + "\n"
		return s
	}

	t := table.NewWriter()
	t.SetStyle(table.StyleLight)

	// 获取本地化的表头
	if m.language == Chinese {
		t.AppendHeader(table.Row{"代码", "名称", "现价", "昨收价", "开盘", "最高", "最低", "成本价", "持股数", "今日涨幅", "当日盈亏", "总盈亏", "盈亏率", "市值"})
	} else {
		t.AppendHeader(table.Row{"Code", "Name", "Price", "PrevClose", "Open", "High", "Low", "Cost", "Quantity", "Today%", "DailyP&L", "TotalP&L", "P&LRate", "Value"})
	}

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
			stock.StartPrice = stockData.StartPrice
			stock.MaxPrice = stockData.MaxPrice
			stock.MinPrice = stockData.MinPrice
			stock.PrevClose = stockData.PrevClose
		}

		if stock.Price > 0 {
			dailyProfit := stock.Change * float64(stock.Quantity)
			totalProfit := (stock.Price - stock.CostPrice) * float64(stock.Quantity)
			profitRate := ((stock.Price - stock.CostPrice) / stock.CostPrice) * 100
			marketValue := stock.Price * float64(stock.Quantity)
			cost := stock.CostPrice * float64(stock.Quantity)

			// 计算今日涨幅：应该基于昨收价，而不是开盘价
			var todayChangeStr string
			// 使用change_percent字段，这是基于昨收价计算的涨跌幅
			if stock.ChangePercent != 0 {
				todayChangeStr = m.formatProfitRateWithColorZeroLang(stock.ChangePercent)
			} else {
				todayChangeStr = "-"
			}

			totalMarketValue += marketValue
			totalCost += cost
			totalDailyProfit += dailyProfit

			// 使用多语言颜色显示函数
			dailyProfitStr := m.formatProfitWithColorZeroLang(dailyProfit)
			totalProfitStr := m.formatProfitWithColorZeroLang(totalProfit)
			profitRateStr := m.formatProfitRateWithColorZeroLang(profitRate)

			t.AppendRow(table.Row{
				stock.Code,
				stock.Name,
				m.formatPriceWithColorLang(stock.Price, stock.PrevClose),
				fmt.Sprintf("%.3f", stock.PrevClose),
				m.formatPriceWithColorLang(stock.StartPrice, stock.PrevClose),
				m.formatPriceWithColorLang(stock.MaxPrice, stock.PrevClose),
				m.formatPriceWithColorLang(stock.MinPrice, stock.PrevClose),
				m.formatPriceWithColorLang(stock.CostPrice, stock.PrevClose),
				stock.Quantity,
				todayChangeStr,
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
		m.getText("total"),
		"",
		"",
		"",
		"",
		"",
		"",
		"",
		"",
		m.formatProfitWithColorLang(totalDailyProfit),
		m.formatProfitWithColorLang(totalPortfolioProfit),
		m.formatProfitRateWithColorLang(totalProfitRate),
		fmt.Sprintf("%.2f", totalMarketValue),
	})

	s += t.Render() + "\n"
	s += "\n" + m.getText("returnToMenu") + "\n"

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

// 默认配置
func getDefaultConfig() Config {
	return Config{
		System: SystemConfig{
			Language:  "en",    // 默认英文
			AutoStart: true,    // 有数据时自动进入监控模式
			DebugMode: false,   // 调试模式关闭
		},
		Display: DisplayConfig{
			ColorScheme:   "professional", // 专业配色方案
			DecimalPlaces: 3,             // 3位小数
			TableStyle:    "light",       // 轻量表格样式
		},
		Update: UpdateConfig{
			RefreshInterval: 5,   // 5秒刷新间隔
			AutoUpdate:      true, // 自动更新开启
		},
	}
}

// 加载配置文件
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
	return config
}

// 保存配置文件
func saveConfig(config Config) error {
	data, err := yaml.Marshal(config)
	if err != nil {
		return err
	}
	return os.WriteFile(configFile, data, 0644)
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

// 支持多语言的颜色显示函数
func (m *Model) formatProfitWithColorLang(profit float64) string {
	if m.language == English {
		// 英文：绿色盈利，红色亏损
		if profit >= 0 {
			return text.FgGreen.Sprintf("+%.2f", profit)
		}
		return text.FgRed.Sprintf("%.2f", profit)
	} else {
		// 中文：红色盈利，绿色亏损
		if profit >= 0 {
			return text.FgRed.Sprintf("+%.2f", profit)
		}
		return text.FgGreen.Sprintf("%.2f", profit)
	}
}

func (m *Model) formatProfitRateWithColorLang(rate float64) string {
	if m.language == English {
		// 英文：绿色盈利，红色亏损
		if rate >= 0 {
			return text.FgGreen.Sprintf("+%.2f%%", rate)
		}
		return text.FgRed.Sprintf("%.2f%%", rate)
	} else {
		// 中文：红色盈利，绿色亏损
		if rate >= 0 {
			return text.FgRed.Sprintf("+%.2f%%", rate)
		}
		return text.FgGreen.Sprintf("%.2f%%", rate)
	}
}

func (m *Model) formatProfitWithColorZeroLang(profit float64) string {
	// 当数值接近0时（考虑浮点数精度），显示白色（无颜色）
	if abs(profit) < 0.001 {
		return fmt.Sprintf("%.2f", profit)
	}
	// 否则使用语言相关颜色逻辑
	return m.formatProfitWithColorLang(profit)
}

func (m *Model) formatProfitRateWithColorZeroLang(rate float64) string {
	// 当数值接近0时（考虑浮点数精度），显示白色（无颜色）
	if abs(rate) < 0.001 {
		return fmt.Sprintf("%.2f%%", rate)
	}
	// 否则使用语言相关颜色逻辑
	return m.formatProfitRateWithColorLang(rate)
}

func (m *Model) formatPriceWithColorLang(currentPrice, prevClose float64) string {
	if prevClose == 0 {
		// 如果昨收价为0，直接显示价格不加颜色
		return fmt.Sprintf("%.3f", currentPrice)
	}

	if currentPrice > prevClose {
		if m.language == English {
			// 英文：高于昨收价显示绿色
			return text.FgGreen.Sprintf("%.3f", currentPrice)
		} else {
			// 中文：高于昨收价显示红色
			return text.FgRed.Sprintf("%.3f", currentPrice)
		}
	} else if currentPrice < prevClose {
		if m.language == English {
			// 英文：低于昨收价显示红色
			return text.FgRed.Sprintf("%.3f", currentPrice)
		} else {
			// 中文：低于昨收价显示绿色
			return text.FgGreen.Sprintf("%.3f", currentPrice)
		}
	} else {
		// 等于昨收价显示白色（无颜色）
		return fmt.Sprintf("%.3f", currentPrice)
	}
}

// 根据数值本身判断颜色显示：0时显示白色，正数红色，负数绿色
func formatProfitWithColorZero(profit float64) string {
	// 当数值接近0时（考虑浮点数精度），显示白色（无颜色）
	if abs(profit) < 0.001 {
		return fmt.Sprintf("%.2f", profit)
	}
	// 否则使用原有颜色逻辑
	return formatProfitWithColor(profit)
}

func formatProfitRateWithColorZero(rate float64) string {
	// 当数值接近0时（考虑浮点数精度），显示白色（无颜色）
	if abs(rate) < 0.001 {
		return fmt.Sprintf("%.2f%%", rate)
	}
	// 否则使用原有颜色逻辑
	return formatProfitRateWithColor(rate)
}

// 辅助函数：计算浮点数绝对值
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// 基于昨收价比较的价格颜色显示函数
func formatPriceWithColor(currentPrice, prevClose float64) string {
	if prevClose == 0 {
		// 如果昨收价为0，直接显示价格不加颜色
		return fmt.Sprintf("%.3f", currentPrice)
	}

	if currentPrice > prevClose {
		// 高于昨收价显示红色
		return text.FgRed.Sprintf("%.3f", currentPrice)
	} else if currentPrice < prevClose {
		// 低于昨收价显示绿色
		return text.FgGreen.Sprintf("%.3f", currentPrice)
	} else {
		// 等于昨收价显示白色（无颜色）
		return fmt.Sprintf("%.3f", currentPrice)
	}
}

func getStockInfo(symbol string) *StockData {
	// 如果输入是中文，尝试通过API搜索
	if containsChineseChars(symbol) {
		return searchChineseStock(symbol)
	}
	return getStockPrice(symbol)
}

// 检查字符串是否包含中文字符
func containsChineseChars(s string) bool {
	for _, r := range s {
		if r >= 0x4e00 && r <= 0x9fff {
			return true
		}
	}
	return false
}

// 通过API搜索中文股票名称
func searchChineseStock(chineseName string) *StockData {
	chineseName = strings.TrimSpace(chineseName)

	// 尝试多种搜索策略
	result := trySearchStrategies(chineseName)

	if result != nil && result.Price > 0 {
		return result
	}

	// 如果所有搜索策略都失败，返回明确的错误信息
	return &StockData{
		Symbol: chineseName,
		Name:   fmt.Sprintf("未找到股票\"%s\"的相关信息", chineseName),
		Price:  0,
	}
}

// 尝试多种搜索策略
func trySearchStrategies(chineseName string) *StockData {
	// 策略1: 尝试常见的股票代码模式
	candidates := generateStockCodeCandidates(chineseName)

	for _, candidate := range candidates {
		result := getStockPrice(candidate)
		if result != nil && result.Price > 0 {
			// 检查返回的股票名称是否与输入匹配
			if isNameMatch(chineseName, result.Name) {
				result.Symbol = candidate
				return result
			}
		}
	}

	// 策略2: 生成模拟数据用于演示
	return generateMockDataForChinese(chineseName)
}

// 生成可能的股票代码候选
func generateStockCodeCandidates(chineseName string) []string {
	var candidates []string

	// 基于常见的股票名称模式生成候选代码
	namePatterns := map[string][]string{
		"茅台":   {"SH600519"},
		"贵州茅台": {"SH600519"},
		"平安":   {"SH601318", "SZ000001"},
		"中国平安": {"SH601318"},
		"平安银行": {"SZ000001"},
		"招商银行": {"SH600036"},
		"工商银行": {"SH601398"},
		"建设银行": {"SH601939"},
		"农业银行": {"SH601288"},
		"中国银行": {"SH601988"},
		"苹果":   {"AAPL"},
		"微软":   {"MSFT"},
		"谷歌":   {"GOOGL"},
		"特斯拉":  {"TSLA"},
		"阿里巴巴": {"BABA"},
		"腾讯":   {"00700.HK"},
		"美团":   {"03690.HK"},
		"小米":   {"01810.HK"},
		"华胜天成": {"SH600410"},
		"用友网络": {"SH600588"},
		"科大讯飞": {"SZ002230"},
		"比亚迪":  {"SZ002594"},
		"宁德时代": {"SZ300750"},
		"五粮液":  {"SZ000858"},
	}

	// 首先检查精确匹配
	if codes, exists := namePatterns[chineseName]; exists {
		candidates = append(candidates, codes...)
	}

	// 然后检查部分匹配
	for pattern, codes := range namePatterns {
		if strings.Contains(chineseName, pattern) || strings.Contains(pattern, chineseName) {
			candidates = append(candidates, codes...)
		}
	}

	return candidates
}

// 检查股票名称是否匹配
func isNameMatch(inputName, stockName string) bool {
	// 简单的名称匹配逻辑
	return strings.Contains(stockName, inputName) || strings.Contains(inputName, stockName)
}

// 为中文输入生成模拟数据
func generateMockDataForChinese(chineseName string) *StockData {
	debugPrint("[调试] 为中文输入生成模拟数据: %s\n", chineseName)

	// 基于名称生成不同的模拟数据
	basePrice := 50.0 + float64((len(chineseName)*7)%100)
	now := time.Now()
	variation := float64((now.Hour()*60+now.Minute())%100) / 100.0
	change := (variation - 0.5) * 4.0
	price := basePrice + change

	// 生成模拟的开盘价、最高价、最低价
	openPrice := price + (variation-0.5)*2.0
	maxPrice := price + float64((now.Second()%10))/10.0*2.0
	minPrice := price - float64((now.Second()%8))/10.0*1.5

	if maxPrice < price {
		maxPrice = price + 0.5
	}
	if minPrice > price {
		minPrice = price - 0.5
	}

	// 计算昨收价（基于当前价格和涨跌额）
	prevClose := price - change

	changePercent := (change / basePrice) * 100
	turnoverRate := float64((now.Minute()+1)%10) + float64(now.Second()%100)/100.0
	volume := int64((now.Hour()*1000000 + now.Minute()*10000 + now.Second()*100) % 50000000)

	return &StockData{
		Symbol:        fmt.Sprintf("模拟-%s", chineseName),
		Name:          fmt.Sprintf("%s (模拟数据)", chineseName),
		Price:         price,
		Change:        change,
		ChangePercent: changePercent,
		StartPrice:    openPrice,
		MaxPrice:      maxPrice,
		MinPrice:      minPrice,
		PrevClose:     prevClose,
		TurnoverRate:  turnoverRate,
		Volume:        volume,
	}
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

	// 解析开盘价、最高价、最低价、换手率、成交量
	var openPrice, maxPrice, minPrice, turnoverRate float64
	var volume int64

	// 腾讯API字段位置：fields[5]=开盘价, fields[33]=最高价, fields[34]=最低价, fields[38]=换手率, fields[36]=成交量
	if len(fields) > 5 {
		openPrice, _ = strconv.ParseFloat(fields[5], 64)
	}
	if len(fields) > 33 {
		maxPrice, _ = strconv.ParseFloat(fields[33], 64)
	}
	if len(fields) > 34 {
		minPrice, _ = strconv.ParseFloat(fields[34], 64)
	}
	if len(fields) > 38 {
		turnoverRate, _ = strconv.ParseFloat(fields[38], 64)
	}
	if len(fields) > 36 {
		volume, _ = strconv.ParseInt(fields[36], 10, 64)
	}

	change := price - previousClose
	changePercent := (change / previousClose) * 100

	debugPrint("[调试] 腾讯API获取成功 - 名称: %s, 价格: %.2f, 涨跌: %.2f (%.2f%%), 开: %.2f, 高: %.2f, 低: %.2f, 换手: %.2f%%, 量: %d\n",
		stockName, price, change, changePercent, openPrice, maxPrice, minPrice, turnoverRate, volume)

	return &StockData{
		Symbol:        symbol,
		Name:          stockName,
		Price:         price,
		Change:        change,
		ChangePercent: changePercent,
		StartPrice:    openPrice,
		MaxPrice:      maxPrice,
		MinPrice:      minPrice,
		PrevClose:     previousClose,
		TurnoverRate:  turnoverRate,
		Volume:        volume,
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
		PrevClose:     previous,
		TurnoverRate:  0,
		Volume:        0,
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

	// 有一定概率生成0变化，用于测试白色显示
	var change float64
	if now.Second()%10 == 0 {
		// 10%概率无变化
		change = 0
	} else {
		change = (variation - 0.5) * 4.0
	}

	price := basePrice + change

	// 生成模拟的开盘价、最高价、最低价
	// 生成不同场景用于测试颜色显示
	var openPrice float64
	switch now.Second() % 4 {
	case 0:
		// 平盘情况：开盘价等于现价（今日涨幅为0%）
		openPrice = price
	case 1:
		// 略微上涨
		openPrice = price - 0.5
	case 2:
		// 略微下跌
		openPrice = price + 0.5
	default:
		// 正常波动
		openPrice = basePrice + (variation-0.5)*2.0
	}

	maxPrice := price + float64((now.Second()%10))/10.0*2.0
	minPrice := price - float64((now.Second()%8))/10.0*1.5

	// 确保最高价 >= 当前价 >= 最低价
	if maxPrice < price {
		maxPrice = price + 0.5
	}
	if minPrice > price {
		minPrice = price - 0.5
	}

	// 平盘情况特殊处理：开盘价=现价时，最高最低价也应该合理
	if openPrice == price {
		// 平盘时，最高价稍高于现价，最低价稍低于现价
		if maxPrice == price {
			maxPrice = price + 0.1
		}
		if minPrice == price {
			minPrice = price - 0.1
		}
	}

	changePercent := (change / basePrice) * 100

	debugPrint("[调试] 模拟数据生成 - 名称: %s, 价格: %.2f, 涨跌: %.2f (%.2f%%), 开: %.2f, 高: %.2f, 低: %.2f\n",
		mockName, price, change, changePercent, openPrice, maxPrice, minPrice)

	// 生成模拟的换手率和成交量
	turnoverRate := float64((now.Minute()+1)%10) + float64(now.Second()%100)/100.0
	volume := int64((now.Hour()*1000000 + now.Minute()*10000 + now.Second()*100) % 50000000)

	return &StockData{
		Symbol:        symbol,
		Name:          mockName,
		Price:         price,
		Change:        change,
		ChangePercent: changePercent,
		StartPrice:    openPrice,
		MaxPrice:      maxPrice,
		MinPrice:      minPrice,
		TurnoverRate:  turnoverRate,
		Volume:        volume,
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

func (m *Model) handleEditingStock(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.state = MainMenu
		m.message = ""
		return m, nil
	case "up", "k", "w":
		if m.editingStep == 0 && m.cursor > 0 {
			m.cursor--
		}
	case "down", "j", "s":
		if m.editingStep == 0 && m.cursor < len(m.portfolio.Stocks)-1 {
			m.cursor++
		}
	case "enter", " ":
		return m.processEditingStep()
	case "backspace":
		if len(m.input) > 0 {
			// 正确处理多字节字符（如中文）的删除
			runes := []rune(m.input)
			if len(runes) > 0 {
				m.input = string(runes[:len(runes)-1])
			}
		}
	default:
		// 改进的输入处理：支持多字节字符（如中文）
		str := msg.String()
		if len(str) > 0 && str != "\n" && str != "\r" && !isControlKey(str) {
			m.input += str
		}
	}
	return m, nil
}

func (m *Model) processEditingStep() (tea.Model, tea.Cmd) {
	switch m.editingStep {
	case 0: // 选择股票
		if len(m.portfolio.Stocks) > 0 {
			m.selectedStockIndex = m.cursor
			m.editingStep = 1
			m.input = fmt.Sprintf("%.3f", m.portfolio.Stocks[m.selectedStockIndex].CostPrice)
		}
	case 1: // 修改成本价
		if m.input == "" {
			m.message = m.getText("costRequired")
			return m, nil
		}
		if newCost, err := strconv.ParseFloat(m.input, 64); err != nil {
			m.message = m.getText("invalidPrice")
			m.input = ""
			return m, nil
		} else {
			m.portfolio.Stocks[m.selectedStockIndex].CostPrice = newCost
			m.editingStep = 2
			m.input = fmt.Sprintf("%d", m.portfolio.Stocks[m.selectedStockIndex].Quantity)
			m.message = ""
		}
	case 2: // 修改数量
		if m.input == "" {
			m.message = m.getText("quantityRequired")
			return m, nil
		}
		if newQuantity, err := strconv.Atoi(m.input); err != nil {
			m.message = m.getText("invalidQuantity")
			m.input = ""
			return m, nil
		} else {
			m.portfolio.Stocks[m.selectedStockIndex].Quantity = newQuantity
			m.savePortfolio()

			stockName := m.portfolio.Stocks[m.selectedStockIndex].Name
			m.state = MainMenu
			m.message = fmt.Sprintf(m.getText("editSuccess"), stockName)
			m.editingStep = 0
			m.input = ""
		}
	}
	return m, nil
}

func (m *Model) viewEditingStock() string {
	s := m.getText("editTitle") + "\n\n"

	switch m.editingStep {
	case 0:
		s += m.getText("selectToEdit") + "\n\n"
		for i, stock := range m.portfolio.Stocks {
			prefix := "  "
			if i == m.cursor {
				prefix = "► "
			}
			// 根据语言显示不同的格式
			if m.language == Chinese {
				s += fmt.Sprintf("%s%d. %s (%s) - 成本价: %.3f, 数量: %d\n",
					prefix, i+1, stock.Name, stock.Code, stock.CostPrice, stock.Quantity)
			} else {
				s += fmt.Sprintf("%s%d. %s (%s) - Cost: %.3f, Quantity: %d\n",
					prefix, i+1, stock.Name, stock.Code, stock.CostPrice, stock.Quantity)
			}
		}
		s += "\n" + m.getText("navHelp") + "\n"
	case 1:
		stock := m.portfolio.Stocks[m.selectedStockIndex]
		if m.language == Chinese {
			s += fmt.Sprintf("股票: %s (%s)\n", stock.Name, stock.Code)
		} else {
			s += fmt.Sprintf("Stock: %s (%s)\n", stock.Name, stock.Code)
		}
		s += fmt.Sprintf(m.getText("currentCost"), stock.CostPrice) + "\n\n"
		s += m.getText("enterNewCost") + m.input + "_\n"
		s += "\n" + m.getText("returnToMenuShort") + "\n"
	case 2:
		stock := m.portfolio.Stocks[m.selectedStockIndex]
		if m.language == Chinese {
			s += fmt.Sprintf("股票: %s (%s)\n", stock.Name, stock.Code)
		} else {
			s += fmt.Sprintf("Stock: %s (%s)\n", stock.Name, stock.Code)
		}
		s += fmt.Sprintf(m.getText("newCost"), stock.CostPrice) + "\n"
		s += fmt.Sprintf(m.getText("currentQuantity"), stock.Quantity) + "\n\n"
		s += m.getText("enterNewQuantity") + m.input + "_\n"
		s += "\n" + m.getText("returnToMenuShort") + "\n"
	}

	if m.message != "" {
		s += "\n" + m.message + "\n"
	}

	return s
}

func (m *Model) handleSearchingStock(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.state = MainMenu
		m.message = ""
		return m, nil
	case "enter":
		if m.searchInput == "" {
			m.message = m.getText("enterSearch")[:len(m.getText("enterSearch"))-2] // 去掉": "后缀
			return m, nil
		}
		m.message = m.getText("searching")
		m.searchResult = getStockInfo(m.searchInput)
		if m.searchResult == nil || m.searchResult.Name == "" {
			m.message = fmt.Sprintf(m.getText("searchNotFound"), m.searchInput)
			return m, nil
		}
		m.state = SearchResult
		m.message = ""
		return m, nil
	case "backspace":
		if len(m.searchInput) > 0 {
			// 正确处理多字节字符（如中文）的删除
			runes := []rune(m.searchInput)
			if len(runes) > 0 {
				m.searchInput = string(runes[:len(runes)-1])
			}
		}
	default:
		// 改进的输入处理：支持多字节字符（如中文）
		str := msg.String()
		if len(str) > 0 && str != "\n" && str != "\r" && !isControlKey(str) {
			m.searchInput += str
		}
	}
	return m, nil
}

func (m *Model) handleSearchResult(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.state = MainMenu
		m.message = ""
		return m, nil
	case "r":
		m.state = SearchingStock
		m.message = ""
		return m, nil
	}
	return m, nil
}

func (m *Model) viewSearchingStock() string {
	s := m.getText("searchTitle") + "\n\n"
	s += m.getText("enterSearch") + m.searchInput + "_\n\n"
	s += m.getText("searchFormats") + "\n\n"
	s += m.getText("searchHelp") + "\n"

	if m.message != "" {
		s += "\n" + m.message + "\n"
	}

	return s
}

func (m *Model) viewSearchResult() string {
	s := m.getText("detailTitle") + "\n\n"

	if m.searchResult == nil {
		s += m.getText("noInfo") + "\n"
		s += "\n" + m.getText("detailHelp") + "\n"
		return s
	}

	// 创建横向表格显示股票详细信息
	t := table.NewWriter()
	t.SetStyle(table.StyleLight)

	// 构建表头和数据行
	var headers []interface{}
	var values []interface{}

	// 基本信息
	if m.language == Chinese {
		headers = append(headers, "股票代码", "股票名称", "现价")
	} else {
		headers = append(headers, "Code", "Name", "Price")
	}
	values = append(values, m.searchResult.Symbol, m.searchResult.Name, m.formatPriceWithColorLang(m.searchResult.Price, m.searchResult.PrevClose))

	// 昨收价
	if m.searchResult.PrevClose > 0 {
		if m.language == Chinese {
			headers = append(headers, "昨收价")
		} else {
			headers = append(headers, "Prev Close")
		}
		values = append(values, fmt.Sprintf("%.3f", m.searchResult.PrevClose))
	}

	// 价格信息（有数据时才显示）
	if m.searchResult.StartPrice > 0 {
		if m.language == Chinese {
			headers = append(headers, "开盘价")
		} else {
			headers = append(headers, "Open")
		}
		values = append(values, m.formatPriceWithColorLang(m.searchResult.StartPrice, m.searchResult.PrevClose))
	}
	if m.searchResult.MaxPrice > 0 {
		if m.language == Chinese {
			headers = append(headers, "最高价")
		} else {
			headers = append(headers, "High")
		}
		values = append(values, m.formatPriceWithColorLang(m.searchResult.MaxPrice, m.searchResult.PrevClose))
	}
	if m.searchResult.MinPrice > 0 {
		if m.language == Chinese {
			headers = append(headers, "最低价")
		} else {
			headers = append(headers, "Low")
		}
		values = append(values, m.formatPriceWithColorLang(m.searchResult.MinPrice, m.searchResult.PrevClose))
	}

	// 涨跌信息
	if m.searchResult.Change != 0 {
		if m.language == Chinese {
			headers = append(headers, "涨跌额")
		} else {
			headers = append(headers, "Change")
		}
		changeStr := m.formatProfitWithColorZeroLang(m.searchResult.Change)
		values = append(values, changeStr)
	}
	if m.searchResult.ChangePercent != 0 {
		if m.language == Chinese {
			headers = append(headers, "今日涨幅")
		} else {
			headers = append(headers, "Change %")
		}
		changePercentStr := m.formatProfitRateWithColorZeroLang(m.searchResult.ChangePercent)
		values = append(values, changePercentStr)
	}

	// 换手率
	if m.searchResult.TurnoverRate > 0 {
		if m.language == Chinese {
			headers = append(headers, "换手率")
		} else {
			headers = append(headers, "Turnover")
		}
		values = append(values, fmt.Sprintf("%.2f%%", m.searchResult.TurnoverRate))
	}

	// 买入量（成交量）
	if m.searchResult.Volume > 0 {
		if m.language == Chinese {
			headers = append(headers, "成交量")
		} else {
			headers = append(headers, "Volume")
		}
		volumeStr := formatVolume(m.searchResult.Volume)
		values = append(values, volumeStr)
	}

	// 添加表头和数据行
	t.AppendHeader(table.Row(headers))
	t.AppendRow(table.Row(values))

	s += t.Render() + "\n\n"
	s += m.getText("detailHelp") + "\n"

	return s
}

func formatVolume(volume int64) string {
	if volume >= 1000000000 {
		return fmt.Sprintf("%.2f十亿", float64(volume)/1000000000)
	} else if volume >= 100000000 {
		return fmt.Sprintf("%.2f亿", float64(volume)/100000000)
	} else if volume >= 10000 {
		return fmt.Sprintf("%.2f万", float64(volume)/10000)
	} else {
		return fmt.Sprintf("%d", volume)
	}
}

// 检查是否为控制键
func isControlKey(str string) bool {
	if len(str) == 0 {
		return true
	}

	// 检查常见的控制键序列
	controlKeys := []string{
		"ctrl+c", "ctrl+d", "ctrl+z", "ctrl+l", "ctrl+r",
		"alt+", "cmd+", "shift+", "ctrl+",
		"up", "down", "left", "right",
		"home", "end", "pgup", "pgdown",
		"f1", "f2", "f3", "f4", "f5", "f6", "f7", "f8", "f9", "f10", "f11", "f12",
		"insert", "delete", "tab",
	}

	for _, key := range controlKeys {
		if strings.HasPrefix(strings.ToLower(str), key) {
			return true
		}
	}

	// 检查单个字符的控制字符（ASCII < 32，除了可打印字符）
	if len(str) == 1 {
		r := rune(str[0])
		if r < 32 && r != '\t' {
			return true
		}
	}

	return false
}

func (m *Model) handleLanguageSelection(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.state = MainMenu
		return m, nil
	case "up", "k", "w":
		if m.languageCursor > 0 {
			m.languageCursor--
		}
	case "down", "j", "s":
		if m.languageCursor < 1 { // 0: Chinese, 1: English
			m.languageCursor++
		}
	case "enter", " ":
		// 选择语言
		if m.languageCursor == 0 {
			m.language = Chinese
			m.config.System.Language = "zh"
		} else {
			m.language = English
			m.config.System.Language = "en"
		}
		// 保存配置到文件
		if err := saveConfig(m.config); err != nil && m.debugMode {
			m.message = fmt.Sprintf("Warning: Failed to save config: %v", err)
		}
		// 更新菜单项
		m.menuItems = m.getMenuItems()
		m.state = MainMenu
		m.message = ""
		return m, nil
	}
	return m, nil
}

func (m *Model) viewLanguageSelection() string {
	s := m.getText("languageTitle") + "\n\n"
	s += m.getText("selectLanguage") + "\n\n"

	// 语言选项
	languages := []string{
		"中文简体",
		"English",
	}

	for i, lang := range languages {
		prefix := "  "
		if i == m.languageCursor {
			prefix = "► "
		}
		s += fmt.Sprintf("%s%s\n", prefix, lang)
	}

	s += "\n" + m.getText("languageHelp") + "\n"

	return s
}

func gbkToUtf8(data []byte) (string, error) {
	reader := transform.NewReader(strings.NewReader(string(data)), simplifiedchinese.GBK.NewDecoder())
	utf8Data, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	return string(utf8Data), nil
}
