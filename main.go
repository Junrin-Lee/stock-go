package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
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
	Symbol        string   `json:"symbol"`
	Name          string   `json:"name"`
	Price         float64  `json:"price"`
	Change        float64  `json:"change"`
	ChangePercent float64  `json:"change_percent"`
	StartPrice    float64  `json:"start_price"`
	MaxPrice      float64  `json:"max_price"`
	MinPrice      float64  `json:"min_price"`
	PrevClose     float64  `json:"prev_close"` // 昨日收盘价
	TurnoverRate  float64  `json:"turnover_rate"`
	Volume        int64    `json:"volume"`
	FundFlow      FundFlow `json:"fund_flow"` // 资金流向数据
}

type Portfolio struct {
	Stocks []Stock `json:"stocks"`
}

// 资金流向数据结构
type FundFlow struct {
	MainNetInflow       float64 `json:"main_net_inflow"`        // 主力净流入净额
	SuperLargeNetInflow float64 `json:"super_large_net_inflow"` // 超大单净流入
	LargeNetInflow      float64 `json:"large_net_inflow"`       // 大单净流入
	MediumNetInflow     float64 `json:"medium_net_inflow"`      // 中单净流入
	SmallNetInflow      float64 `json:"small_net_inflow"`       // 小单净流入
	NetInflowRatio      float64 `json:"net_inflow_ratio"`       // 净流入占比
	ActiveBuyAmount     float64 `json:"active_buy_amount"`      // 主动买入金额
	ActiveSellAmount    float64 `json:"active_sell_amount"`     // 主动卖出金额
}

// 资金流向缓存条目
type FundFlowCacheEntry struct {
	Data       FundFlow  `json:"data"`        // 资金流向数据
	UpdateTime time.Time `json:"update_time"` // 数据更新时间
	IsUpdating bool      `json:"is_updating"` // 是否正在更新中
}

// 股价缓存条目结构
type StockPriceCacheEntry struct {
	Data       *StockData `json:"data"`        // 股价数据
	UpdateTime time.Time  `json:"update_time"` // 数据更新时间
	IsUpdating bool       `json:"is_updating"` // 是否正在更新中
}

// 自选股票数据结构
type WatchlistStock struct {
	Code     string   `json:"code"`
	Name     string   `json:"name"`
	Tags     []string `json:"tags"`      // 标签字段，支持多个标签
	FundFlow FundFlow `json:"fund_flow"` // 资金流向数据
}

type Watchlist struct {
	Stocks []WatchlistStock `json:"stocks"`
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
	Language      string `yaml:"language"`       // 默认语言 "zh" 或 "en"
	AutoStart     bool   `yaml:"auto_start"`     // 有数据时自动进入监控模式
	StartupModule string `yaml:"startup_module"` // 启动模块 "portfolio"(持股) 或 "watchlist"(自选)
	DebugMode     bool   `yaml:"debug_mode"`     // 调试模式开关
}

type DisplayConfig struct {
	ColorScheme   string `yaml:"color_scheme"`   // 颜色方案 "professional", "simple"
	DecimalPlaces int    `yaml:"decimal_places"` // 价格显示小数位数
	TableStyle    string `yaml:"table_style"`    // 表格样式 "light", "bold", "simple"
	MaxLines      int    `yaml:"max_lines"`      // 列表每页最大显示行数
}

type UpdateConfig struct {
	RefreshInterval int  `yaml:"refresh_interval"` // 刷新间隔（秒）
	AutoUpdate      bool `yaml:"auto_update"`      // 是否自动更新
}

// 文本映射结构
type TextMap map[string]string

// i18n 配置
var texts map[Language]TextMap

// 加载 i18n 文件
func loadI18nFiles() {
	texts = make(map[Language]TextMap)

	// 读取中文配置
	if zhData, err := os.ReadFile("i18n/zh.json"); err == nil {
		var zhTexts TextMap
		if err := json.Unmarshal(zhData, &zhTexts); err == nil {
			texts[Chinese] = zhTexts
		} else {
			fmt.Printf("Warning: Failed to parse i18n/zh.json: %v\n", err)
		}
	} else {
		fmt.Printf("Warning: Failed to read i18n/zh.json: %v\n", err)
	}

	// 读取英文配置
	if enData, err := os.ReadFile("i18n/en.json"); err == nil {
		var enTexts TextMap
		if err := json.Unmarshal(enData, &enTexts); err == nil {
			texts[English] = enTexts
		} else {
			fmt.Printf("Warning: Failed to parse i18n/en.json: %v\n", err)
		}
	} else {
		fmt.Printf("Warning: Failed to read i18n/en.json: %v\n", err)
	}

	// 如果没有成功加载任何语言文件，退出程序
	if len(texts) == 0 {
		fmt.Println("Error: No i18n files could be loaded. Please ensure i18n/zh.json and i18n/en.json exist.")
		os.Exit(1)
	}
}

type Model struct {
	state           AppState
	currentMenuItem int
	menuItems       []string
	cursor          int
	input           string
	message         string
	portfolio       Portfolio
	watchlist       Watchlist // 自选股票列表
	config          Config    // 系统配置
	debugMode       bool
	language        Language
	debugLogs       []string // 调试日志存储
	debugScrollPos  int      // debug日志滚动位置

	// For stock addition
	addingStep    int
	tempCode      string
	tempCost      string
	tempQuantity  string
	stockInfo     *StockData
	fromSearch    bool     // 标记是否从搜索结果添加
	previousState AppState // 记录进入编辑/删除前的状态

	// For stock editing
	editingStep        int
	selectedStockIndex int

	// For stock searching
	searchInput         string
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
	selectedTag      string   // 当前选择的标签过滤
	availableTags    []string // 所有可用的标签列表
	tagInput         string   // 标签输入框内容
	tagSelectCursor  int      // 标签选择界面的游标位置
	currentStockTags []string // 当前选中股票的标签列表（用于删除管理）
	tagManageCursor  int      // 标签管理界面的游标位置
	tagRemoveCursor  int      // 标签删除选择界面的游标位置
	isInRemoveMode   bool     // 是否处于删除模式

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

	// For fund flow async data - 资金流向异步数据
	fundFlowCache      map[string]*FundFlowCacheEntry // 资金流向数据缓存
	fundFlowMutex      sync.RWMutex                   // 资金流向数据读写锁
	fundFlowUpdateTime time.Time                      // 上次更新资金流向数据的时间
	fundFlowContext    context.Context                // 资金流向异步获取的上下文
	fundFlowCancel     context.CancelFunc             // 取消资金流向异步获取的函数

	// For stock price async data - 股价异步数据
	stockPriceCache      map[string]*StockPriceCacheEntry // 股价数据缓存
	stockPriceMutex      sync.RWMutex                     // 股价数据读写锁
	stockPriceUpdateTime time.Time                        // 上次更新股价数据的时间
}

type tickMsg struct{}

// 资金流向数据更新消息
type fundFlowUpdateMsg struct {
	Symbol string
	Data   *FundFlow
	Error  error
}

// 股价数据更新消息
type stockPriceUpdateMsg struct {
	Symbol string
	Data   *StockData
	Error  error
}

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

// 获取主菜单项
func (m *Model) getMenuItems() []string {
	return []string{
		m.getText("stockList"),
		m.getText("watchlist"),
		m.getText("stockSearch"),
		m.getText("debugMode"),
		m.getText("language"),
		m.getText("exit"),
	}
}

func main() {
	// 确保目录存在
	os.MkdirAll("data", 0755)
	os.MkdirAll("cmd/conf", 0755)
	os.MkdirAll("i18n", 0755)

	// 加载 i18n 文件
	loadI18nFiles()

	// 加载配置文件
	config := loadConfig()
	portfolio := loadPortfolio()
	watchlist := loadWatchlist()

	// 根据配置和是否有股票数据决定初始状态
	initialState := MainMenu
	var lastUpdate time.Time
	if config.System.AutoStart {
		// 根据startup_module配置决定启动哪个模块
		switch config.System.StartupModule {
		case "portfolio":
			// 启动持股模块，需要有持股数据
			if len(portfolio.Stocks) > 0 {
				initialState = Monitoring
				lastUpdate = time.Now()
			}
		case "watchlist":
			// 启动自选模块，需要有自选数据
			if len(watchlist.Stocks) > 0 {
				initialState = WatchlistViewing
				lastUpdate = time.Now()
			}
		default:
			// 默认行为：如果有持股数据则进入持股模块
			if len(portfolio.Stocks) > 0 {
				initialState = Monitoring
				lastUpdate = time.Now()
			}
		}
	}

	// 根据配置文件设置语言
	language := English // 默认英文
	if config.System.Language == "zh" {
		language = Chinese
	}

	// 创建资金流向异步上下文
	fundFlowCtx, fundFlowCancel := context.WithCancel(context.Background())

	m := Model{
		state:              initialState,
		currentMenuItem:    0,
		portfolio:          portfolio,
		watchlist:          watchlist,
		config:             config,
		debugMode:          config.System.DebugMode,
		language:           language,
		lastUpdate:         lastUpdate,
		debugLogs:          make([]string, 0),
		debugScrollPos:     0,     // 初始滚动位置
		portfolioScrollPos: 0,     // 持股列表滚动位置
		watchlistScrollPos: 0,     // 自选列表滚动位置
		portfolioCursor:    0,     // 持股列表游标
		watchlistCursor:    0,     // 自选列表游标
		portfolioIsSorted:  false, // 持股列表默认未排序状态
		watchlistIsSorted:  false, // 自选列表默认未排序状态
		// 资金流向缓存初始化
		fundFlowCache:      make(map[string]*FundFlowCacheEntry),
		fundFlowUpdateTime: time.Time{}, // 初始化为零时间
		fundFlowContext:    fundFlowCtx,
		fundFlowCancel:     fundFlowCancel,
		// 股价缓存初始化
		stockPriceCache:      make(map[string]*StockPriceCacheEntry),
		stockPriceUpdateTime: time.Time{}, // 初始化为零时间
	}

	// 根据语言设置菜单项
	m.menuItems = m.getMenuItems()

	// 设置全局模型引用用于调试日志
	globalModel = &m

	p := tea.NewProgram(&m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}

func (m *Model) Init() tea.Cmd {
	if m.state == Monitoring || m.state == WatchlistViewing {
		return m.tickCmd()
	}
	return nil
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var newModel tea.Model
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// debug滚动快捷键，在任何状态下都可用
		if m.debugMode {
			keyStr := msg.String()

			switch keyStr {
			case "pgup":
				m.scrollDebugUp()
				return m, nil
			case "pgdown":
				m.scrollDebugDown()
				return m, nil
			case "home":
				m.scrollDebugToTop()
				return m, nil
			case "end":
				m.scrollDebugToBottom()
				return m, nil
			}
		}

		// 持股列表和自选列表滚动快捷键
		if m.state == Monitoring || m.state == WatchlistViewing {
			keyStr := msg.String()
			switch keyStr {
			case "up":
				if m.state == Monitoring {
					m.scrollPortfolioUp()
				} else {
					m.scrollWatchlistUp()
				}
				return m, nil
			case "down":
				if m.state == Monitoring {
					m.scrollPortfolioDown()
				} else {
					m.scrollWatchlistDown()
				}
				return m, nil
			}
		}

		// 处理各状态的正常按键
		switch m.state {
		case MainMenu:
			newModel, cmd = m.handleMainMenu(msg)
		case AddingStock:
			newModel, cmd = m.handleAddingStock(msg)
		case Monitoring:
			newModel, cmd = m.handleMonitoring(msg)
		case EditingStock:
			newModel, cmd = m.handleEditingStock(msg)
		case SearchingStock:
			newModel, cmd = m.handleSearchingStock(msg)
		case SearchResult:
			newModel, cmd = m.handleSearchResult(msg)
		case SearchResultWithActions:
			newModel, cmd = m.handleSearchResultWithActions(msg)
		case WatchlistSearchConfirm:
			newModel, cmd = m.handleWatchlistSearchConfirm(msg)
		case LanguageSelection:
			newModel, cmd = m.handleLanguageSelection(msg)
		case WatchlistViewing:
			newModel, cmd = m.handleWatchlistViewing(msg)
		case WatchlistTagging:
			newModel, cmd = m.handleWatchlistTagging(msg)
		case WatchlistTagSelect:
			newModel, cmd = m.handleWatchlistTagSelect(msg)
		case WatchlistTagManage:
			newModel, cmd = m.handleWatchlistTagManage(msg)
		case WatchlistTagRemoveSelect:
			newModel, cmd = m.handleWatchlistTagRemoveSelect(msg)
		case WatchlistGroupSelect:
			newModel, cmd = m.handleWatchlistGroupSelect(msg)
		case PortfolioSorting:
			newModel, cmd = m.handlePortfolioSorting(msg)
		case WatchlistSorting:
			newModel, cmd = m.handleWatchlistSorting(msg)
		default:
			newModel, cmd = m, nil
		}
	case tickMsg:
		if m.state == Monitoring || m.state == WatchlistViewing {
			m.lastUpdate = time.Now()

			// 启动异步数据更新
			var cmds []tea.Cmd
			cmds = append(cmds, m.tickCmd())

			// 启动股价数据更新（持股和自选页面都需要）
			if stockPriceCmd := m.startStockPriceUpdates(); stockPriceCmd != nil {
				cmds = append(cmds, stockPriceCmd)
			}

			if m.state == WatchlistViewing {
				if fundFlowCmd := m.startFundFlowUpdates(); fundFlowCmd != nil {
					cmds = append(cmds, fundFlowCmd)
				}
			}

			newModel, cmd = m, tea.Batch(cmds...)
		} else {
			newModel, cmd = m, nil
		}
	case fundFlowUpdateMsg:
		// 处理资金流向数据更新
		if msg.Error == nil && msg.Data != nil {
			// 更新缓存
			m.fundFlowMutex.Lock()
			if entry, exists := m.fundFlowCache[msg.Symbol]; exists {
				entry.Data = *msg.Data
				entry.UpdateTime = time.Now()
				entry.IsUpdating = false
			} else {
				m.fundFlowCache[msg.Symbol] = &FundFlowCacheEntry{
					Data:       *msg.Data,
					UpdateTime: time.Now(),
					IsUpdating: false,
				}
			}
			m.fundFlowMutex.Unlock()
			debugPrint("[信息] 资金流向缓存已更新: %s\n", msg.Symbol)
		} else {
			// 更新失败，标记为未更新状态
			m.fundFlowMutex.Lock()
			if entry, exists := m.fundFlowCache[msg.Symbol]; exists {
				entry.IsUpdating = false
			}
			m.fundFlowMutex.Unlock()
			debugPrint("[错误] 资金流向数据更新失败: %s, %v\n", msg.Symbol, msg.Error)
		}
		newModel, cmd = m, nil
	case stockPriceUpdateMsg:
		// 处理股价数据更新
		if msg.Error == nil && msg.Data != nil {
			// 更新缓存
			m.stockPriceMutex.Lock()
			if entry, exists := m.stockPriceCache[msg.Symbol]; exists {
				entry.Data = msg.Data
				entry.UpdateTime = time.Now()
				entry.IsUpdating = false
			} else {
				m.stockPriceCache[msg.Symbol] = &StockPriceCacheEntry{
					Data:       msg.Data,
					UpdateTime: time.Now(),
					IsUpdating: false,
				}
			}
			m.stockPriceMutex.Unlock()
			debugPrint("[信息] 股价缓存已更新: %s\n", msg.Symbol)
		} else {
			// 更新失败，标记为未更新状态
			m.stockPriceMutex.Lock()
			if entry, exists := m.stockPriceCache[msg.Symbol]; exists {
				entry.IsUpdating = false
			}
			m.stockPriceMutex.Unlock()
			debugPrint("[错误] 股价数据更新失败: %s, %v\n", msg.Symbol, msg.Error)
		}
		newModel, cmd = m, nil
	default:
		newModel, cmd = m, nil
	}

	// 更新全局模型引用以保持调试日志同步
	if newModel != nil {
		if modelPtr, ok := newModel.(*Model); ok {
			globalModel = modelPtr
		}
	}

	return newModel, cmd
}

func (m *Model) View() string {
	var mainContent string
	switch m.state {
	case MainMenu:
		mainContent = m.viewMainMenu()
	case AddingStock:
		mainContent = m.viewAddingStock()
	case Monitoring:
		mainContent = m.viewMonitoring()
	case EditingStock:
		mainContent = m.viewEditingStock()
	case SearchingStock:
		mainContent = m.viewSearchingStock()
	case SearchResult:
		mainContent = m.viewSearchResult()
	case SearchResultWithActions:
		mainContent = m.viewSearchResultWithActions()
	case WatchlistSearchConfirm:
		mainContent = m.viewWatchlistSearchConfirm()
	case LanguageSelection:
		mainContent = m.viewLanguageSelection()
	case WatchlistViewing:
		mainContent = m.viewWatchlistViewing()
	case WatchlistTagging:
		mainContent = m.viewWatchlistTagging()
	case WatchlistTagSelect:
		mainContent = m.viewWatchlistTagSelect()
	case WatchlistTagManage:
		mainContent = m.viewWatchlistTagManage()
	case WatchlistTagRemoveSelect:
		mainContent = m.viewWatchlistTagRemoveSelect()
	case WatchlistGroupSelect:
		mainContent = m.viewWatchlistGroupSelect()
	case PortfolioSorting:
		mainContent = m.viewPortfolioSorting()
	case WatchlistSorting:
		mainContent = m.viewWatchlistSorting()
	default:
		mainContent = ""
	}

	// 添加调试面板
	return mainContent + m.renderDebugPanel()
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
	m.message = "" // 清除之前的消息
	switch m.currentMenuItem {
	case 0: // 股票列表
		m.logUserAction("进入持股监控页面")
		m.state = Monitoring
		m.resetPortfolioCursor() // 重置游标到第一只股票
		m.lastUpdate = time.Now()
		return m, m.tickCmd()
	case 1: // 自选股票
		m.logUserAction("进入自选股票页面")
		m.state = WatchlistViewing
		m.resetWatchlistCursor() // 重置游标到第一只股票
		m.cursor = 0
		m.message = ""
		m.lastUpdate = time.Now()

		// 立即启动数据更新，而不等待定时器
		var cmds []tea.Cmd
		cmds = append(cmds, m.tickCmd())

		// 强制启动股价数据更新
		if stockPriceCmd := m.startStockPriceUpdates(); stockPriceCmd != nil {
			cmds = append(cmds, stockPriceCmd)
		}

		// 强制启动资金流向数据更新
		if fundFlowCmd := m.startFundFlowUpdates(); fundFlowCmd != nil {
			cmds = append(cmds, fundFlowCmd)
		}

		return m, tea.Batch(cmds...)
	case 2: // 股票搜索
		m.logUserAction("进入股票搜索页面")
		m.state = SearchingStock
		m.searchInput = ""
		m.searchResult = nil
		m.searchFromWatchlist = false
		m.message = ""
		return m, nil
	case 3: // 调试模式
		if m.debugMode {
			m.logUserAction("关闭调试模式")
		} else {
			m.logUserAction("开启调试模式")
		}
		m.debugMode = !m.debugMode
		m.config.System.DebugMode = m.debugMode
		// 保存配置到文件
		if err := saveConfig(m.config); err != nil && m.debugMode {
			m.message = fmt.Sprintf("Warning: Failed to save config: %v", err)
		}
		return m, nil
	case 4: // 语言选择页面
		m.logUserAction("进入语言选择页面")
		m.state = LanguageSelection
		m.languageCursor = 0
		if m.language == English {
			m.languageCursor = 1
		}
		return m, nil
	case 5: // 退出
		m.logUserAction("用户退出程序")
		m.savePortfolio()
		m.saveWatchlist()
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

		if item == "debugMode" {
			debugStatus := m.getText("off")
			if m.debugMode {
				debugStatus = m.getText("on")
			}
			s += fmt.Sprintf("%s%s: %s\n", prefix, item, debugStatus)
		} else if i == 4 { // 语言选择
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
	s += "==================================================\n"

	if m.message != "" {
		s += "\n" + m.message + "\n"
	}

	return s
}

func (m *Model) handleAddingStock(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		// 根据来源决定返回目标
		if m.fromSearch {
			// 从持股列表或搜索结果进入，返回相应页面
			if m.previousState == Monitoring {
				m.state = Monitoring
				m.resetPortfolioCursor() // 重置游标到第一只股票
				m.lastUpdate = time.Now()
			} else {
				m.state = SearchResultWithActions
			}
			m.fromSearch = false // 重置标志
		} else {
			m.state = MainMenu
		}
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
	case 0: // 搜索股票
		if m.input == "" {
			m.message = m.getText("codeRequired")
			return m, nil
		}
		m.message = m.getText("searching")

		// 使用搜索功能
		var stockData *StockData
		if containsChineseChars(m.input) {
			stockData = searchChineseStock(m.input)
		} else {
			// 对于非中文输入，先尝试直接获取价格，然后尝试搜索
			stockData = getStockPrice(m.input)

			// 如果直接获取失败，尝试作为搜索关键词搜索
			if stockData == nil || stockData.Price <= 0 {
				debugPrint("[调试] 添加股票时直接获取价格失败，尝试通过搜索查找: %s\n", m.input)
				stockData = searchStockBySymbol(m.input)
			}
		}

		if stockData == nil || stockData.Name == "" {
			m.message = fmt.Sprintf(m.getText("searchNotFound"), m.input)
			m.input = ""
			return m, nil
		}

		// 保存搜索结果并转到输入成本价步骤
		m.stockInfo = stockData
		m.tempCode = stockData.Symbol
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
		m.portfolioIsSorted = false // 添加股票后重置持股列表排序状态

		// 根据来源决定跳转目标
		if m.fromSearch {
			// 从搜索结果添加，跳转到持股列表（监控）页面
			m.state = Monitoring
			m.resetPortfolioCursor() // 重置游标到第一只股票
			m.lastUpdate = time.Now()
			m.fromSearch = false // 重置标志
			m.message = fmt.Sprintf(m.getText("addSuccess"), m.stockInfo.Name, m.tempCode)
			m.addingStep = 0
			m.input = ""
			return m, m.tickCmd() // 跳转到监控页面时启动定时器
		} else {
			// 从主菜单添加，返回主菜单
			m.state = MainMenu
			m.message = fmt.Sprintf(m.getText("addSuccess"), m.stockInfo.Name, m.tempCode)
			m.addingStep = 0
			m.input = ""
			return m, nil
		}
	}
	return m, nil
}

func (m *Model) viewAddingStock() string {
	s := m.getText("addingTitle") + "\n\n"

	switch m.addingStep {
	case 0:
		s += m.getText("enterSearch") + m.input + "_\n"
		s += "\n" + m.getText("searchFormats") + "\n"
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

	s += "\n" + m.getText("returnEscOnly") + "\n"

	if m.message != "" {
		s += "\n" + m.message + "\n"
	}

	return s
}

func (m *Model) handleMonitoring(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "m":
		m.state = MainMenu
		m.message = "" // 清除消息
		return m, nil
	case "e":
		// 编辑当前光标指向的股票
		if len(m.portfolio.Stocks) == 0 {
			m.message = m.getText("emptyPortfolio")
			return m, nil
		}
		m.logUserAction("从持股列表进入编辑股票页面")
		m.previousState = m.state // 记录当前状态
		m.state = EditingStock
		m.editingStep = 1 // 开始编辑成本价
		m.selectedStockIndex = m.portfolioCursor
		m.tempCode = m.portfolio.Stocks[m.portfolioCursor].Code
		m.tempCost = ""
		m.tempQuantity = ""
		m.input = fmt.Sprintf("%.2f", m.portfolio.Stocks[m.portfolioCursor].CostPrice) // 预填充当前成本价
		m.message = ""
		return m, nil
	case "d":
		// 直接删除光标指向的股票
		if len(m.portfolio.Stocks) == 0 {
			m.message = m.getText("emptyPortfolio")
			return m, nil
		}
		// 删除当前光标指向的股票
		removedStock := m.portfolio.Stocks[m.portfolioCursor]
		m.portfolio.Stocks = append(m.portfolio.Stocks[:m.portfolioCursor], m.portfolio.Stocks[m.portfolioCursor+1:]...)
		m.savePortfolio()
		m.portfolioIsSorted = false // 删除股票后重置持股列表排序状态
		// 调整光标位置
		if m.portfolioCursor >= len(m.portfolio.Stocks) && len(m.portfolio.Stocks) > 0 {
			m.portfolioCursor = len(m.portfolio.Stocks) - 1
		}
		m.message = fmt.Sprintf(m.getText("removeSuccess"), removedStock.Name, removedStock.Code)
		return m, nil
	case "a":
		// 跳转到添加股票页面
		m.logUserAction("从持股列表跳转到添加股票页面")
		m.previousState = m.state // 记录当前状态
		m.state = AddingStock
		m.addingStep = 0
		m.tempCode = ""
		m.tempCost = ""
		m.tempQuantity = ""
		m.stockInfo = nil
		m.input = ""
		m.message = ""
		m.fromSearch = true // 设置标志，表示从持股列表进入，完成后应该回到监控页面
		return m, nil
	case "s":
		// 进入排序菜单
		m.logUserAction("从持股列表进入排序菜单")
		m.state = PortfolioSorting
		// 智能定位光标到当前排序字段
		m.portfolioSortCursor = m.findSortFieldIndex(m.portfolioSortField, true)
		m.message = ""
		return m, nil
	case "up", "k", "w":
		if m.portfolioCursor > 0 {
			m.portfolioCursor--
		}
		return m, nil
	case "down", "j":
		if m.portfolioCursor < len(m.portfolio.Stocks)-1 {
			m.portfolioCursor++
		}
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
		s += m.getText("holdingsHelp") + "\n"
		return s
	}

	t := table.NewWriter()
	t.SetStyle(table.StyleLight)

	// 获取带排序指示器的表头
	t.AppendHeader(m.getPortfolioHeaderWithSortIndicator())

	var totalMarketValue float64
	var totalCost float64

	// 显示滚动信息
	totalStocks := len(m.portfolio.Stocks)
	maxPortfolioLines := m.config.Display.MaxLines
	if totalStocks > 0 {
		currentPos := m.portfolioCursor + 1 // 显示从1开始的位置
		if m.language == Chinese {
			s += fmt.Sprintf("📊 持股列表 (%d/%d) [↑/↓:翻页]\n", currentPos, totalStocks)
		} else {
			s += fmt.Sprintf("📊 Portfolio (%d/%d) [↑/↓:scroll]\n", currentPos, totalStocks)
		}
		s += "\n"
	}

	// 计算要显示的股票范围
	stocks := m.portfolio.Stocks
	endIndex := len(stocks) - m.portfolioScrollPos
	startIndex := endIndex - maxPortfolioLines
	if startIndex < 0 {
		startIndex = 0
	}
	if endIndex > len(stocks) {
		endIndex = len(stocks)
	}

	// 首先计算所有股票的总计（用于汇总行）
	for i := range m.portfolio.Stocks {
		stock := &m.portfolio.Stocks[i]
		// 从缓存获取股价数据（非阻塞）
		stockData := m.getStockPriceFromCache(stock.Code)
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
			marketValue := stock.Price * float64(stock.Quantity)
			cost := stock.CostPrice * float64(stock.Quantity)

			totalMarketValue += marketValue
			totalCost += cost
		}
	}

	// 然后显示当前范围内的股票
	for i := startIndex; i < endIndex; i++ {
		stock := &m.portfolio.Stocks[i]

		if stock.Price > 0 {
			positionProfit := stock.CalculatePositionProfit()
			profitRate := ((stock.Price - stock.CostPrice) / stock.CostPrice) * 100
			marketValue := stock.Price * float64(stock.Quantity)

			// 计算今日涨幅：应该基于昨收价，而不是开盘价
			var todayChangeStr string
			// 使用change_percent字段，这是基于昨收价计算的涨跌幅
			// 当数据有效时显示百分比（包括0.00%），只有无法获取数据时才显示"-"
			if stock.PrevClose > 0 && stock.Price > 0 {
				todayChangeStr = m.formatProfitRateWithColorZeroLang(stock.ChangePercent)
			} else {
				todayChangeStr = "-"
			}

			// 使用多语言颜色显示函数
			positionProfitStr := m.formatProfitWithColorZeroLang(positionProfit)
			profitRateStr := m.formatProfitRateWithColorZeroLang(profitRate)

			// 光标列 - 检查光标是否在当前可见范围内且指向此行
			cursorCol := ""
			if m.portfolioCursor >= startIndex && m.portfolioCursor < endIndex && i == m.portfolioCursor {
				cursorCol = "►"
			}

			t.AppendRow(table.Row{
				cursorCol,
				stock.Code,                           // 代码
				stock.Name,                           // 名称
				fmt.Sprintf("%.3f", stock.PrevClose), // 昨收价（无颜色）
				m.formatPriceWithColorLang(stock.StartPrice, stock.PrevClose), // 开盘
				m.formatPriceWithColorLang(stock.MaxPrice, stock.PrevClose),   // 最高
				m.formatPriceWithColorLang(stock.MinPrice, stock.PrevClose),   // 最低
				m.formatPriceWithColorLang(stock.Price, stock.PrevClose),      // 现价（有颜色）
				fmt.Sprintf("%.3f", stock.CostPrice),                          // 成本价（无颜色）
				stock.Quantity,                                                // 持股数
				todayChangeStr,                                                // 今日涨幅
				positionProfitStr,                                             // 持仓盈亏（基于成本价）
				profitRateStr,                                                 // 盈亏率
				fmt.Sprintf("%.2f", marketValue),                              // 市值
			})

			// 在每个股票后添加分隔线（除了显示范围内的最后一个）
			if i < endIndex-1 {
				t.AppendSeparator()
			}
		} else {
			// 如果无法获取数据，显示基本信息但标记数据不可用
			// 光标列 - 检查光标是否在当前可见范围内且指向此行
			cursorCol := ""
			if m.portfolioCursor >= startIndex && m.portfolioCursor < endIndex && i == m.portfolioCursor {
				cursorCol = "►"
			}

			t.AppendRow(table.Row{
				cursorCol,
				stock.Code,                           // 代码
				stock.Name,                           // 名称
				"-",                                  // 昨收价
				"-",                                  // 开盘
				"-",                                  // 最高
				"-",                                  // 最低
				"-",                                  // 现价
				fmt.Sprintf("%.3f", stock.CostPrice), // 成本价
				stock.Quantity,                       // 持股数
				"-",                                  // 今日涨幅
				"-",                                  // 持仓盈亏
				"-",                                  // 盈亏率
				"-",                                  // 市值
			})
			// 在每个股票后添加分隔线（除了显示范围内的最后一个）
			if i < endIndex-1 {
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
		"",                 // 光标列
		"",                 // 代码
		m.getText("total"), // 名称 -> 总计
		"",                 // 昨收价
		"",                 // 现价
		"",                 // 成本价
		"",                 // 开盘
		"",                 // 最高
		"",                 // 最低
		"",                 // 持股数
		"",                 // 今日涨幅
		m.formatProfitWithColorLang(totalPortfolioProfit), // 持仓盈亏（总持仓盈亏）
		m.formatProfitRateWithColorLang(totalProfitRate),  // 盈亏率（总盈亏率）
		fmt.Sprintf("%.2f", totalMarketValue),             // 市值（总市值）
	})

	s += t.Render() + "\n"

	// 如果可以滚动，显示滚动指示
	if totalStocks > maxPortfolioLines {
		s += strings.Repeat("-", 80) + "\n"
		if m.portfolioScrollPos > 0 {
			if m.language == Chinese {
				s += "↑ 有更新的股票 (按↓查看)\n"
			} else {
				s += "↑ Newer stocks available (press ↓)\n"
			}
		}
		if m.portfolioScrollPos < totalStocks-1 {
			if m.language == Chinese {
				s += "↓ 有更多历史股票 (按↑查看)\n"
			} else {
				s += "↓ More stocks available (press ↑)\n"
			}
		}
	}

	s += "\n" + m.getText("holdingsHelp") + "\n"

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
			Language:      "en",        // 默认英文
			AutoStart:     true,        // 有数据时自动进入监控模式
			StartupModule: "portfolio", // 默认启动持股模块
			DebugMode:     false,       // 调试模式关闭
		},
		Display: DisplayConfig{
			ColorScheme:   "professional", // 专业配色方案
			DecimalPlaces: 3,              // 3位小数
			TableStyle:    "light",        // 轻量表格样式
			MaxLines:      10,             // 默认每页显示10行
		},
		Update: UpdateConfig{
			RefreshInterval: 5,    // 5秒刷新间隔
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

	// 验证配置的合理性
	if config.Display.MaxLines <= 0 || config.Display.MaxLines > 50 {
		config.Display.MaxLines = 10 // 默认值
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

// 计算股票的持仓盈亏
func (s *Stock) CalculatePositionProfit() float64 {
	// 使用简化的加权平均成本价计算
	return (s.Price - s.CostPrice) * float64(s.Quantity)
}

// 计算股票的加权平均成本价
func (s *Stock) CalculateWeightedAverageCost() float64 {
	return s.CostPrice // 直接返回成本价
}

// 计算总持股数量
func (s *Stock) CalculateTotalQuantity() int {
	return s.Quantity // 直接返回持股数量
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

// 格式化资金流向数据，自动选择万元或亿元单位，支持股票类型检测
func (m *Model) formatFundFlowWithColorAndUnitForStock(amount float64, symbol string) string {
	// 对于非A股（如美股），显示 "-" 表示数据不可用
	if !isChinaStock(symbol) {
		return "-"
	}
	return m.formatFundFlowWithColorAndUnit(amount)
}

// 格式化盈亏率数据，支持股票类型检测
func (m *Model) formatProfitRateWithColorZeroLangForStock(rate float64, symbol string) string {
	// 对于非A股（如美股），显示 "-" 表示数据不可用
	if !isChinaStock(symbol) {
		return "-"
	}
	return m.formatProfitRateWithColorZeroLang(rate)
}

// 格式化资金流向数据，自动选择万元或亿元单位
func (m *Model) formatFundFlowWithColorAndUnit(amount float64) string {
	// 当数值接近0时（考虑浮点数精度），显示白色（无颜色）
	if abs(amount) < 1000 {
		return "0"
	}

	var formattedValue string
	var unit string

	// 根据金额大小选择单位
	if abs(amount) >= 100000000 { // 1亿以上显示为亿元
		value := amount / 100000000
		if m.language == Chinese {
			unit = "亿"
		} else {
			unit = "B" // Billion
		}
		formattedValue = fmt.Sprintf("%.2f%s", value, unit)
	} else { // 1亿以下显示为万元
		value := amount / 10000
		if m.language == Chinese {
			unit = "万"
		} else {
			unit = "W" // 万 (Wan)
		}
		formattedValue = fmt.Sprintf("%.1f%s", value, unit)
	}

	// 应用颜色逻辑
	if m.language == English {
		// 英文：绿色盈利，红色亏损
		if amount >= 0 {
			return text.FgGreen.Sprintf("+%s", formattedValue)
		}
		return text.FgRed.Sprintf("%s", formattedValue)
	} else {
		// 中文：红色盈利，绿色亏损
		if amount >= 0 {
			return text.FgRed.Sprintf("+%s", formattedValue)
		}
		return text.FgGreen.Sprintf("%s", formattedValue)
	}
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

// 辅助函数：计算浮点数绝对值
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func getStockInfo(symbol string) *StockData {
	var stockData *StockData

	// 如果输入是中文，尝试通过API搜索
	if containsChineseChars(symbol) {
		stockData = searchChineseStock(symbol)
	} else {
		// 对于非中文输入，先尝试直接获取价格，然后尝试搜索
		stockData = getStockPrice(symbol)

		// 如果直接获取失败，尝试作为搜索关键词搜索
		if stockData == nil || stockData.Price <= 0 {
			debugPrint("[调试] 直接获取股票价格失败，尝试通过搜索查找: %s\n", symbol)
			stockData = searchStockBySymbol(symbol)
		}
	}

	// 如果获取到股票数据且是中国股票，尝试获取资金流向数据
	if stockData != nil && stockData.Symbol != "" && isChinaStock(stockData.Symbol) {
		fundFlow := getFundFlowDataSync(stockData.Symbol)
		if fundFlow != nil {
			stockData.FundFlow = *fundFlow
		}
	}

	return stockData
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

// 通过符号搜索股票（支持美股等国际股票）
func searchStockBySymbol(symbol string) *StockData {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	debugPrint("[调试] 开始通过符号搜索股票: %s\n", symbol)

	// 策略1: 使用TwelveData搜索API
	result := searchStockByTwelveDataAPI(symbol)
	if result != nil && result.Price > 0 {
		debugPrint("[调试] TwelveData符号搜索成功找到: %s (%s)\n", result.Name, result.Symbol)
		return result
	}

	// 策略2: 尝试腾讯API（可能支持部分国际股票）
	result = searchStockByTencentAPI(symbol)
	if result != nil && result.Price > 0 {
		debugPrint("[调试] 腾讯符号搜索成功找到: %s (%s)\n", result.Name, result.Symbol)
		return result
	}

	// 策略3: 尝试新浪API（可能支持部分国际股票）
	result = searchStockBySinaAPI(symbol)
	if result != nil && result.Price > 0 {
		debugPrint("[调试] 新浪符号搜索成功找到: %s (%s)\n", result.Name, result.Symbol)
		return result
	}

	debugPrint("[调试] 所有符号搜索策略都失败，未找到股票数据\n")
	return nil
}

// 使用TwelveData搜索API查找股票
func searchStockByTwelveDataAPI(keyword string) *StockData {
	debugPrint("[调试] 使用TwelveData搜索API查找: %s\n", keyword)

	// 先尝试符号搜索
	searchUrl := fmt.Sprintf("https://api.twelvedata.com/symbol_search?symbol=%s&apikey=demo", keyword)
	debugPrint("[调试] TwelveData搜索请求URL: %s\n", searchUrl)

	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Get(searchUrl)
	if err != nil {
		debugPrint("[错误] TwelveData搜索API HTTP请求失败: %v\n", err)
		return nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		debugPrint("[错误] TwelveData搜索API读取响应失败: %v\n", err)
		return nil
	}

	debugPrint("[调试] TwelveData搜索响应: %s\n", string(body))

	var searchResult struct {
		Data []struct {
			Symbol         string `json:"symbol"`
			InstrumentName string `json:"instrument_name"`
			Exchange       string `json:"exchange"`
			Country        string `json:"country"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &searchResult); err != nil {
		debugPrint("[错误] TwelveData搜索JSON解析失败: %v\n", err)
		return nil
	}

	if len(searchResult.Data) == 0 {
		debugPrint("[调试] TwelveData搜索未找到匹配结果\n")
		return nil
	}

	// 选择第一个匹配的结果，优先选择美国市场的股票
	var selectedSymbol, selectedName string
	for _, item := range searchResult.Data {
		if item.Country == "United States" && item.Exchange == "NASDAQ" {
			selectedSymbol = item.Symbol
			selectedName = item.InstrumentName
			break
		}
	}

	// 如果没有找到美国NASDAQ的，就用第一个结果
	if selectedSymbol == "" {
		selectedSymbol = searchResult.Data[0].Symbol
		selectedName = searchResult.Data[0].InstrumentName
	}

	debugPrint("[调试] TwelveData搜索选择股票: %s (%s)\n", selectedName, selectedSymbol)

	// 获取股票报价
	return tryTwelveDataAPI(selectedSymbol)
}

// 通过API搜索中文股票名称
func searchChineseStock(chineseName string) *StockData {
	chineseName = strings.TrimSpace(chineseName)
	debugPrint("[调试] 开始搜索中文股票: %s\n", chineseName)

	// 策略1: 使用腾讯搜索API
	result := searchStockByTencentAPI(chineseName)
	if result != nil && result.Price > 0 {
		debugPrint("[调试] 腾讯搜索API成功找到: %s (%s)\n", result.Name, result.Symbol)
		return result
	}

	// 策略2: 尝试新浪财经搜索API
	result = searchStockBySinaAPI(chineseName)
	if result != nil && result.Price > 0 {
		debugPrint("[调试] 新浪搜索API成功找到: %s (%s)\n", result.Name, result.Symbol)
		return result
	}

	// 策略3: 尝试更多的搜索关键词变形
	result = tryAdvancedSearch(chineseName)
	if result != nil && result.Price > 0 {
		debugPrint("[调试] 高级搜索成功找到: %s (%s)\n", result.Name, result.Symbol)
		return result
	}

	// 所有搜索策略都失败
	debugPrint("[调试] 所有搜索策略都失败，未找到股票数据\n")
	return nil
}

// 使用腾讯搜索API查找股票
func searchStockByTencentAPI(keyword string) *StockData {
	debugPrint("[调试] 使用腾讯搜索API查找: %s\n", keyword)

	// 腾讯股票搜索API URL - 使用更完整的搜索接口
	url := fmt.Sprintf("https://smartbox.gtimg.cn/s3/?q=%s&t=gp", keyword)
	debugPrint("[调试] 腾讯搜索请求URL: %s\n", url)

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		debugPrint("[错误] 腾讯搜索API创建请求失败: %v\n", err)
		return nil
	}

	// 添加必要的请求头，提高成功率
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://stockapp.finance.qq.com/")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")

	resp, err := client.Do(req)
	if err != nil {
		debugPrint("[错误] 腾讯搜索API HTTP请求失败: %v\n", err)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		debugPrint("[错误] 腾讯搜索API返回非200状态码: %d\n", resp.StatusCode)
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		debugPrint("[错误] 腾讯搜索API读取响应失败: %v\n", err)
		return nil
	}

	content, err := gbkToUtf8(body)
	if err != nil {
		debugPrint("[错误] 腾讯搜索API编码转换失败: %v\n", err)
		content = string(body)
	}
	debugPrint("[调试] 腾讯搜索API响应: %s\n", content[:min(300, len(content))])

	// 解析搜索结果
	return parseSearchResults(content, keyword)
}

// 解析腾讯搜索结果
func parseSearchResults(content, keyword string) *StockData {
	debugPrint("[调试] 开始解析搜索结果\n")

	// 尝试解析新的腾讯格式 (v_hint=)
	result := parseTencentHintFormat(content)
	if result != nil {
		return result
	}

	// 尝试解析JSON格式的响应
	result = parseJSONSearchResults(content, keyword)
	if result != nil {
		return result
	}

	// 如果JSON解析失败，尝试解析旧格式
	return parseLegacySearchResults(content, keyword)
}

// 解析腾讯Hint格式的搜索结果
func parseTencentHintFormat(content string) *StockData {
	// 格式: v_hint="sz~000880~潍柴重机~wczj~GP-A"
	debugPrint("[调试] 尝试解析腾讯Hint格式\n")

	// 查找v_hint=
	if !strings.Contains(content, "v_hint=") {
		return nil
	}

	// 提取引号内的内容
	startPos := strings.Index(content, "v_hint=\"")
	if startPos == -1 {
		return nil
	}
	startPos += len("v_hint=\"")

	endPos := strings.Index(content[startPos:], "\"")
	if endPos == -1 {
		return nil
	}

	data := content[startPos : startPos+endPos]
	debugPrint("[调试] 提取的数据: %s\n", data)

	// 按^分割多个结果，取第一个
	results := strings.Split(data, "^")
	if len(results) == 0 {
		debugPrint("[调试] 未找到搜索结果\n")
		return nil
	}

	// 处理第一个结果
	firstResult := results[0]
	fields := strings.Split(firstResult, "~")
	if len(fields) < 3 {
		debugPrint("[调试] 字段数量不足: %d\n", len(fields))
		return nil
	}

	market := fields[0] // sz, sh, hk
	code := fields[1]   // 000880
	name := fields[2]   // 潍柴重机（可能是Unicode编码）

	// 尝试解码Unicode字符串
	decodedName, err := strconv.Unquote(`"` + name + `"`)
	if err == nil {
		name = decodedName
	}

	debugPrint("[调试] 解析结果 - 市场: %s, 代码: %s, 名称: %s\n", market, code, name)

	// 对于搜索结果，直接返回第一个匹配项（因为用户输入的关键词已经被API处理过了）
	if true {
		// 转换为标准格式
		standardCode := strings.ToUpper(market) + code
		debugPrint("[调试] 腾讯Hint格式找到匹配股票: %s (%s)\n", name, standardCode)

		// 获取详细信息
		stockData := getStockPrice(standardCode)
		if stockData != nil && stockData.Price > 0 {
			stockData.Symbol = standardCode
			stockData.Name = name
			return stockData
		}
	}

	return nil
}

// 解析JSON格式的搜索结果
func parseJSONSearchResults(content, keyword string) *StockData {
	// 尝试解析为JSON
	var searchResult map[string]interface{}
	if err := json.Unmarshal([]byte(content), &searchResult); err != nil {
		debugPrint("[调试] JSON解析失败: %v\n", err)
		return nil
	}

	// 查找数据字段
	data, ok := searchResult["data"]
	if !ok {
		debugPrint("[调试] 找不到data字段\n")
		return nil
	}

	dataArray, ok := data.([]interface{})
	if !ok {
		debugPrint("[调试] data不是数组格式\n")
		return nil
	}

	for _, item := range dataArray {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		// 提取股票信息
		code, _ := itemMap["code"].(string)
		name, _ := itemMap["name"].(string)

		if code == "" || name == "" {
			continue
		}

		// 检查名称是否匹配关键词
		if strings.Contains(name, keyword) {
			debugPrint("[调试] JSON格式找到匹配股票: %s (%s)\n", name, code)

			// 转换为标准格式
			standardCode := convertJSONCodeToStandard(code)

			// 获取详细信息
			stockData := getStockPrice(standardCode)
			if stockData != nil && stockData.Price > 0 {
				stockData.Symbol = standardCode
				stockData.Name = name
				return stockData
			}
		}
	}

	return nil
}

// 解析旧格式的搜索结果
func parseLegacySearchResults(content, keyword string) *StockData {
	debugPrint("[调试] 使用旧格式解析\n")
	// 腾讯搜索结果格式分析
	// 格式类似: v_s_关键词="sz002415~海康威视~002415~7.450~-0.160~-2.105~15270~7705~7565~7.610"
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		if !strings.Contains(line, "~") {
			continue
		}

		// 找到符号="的位置
		startPos := strings.Index(line, "\"")
		endPos := strings.LastIndex(line, "\"")
		if startPos == -1 || endPos == -1 || startPos >= endPos {
			continue
		}

		// 提取数据部分
		data := line[startPos+1 : endPos]
		fields := strings.Split(data, "~")

		if len(fields) < 4 {
			continue
		}

		// 解析字段
		code := fields[0]
		name := fields[1]
		shortCode := fields[2]

		// 检查名称是否匹配关键词
		if strings.Contains(name, keyword) {
			debugPrint("[调试] 旧格式找到匹配股票: %s (%s)\n", name, code)

			// 转换为标准格式
			standardCode := convertToStandardCode(code, shortCode)

			// 获取详细信息
			stockData := getStockPrice(standardCode)
			if stockData != nil && stockData.Price > 0 {
				stockData.Symbol = standardCode
				stockData.Name = name
				return stockData
			}
		}
	}

	return nil
}

// 转换JSON格式的股票代码为标准格式
func convertJSONCodeToStandard(code string) string {
	code = strings.TrimSpace(code)

	// 如果已经是标准格式，直接返回
	if strings.HasPrefix(code, "SH") || strings.HasPrefix(code, "SZ") || strings.HasPrefix(code, "HK") {
		return code
	}

	// 根据数字开头判断市场
	if len(code) == 6 {
		if strings.HasPrefix(code, "6") {
			return "SH" + code
		} else if strings.HasPrefix(code, "0") || strings.HasPrefix(code, "3") {
			return "SZ" + code
		}
	}

	return code
}

// 将腾讯的股票代码转换为标准格式
func convertToStandardCode(code, shortCode string) string {
	code = strings.ToLower(strings.TrimSpace(code))

	if strings.HasPrefix(code, "sh") {
		return "SH" + shortCode
	} else if strings.HasPrefix(code, "sz") {
		return "SZ" + shortCode
	} else if strings.HasPrefix(code, "hk") {
		return "HK" + shortCode
	}

	// 如果无法识别，返回原始代码
	return code
}

// 使用新浪财经搜索API查找股票
func searchStockBySinaAPI(keyword string) *StockData {
	debugPrint("[调试] 使用新浪财经搜索API查找: %s\n", keyword)

	// 新浪财经搜索API URL
	url := fmt.Sprintf("https://suggest3.sinajs.cn/suggest/type=11,12,13,14,15&key=%s", keyword)
	debugPrint("[调试] 新浪财经请求URL: %s\n", url)

	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		debugPrint("[错误] 新浪财经搜索API HTTP请求失败: %v\n", err)
		return nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		debugPrint("[错误] 新浪财经搜索API读取响应失败: %v\n", err)
		return nil
	}

	content := string(body)
	debugPrint("[调试] 新浪财经搜索API响应: %s\n", content)

	// 解析新浪搜索结果
	return parseSinaSearchResults(content, keyword)
}

// 解析新浪搜索结果
func parseSinaSearchResults(content, keyword string) *StockData {
	// 新浪返回格式类似: var suggestvalue="sz000858,五粮液;sh600519,贵州茅台;";
	lines := strings.Split(content, ";")

	for _, line := range lines {
		if !strings.Contains(line, ",") {
			continue
		}

		// 提取股票信息
		parts := strings.Split(line, ",")
		if len(parts) < 2 {
			continue
		}

		code := strings.TrimSpace(parts[0])
		name := strings.TrimSpace(parts[1])

		// 清理代码和名称中的特殊字符
		code = strings.Trim(code, "\"'")
		name = strings.Trim(name, "\"'")

		if code == "" || name == "" {
			continue
		}

		// 检查名称是否匹配关键词
		if strings.Contains(name, keyword) {
			debugPrint("[调试] 新浪搜索找到匹配股票: %s (%s)\n", name, code)

			// 转换为标准格式
			standardCode := convertSinaCodeToStandard(code)

			// 获取详细信息
			stockData := getStockPrice(standardCode)
			if stockData != nil && stockData.Price > 0 {
				stockData.Symbol = standardCode
				stockData.Name = name
				return stockData
			}
		}
	}

	return nil
}

// 转换新浪的股票代码为标准格式
func convertSinaCodeToStandard(code string) string {
	code = strings.ToLower(strings.TrimSpace(code))

	// 如果已经是标准格式，直接返回
	if strings.HasPrefix(strings.ToUpper(code), "SH") || strings.HasPrefix(strings.ToUpper(code), "SZ") {
		return strings.ToUpper(code)
	}

	if strings.HasPrefix(code, "sh") {
		return "SH" + strings.TrimPrefix(code, "sh")
	} else if strings.HasPrefix(code, "sz") {
		return "SZ" + strings.TrimPrefix(code, "sz")
	} else if strings.HasPrefix(code, "hk") {
		return "HK" + strings.TrimPrefix(code, "hk")
	}

	// 如果是6位数字，根据开头判断市场
	if len(code) == 6 {
		if strings.HasPrefix(code, "6") {
			return "SH" + code
		} else if strings.HasPrefix(code, "0") || strings.HasPrefix(code, "3") {
			return "SZ" + code
		}
	}

	return strings.ToUpper(code)
}

// 高级搜索策略：尝试多种关键词变形
func tryAdvancedSearch(chineseName string) *StockData {
	// 生成搜索关键词变形
	keywords := generateSearchKeywords(chineseName)

	for _, keyword := range keywords {
		if keyword == chineseName {
			continue // 跳过原始关键词，避免重复搜索
		}

		debugPrint("[调试] 尝试搜索关键词变形: %s\n", keyword)
		result := searchStockByTencentAPI(keyword)
		if result != nil && result.Price > 0 {
			return result
		}
	}

	return nil
}

// 生成搜索关键词变形
func generateSearchKeywords(name string) []string {
	var keywords []string

	// 原始关键词
	keywords = append(keywords, name)

	// 如果名称包含“股份”、“集团”等后缀，尝试去掉
	suffixes := []string{"股份", "集团", "公司", "有限公司", "科技", "实业"}
	for _, suffix := range suffixes {
		if strings.HasSuffix(name, suffix) {
			shortName := strings.TrimSuffix(name, suffix)
			if len(shortName) > 1 {
				keywords = append(keywords, shortName)
			}
		}
	}

	// 如果名称包含“中国”、“上海”等前缀，尝试去掉
	prefixes := []string{"中国", "上海", "北京", "广东", "深圳", "天津"}
	for _, prefix := range prefixes {
		if strings.HasPrefix(name, prefix) && len(name) > len(prefix)+1 {
			shortName := strings.TrimPrefix(name, prefix)
			if len(shortName) > 1 {
				keywords = append(keywords, shortName)
			}
		}
	}

	// 如果名称较长，尝试取前几个字符作为关键词
	if len([]rune(name)) > 4 {
		runes := []rune(name)
		// 取前3个字符
		if len(runes) >= 3 {
			keywords = append(keywords, string(runes[:3]))
		}
		// 取前4个字符
		if len(runes) >= 4 {
			keywords = append(keywords, string(runes[:4]))
		}
	}

	return keywords
}

// 从缓存获取资金流向数据（非阻塞）
func (m *Model) getFundFlowDataFromCache(symbol string) *FundFlow {
	if !isChinaStock(symbol) {
		return &FundFlow{}
	}

	m.fundFlowMutex.RLock()
	defer m.fundFlowMutex.RUnlock()

	if entry, exists := m.fundFlowCache[symbol]; exists {
		return &entry.Data
	}

	// 如果缓存中没有数据，返回空数据
	return &FundFlow{}
}

// 从缓存获取股价数据（非阻塞）
func (m *Model) getStockPriceFromCache(symbol string) *StockData {
	m.stockPriceMutex.RLock()
	defer m.stockPriceMutex.RUnlock()
	if entry, exists := m.stockPriceCache[symbol]; exists {
		// 检查缓存是否过期（超过30秒）
		if time.Since(entry.UpdateTime) < 30*time.Second {
			return entry.Data
		}
	}
	// 如果缓存中没有数据或已过期，返回nil，触发异步更新
	return nil
}

// 同步获取资金流向数据（用于搜索结果）
func getFundFlowDataSync(symbol string) *FundFlow {
	if !isChinaStock(symbol) {
		return &FundFlow{}
	}

	// 调用Python脚本获取AKShare数据
	cmd := exec.Command("venv/bin/python", "scripts/akshare_fund_flow.py", symbol)
	output, err := cmd.Output()
	if err != nil {
		debugPrint("[错误] 同步获取资金流向失败 %s: %v\n", symbol, err)
		return &FundFlow{}
	}

	var fundFlow FundFlow
	err = json.Unmarshal(output, &fundFlow)
	if err != nil {
		debugPrint("[错误] 解析资金流向数据失败 %s: %v\n", symbol, err)
		return &FundFlow{}
	}

	debugPrint("[调试] 同步获取资金流向成功 %s: 主力净流入 %.2f\n", symbol, fundFlow.MainNetInflow)
	return &fundFlow
}

// 启动异步资金流向数据更新（1分钟间隔）
func (m *Model) startFundFlowUpdates() tea.Cmd {
	// 检查是否需要开始新的更新周期
	if time.Since(m.fundFlowUpdateTime) < time.Minute {
		return nil // 还未到更新时间
	}

	// 收集所有需要更新的股票代码
	stockCodes := make([]string, 0)

	// 添加自选列表中的股票
	for _, stock := range m.watchlist.Stocks {
		if isChinaStock(stock.Code) {
			stockCodes = append(stockCodes, stock.Code)
		}
	}

	if len(stockCodes) == 0 {
		return nil
	}

	// 更新开始时间
	m.fundFlowUpdateTime = time.Now()

	// 逐个发起异步获取请求
	var cmds []tea.Cmd
	for _, code := range stockCodes {
		// 标记正在更新
		m.fundFlowMutex.Lock()
		if entry, exists := m.fundFlowCache[code]; exists {
			entry.IsUpdating = true
		} else {
			m.fundFlowCache[code] = &FundFlowCacheEntry{
				Data:       FundFlow{},
				UpdateTime: time.Time{},
				IsUpdating: true,
			}
		}
		m.fundFlowMutex.Unlock()

		// 为每个股票添加一个延迟，避免同时请求太多
		delay := time.Duration(len(cmds)) * 200 * time.Millisecond
		// 修复闭包问题：将code变量复制到局部变量
		stockCode := code
		cmds = append(cmds, tea.Tick(delay, func(t time.Time) tea.Msg {
			// 直接在这里执行获取操作
			// 调用Python脚本获取AKShare数据
			cmd := exec.Command("venv/bin/python", "scripts/akshare_fund_flow.py", stockCode)
			output, err := cmd.Output()
			if err != nil {
				debugPrint("[错误] AKShare资金流向获取失败 %s: %v\n", stockCode, err)
				return fundFlowUpdateMsg{Symbol: stockCode, Data: nil, Error: err}
			}

			var fundFlow FundFlow
			err = json.Unmarshal(output, &fundFlow)
			if err != nil {
				debugPrint("[错误] 解析资金流向数据失败 %s: %v\n", stockCode, err)
				return fundFlowUpdateMsg{Symbol: stockCode, Data: nil, Error: err}
			}

			debugPrint("[信息] 资金流向数据获取成功: %s\n", stockCode)
			return fundFlowUpdateMsg{Symbol: stockCode, Data: &fundFlow, Error: nil}
		}))
	}

	return tea.Batch(cmds...)
}

// 启动股价异步更新
func (m *Model) startStockPriceUpdates() tea.Cmd {
	// 检查是否需要开始新的更新周期
	if time.Since(m.stockPriceUpdateTime) < 5*time.Second {
		debugPrint("[调试] 股价更新间隔未到，跳过更新 (距上次更新: %v)\n", time.Since(m.stockPriceUpdateTime))
		return nil // 还未到更新时间
	}

	// 收集所有需要更新的股票代码
	stockCodes := make([]string, 0)

	// 添加自选列表中的股票 - 注意：这里应该获取所有自选股票，而不是过滤后的
	for _, stock := range m.watchlist.Stocks {
		stockCodes = append(stockCodes, stock.Code)
	}

	// 添加持股列表中的股票
	for _, stock := range m.portfolio.Stocks {
		stockCodes = append(stockCodes, stock.Code)
	}

	if len(stockCodes) == 0 {
		debugPrint("[调试] 没有需要更新的股票代码，跳过股价更新\n")
		return nil
	}

	// 去重股票代码
	uniqueCodes := make(map[string]bool)
	var uniqueStockCodes []string
	for _, code := range stockCodes {
		if !uniqueCodes[code] {
			uniqueCodes[code] = true
			uniqueStockCodes = append(uniqueStockCodes, code)
		}
	}

	// 更新开始时间
	m.stockPriceUpdateTime = time.Now()

	debugPrint("[调试] 开始股价异步更新，共 %d 个股票代码\n", len(uniqueStockCodes))

	// 逐个发起异步获取请求
	var cmds []tea.Cmd
	for _, code := range uniqueStockCodes {
		// 标记正在更新
		m.stockPriceMutex.Lock()
		if entry, exists := m.stockPriceCache[code]; exists {
			entry.IsUpdating = true
		} else {
			m.stockPriceCache[code] = &StockPriceCacheEntry{
				Data:       nil,
				UpdateTime: time.Time{},
				IsUpdating: true,
			}
		}
		m.stockPriceMutex.Unlock()

		// 为每个股票添加一个延迟，避免同时请求太多
		delay := time.Duration(len(cmds)) * 100 * time.Millisecond
		// 修复闭包问题：将code变量复制到局部变量
		stockCode := code
		cmds = append(cmds, tea.Tick(delay, func(t time.Time) tea.Msg {
			// 直接在这里执行获取操作，而不是返回Command
			data := getStockPrice(stockCode)

			// 更新缓存
			m.stockPriceMutex.Lock()
			defer m.stockPriceMutex.Unlock()

			// 只有在成功获取数据时才更新缓存
			if data != nil && data.Price > 0 {
				m.stockPriceCache[stockCode] = &StockPriceCacheEntry{
					Data:       data,
					UpdateTime: time.Now(),
					IsUpdating: false,
				}
			} else {
				// 获取失败时，标记为不在更新状态，但不更新缓存，这样下次还会尝试获取
				if entry, exists := m.stockPriceCache[stockCode]; exists {
					entry.IsUpdating = false
				}
			}

			var err error
			if data == nil || data.Price <= 0 {
				err = fmt.Errorf("failed to get stock price for %s", stockCode)
			}

			return stockPriceUpdateMsg{
				Symbol: stockCode,
				Data:   data,
				Error:  err,
			}
		}))
	}

	return tea.Batch(cmds...)
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

	debugPrint("[调试] 所有API都失败，未找到股票数据\n")
	return nil
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
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		debugPrint("[错误] 腾讯价格API创建请求失败: %v\n", err)
		return &StockData{Symbol: symbol, Price: 0}
	}

	// 添加必要的请求头，与搜索API保持一致
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://stockapp.finance.qq.com/")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")

	resp, err := client.Do(req)
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
	// 策略1: 尝试TwelveData API
	data := tryTwelveDataAPI(symbol)
	if data != nil && data.Price > 0 {
		return data
	}

	// 策略2: 尝试免费的 FMP API (无需API key的基础数据)
	data = tryFMPFreeAPI(symbol)
	if data != nil && data.Price > 0 {
		return data
	}

	// 策略3: 尝试Yahoo Finance API
	data = tryYahooFinanceAPI(symbol)
	if data != nil && data.Price > 0 {
		return data
	}

	debugPrint("[调试] 所有美股API都失败，建议配置有效的API key\n")
	return &StockData{Symbol: symbol, Price: 0}
}

func tryTwelveDataAPI(symbol string) *StockData {
	convertedSymbol := strings.ToUpper(strings.TrimSpace(symbol))
	debugPrint("[调试] TwelveData - 原始代码: %s -> 转换后: %s\n", symbol, convertedSymbol)

	// 使用TwelveData API获取股票报价
	url := fmt.Sprintf("https://api.twelvedata.com/quote?symbol=%s&apikey=demo", convertedSymbol)
	debugPrint("[调试] TwelveData请求URL: %s\n", url)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		debugPrint("[错误] TwelveData HTTP请求失败: %v\n", err)
		return &StockData{Symbol: symbol, Price: 0}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		debugPrint("[错误] TwelveData读取响应失败: %v\n", err)
		return &StockData{Symbol: symbol, Price: 0}
	}

	debugPrint("[调试] TwelveData响应: %s\n", string(body))

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		debugPrint("[错误] TwelveData JSON解析失败: %v\n", err)
		return &StockData{Symbol: symbol, Price: 0}
	}

	// 检查是否有错误信息
	if errMsg, hasErr := result["message"]; hasErr {
		debugPrint("[调试] TwelveData API错误: %v\n", errMsg)
		return &StockData{Symbol: symbol, Price: 0}
	}

	// 解析股票数据
	name, _ := result["name"].(string)
	if name == "" {
		name = symbol
	}

	closeStr, closeOk := result["close"].(string)
	prevCloseStr, prevOk := result["previous_close"].(string)

	if !closeOk || !prevOk {
		debugPrint("[调试] TwelveData数据无效或为空\n")
		return &StockData{Symbol: symbol, Price: 0}
	}

	current, err := strconv.ParseFloat(closeStr, 64)
	if err != nil {
		debugPrint("[错误] TwelveData price解析失败: %v\n", err)
		return &StockData{Symbol: symbol, Price: 0}
	}

	previous, err := strconv.ParseFloat(prevCloseStr, 64)
	if err != nil {
		debugPrint("[错误] TwelveData previous_close解析失败: %v\n", err)
		return &StockData{Symbol: symbol, Price: 0}
	}

	if current <= 0 {
		debugPrint("[调试] TwelveData价格无效\n")
		return &StockData{Symbol: symbol, Price: 0}
	}

	// 解析开盘价、最高价、最低价、成交量
	var openPrice, maxPrice, minPrice float64
	var volume int64

	if openStr, ok := result["open"].(string); ok {
		openPrice, _ = strconv.ParseFloat(openStr, 64)
	}
	if highStr, ok := result["high"].(string); ok {
		maxPrice, _ = strconv.ParseFloat(highStr, 64)
	}
	if lowStr, ok := result["low"].(string); ok {
		minPrice, _ = strconv.ParseFloat(lowStr, 64)
	}
	if volumeStr, ok := result["volume"].(string); ok {
		volume, _ = strconv.ParseInt(volumeStr, 10, 64)
	}

	change := current - previous
	changePercent := 0.0
	if previous > 0 {
		changePercent = (change / previous) * 100
	}

	debugPrint("[调试] TwelveData获取成功 - 名称: %s, 价格: %.2f, 涨跌: %.2f (%.2f%%), 开: %.2f, 高: %.2f, 低: %.2f, 量: %d\n",
		name, current, change, changePercent, openPrice, maxPrice, minPrice, volume)

	return &StockData{
		Symbol:        symbol,
		Name:          name,
		Price:         current,
		Change:        change,
		ChangePercent: changePercent,
		StartPrice:    openPrice,
		MaxPrice:      maxPrice,
		MinPrice:      minPrice,
		PrevClose:     previous,
		TurnoverRate:  0, // TwelveData不提供换手率
		Volume:        volume,
	}
}

// 使用免费的Financial Modeling Prep API (不需要API key的基础功能)
func tryFMPFreeAPI(symbol string) *StockData {
	convertedSymbol := strings.ToUpper(strings.TrimSpace(symbol))
	debugPrint("[调试] FMPFree - 查找股票: %s\n", convertedSymbol)

	// 尝试使用免费的实时报价接口
	url := fmt.Sprintf("https://financialmodelingprep.com/api/v3/quote/%s", convertedSymbol)
	debugPrint("[调试] FMPFree请求URL: %s\n", url)

	client := &http.Client{Timeout: 8 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		debugPrint("[错误] FMPFree请求创建失败: %v\n", err)
		return &StockData{Symbol: symbol, Price: 0}
	}

	// 添加用户代理避免被阻止
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; StockMonitor/1.0)")

	resp, err := client.Do(req)
	if err != nil {
		debugPrint("[错误] FMPFree HTTP请求失败: %v\n", err)
		return &StockData{Symbol: symbol, Price: 0}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		debugPrint("[错误] FMPFree读取响应失败: %v\n", err)
		return &StockData{Symbol: symbol, Price: 0}
	}

	debugPrint("[调试] FMPFree响应: %s\n", string(body))

	// 检查是否是错误响应
	if strings.Contains(string(body), "Error Message") {
		debugPrint("[调试] FMPFree返回错误信息\n")
		return &StockData{Symbol: symbol, Price: 0}
	}

	var results []map[string]any
	if err := json.Unmarshal(body, &results); err != nil {
		debugPrint("[错误] FMPFree JSON解析失败: %v\n", err)
		return &StockData{Symbol: symbol, Price: 0}
	}

	if len(results) == 0 {
		debugPrint("[调试] FMPFree无返回数据\n")
		return &StockData{Symbol: symbol, Price: 0}
	}

	result := results[0]

	// 解析价格数据
	var price, previousClose, dayLow, dayHigh, open float64
	var volume int64
	var name string

	if p, ok := result["price"].(float64); ok {
		price = p
	}
	if pc, ok := result["previousClose"].(float64); ok {
		previousClose = pc
	}
	if low, ok := result["dayLow"].(float64); ok {
		dayLow = low
	}
	if high, ok := result["dayHigh"].(float64); ok {
		dayHigh = high
	}
	if o, ok := result["open"].(float64); ok {
		open = o
	}
	if vol, ok := result["volume"].(float64); ok {
		volume = int64(vol)
	}
	if n, ok := result["name"].(string); ok {
		name = n
	}

	if name == "" {
		name = symbol
	}

	if price <= 0 {
		debugPrint("[调试] FMPFree价格无效\n")
		return &StockData{Symbol: symbol, Price: 0}
	}

	change := price - previousClose
	changePercent := 0.0
	if previousClose > 0 {
		changePercent = (change / previousClose) * 100
	}

	debugPrint("[调试] FMPFree获取成功 - 名称: %s, 价格: %.2f, 涨跌: %.2f (%.2f%%)\n",
		name, price, change, changePercent)

	return &StockData{
		Symbol:        symbol,
		Name:          name,
		Price:         price,
		Change:        change,
		ChangePercent: changePercent,
		StartPrice:    open,
		MaxPrice:      dayHigh,
		MinPrice:      dayLow,
		PrevClose:     previousClose,
		TurnoverRate:  0,
		Volume:        volume,
	}
}

// 使用Yahoo Finance API作为备用方案
func tryYahooFinanceAPI(symbol string) *StockData {
	convertedSymbol := strings.ToUpper(strings.TrimSpace(symbol))
	debugPrint("[调试] Yahoo - 查找股票: %s\n", convertedSymbol)

	// 使用Yahoo Finance的chart API接口，这个接口更稳定
	url := fmt.Sprintf("https://query1.finance.yahoo.com/v8/finance/chart/%s?interval=1d&range=1d", convertedSymbol)
	debugPrint("[调试] Yahoo请求URL: %s\n", url)

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		debugPrint("[错误] Yahoo请求创建失败: %v\n", err)
		return &StockData{Symbol: symbol, Price: 0}
	}

	// 添加完整的浏览器请求头以避免被阻止
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Cache-Control", "no-cache")

	resp, err := client.Do(req)
	if err != nil {
		debugPrint("[错误] Yahoo HTTP请求失败: %v\n", err)
		return &StockData{Symbol: symbol, Price: 0}
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 {
		debugPrint("[调试] Yahoo API限流\n")
		return &StockData{Symbol: symbol, Price: 0}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		debugPrint("[错误] Yahoo读取响应失败: %v\n", err)
		return &StockData{Symbol: symbol, Price: 0}
	}

	debugPrint("[调试] Yahoo响应: %s\n", string(body))

	var yahooResp struct {
		Chart struct {
			Result []struct {
				Meta struct {
					Symbol               string  `json:"symbol"`
					LongName             string  `json:"longName"`
					ShortName            string  `json:"shortName"`
					RegularMarketPrice   float64 `json:"regularMarketPrice"`
					ChartPreviousClose   float64 `json:"chartPreviousClose"`
					RegularMarketDayHigh float64 `json:"regularMarketDayHigh"`
					RegularMarketDayLow  float64 `json:"regularMarketDayLow"`
					RegularMarketVolume  int64   `json:"regularMarketVolume"`
				} `json:"meta"`
				Indicators struct {
					Quote []struct {
						Open   []float64 `json:"open"`
						High   []float64 `json:"high"`
						Low    []float64 `json:"low"`
						Close  []float64 `json:"close"`
						Volume []int64   `json:"volume"`
					} `json:"quote"`
				} `json:"indicators"`
			} `json:"result"`
			Error any `json:"error"`
		} `json:"chart"`
	}

	if err := json.Unmarshal(body, &yahooResp); err != nil {
		debugPrint("[错误] Yahoo JSON解析失败: %v\n", err)
		return &StockData{Symbol: symbol, Price: 0}
	}

	if yahooResp.Chart.Error != nil {
		debugPrint("[调试] Yahoo返回错误: %v\n", yahooResp.Chart.Error)
		return &StockData{Symbol: symbol, Price: 0}
	}

	if len(yahooResp.Chart.Result) == 0 {
		debugPrint("[调试] Yahoo无返回数据\n")
		return &StockData{Symbol: symbol, Price: 0}
	}

	result := yahooResp.Chart.Result[0]
	meta := result.Meta

	if meta.RegularMarketPrice <= 0 {
		debugPrint("[调试] Yahoo价格无效\n")
		return &StockData{Symbol: symbol, Price: 0}
	}

	// 获取开盘价、最高价、最低价
	var openPrice, highPrice, lowPrice float64
	var volume int64

	if len(result.Indicators.Quote) > 0 && len(result.Indicators.Quote[0].Open) > 0 {
		openPrice = result.Indicators.Quote[0].Open[0]
	}
	if len(result.Indicators.Quote) > 0 && len(result.Indicators.Quote[0].High) > 0 {
		highPrice = result.Indicators.Quote[0].High[0]
	}
	if len(result.Indicators.Quote) > 0 && len(result.Indicators.Quote[0].Low) > 0 {
		lowPrice = result.Indicators.Quote[0].Low[0]
	}
	if len(result.Indicators.Quote) > 0 && len(result.Indicators.Quote[0].Volume) > 0 {
		volume = result.Indicators.Quote[0].Volume[0]
	}

	// 如果没有从indicators获取到数据，使用meta中的数据
	if highPrice == 0 {
		highPrice = meta.RegularMarketDayHigh
	}
	if lowPrice == 0 {
		lowPrice = meta.RegularMarketDayLow
	}
	if volume == 0 {
		volume = meta.RegularMarketVolume
	}

	change := meta.RegularMarketPrice - meta.ChartPreviousClose
	changePercent := 0.0
	if meta.ChartPreviousClose > 0 {
		changePercent = (change / meta.ChartPreviousClose) * 100
	}

	name := meta.LongName
	if name == "" {
		name = meta.ShortName
	}
	if name == "" {
		name = symbol
	}

	debugPrint("[调试] Yahoo获取成功 - 名称: %s, 价格: %.2f, 涨跌: %.2f (%.2f%%), 开: %.2f, 高: %.2f, 低: %.2f, 量: %d\n",
		name, meta.RegularMarketPrice, change, changePercent, openPrice, highPrice, lowPrice, volume)

	return &StockData{
		Symbol:        symbol,
		Name:          name,
		Price:         meta.RegularMarketPrice,
		Change:        change,
		ChangePercent: changePercent,
		StartPrice:    openPrice,
		MaxPrice:      highPrice,
		MinPrice:      lowPrice,
		PrevClose:     meta.ChartPreviousClose,
		TurnoverRate:  0,
		Volume:        volume,
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Global variable to hold model reference for debug logging
var globalModel *Model

func debugPrint(format string, args ...any) {
	if globalModel != nil && globalModel.debugMode {
		timestamp := time.Now().Format("15:04:05")
		logMsg := fmt.Sprintf("[%s] %s", timestamp, fmt.Sprintf(format, args...))
		globalModel.addDebugLog(logMsg)
	}
}

func (m *Model) addDebugLog(msg string) {
	// 无限制存储所有调试日志
	m.debugLogs = append(m.debugLogs, msg)

	// 关键修复：当新日志添加时，如果用户不在底部查看，需要调整滚动位置
	// 以保持用户当前查看的内容不发生错位
	if m.debugScrollPos > 0 {
		// 用户在查看历史日志，滚动位置需要增加1以保持查看的内容不变
		m.debugScrollPos++
	}
	// 如果 debugScrollPos == 0，用户在底部，自动跟随最新日志（无需调整）
}

// debug日志滚动控制方法
func (m *Model) scrollDebugUp() {
	maxScroll := len(m.debugLogs) - 1
	if m.debugScrollPos < maxScroll {
		m.debugScrollPos++
	}
}

func (m *Model) scrollDebugDown() {
	if m.debugScrollPos > 0 {
		m.debugScrollPos--
	}
}

func (m *Model) scrollDebugToTop() {
	if len(m.debugLogs) > 0 {
		m.debugScrollPos = len(m.debugLogs) - 1
	}
}

func (m *Model) scrollDebugToBottom() {
	m.debugScrollPos = 0
}

// ========== 持股列表滚动控制方法 ==========

func (m *Model) scrollPortfolioUp() {
	// 向上翻页：显示更早的股票，光标也向上移动
	if m.portfolioCursor > 0 {
		m.portfolioCursor--
	}
	// 确保光标在可见范围内，如果需要则调整滚动位置
	maxPortfolioLines := m.config.Display.MaxLines
	endIndex := len(m.portfolio.Stocks) - m.portfolioScrollPos
	startIndex := endIndex - maxPortfolioLines
	if startIndex < 0 {
		startIndex = 0
	}

	// 如果光标超出可见范围的上边界，调整滚动位置
	if m.portfolioCursor < startIndex {
		m.portfolioScrollPos = len(m.portfolio.Stocks) - m.portfolioCursor - maxPortfolioLines
		if m.portfolioScrollPos < 0 {
			m.portfolioScrollPos = 0
		}
	}
}

func (m *Model) scrollPortfolioDown() {
	// 向下翻页：显示更新的股票，光标也向下移动
	if m.portfolioCursor < len(m.portfolio.Stocks)-1 {
		m.portfolioCursor++
	}
	// 确保光标在可见范围内，如果需要则调整滚动位置
	maxPortfolioLines := m.config.Display.MaxLines
	endIndex := len(m.portfolio.Stocks) - m.portfolioScrollPos
	startIndex := endIndex - maxPortfolioLines
	if startIndex < 0 {
		startIndex = 0
	}

	// 如果光标超出可见范围的下边界，调整滚动位置
	if m.portfolioCursor >= endIndex {
		m.portfolioScrollPos = len(m.portfolio.Stocks) - m.portfolioCursor - 1
		if m.portfolioScrollPos < 0 {
			m.portfolioScrollPos = 0
		}
	}
}

// ========== 自选列表滚动控制方法 ==========

func (m *Model) scrollWatchlistUp() {
	// 向上翻页：显示更早的股票，光标也向上移动
	if m.watchlistCursor > 0 {
		m.watchlistCursor--
		// 获取一次过滤后的列表，避免重复调用
		filteredStocks := m.getFilteredWatchlist()
		m.adjustWatchlistScroll(filteredStocks)
	}
}

func (m *Model) scrollWatchlistDown() {
	// 获取一次过滤后的列表，避免重复调用
	filteredStocks := m.getFilteredWatchlist()
	// 向下翻页：显示更新的股票，光标也向下移动
	if m.watchlistCursor < len(filteredStocks)-1 {
		m.watchlistCursor++
		m.adjustWatchlistScroll(filteredStocks)
	}
}

func (m *Model) logUserAction(action string) {
	if m.debugMode {
		timestamp := time.Now().Format("15:04:05")
		logMsg := fmt.Sprintf("[%s] 用户操作: %s", timestamp, action)
		m.addDebugLog(logMsg)
	}
}

func (m *Model) renderDebugPanel() string {
	if !m.debugMode {
		return ""
	}

	// 显示最多8条完整日志，支持滚动查看
	maxDebugLines := 8

	// 只有在有日志时才显示debug面板
	if len(m.debugLogs) == 0 {
		return "\n🔧 Debug Mode: ON (暂无日志)"
	}

	s := "\n" + strings.Repeat("=", 80) + "\n"

	// 显示滚动信息和快捷键提示
	totalLogs := len(m.debugLogs)
	currentPos := totalLogs - m.debugScrollPos

	if m.language == Chinese {
		s += fmt.Sprintf("🔧 调试日志 (%d/%d) [PageUp/PageDown:翻页 Home/End:首尾]\n", currentPos, totalLogs)
	} else {
		s += fmt.Sprintf("🔧 Debug Logs (%d/%d) [PageUp/PageDown:scroll Home/End:top/bottom]\n", currentPos, totalLogs)
	}
	s += strings.Repeat("-", 80) + "\n"

	// 根据滚动位置计算要显示的日志范围
	logs := m.debugLogs
	endIndex := len(logs) - m.debugScrollPos
	startIndex := endIndex - maxDebugLines
	if startIndex < 0 {
		startIndex = 0
	}
	if endIndex > len(logs) {
		endIndex = len(logs)
	}

	// 显示当前窗口内的日志
	for i := startIndex; i < endIndex; i++ {
		// 显示完整的日志内容，不截断
		prefix := ""
		if i == endIndex-1 && m.debugScrollPos == 0 {
			prefix = "→ " // 标记最新日志
		}
		s += prefix + logs[i] + "\n"
	}

	// 如果可以滚动，显示滚动指示
	if totalLogs > maxDebugLines {
		s += strings.Repeat("-", 80) + "\n"
		if m.debugScrollPos > 0 {
			if m.language == Chinese {
				s += "↑ 有更新的日志 (按PageDown查看 或 End键跳到最新)\n"
			} else {
				s += "↑ Newer logs available (press PageDown or End to jump to latest)\n"
			}
		}
		if m.debugScrollPos < totalLogs-1 {
			if m.language == Chinese {
				s += "↓ 有更多历史日志 (按PageUp查看 或 Home键跳到最早)\n"
			} else {
				s += "↓ More history logs (press PageUp or Home to jump to oldest)\n"
			}
		}
	}

	s += strings.Repeat("=", 80)

	return s
}

func (m *Model) handleEditingStock(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		// 根据之前的状态决定返回到哪里
		if m.previousState == Monitoring {
			m.state = Monitoring
			m.resetPortfolioCursor() // 重置游标到第一只股票
			m.lastUpdate = time.Now()
			m.message = ""
			return m, m.tickCmd()
		} else {
			m.state = MainMenu
			m.message = ""
			return m, nil
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
			m.portfolioIsSorted = false // 修改股票后重置持股列表排序状态

			stockName := m.portfolio.Stocks[m.selectedStockIndex].Name
			// 根据之前的状态决定返回到哪里
			if m.previousState == Monitoring {
				m.state = Monitoring
				m.resetPortfolioCursor() // 重置游标到第一只股票
				m.lastUpdate = time.Now()
				m.message = fmt.Sprintf(m.getText("editSuccess"), stockName)
				m.editingStep = 0
				m.input = ""
				return m, m.tickCmd()
			} else {
				m.state = MainMenu
				m.message = fmt.Sprintf(m.getText("editSuccess"), stockName)
				m.editingStep = 0
				m.input = ""
			}
		}
	}
	return m, nil
}

func (m *Model) viewEditingStock() string {
	s := m.getText("editTitle") + "\n\n"

	switch m.editingStep {
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
	case "esc":
		if m.searchFromWatchlist {
			m.state = WatchlistViewing
			m.resetWatchlistCursor() // 重置游标到第一只股票
			m.searchFromWatchlist = false
			m.message = ""
			return m, m.tickCmd() // 重启定时器
		} else {
			m.state = MainMenu
		}
		m.message = ""
		return m, nil
	case "enter":
		if m.searchInput == "" {
			m.message = m.getText("enterSearch")[:len(m.getText("enterSearch"))-2] // 去掉": "后缀
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

		// 如果是从自选列表进入的搜索，跳转到确认页面
		if m.searchFromWatchlist {
			m.state = WatchlistSearchConfirm
		} else {
			m.state = SearchResultWithActions
		}
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
	case "esc":
		m.state = MainMenu
		m.message = ""
		return m, nil
	case "r":
		m.state = SearchingStock
		m.searchFromWatchlist = false
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

	// 资金流向数据（仅A股显示）
	if isChinaStock(m.searchResult.Symbol) {
		fundFlow := &m.searchResult.FundFlow

		// 主力净流入
		if m.language == Chinese {
			headers = append(headers, "主力净流入")
		} else {
			headers = append(headers, "Main Flow")
		}
		mainFlowStr := m.formatFundFlowWithColorAndUnit(fundFlow.MainNetInflow)
		values = append(values, mainFlowStr)

		// 超大单净流入
		if m.language == Chinese {
			headers = append(headers, "超大单")
		} else {
			headers = append(headers, "Super Large")
		}
		superLargeStr := m.formatFundFlowWithColorAndUnit(fundFlow.SuperLargeNetInflow)
		values = append(values, superLargeStr)

		// 大单净流入
		if m.language == Chinese {
			headers = append(headers, "大单")
		} else {
			headers = append(headers, "Large")
		}
		largeStr := m.formatFundFlowWithColorAndUnit(fundFlow.LargeNetInflow)
		values = append(values, largeStr)

		// 中单净流入
		if m.language == Chinese {
			headers = append(headers, "中单")
		} else {
			headers = append(headers, "Medium")
		}
		mediumStr := m.formatFundFlowWithColorAndUnit(fundFlow.MediumNetInflow)
		values = append(values, mediumStr)

		// 小单净流入
		if m.language == Chinese {
			headers = append(headers, "小单")
		} else {
			headers = append(headers, "Small")
		}
		smallStr := m.formatFundFlowWithColorAndUnit(fundFlow.SmallNetInflow)
		values = append(values, smallStr)

		// 净流入占比
		if m.language == Chinese {
			headers = append(headers, "净流入占比")
		} else {
			headers = append(headers, "Net Ratio")
		}
		flowRatioStr := m.formatProfitRateWithColorZeroLang(fundFlow.NetInflowRatio)
		values = append(values, flowRatioStr)
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
		"insert", "delete", "tab", "enter", "backspace", "esc",
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
		m.message = "" // 清除消息
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

// ========== 自选股票相关功能 ==========

// 兼容性结构体 - 用于处理旧格式数据
type WatchlistStockLegacy struct {
	Code     string   `json:"code"`
	Name     string   `json:"name"`
	Tag      string   `json:"tag,omitempty"`  // 旧格式的单个标签
	Tags     []string `json:"tags,omitempty"` // 新格式的多个标签
	FundFlow FundFlow `json:"fund_flow"`      // 资金流向数据
}

type WatchlistLegacy struct {
	Stocks []WatchlistStockLegacy `json:"stocks"`
}

// 加载自选股票列表
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
			Code:     legacyStock.Code,
			Name:     legacyStock.Name,
			FundFlow: legacyStock.FundFlow,
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

// 保存自选股票列表
func (m *Model) saveWatchlist() {
	data, err := json.MarshalIndent(m.watchlist, "", "  ")
	if err != nil {
		return
	}
	os.WriteFile(watchlistFile, data, 0644)
}

// 获取所有可用的标签
func (m *Model) getAvailableTags() []string {
	tagMap := make(map[string]bool)

	for _, stock := range m.watchlist.Stocks {
		for _, tag := range stock.Tags {
			if tag != "" && tag != "-" {
				tagMap[tag] = true
			}
		}
	}

	tags := make([]string, 0, len(tagMap))
	for tag := range tagMap {
		tags = append(tags, tag)
	}

	return tags
}

// 检查股票是否包含指定标签
func (stock *WatchlistStock) hasTag(tag string) bool {
	for _, t := range stock.Tags {
		if t == tag {
			return true
		}
	}
	return false
}

// 添加标签到股票（避免重复）
func (stock *WatchlistStock) addTag(tag string) {
	if tag == "" || tag == "-" {
		return
	}
	if !stock.hasTag(tag) {
		stock.Tags = append(stock.Tags, tag)
	}
}

// 移除股票的标签
func (stock *WatchlistStock) removeTag(tag string) {
	for i, t := range stock.Tags {
		if t == tag {
			stock.Tags = append(stock.Tags[:i], stock.Tags[i+1:]...)
			break
		}
	}
}

// 获取股票标签的显示字符串
func (stock *WatchlistStock) getTagsDisplay() string {
	if len(stock.Tags) == 0 {
		return "-"
	}

	// 过滤掉空标签和默认标签
	var validTags []string
	for _, tag := range stock.Tags {
		if tag != "" && tag != "-" {
			validTags = append(validTags, tag)
		}
	}

	if len(validTags) == 0 {
		return "-"
	}

	if len(validTags) == 1 {
		return validTags[0]
	}

	// 多个标签时，用逗号分隔，但如果太长则显示数量
	display := validTags[0]
	if len(validTags) > 1 {
		totalLen := len(display)
		for _, tag := range validTags[1:] {
			totalLen += len(tag) + 1 // +1 for comma
		}

		if totalLen > 15 { // 如果总长度超过15字符，显示数量
			return fmt.Sprintf("%s+%d", validTags[0], len(validTags)-1)
		} else {
			for _, tag := range validTags[1:] {
				display += "," + tag
			}
		}
	}

	return display
}

// 根据标签过滤自选股票（带缓存优化）
func (m *Model) getFilteredWatchlist() []WatchlistStock {
	// 如果没有过滤标签，直接返回完整列表
	if m.selectedTag == "" {
		return m.watchlist.Stocks
	}

	// 检查缓存是否有效
	if m.isFilteredWatchlistValid && m.cachedFilterTag == m.selectedTag {
		return m.cachedFilteredWatchlist
	}

	// 重新计算过滤结果并缓存
	var filtered []WatchlistStock
	for _, stock := range m.watchlist.Stocks {
		if stock.hasTag(m.selectedTag) {
			filtered = append(filtered, stock)
		}
	}

	// 更新缓存
	m.cachedFilteredWatchlist = filtered
	m.cachedFilterTag = m.selectedTag
	m.isFilteredWatchlistValid = true

	return filtered
}

// 使缓存失效的辅助函数
func (m *Model) invalidateWatchlistCache() {
	m.isFilteredWatchlistValid = false
	m.cachedFilteredWatchlist = nil
	m.cachedFilterTag = ""
}

// 重置持股列表游标到第一只股票
func (m *Model) resetPortfolioCursor() {
	if len(m.portfolio.Stocks) > 0 {
		m.portfolioCursor = 0
		maxPortfolioLines := m.config.Display.MaxLines
		if len(m.portfolio.Stocks) > maxPortfolioLines {
			// 显示前N条：滚动位置设置为显示从索引0开始的N条
			m.portfolioScrollPos = len(m.portfolio.Stocks) - maxPortfolioLines
		} else {
			// 股票数量不超过显示行数，显示全部
			m.portfolioScrollPos = 0
		}
	}
}

// 重置自选列表游标到第一只股票（基于过滤后的列表）
func (m *Model) resetWatchlistCursor() {
	filteredStocks := m.getFilteredWatchlist()
	if len(filteredStocks) > 0 {
		m.watchlistCursor = 0
		maxWatchlistLines := m.config.Display.MaxLines
		if len(filteredStocks) > maxWatchlistLines {
			// 显示前N条：滚动位置设置为显示从索引0开始的N条
			m.watchlistScrollPos = len(filteredStocks) - maxWatchlistLines
		} else {
			// 股票数量不超过显示行数，显示全部
			m.watchlistScrollPos = 0
		}
	} else {
		// 没有股票时重置
		m.watchlistCursor = 0
		m.watchlistScrollPos = 0
	}
}

// 调整自选列表滚动位置（基于过滤后的列表）
func (m *Model) adjustWatchlistScroll(filteredStocks []WatchlistStock) {
	maxWatchlistLines := m.config.Display.MaxLines
	totalStocks := len(filteredStocks)

	if totalStocks <= maxWatchlistLines {
		m.watchlistScrollPos = 0
		return
	}

	// 确保光标在可见范围内
	endIndex := totalStocks - m.watchlistScrollPos
	startIndex := endIndex - maxWatchlistLines
	if startIndex < 0 {
		startIndex = 0
	}

	// 如果光标超出可见范围的上边界，调整滚动位置
	if m.watchlistCursor < startIndex {
		m.watchlistScrollPos = totalStocks - m.watchlistCursor - maxWatchlistLines
		if m.watchlistScrollPos < 0 {
			m.watchlistScrollPos = 0
		}
	}

	// 如果光标超出可见范围的下边界，调整滚动位置
	if m.watchlistCursor >= endIndex {
		m.watchlistScrollPos = totalStocks - m.watchlistCursor - 1
		if m.watchlistScrollPos < 0 {
			m.watchlistScrollPos = 0
		}
	}
}

// 处理自选股票打标签
func (m *Model) handleWatchlistTagging(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		if m.tagInput == "" {
			// 回到标签管理界面
			m.availableTags = m.getAvailableTags()
			m.state = WatchlistTagManage
			m.tagManageCursor = 0
			return m, nil
		}

		// 更新当前选中股票的标签（基于过滤后的列表）
		filteredStocks := m.getFilteredWatchlist()
		if m.watchlistCursor >= 0 && m.watchlistCursor < len(filteredStocks) {
			stockToTag := filteredStocks[m.watchlistCursor]

			// 在原始列表中找到该股票并添加标签
			for i, stock := range m.watchlist.Stocks {
				if stock.Code == stockToTag.Code {
					// 处理多个标签（逗号分隔）
					newTags := strings.Split(m.tagInput, ",")
					for _, tag := range newTags {
						tag = strings.TrimSpace(tag)
						if tag != "" && tag != "-" {
							m.watchlist.Stocks[i].addTag(tag)
						}
					}
					// 如果没有有效标签，确保至少有默认标签
					if len(m.watchlist.Stocks[i].Tags) == 0 {
						m.watchlist.Stocks[i].Tags = []string{"-"}
					}

					// 更新当前股票标签列表
					m.currentStockTags = make([]string, 0)
					for _, tag := range m.watchlist.Stocks[i].Tags {
						if tag != "" && tag != "-" {
							m.currentStockTags = append(m.currentStockTags, tag)
						}
					}
					break
				}
			}

			m.invalidateWatchlistCache() // 使缓存失效
			m.saveWatchlist()

			if m.language == Chinese {
				m.message = fmt.Sprintf("已为 %s 添加标签: %s",
					stockToTag.Name, m.tagInput)
			} else {
				m.message = fmt.Sprintf("Added tags to %s: %s",
					stockToTag.Name, m.tagInput)
			}
		}

		// 回到标签管理界面，更新可用标签列表
		m.availableTags = m.getAvailableTags()
		m.state = WatchlistTagManage
		m.tagManageCursor = 0
		m.tagInput = ""
		return m, nil
	case "esc", "q":
		// 回到标签管理界面
		m.availableTags = m.getAvailableTags()
		m.state = WatchlistTagManage
		m.tagManageCursor = 0
		m.tagInput = ""
		m.message = ""
		return m, nil
	case "backspace":
		if len(m.tagInput) > 0 {
			// 正确处理UTF-8字符（包括中文）的删除
			runes := []rune(m.tagInput)
			if len(runes) > 0 {
				m.tagInput = string(runes[:len(runes)-1])
			}
		}
		return m, nil
	default:
		// 使用与项目其他输入处理相同的逻辑，支持中文字符
		str := msg.String()
		if len(str) > 0 && str != "\n" && str != "\r" && !isControlKey(str) {
			m.tagInput += str
		}
		return m, nil
	}
}

// 处理自选股票分组选择
func (m *Model) handleWatchlistGroupSelect(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		if m.cursor >= 0 && m.cursor < len(m.availableTags) {
			m.selectedTag = m.availableTags[m.cursor]
		}
		m.invalidateWatchlistCache() // 使缓存失效
		m.state = WatchlistViewing
		m.message = ""
		m.resetWatchlistCursor() // 重置游标到第一只股票（考虑过滤）
		return m, m.tickCmd() // 重启定时器
	case "esc", "q":
		m.selectedTag = ""           // 清除过滤
		m.invalidateWatchlistCache() // 使缓存失效
		m.state = WatchlistViewing
		m.resetWatchlistCursor() // 重置游标到第一只股票
		m.message = ""
		return m, m.tickCmd() // 重启定时器
	case "c":
		// 清除过滤，显示所有股票
		m.selectedTag = ""
		m.invalidateWatchlistCache() // 使缓存失效
		m.state = WatchlistViewing
		m.resetWatchlistCursor() // 重置游标到第一只股票
		m.message = ""
		return m, m.tickCmd() // 重启定时器
	case "up", "k", "w":
		if m.cursor > 0 {
			m.cursor--
		}
		return m, nil
	case "down", "j", "s":
		if m.cursor < len(m.availableTags)-1 {
			m.cursor++
		}
		return m, nil
	}
	return m, nil
}

// 打标签视图
func (m *Model) viewWatchlistTagging() string {
	var s string

	if m.language == Chinese {
		s += "=== 设置标签 ===\n\n"
	} else {
		s += "=== Set Tag ===\n\n"
	}

	filteredStocks := m.getFilteredWatchlist()
	if m.watchlistCursor >= 0 && m.watchlistCursor < len(filteredStocks) {
		stock := filteredStocks[m.watchlistCursor]
		if m.language == Chinese {
			s += fmt.Sprintf("股票: %s (%s)\n", stock.Name, stock.Code)
			s += fmt.Sprintf("当前标签: %s\n\n", stock.getTagsDisplay())
			s += "请输入新标签(多个标签用逗号分隔): " + m.tagInput + "_\n\n"
			s += "按Enter确认，ESC或Q键取消"
		} else {
			s += fmt.Sprintf("Stock: %s (%s)\n", stock.Name, stock.Code)
			s += fmt.Sprintf("Current tags: %s\n\n", stock.getTagsDisplay())
			s += "Enter new tags (comma separated): " + m.tagInput + "_\n\n"
			s += "Press Enter to confirm, ESC or Q to cancel"
		}
	}

	return s
}

// 分组选择视图
func (m *Model) viewWatchlistGroupSelect() string {
	var s string

	if m.language == Chinese {
		s += "=== 选择标签分组 ===\n\n"
	} else {
		s += "=== Select Tag Group ===\n\n"
	}

	// 显示当前过滤状态
	if m.selectedTag != "" {
		if m.language == Chinese {
			s += fmt.Sprintf("当前过滤: %s\n\n", m.selectedTag)
		} else {
			s += fmt.Sprintf("Current filter: %s\n\n", m.selectedTag)
		}
	}

	// 显示标签选项
	for i, tag := range m.availableTags {
		cursor := " "
		if i == m.cursor {
			cursor = "►"
		}
		s += fmt.Sprintf("%s %s\n", cursor, tag)
	}

	s += "\n"
	if m.language == Chinese {
		s += "按Enter选择标签，C键清除过滤，ESC或Q键返回"
	} else {
		s += "Press Enter to select tag, C to clear filter, ESC or Q to return"
	}

	return s
}

// 处理自选股票标签选择
func (m *Model) handleWatchlistTagSelect(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		// 根据当前选择的选项来执行操作
		if m.tagSelectCursor == len(m.availableTags) {
			// 选择了"手动输入新标签"选项
			m.state = WatchlistTagging
			m.tagInput = ""
			return m, nil
		} else if m.tagSelectCursor >= 0 && m.tagSelectCursor < len(m.availableTags) {
			// 选择了现有标签
			selectedTag := m.availableTags[m.tagSelectCursor]

			// 更新当前选中股票的标签（基于过滤后的列表）
			filteredStocks := m.getFilteredWatchlist()
			if m.watchlistCursor >= 0 && m.watchlistCursor < len(filteredStocks) {
				stockToTag := filteredStocks[m.watchlistCursor]

				// 在原始列表中找到该股票并添加标签
				for i, stock := range m.watchlist.Stocks {
					if stock.Code == stockToTag.Code {
						m.watchlist.Stocks[i].addTag(selectedTag)
						break
					}
				}

				m.invalidateWatchlistCache() // 使缓存失效
				m.saveWatchlist()

				if m.language == Chinese {
					m.message = fmt.Sprintf("已为 %s 添加标签: %s",
						stockToTag.Name, selectedTag)
				} else {
					m.message = fmt.Sprintf("Added tag to %s: %s",
						stockToTag.Name, selectedTag)
				}
			}

			m.state = WatchlistViewing
			m.tagInput = ""
			m.resetWatchlistCursor() // 重置游标到第一只股票
			return m, m.tickCmd() // 重启定时器
		}
		return m, nil
	case "d":
		// 进入标签删除选择模式
		filteredStocks := m.getFilteredWatchlist()
		if m.watchlistCursor >= 0 && m.watchlistCursor < len(filteredStocks) {
			stockToModify := filteredStocks[m.watchlistCursor]

			// 获取该股票的有效标签（排除默认标签）
			var validTags []string
			for _, stock := range m.watchlist.Stocks {
				if stock.Code == stockToModify.Code {
					for _, tag := range stock.Tags {
						if tag != "" && tag != "-" {
							validTags = append(validTags, tag)
						}
					}
					break
				}
			}

			if len(validTags) == 0 {
				if m.language == Chinese {
					m.message = fmt.Sprintf("%s 没有可删除的标签", stockToModify.Name)
				} else {
					m.message = fmt.Sprintf("%s has no tags to remove", stockToModify.Name)
				}
				return m, nil
			}

			// 设置删除标签的状态
			m.currentStockTags = validTags
			m.tagRemoveCursor = 0
			m.state = WatchlistTagRemoveSelect
			return m, nil
		}
		return m, nil
	case "esc", "q":
		m.state = WatchlistViewing
		m.tagInput = ""
		m.message = ""
		m.resetWatchlistCursor() // 重置游标到第一只股票
		return m, m.tickCmd() // 重启定时器
	case "up", "k", "w":
		if m.tagSelectCursor > 0 {
			m.tagSelectCursor--
		}
		return m, nil
	case "down", "j", "s":
		maxCursor := len(m.availableTags) // 包括"手动输入新标签"选项
		if m.tagSelectCursor < maxCursor {
			m.tagSelectCursor++
		}
		return m, nil
	}
	return m, nil
}

// 标签选择视图
func (m *Model) viewWatchlistTagSelect() string {
	var s string

	if m.language == Chinese {
		s += "=== 管理标签 ===\n\n"
	} else {
		s += "=== Manage Tags ===\n\n"
	}

	filteredStocks := m.getFilteredWatchlist()
	if m.watchlistCursor >= 0 && m.watchlistCursor < len(filteredStocks) {
		stock := filteredStocks[m.watchlistCursor]
		if m.language == Chinese {
			s += fmt.Sprintf("股票: %s (%s)\n", stock.Name, stock.Code)
			s += fmt.Sprintf("当前标签: %s\n\n", stock.getTagsDisplay())

			// 显示该股票的标签，供删除使用
			if len(stock.Tags) > 0 {
				hasValidTags := false
				for _, tag := range stock.Tags {
					if tag != "" && tag != "-" {
						hasValidTags = true
						break
					}
				}
				if hasValidTags {
					s += "当前标签(按D键删除):\n"
					for _, tag := range stock.Tags {
						if tag != "" && tag != "-" {
							s += fmt.Sprintf("  • %s\n", tag)
						}
					}
					s += "\n"
				}
			}
		} else {
			s += fmt.Sprintf("Stock: %s (%s)\n", stock.Name, stock.Code)
			s += fmt.Sprintf("Current tags: %s\n\n", stock.getTagsDisplay())

			// 显示该股票的标签，供删除使用
			if len(stock.Tags) > 0 {
				hasValidTags := false
				for _, tag := range stock.Tags {
					if tag != "" && tag != "-" {
						hasValidTags = true
						break
					}
				}
				if hasValidTags {
					s += "Current tags (press D to remove):\n"
					for _, tag := range stock.Tags {
						if tag != "" && tag != "-" {
							s += fmt.Sprintf("  • %s\n", tag)
						}
					}
					s += "\n"
				}
			}
		}
	}

	// 显示现有标签选项
	if len(m.availableTags) > 0 {
		if m.language == Chinese {
			s += "可添加的系统标签:\n"
		} else {
			s += "Available system tags to add:\n"
		}

		for i, tag := range m.availableTags {
			cursor := "  "
			if i == m.tagSelectCursor {
				cursor = "► "
			}
			s += fmt.Sprintf("%s%s\n", cursor, tag)
		}
		s += "\n"
	}

	// 添加"手动输入新标签"选项
	cursor := "  "
	if m.tagSelectCursor == len(m.availableTags) {
		cursor = "► "
	}
	if m.language == Chinese {
		s += fmt.Sprintf("%s手动输入新标签\n\n", cursor)
		s += "操作: ↑↓选择 Enter添加标签 D进入删除模式 ESC/Q取消"
	} else {
		s += fmt.Sprintf("%sManually enter new tag\n\n", cursor)
		s += "Actions: ↑↓ select, Enter add tag, D enter remove mode, ESC/Q cancel"
	}

	return s
}

// ========== 新的标签管理界面 ==========

// 处理标签管理界面
func (m *Model) handleWatchlistTagManage(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.state = WatchlistViewing
		m.message = ""
		m.resetWatchlistCursor()
		return m, m.tickCmd() // 重启定时器
	case "n":
		// 手动输入新标签
		m.state = WatchlistTagging
		m.tagInput = ""
		return m, nil
	case "d":
		// 删除当前选中的标签（如果当前股票拥有该标签）
		if len(m.availableTags) == 0 {
			if m.language == Chinese {
				m.message = "没有可删除的标签"
			} else {
				m.message = "No tags to remove"
			}
			return m, nil
		}

		// 获取当前选中的标签
		selectedTag := m.availableTags[m.tagManageCursor]

		// 检查当前股票是否拥有这个标签
		filteredStocks := m.getFilteredWatchlist()
		if m.watchlistCursor >= 0 && m.watchlistCursor < len(filteredStocks) {
			currentStock := filteredStocks[m.watchlistCursor]

			// 查找并删除标签
			stockFound := false
			for i, stock := range m.watchlist.Stocks {
				if stock.Code == currentStock.Code {
					if stock.hasTag(selectedTag) {
						m.watchlist.Stocks[i].removeTag(selectedTag)
						m.saveWatchlist()
						m.invalidateWatchlistCache()

						// 更新当前股票标签列表
						m.currentStockTags = make([]string, 0)
						for _, tag := range m.watchlist.Stocks[i].Tags {
							if tag != "" && tag != "-" {
								m.currentStockTags = append(m.currentStockTags, tag)
							}
						}

						// 更新可用标签列表
						m.availableTags = m.getAvailableTags()

						// 调整光标位置
						if m.tagManageCursor >= len(m.availableTags) && len(m.availableTags) > 0 {
							m.tagManageCursor = len(m.availableTags) - 1
						}

						if m.language == Chinese {
							m.message = fmt.Sprintf("已删除标签: %s", selectedTag)
						} else {
							m.message = fmt.Sprintf("Removed tag: %s", selectedTag)
						}
						stockFound = true
					} else {
						if m.language == Chinese {
							m.message = fmt.Sprintf("该股票没有标签: %s", selectedTag)
						} else {
							m.message = fmt.Sprintf("Stock doesn't have tag: %s", selectedTag)
						}
						stockFound = true
					}
					break
				}
			}

			if !stockFound {
				if m.language == Chinese {
					m.message = "找不到对应的股票"
				} else {
					m.message = "Stock not found"
				}
			}
		}
		return m, nil
	case "up", "k", "w":
		if len(m.availableTags) > 0 && m.tagManageCursor > 0 {
			m.tagManageCursor--
		}
		return m, nil
	case "down", "j", "s":
		if len(m.availableTags) > 0 && m.tagManageCursor < len(m.availableTags)-1 {
			m.tagManageCursor++
		}
		return m, nil
	case "enter":
		// 为当前股票添加选中的标签
		if len(m.availableTags) == 0 {
			if m.language == Chinese {
				m.message = "没有可添加的标签，按N键创建新标签"
			} else {
				m.message = "No tags to add, press N to create new tag"
			}
			return m, nil
		}

		selectedTag := m.availableTags[m.tagManageCursor]

		// 获取当前选中的股票
		filteredStocks := m.getFilteredWatchlist()
		if m.watchlistCursor >= 0 && m.watchlistCursor < len(filteredStocks) {
			currentStock := filteredStocks[m.watchlistCursor]

			// 查找并添加标签
			stockFound := false
			for i, stock := range m.watchlist.Stocks {
				if stock.Code == currentStock.Code {
					if !stock.hasTag(selectedTag) {
						m.watchlist.Stocks[i].addTag(selectedTag)
						m.saveWatchlist()
						m.invalidateWatchlistCache()

						// 更新当前股票标签列表
						m.currentStockTags = make([]string, 0)
						for _, tag := range m.watchlist.Stocks[i].Tags {
							if tag != "" && tag != "-" {
								m.currentStockTags = append(m.currentStockTags, tag)
							}
						}

						if m.language == Chinese {
							m.message = fmt.Sprintf("已添加标签: %s", selectedTag)
						} else {
							m.message = fmt.Sprintf("Added tag: %s", selectedTag)
						}
					} else {
						if m.language == Chinese {
							m.message = fmt.Sprintf("该股票已有标签: %s", selectedTag)
						} else {
							m.message = fmt.Sprintf("Stock already has tag: %s", selectedTag)
						}
					}
					stockFound = true
					break
				}
			}

			if !stockFound {
				if m.language == Chinese {
					m.message = "找不到对应的股票"
				} else {
					m.message = "Stock not found"
				}
			}
		}
		return m, nil
	}
	return m, nil
}

// 处理标签删除选择界面
func (m *Model) handleWatchlistTagRemoveSelect(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.state = WatchlistTagManage
		return m, nil
	case "enter":
		if m.tagRemoveCursor >= 0 && m.tagRemoveCursor < len(m.currentStockTags) {
			tagToRemove := m.currentStockTags[m.tagRemoveCursor]

			// 从当前股票中删除选中的标签
			filteredStocks := m.getFilteredWatchlist()
			if m.watchlistCursor >= 0 && m.watchlistCursor < len(filteredStocks) {
				stockToModify := filteredStocks[m.watchlistCursor]

				// 在原始列表中找到该股票并删除指定标签
				for i, stock := range m.watchlist.Stocks {
					if stock.Code == stockToModify.Code {
						m.watchlist.Stocks[i].removeTag(tagToRemove)
						// 如果删除后没有标签，添加默认标签
						if len(m.watchlist.Stocks[i].Tags) == 0 {
							m.watchlist.Stocks[i].Tags = []string{"-"}
						}
						break
					}
				}

				m.invalidateWatchlistCache()
				m.saveWatchlist()

				// 更新当前股票标签列表
				m.currentStockTags = make([]string, 0)
				for _, stock := range m.watchlist.Stocks {
					if stock.Code == stockToModify.Code {
						for _, tag := range stock.Tags {
							if tag != "" && tag != "-" {
								m.currentStockTags = append(m.currentStockTags, tag)
							}
						}
						break
					}
				}

				if m.language == Chinese {
					m.message = fmt.Sprintf("已从 %s 删除标签: %s", stockToModify.Name, tagToRemove)
				} else {
					m.message = fmt.Sprintf("Removed tag from %s: %s", stockToModify.Name, tagToRemove)
				}

				// 如果没有更多标签可删除，返回标签管理界面
				if len(m.currentStockTags) == 0 {
					m.state = WatchlistTagManage
				} else {
					// 调整光标位置
					if m.tagRemoveCursor >= len(m.currentStockTags) {
						m.tagRemoveCursor = len(m.currentStockTags) - 1
					}
				}
			}
		}
		return m, nil
	case "up", "k", "w":
		if m.tagRemoveCursor > 0 {
			m.tagRemoveCursor--
		}
		return m, nil
	case "down", "j", "s":
		if m.tagRemoveCursor < len(m.currentStockTags)-1 {
			m.tagRemoveCursor++
		}
		return m, nil
	}
	return m, nil
}

// 标签管理界面视图
func (m *Model) viewWatchlistTagManage() string {
	var s string

	if m.language == Chinese {
		s += "=== 标签管理 ===\n\n"
	} else {
		s += "=== Tag Management ===\n\n"
	}

	filteredStocks := m.getFilteredWatchlist()
	if m.watchlistCursor >= 0 && m.watchlistCursor < len(filteredStocks) {
		stock := filteredStocks[m.watchlistCursor]
		if m.language == Chinese {
			s += fmt.Sprintf("股票: %s (%s)\n", stock.Name, stock.Code)
			s += fmt.Sprintf("当前标签: %s\n\n", stock.getTagsDisplay())
		} else {
			s += fmt.Sprintf("Stock: %s (%s)\n", stock.Name, stock.Code)
			s += fmt.Sprintf("Current tags: %s\n\n", stock.getTagsDisplay())
		}

		// 显示所有可用标签，标记当前股票拥有的标签
		if len(m.availableTags) > 0 {
			if m.language == Chinese {
				s += "所有可用标签:\n"
			} else {
				s += "All available tags:\n"
			}

			for i, tag := range m.availableTags {
				cursor := "  "
				if i == m.tagManageCursor {
					cursor = "► "
				}

				// 检查当前股票是否拥有这个标签
				hasTag := stock.hasTag(tag)
				status := ""
				if hasTag {
					if m.language == Chinese {
						status = " ✓ (已拥有)"
					} else {
						status = " ✓ (owned)"
					}
				}

				s += fmt.Sprintf("%s%s%s\n", cursor, tag, status)
			}
			s += "\n"
		} else {
			if m.language == Chinese {
				s += "暂无可用标签，按N键创建新标签\n\n"
			} else {
				s += "No available tags, press N to create new tag\n\n"
			}
		}

		// 操作提示
		if m.language == Chinese {
			s += "操作说明:\n"
			s += "  ↑↓ - 选择标签\n"
			s += "  Enter - 添加/切换选中标签\n"
			s += "  D - 删除选中标签(如果当前股票拥有)\n"
			s += "  N - 创建新标签\n"
			s += "  ESC/Q - 返回自选列表\n"
		} else {
			s += "Actions:\n"
			s += "  ↑↓ - Select tag\n"
			s += "  Enter - Add/toggle selected tag\n"
			s += "  D - Remove selected tag (if owned by current stock)\n"
			s += "  N - Create new tag\n"
			s += "  ESC/Q - Return to watchlist\n"
		}
	}

	return s
}

// 标签删除选择界面视图
func (m *Model) viewWatchlistTagRemoveSelect() string {
	var s string

	if m.language == Chinese {
		s += "=== 选择要删除的标签 ===\n\n"
	} else {
		s += "=== Select Tag to Remove ===\n\n"
	}

	filteredStocks := m.getFilteredWatchlist()
	if m.watchlistCursor >= 0 && m.watchlistCursor < len(filteredStocks) {
		stock := filteredStocks[m.watchlistCursor]
		if m.language == Chinese {
			s += fmt.Sprintf("股票: %s (%s)\n\n", stock.Name, stock.Code)
			s += "请选择要删除的标签:\n\n"
		} else {
			s += fmt.Sprintf("Stock: %s (%s)\n\n", stock.Name, stock.Code)
			s += "Select tag to remove:\n\n"
		}

		// 显示可删除的标签
		for i, tag := range m.currentStockTags {
			cursor := "  "
			if i == m.tagRemoveCursor {
				cursor = "► "
			}
			s += fmt.Sprintf("%s%s\n", cursor, tag)
		}

		s += "\n"
		if m.language == Chinese {
			s += "操作: ↑↓选择标签 Enter删除 ESC/Q取消"
		} else {
			s += "Actions: ↑↓ select tag, Enter remove, ESC/Q cancel"
		}
	}

	return s
}

// 检查股票是否已在自选列表中
func (m *Model) isStockInWatchlist(code string) bool {
	for _, stock := range m.watchlist.Stocks {
		if stock.Code == code {
			return true
		}
	}
	return false
}

// 添加股票到自选列表
func (m *Model) addToWatchlist(code, name string) bool {
	if m.isStockInWatchlist(code) {
		return false // 已在列表中
	}

	watchStock := WatchlistStock{
		Code: code,
		Name: name,
		Tags: []string{"-"}, // 默认标签
	}
	// 将新股票插入到列表首位，而不是末尾
	m.watchlist.Stocks = append([]WatchlistStock{watchStock}, m.watchlist.Stocks...)
	m.invalidateWatchlistCache() // 使缓存失效
	m.watchlistIsSorted = false  // 添加自选股票后重置自选列表排序状态
	m.saveWatchlist()
	return true
}

// 从自选列表删除股票
func (m *Model) removeFromWatchlist(index int) {
	if index >= 0 && index < len(m.watchlist.Stocks) {
		m.watchlist.Stocks = append(m.watchlist.Stocks[:index], m.watchlist.Stocks[index+1:]...)
		m.invalidateWatchlistCache() // 使缓存失效
		m.saveWatchlist()
		m.watchlistIsSorted = false // 删除自选股票后重置自选列表排序状态
	}
}

// ========== 搜索结果带操作按钮处理 ==========

func (m *Model) handleSearchResultWithActions(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.state = MainMenu
		m.message = ""
		return m, nil
	case "r":
		m.state = SearchingStock
		m.searchFromWatchlist = false
		m.message = ""
		return m, nil
	case "1":
		// 添加到自选列表并跳转到自选页面
		if m.searchResult != nil {
			if m.addToWatchlist(m.searchResult.Symbol, m.searchResult.Name) {
				m.message = fmt.Sprintf(m.getText("addWatchSuccess"), m.searchResult.Name, m.searchResult.Symbol)
			} else {
				m.message = fmt.Sprintf(m.getText("alreadyInWatch"), m.searchResult.Symbol)
			}
			// 跳转到自选列表页面
			m.state = WatchlistViewing
			m.resetWatchlistCursor() // 重置游标到第一只股票
			m.cursor = 0
			m.lastUpdate = time.Now()
		}
		return m, m.tickCmd()
	case "2":
		// 添加到持股列表（进入添加流程）
		if m.searchResult != nil {
			m.state = AddingStock
			m.addingStep = 1 // 跳过代码输入，直接到成本价输入
			m.tempCode = m.searchResult.Symbol
			m.stockInfo = &StockData{
				Symbol: m.searchResult.Symbol,
				Name:   m.searchResult.Name,
				Price:  m.searchResult.Price,
			}
			m.input = ""
			m.message = ""
			m.fromSearch = true // 标记从搜索结果添加
		}
		return m, nil
	}
	return m, nil
}

func (m *Model) viewSearchResultWithActions() string {
	s := m.getText("detailTitle") + "\n\n"

	if m.searchResult == nil {
		s += m.getText("noInfo") + "\n"
		s += "\n" + m.getText("actionHelp") + "\n"
		return s
	}

	// 复用原有的搜索结果显示逻辑
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

	// 资金流向数据（仅A股显示）
	if isChinaStock(m.searchResult.Symbol) {
		fundFlow := &m.searchResult.FundFlow

		// 主力净流入
		if m.language == Chinese {
			headers = append(headers, "主力净流入")
		} else {
			headers = append(headers, "Main Flow")
		}
		mainFlowStr := m.formatFundFlowWithColorAndUnit(fundFlow.MainNetInflow)
		values = append(values, mainFlowStr)

		// 超大单净流入
		if m.language == Chinese {
			headers = append(headers, "超大单")
		} else {
			headers = append(headers, "Super Large")
		}
		superLargeStr := m.formatFundFlowWithColorAndUnit(fundFlow.SuperLargeNetInflow)
		values = append(values, superLargeStr)

		// 大单净流入
		if m.language == Chinese {
			headers = append(headers, "大单")
		} else {
			headers = append(headers, "Large")
		}
		largeStr := m.formatFundFlowWithColorAndUnit(fundFlow.LargeNetInflow)
		values = append(values, largeStr)

		// 中单净流入
		if m.language == Chinese {
			headers = append(headers, "中单")
		} else {
			headers = append(headers, "Medium")
		}
		mediumStr := m.formatFundFlowWithColorAndUnit(fundFlow.MediumNetInflow)
		values = append(values, mediumStr)

		// 小单净流入
		if m.language == Chinese {
			headers = append(headers, "小单")
		} else {
			headers = append(headers, "Small")
		}
		smallStr := m.formatFundFlowWithColorAndUnit(fundFlow.SmallNetInflow)
		values = append(values, smallStr)

		// 净流入占比
		if m.language == Chinese {
			headers = append(headers, "净流入占比")
		} else {
			headers = append(headers, "Net Ratio")
		}
		flowRatioStr := m.formatProfitRateWithColorZeroLang(fundFlow.NetInflowRatio)
		values = append(values, flowRatioStr)
	}

	// 添加表头和数据行
	t.AppendHeader(table.Row(headers))
	t.AppendRow(table.Row(values))

	s += t.Render() + "\n\n"

	// 操作按钮提示
	s += m.getText("actionHelp") + "\n"

	if m.message != "" {
		s += "\n" + m.message + "\n"
	}

	return s
}

// ========== 自选股票查看处理 ==========

func (m *Model) handleWatchlistViewing(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "m":
		m.state = MainMenu
		m.message = ""
		return m, nil
	case "d":
		// 直接删除光标指向的自选股票
		filteredStocks := m.getFilteredWatchlist()
		if len(filteredStocks) == 0 {
			m.message = m.getText("emptyWatchlist")
			return m, nil
		}

		// 获取要删除的股票（从过滤列表中）
		if m.watchlistCursor >= 0 && m.watchlistCursor < len(filteredStocks) {
			stockToRemove := filteredStocks[m.watchlistCursor]

			// 在原始列表中找到该股票并删除
			for i, stock := range m.watchlist.Stocks {
				if stock.Code == stockToRemove.Code {
					m.removeFromWatchlist(i)
					break
				}
			}

			// 调整光标位置（基于过滤后的列表）
			newFilteredStocks := m.getFilteredWatchlist()
			if m.watchlistCursor >= len(newFilteredStocks) && len(newFilteredStocks) > 0 {
				m.watchlistCursor = len(newFilteredStocks) - 1
			}

			m.message = fmt.Sprintf(m.getText("removeWatchSuccess"), stockToRemove.Name, stockToRemove.Code)
		}
		return m, nil
	case "a":
		// 跳转到股票搜索页面
		m.logUserAction("从自选列表跳转到股票搜索页面")
		m.state = SearchingStock
		m.searchInput = ""
		m.searchResult = nil
		m.searchFromWatchlist = true
		m.message = ""
		return m, nil
	case "s":
		// 进入排序菜单
		m.logUserAction("从自选列表进入排序菜单")
		m.state = WatchlistSorting
		// 智能定位光标到当前排序字段
		m.watchlistSortCursor = m.findSortFieldIndex(m.watchlistSortField, false)
		m.message = ""
		return m, nil
	case "t":
		// 给当前选中的股票管理标签 - 进入标签管理界面
		filteredStocks := m.getFilteredWatchlist()
		if len(filteredStocks) == 0 {
			m.message = m.getText("emptyWatchlist")
			return m, nil
		}

		// 获取当前选中股票的标签信息
		currentStock := filteredStocks[m.watchlistCursor]
		m.currentStockTags = make([]string, 0)
		for _, tag := range currentStock.Tags {
			if tag != "" && tag != "-" {
				m.currentStockTags = append(m.currentStockTags, tag)
			}
		}

		// 获取所有可用标签
		m.availableTags = m.getAvailableTags()
		m.state = WatchlistTagManage
		m.tagManageCursor = 0
		m.tagInput = ""
		m.isInRemoveMode = false
		m.message = ""
		return m, nil
	case "g":
		// 分组查看
		m.availableTags = m.getAvailableTags()
		if len(m.availableTags) == 0 {
			if m.language == Chinese {
				m.message = "没有可用的标签"
			} else {
				m.message = "No available tags"
			}
			return m, nil
		}
		m.state = WatchlistGroupSelect
		m.cursor = 0
		m.message = ""
		return m, nil
	case "c":
		// 清除标签过滤
		if m.selectedTag != "" {
			m.selectedTag = ""
			m.invalidateWatchlistCache() // 使缓存失效
			m.resetWatchlistCursor()     // 重置游标到第一只股票
			if m.language == Chinese {
				m.message = "已清除标签过滤"
			} else {
				m.message = "Tag filter cleared"
			}
		}
		return m, nil
	case "up", "k", "w":
		// 获取一次过滤后的列表，避免重复调用
		filteredStocks := m.getFilteredWatchlist()
		if m.watchlistCursor > 0 {
			m.watchlistCursor--
			// 只在光标移动时调整滚动
			m.adjustWatchlistScroll(filteredStocks)
		}
		return m, nil
	case "down", "j":
		// 获取一次过滤后的列表，避免重复调用
		filteredStocks := m.getFilteredWatchlist()
		if m.watchlistCursor < len(filteredStocks)-1 {
			m.watchlistCursor++
			// 只在光标移动时调整滚动
			m.adjustWatchlistScroll(filteredStocks)
		}
		return m, nil
	}
	return m, nil
}

func (m *Model) viewWatchlistViewing() string {
	s := m.getText("watchlistTitle") + "\n"
	s += fmt.Sprintf(m.getText("updateTime"), m.lastUpdate.Format("2006-01-02 15:04:05")) + "\n"

	// 显示当前过滤状态
	if m.selectedTag != "" {
		if m.language == Chinese {
			s += fmt.Sprintf("当前过滤: %s\n", m.selectedTag)
		} else {
			s += fmt.Sprintf("Current filter: %s\n", m.selectedTag)
		}
	}
	s += "\n"

	// 获取过滤后的股票列表
	filteredStocks := m.getFilteredWatchlist()

	if len(filteredStocks) == 0 {
		if m.selectedTag != "" {
			if m.language == Chinese {
				s += fmt.Sprintf("标签 '%s' 下没有股票\n\n", m.selectedTag)
				s += "按G键选择其他标签，或按C键清除过滤\n"
			} else {
				s += fmt.Sprintf("No stocks under tag '%s'\n\n", m.selectedTag)
				s += "Press G to select other tags, or C to clear filter\n"
			}
		} else {
			s += m.getText("emptyWatchlist") + "\n\n"
			s += m.getText("addToWatchFirst") + "\n\n"
		}
		s += m.getText("watchlistHelp") + "\n"
		return s
	}

	// 显示滚动信息
	totalWatchStocks := len(filteredStocks)
	maxWatchlistLines := m.config.Display.MaxLines
	if totalWatchStocks > 0 {
		currentPos := m.watchlistCursor + 1 // 显示从1开始的位置
		if m.language == Chinese {
			s += fmt.Sprintf("⭐ 自选列表 (%d/%d) [↑/↓:翻页]\n", currentPos, totalWatchStocks)
		} else {
			s += fmt.Sprintf("⭐ Watchlist (%d/%d) [↑/↓:scroll]\n", currentPos, totalWatchStocks)
		}
		s += "\n"
	}

	// 创建表格显示自选股票列表
	t := table.NewWriter()
	t.SetStyle(table.StyleLight)

	// 获取带排序指示器的表头
	t.AppendHeader(m.getWatchlistHeaderWithSortIndicator())

	// 计算要显示的股票范围
	endIndex := len(filteredStocks) - m.watchlistScrollPos
	startIndex := endIndex - maxWatchlistLines
	if startIndex < 0 {
		startIndex = 0
	}
	if endIndex > len(filteredStocks) {
		endIndex = len(filteredStocks)
	}

	for i := startIndex; i < endIndex; i++ {
		watchStock := filteredStocks[i]
		// 从缓存获取股价数据（非阻塞）
		stockData := m.getStockPriceFromCache(watchStock.Code)
		// 从缓存获取资金流向数据（非阻塞）
		fundFlowData := m.getFundFlowDataFromCache(watchStock.Code)

		if stockData != nil {
			// 计算今日涨幅：应该基于昨收价，而不是开盘价
			var todayChangeStr string
			// 使用change_percent字段，这是基于昨收价计算的涨跌幅
			if stockData.ChangePercent != 0 {
				todayChangeStr = m.formatProfitRateWithColorZeroLang(stockData.ChangePercent)
			} else {
				todayChangeStr = "-"
			}

			// 换手率显示
			var turnoverStr string
			if stockData.TurnoverRate > 0 {
				turnoverStr = fmt.Sprintf("%.2f%%", stockData.TurnoverRate)
			} else {
				turnoverStr = "-"
			}

			// 成交量显示
			volumeStr := formatVolume(stockData.Volume)

			// 格式化资金流向数据，带单位显示，对非A股显示"-"
			mainFlowStr := m.formatFundFlowWithColorAndUnitForStock(fundFlowData.MainNetInflow, watchStock.Code)
			superLargeStr := m.formatFundFlowWithColorAndUnitForStock(fundFlowData.SuperLargeNetInflow, watchStock.Code)
			largeStr := m.formatFundFlowWithColorAndUnitForStock(fundFlowData.LargeNetInflow, watchStock.Code)
			mediumStr := m.formatFundFlowWithColorAndUnitForStock(fundFlowData.MediumNetInflow, watchStock.Code)
			smallStr := m.formatFundFlowWithColorAndUnitForStock(fundFlowData.SmallNetInflow, watchStock.Code)
			flowRatioStr := m.formatProfitRateWithColorZeroLangForStock(fundFlowData.NetInflowRatio, watchStock.Code)

			// 光标列 - 检查光标是否在当前可见范围内且指向此行
			cursorCol := ""
			if m.watchlistCursor >= startIndex && m.watchlistCursor < endIndex && i == m.watchlistCursor {
				cursorCol = "►"
			}

			t.AppendRow(table.Row{
				cursorCol,
				watchStock.getTagsDisplay(), // 显示标签
				watchStock.Code,
				watchStock.Name,
				m.formatPriceWithColorLang(stockData.Price, stockData.PrevClose),
				fmt.Sprintf("%.3f", stockData.PrevClose),
				m.formatPriceWithColorLang(stockData.StartPrice, stockData.PrevClose),
				m.formatPriceWithColorLang(stockData.MaxPrice, stockData.PrevClose),
				m.formatPriceWithColorLang(stockData.MinPrice, stockData.PrevClose),
				todayChangeStr,
				turnoverStr,
				volumeStr,
				mainFlowStr,   // 主力净流入
				superLargeStr, // 超大单净流入
				largeStr,      // 大单净流入
				mediumStr,     // 中单净流入
				smallStr,      // 小单净流入
				flowRatioStr,  // 净流入占比
			})
		} else {
			// 如果无法获取数据，显示基本信息
			// 光标列 - 检查光标是否在当前可见范围内且指向此行
			cursorCol := ""
			if m.watchlistCursor >= startIndex && m.watchlistCursor < endIndex && i == m.watchlistCursor {
				cursorCol = "►"
			}

			t.AppendRow(table.Row{
				cursorCol,
				watchStock.getTagsDisplay(), // 显示标签
				watchStock.Code,
				watchStock.Name,
				"-",
				"-",
				"-",
				"-",
				"-",
				"-",
				"-",
				"-",
				"-", // 主力净流入
				"-", // 超大单净流入
				"-", // 大单净流入
				"-", // 中单净流入
				"-", // 小单净流入
				"-", // 净流入占比
			})
		}

		// 在每个股票后添加分隔线（除了显示范围内的最后一个）
		if i < endIndex-1 {
			t.AppendSeparator()
		}
	}

	s += t.Render() + "\n"

	// 如果可以滚动，显示滚动指示
	if totalWatchStocks > maxWatchlistLines {
		s += "\n" + strings.Repeat("-", 80) + "\n"
		if m.watchlistScrollPos > 0 {
			if m.language == Chinese {
				s += "↑ 有更新的自选股票 (按↓查看)\n"
			} else {
				s += "↑ Newer watchlist stocks available (press ↓)\n"
			}
		}
		if m.watchlistScrollPos < totalWatchStocks-1 {
			if m.language == Chinese {
				s += "↓ 有更多历史自选股票 (按↑查看)\n"
			} else {
				s += "↓ More watchlist stocks available (press ↑)\n"
			}
		}
	}

	// 使用统一的帮助文本
	s += "\n" + m.getText("watchlistHelp") + "\n"

	if m.message != "" {
		s += "\n" + m.message + "\n"
	}

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

// ========== 自选股票搜索确认处理 ==========

func (m *Model) handleWatchlistSearchConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.state = WatchlistViewing
		m.resetWatchlistCursor() // 重置游标到第一只股票
		m.searchFromWatchlist = false
		m.message = ""
		return m, m.tickCmd() // 重启定时器
	case "enter":
		// 确认添加到自选列表
		if m.searchResult != nil {
			if m.addToWatchlist(m.searchResult.Symbol, m.searchResult.Name) {
				m.message = fmt.Sprintf(m.getText("addWatchSuccess"), m.searchResult.Name, m.searchResult.Symbol)
				m.logUserAction(fmt.Sprintf("添加到自选列表: %s (%s)", m.searchResult.Name, m.searchResult.Symbol))
			} else {
				m.message = fmt.Sprintf(m.getText("alreadyInWatch"), m.searchResult.Symbol)
			}
			m.state = WatchlistViewing
			m.resetWatchlistCursor() // 重置游标到第一只股票
			m.searchFromWatchlist = false
			return m, m.tickCmd()
		}
		return m, nil
	case "r":
		// 重新搜索
		m.state = SearchingStock
		m.searchInput = ""
		m.searchResult = nil
		m.message = ""
		return m, nil
	}
	return m, nil
}

func (m *Model) viewWatchlistSearchConfirm() string {
	if m.searchResult == nil {
		return m.getText("searchNotFound")
	}

	s := m.getText("searchTitle") + "\n\n"

	// 创建表格显示股票信息
	t := table.NewWriter()
	t.SetStyle(table.StyleLight)

	// 设置表头
	if m.language == Chinese {
		t.AppendHeader(table.Row{"名称", "现价", "昨收价", "开盘", "最高", "最低", "今日涨幅", "换手率", "成交量"})
	} else {
		t.AppendHeader(table.Row{"Name", "Price", "PrevClose", "Open", "High", "Low", "Today%", "Turnover", "Volume"})
	}

	// 构建数据行
	var values []interface{}

	// 名称
	values = append(values, m.searchResult.Name)

	// 现价 (带颜色)
	priceStr := m.formatPriceWithColorLang(m.searchResult.Price, m.searchResult.PrevClose)
	values = append(values, priceStr)

	// 昨收价
	values = append(values, fmt.Sprintf("%.3f", m.searchResult.PrevClose))

	// 开盘价
	if m.searchResult.StartPrice > 0 {
		openStr := m.formatPriceWithColorLang(m.searchResult.StartPrice, m.searchResult.PrevClose)
		values = append(values, openStr)
	} else {
		values = append(values, "-")
	}

	// 最高价
	if m.searchResult.MaxPrice > 0 {
		highStr := m.formatPriceWithColorLang(m.searchResult.MaxPrice, m.searchResult.PrevClose)
		values = append(values, highStr)
	} else {
		values = append(values, "-")
	}

	// 最低价
	if m.searchResult.MinPrice > 0 {
		lowStr := m.formatPriceWithColorLang(m.searchResult.MinPrice, m.searchResult.PrevClose)
		values = append(values, lowStr)
	} else {
		values = append(values, "-")
	}

	// 今日涨幅
	if m.searchResult.ChangePercent != 0 {
		changePercentStr := m.formatProfitRateWithColorZeroLang(m.searchResult.ChangePercent)
		values = append(values, changePercentStr)
	} else {
		values = append(values, "-")
	}

	// 换手率
	if m.searchResult.TurnoverRate > 0 {
		values = append(values, fmt.Sprintf("%.2f%%", m.searchResult.TurnoverRate))
	} else {
		values = append(values, "-")
	}

	// 成交量
	if m.searchResult.Volume > 0 {
		if m.searchResult.Volume >= 100000000 { // 大于等于1亿
			values = append(values, fmt.Sprintf("%.2f亿", float64(m.searchResult.Volume)/100000000))
		} else if m.searchResult.Volume >= 10000 { // 大于等于1万
			values = append(values, fmt.Sprintf("%.2f万", float64(m.searchResult.Volume)/10000))
		} else {
			values = append(values, fmt.Sprintf("%d", m.searchResult.Volume))
		}
	} else {
		values = append(values, "-")
	}

	t.AppendRow(values)

	s += t.Render() + "\n\n"

	if m.language == Chinese {
		s += "按回车键添加到自选列表，ESC键返回，R键重新搜索\n"
	} else {
		s += "Press Enter to add to watchlist, ESC to return, R to search again\n"
	}

	return s
}



// 获取排序字段的显示名称
func (m *Model) getSortFieldName(field SortField) string {
	switch field {
	case SortByCode:
		return m.getText("sortCode")
	case SortByName:
		return m.getText("sortName")
	case SortByPrice:
		return m.getText("sortPrice")
	case SortByCostPrice:
		return m.getText("sortCostPrice")
	case SortByChange:
		return m.getText("sortChange")
	case SortByChangePercent:
		return m.getText("sortChangePercent")
	case SortByQuantity:
		return m.getText("sortQuantity")
	case SortByTotalProfit:
		return m.getText("sortTotalProfit")
	case SortByProfitRate:
		return m.getText("sortProfitRate")
	case SortByMarketValue:
		return m.getText("sortMarketValue")
	case SortByTag:
		return m.getText("sortTag")
	case SortByTurnoverRate:
		return m.getText("sortTurnoverRate")
	case SortByVolume:
		return m.getText("sortVolume")
	default:
		return "Unknown"
	}
}

// 获取排序方向的显示名称
func (m *Model) getSortDirectionName(direction SortDirection) string {
	if direction == SortAsc {
		return m.getText("sortAsc")
	}
	return m.getText("sortDesc")
}

// 获取持股列表可用的排序字段
func (m *Model) getPortfolioSortFields() []SortField {
	return []SortField{
		SortByCode, SortByName, SortByPrice, SortByCostPrice,
		SortByChange, SortByChangePercent, SortByQuantity,
		SortByTotalProfit, SortByProfitRate, SortByMarketValue,
	}
}

// 获取自选列表可用的排序字段
func (m *Model) getWatchlistSortFields() []SortField {
	return []SortField{
		SortByCode, SortByName, SortByPrice, SortByTag,
		SortByChangePercent, SortByTurnoverRate, SortByVolume,
	}
}

// 查找排序字段在字段列表中的索引，如果找不到返回0
func (m *Model) findSortFieldIndex(field SortField, isPortfolio bool) int {
	var fields []SortField
	if isPortfolio {
		fields = m.getPortfolioSortFields()
	} else {
		fields = m.getWatchlistSortFields()
	}

	for i, f := range fields {
		if f == field {
			return i
		}
	}

	// 如果没找到当前排序字段，返回0（第一个字段）
	return 0
}

// 生成带排序指示器的持股列表表头
func (m *Model) getPortfolioHeaderWithSortIndicator() table.Row {
	var baseHeaders table.Row
	if m.language == Chinese {
		baseHeaders = table.Row{"", "代码", "名称", "昨收价", "开盘", "最高", "最低", "现价", "成本价", "持股数", "今日涨幅", "持仓盈亏", "盈亏率", "市值"}
	} else {
		baseHeaders = table.Row{"", "Code", "Name", "PrevClose", "Open", "High", "Low", "Price", "Cost", "Quantity", "Today%", "PositionP&L", "P&LRate", "Value"}
	}

	// 排序字段到表头列索引的映射（跳过第一列的光标列）
	// 新顺序：代码，名称，昨收价，开盘，最高，最低，现价，成本价，持股数，今日涨幅，持仓盈亏，盈亏率，市值
	sortFieldToColumnIndex := map[SortField]int{
		SortByCode:          1,  // 代码
		SortByName:          2,  // 名称
		SortByPrice:         7,  // 现价
		SortByCostPrice:     8,  // 成本价
		SortByQuantity:      9,  // 持股数
		SortByChangePercent: 10, // 今日涨幅
		SortByTotalProfit:   11, // 持仓盈亏
		SortByProfitRate:    12, // 盈亏率
		SortByMarketValue:   13, // 市值
	}

	// 添加排序指示器（只有在已排序状态下才显示）
	if m.portfolioIsSorted {
		if columnIndex, exists := sortFieldToColumnIndex[m.portfolioSortField]; exists {
			sortIndicator := "↑"
			if m.portfolioSortDirection == SortDesc {
				sortIndicator = "↓"
			}
			baseHeaders[columnIndex] = fmt.Sprintf("%s %s", baseHeaders[columnIndex], sortIndicator)
		}
	}

	return baseHeaders
}

// 生成带排序指示器的自选列表表头
func (m *Model) getWatchlistHeaderWithSortIndicator() table.Row {
	var baseHeaders table.Row
	if m.language == Chinese {
		baseHeaders = table.Row{"", "标签", "代码", "名称", "现价", "昨收价", "开盘", "最高", "最低", "今日涨幅", "换手率", "成交量", "主力净流入", "超大单", "大单", "中单", "小单", "净流入占比"}
	} else {
		baseHeaders = table.Row{"", "Tag", "Code", "Name", "Price", "PrevClose", "Open", "High", "Low", "Today%", "Turnover", "Volume", "MainFlow", "SuperLarge", "Large", "Medium", "Small", "FlowRatio"}
	}

	// 排序字段到表头列索引的映射（跳过第一列的光标列）
	sortFieldToColumnIndex := map[SortField]int{
		SortByTag:           1,  // 标签
		SortByCode:          2,  // 代码
		SortByName:          3,  // 名称
		SortByPrice:         4,  // 现价
		SortByChangePercent: 9,  // 今日涨幅
		SortByTurnoverRate:  10, // 换手率
		SortByVolume:        11, // 成交量
	}

	// 添加排序指示器（只有在已排序状态下才显示）
	if m.watchlistIsSorted {
		if columnIndex, exists := sortFieldToColumnIndex[m.watchlistSortField]; exists {
			sortIndicator := "↑"
			if m.watchlistSortDirection == SortDesc {
				sortIndicator = "↓"
			}
			baseHeaders[columnIndex] = fmt.Sprintf("%s %s", baseHeaders[columnIndex], sortIndicator)
		}
	}

	return baseHeaders
}

// 处理持股列表排序
func (m *Model) handlePortfolioSorting(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	sortFields := m.getPortfolioSortFields()

	switch msg.String() {
	case "up", "k", "w":
		if m.portfolioSortCursor > 0 {
			m.portfolioSortCursor--
		}
	case "down", "j", "s":
		if m.portfolioSortCursor < len(sortFields)-1 {
			m.portfolioSortCursor++
		}
	case "enter", " ":
		// 切换排序方向或应用排序
		selectedField := sortFields[m.portfolioSortCursor]
		if m.portfolioSortField == selectedField {
			// 切换排序方向
			if m.portfolioSortDirection == SortAsc {
				m.portfolioSortDirection = SortDesc
			} else {
				m.portfolioSortDirection = SortAsc
			}
		} else {
			// 设置新的排序字段，默认升序
			m.portfolioSortField = selectedField
			m.portfolioSortDirection = SortAsc
		}
		// 执行排序并标记为已排序状态
		m.optimizedSortPortfolio(m.portfolioSortField, m.portfolioSortDirection)
		m.portfolioIsSorted = true
		m.resetPortfolioCursor()
		// 返回持股列表页面
		m.state = Monitoring
		m.message = ""
		return m, nil
	case "c", "C":
		// 清除当前排序 - 重新加载原始数据顺序
		m.portfolioIsSorted = false
		// 清除排序字段和方向状态
		m.portfolioSortField = SortByCode  // 重置为默认值
		m.portfolioSortDirection = SortAsc // 重置为默认值
		// 重新加载原始数据顺序
		m.portfolio = loadPortfolio()
		m.resetPortfolioCursor()
		// 返回持股列表页面
		m.state = Monitoring
		m.message = m.getText("sortCleared")
		return m, nil
	case "esc", "q":
		// 返回持股列表页面
		m.state = Monitoring
		m.message = ""
		return m, nil
	}
	return m, nil
}

// 处理自选列表排序
func (m *Model) handleWatchlistSorting(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	sortFields := m.getWatchlistSortFields()

	switch msg.String() {
	case "up", "k", "w":
		if m.watchlistSortCursor > 0 {
			m.watchlistSortCursor--
		}
	case "down", "j", "s":
		if m.watchlistSortCursor < len(sortFields)-1 {
			m.watchlistSortCursor++
		}
	case "enter", " ":
		// 切换排序方向或应用排序
		selectedField := sortFields[m.watchlistSortCursor]
		if m.watchlistSortField == selectedField {
			// 切换排序方向
			if m.watchlistSortDirection == SortAsc {
				m.watchlistSortDirection = SortDesc
			} else {
				m.watchlistSortDirection = SortAsc
			}
		} else {
			// 设置新的排序字段，默认升序
			m.watchlistSortField = selectedField
			m.watchlistSortDirection = SortAsc
		}
		// 执行排序并标记为已排序状态
		m.optimizedSortWatchlist(m.watchlistSortField, m.watchlistSortDirection)
		m.watchlistIsSorted = true
		m.resetWatchlistCursor()
		// 返回自选列表页面
		m.state = WatchlistViewing
		m.message = ""
		return m, m.tickCmd() // 重启定时器
	case "c", "C":
		// 清除当前排序 - 重新加载原始数据顺序
		m.watchlistIsSorted = false
		// 清除排序字段和方向状态
		m.watchlistSortField = SortByCode  // 重置为默认值
		m.watchlistSortDirection = SortAsc // 重置为默认值
		// 重新加载原始数据顺序
		m.watchlist = loadWatchlist()
		m.resetWatchlistCursor()
		// 返回自选列表页面
		m.state = WatchlistViewing
		m.message = m.getText("sortCleared")
		return m, m.tickCmd() // 重启定时器
	case "esc", "q":
		// 返回自选列表页面
		m.state = WatchlistViewing
		m.message = ""
		return m, m.tickCmd() // 重启定时器
	}
	return m, nil
}

// 排序菜单视图 - 持股列表
func (m *Model) viewPortfolioSorting() string {
	s := m.getText("sortTitle") + "\n\n"
	s += m.getText("selectSortField") + "\n\n"

	sortFields := m.getPortfolioSortFields()
	for i, field := range sortFields {
		prefix := "  "
		if i == m.portfolioSortCursor {
			prefix = "► "
		}

		fieldName := m.getSortFieldName(field)
		if m.portfolioIsSorted && m.portfolioSortField == field {
			// 显示当前排序状态（只有在已排序时才显示）
			directionName := m.getSortDirectionName(m.portfolioSortDirection)
			s += fmt.Sprintf("%s%s (%s)\n", prefix, fieldName, directionName)
		} else {
			s += fmt.Sprintf("%s%s\n", prefix, fieldName)
		}
	}

	s += "\n" + m.getText("sortHelp") + "\n"
	return s
}

// 排序菜单视图 - 自选列表
func (m *Model) viewWatchlistSorting() string {
	s := m.getText("sortTitle") + "\n\n"
	s += m.getText("selectSortField") + "\n\n"

	sortFields := m.getWatchlistSortFields()
	for i, field := range sortFields {
		prefix := "  "
		if i == m.watchlistSortCursor {
			prefix = "► "
		}

		fieldName := m.getSortFieldName(field)
		if m.watchlistIsSorted && m.watchlistSortField == field {
			// 显示当前排序状态（只有在已排序时才显示）
			directionName := m.getSortDirectionName(m.watchlistSortDirection)
			s += fmt.Sprintf("%s%s (%s)\n", prefix, fieldName, directionName)
		} else {
			s += fmt.Sprintf("%s%s\n", prefix, fieldName)
		}
	}

	s += "\n" + m.getText("sortHelp") + "\n"
	return s
}
