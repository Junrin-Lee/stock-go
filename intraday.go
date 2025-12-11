package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// IntradayDataPoint represents a single minute's price data
type IntradayDataPoint struct {
	Time  string  `json:"time"`  // Format: "09:31" (HH:MM)
	Price float64 `json:"price"` // Closing price for that minute
}

// IntradayData represents the complete intraday data for a stock on a given day
type IntradayData struct {
	Code       string              `json:"code"`                 // e.g., "SH600000"
	Name       string              `json:"name"`                 // e.g., "浦发银行"
	Date       string              `json:"date"`                 // Format: "20251126"
	Market     MarketType          `json:"market,omitempty"`     // 市场类型 (向后兼容)
	Datapoints []IntradayDataPoint `json:"datapoints"`           // Minute-by-minute data
	UpdatedAt  string              `json:"updated_at"`           // Format: "2025-11-26 15:00:00"
	PrevClose  float64             `json:"prev_close,omitempty"` // 昨日收盘价（向后兼容）
}

// IntradayManager manages background fetching of intraday data
type IntradayManager struct {
	activeStocks  map[string]bool      // Currently tracking stocks
	workerPool    chan struct{}        // Semaphore for max 10 concurrent workers
	cancelChan    chan struct{}        // Channel to stop all workers
	mu            sync.RWMutex         // Protects activeStocks
	lastFetchTime map[string]time.Time // Track last fetch per stock
	fetchInterval time.Duration        // 1 minute
}

// File locks for thread-safe file operations
var intradayFileLocks sync.Map // map[string]*sync.Mutex

// newIntradayManager creates and initializes an IntradayManager
func newIntradayManager() *IntradayManager {
	return &IntradayManager{
		activeStocks:  make(map[string]bool),
		workerPool:    make(chan struct{}, 10), // Max 10 concurrent workers
		cancelChan:    make(chan struct{}),
		lastFetchTime: make(map[string]time.Time),
		fetchInterval: 1 * time.Minute,
	}
}

// startWorker launches a background goroutine to fetch data for one stock
func (im *IntradayManager) startWorker(stockCode, stockName string, m *Model) {
	// Prevent duplicate workers
	im.mu.Lock()
	if im.activeStocks[stockCode] {
		im.mu.Unlock()
		return
	}
	im.activeStocks[stockCode] = true
	im.mu.Unlock()

	go func() {
		// Cleanup function
		defer func() {
			im.mu.Lock()
			delete(im.activeStocks, stockCode)
			im.mu.Unlock()
			debugPrint("debug.intraday.workerStop", stockCode)
		}()

		debugPrint("debug.intraday.workerStart", stockCode, stockName)

		// Create ticker
		ticker := time.NewTicker(im.fetchInterval)
		defer ticker.Stop()

		// Initial fetch (skip market hours check to get today's data even after market close)
		im.fetchAndSaveIntradayData(stockCode, stockName, m, false)

		// Periodic loop
		for {
			select {
			case <-ticker.C:
				// Check market hours for periodic updates
				if !isMarketOpen(stockCode, m) {
					continue
				}

				// Acquire worker slot (blocks if all 10 slots are busy)
				im.workerPool <- struct{}{}

				// Fetch with timeout
				go func() {
					defer func() {
						<-im.workerPool // Release slot
					}()
					im.fetchAndSaveIntradayData(stockCode, stockName, m, true)
				}()

			case <-im.cancelChan:
				return // Graceful exit
			}
		}
	}()
}

// fetchAndSaveIntradayData performs one fetch-merge-save cycle for a stock
func (im *IntradayManager) fetchAndSaveIntradayData(stockCode, stockName string, m *Model, checkMarketHours bool) {
	// Check if market is open (only if requested)
	if checkMarketHours && !isMarketOpen(stockCode, m) {
		return
	}

	// Fetch from API
	datapoints, err := fetchIntradayDataFromAPI(stockCode)
	if err != nil {
		debugPrint("debug.intraday.fetchFail", stockCode, err)
		return
	}

	if len(datapoints) == 0 {
		debugPrint("debug.intraday.noData", stockCode)
		return
	}

	// Prepare file path (使用市场时区的日期)
	market := getMarketType(stockCode)
	today := getCurrentDateForMarket(market, m)
	marketDir := getMarketDirectory(stockCode)
	filePath := filepath.Join("data", "intraday", marketDir, stockCode, today+".json")

	// Ensure directory exists (using new market-based structure)
	if err := ensureIntradayDirectoryWithMarket(stockCode); err != nil {
		debugPrint("debug.intraday.mkdirFail", stockCode, err)
		return
	}

	// 获取市场类型（已在上面获取）
	// market := getMarketType(stockCode)

	// Read existing data (if any)
	existingData := &IntradayData{
		Code:       stockCode,
		Name:       stockName,
		Date:       today,
		Market:     market, // 保存市场类型
		Datapoints: []IntradayDataPoint{},
	}

	if fileExists(filePath) {
		data, err := os.ReadFile(filePath)
		if err == nil {
			json.Unmarshal(data, existingData)
		}
	}

	// Merge datapoints (deduplicate by time)
	existingData.Datapoints = mergeDatapoints(existingData.Datapoints, datapoints)
	existingData.UpdatedAt = time.Now().Format("2006-01-02 15:04:05")

	// NEW: 如果 existingData.PrevClose 为空，从缓存获取
	if existingData.PrevClose == 0 {
		m.stockPriceMutex.RLock()
		if entry, exists := m.stockPriceCache[stockCode]; exists && entry.Data != nil {
			existingData.PrevClose = entry.Data.PrevClose
		}
		m.stockPriceMutex.RUnlock()

		if existingData.PrevClose > 0 {
			debugPrint("debug.intraday.prevCloseSet", stockCode, existingData.PrevClose)
		}
	}

	// Write back to file
	if err := saveIntradayData(filePath, existingData); err != nil {
		debugPrint("debug.intraday.saveFail", stockCode, err)
		return
	}

	debugPrint("debug.intraday.saveSuccess", stockCode, len(existingData.Datapoints))
}

// fetchIntradayDataFromAPI tries all APIs in fallback order based on market type
func fetchIntradayDataFromAPI(stockCode string) ([]IntradayDataPoint, error) {
	var lastErr error
	market := getMarketType(stockCode)

	// US stocks: Use Yahoo Finance API (best for US stocks)
	if market == MarketUS {
		debugPrint("debug.intraday.marketTypeUS", stockCode)

		data, err := tryGetIntradayFromYahoo(stockCode)
		if err == nil && len(data) > 0 {
			debugPrint("debug.intraday.yahooSuccess", stockCode, len(data))
			return data, nil
		}
		if err != nil {
			lastErr = err
			debugPrint("debug.intraday.yahooFail", stockCode, err)
		} else {
			debugPrint("debug.intraday.yahooNoData", stockCode)
		}

		return nil, fmt.Errorf("Yahoo Finance API失败: %w", lastErr)
	}

	// Hong Kong stocks: Try Tencent first, then Yahoo Finance as fallback
	if market == MarketHongKong {
		debugPrint("debug.intraday.marketTypeHK", stockCode)

		// Try Tencent API (primary for HK stocks)
		data, err := tryGetIntradayFromTencent(stockCode)
		if err == nil && len(data) > 0 {
			debugPrint("debug.intraday.tencentSuccess", stockCode, len(data))
			return data, nil
		}
		if err != nil {
			lastErr = err
			debugPrint("debug.intraday.tencentFail", stockCode, err)
		} else {
			debugPrint("debug.intraday.tencentNoData", stockCode)
		}

		// Try Yahoo Finance API (fallback for HK stocks)
		data, err = tryGetIntradayFromYahoo(stockCode)
		if err == nil && len(data) > 0 {
			debugPrint("debug.intraday.yahooSuccess", stockCode, len(data))
			return data, nil
		}
		if err != nil {
			lastErr = err
			debugPrint("debug.intraday.yahooFail", stockCode, err)
		} else {
			debugPrint("debug.intraday.yahooNoData", stockCode)
		}

		// Try EastMoney API (secondary fallback)
		data, err = tryGetIntradayFromEastMoney(stockCode)
		if err == nil && len(data) > 0 {
			debugPrint("debug.intraday.eastMoneySuccess", stockCode, len(data))
			return data, nil
		}
		if err != nil {
			lastErr = err
			debugPrint("debug.intraday.eastMoneyFail", stockCode, err)
		} else {
			debugPrint("debug.intraday.eastMoneyNoData", stockCode)
		}

		return nil, fmt.Errorf("所有港股API失败, 最后错误: %w", lastErr)
	}

	// China A-shares: Use Chinese APIs (Tencent, EastMoney, Sina)
	debugPrint("debug.intraday.marketTypeChina", stockCode)

	// Try Tencent API (primary - most reliable for A-shares)
	data, err := tryGetIntradayFromTencent(stockCode)
	if err == nil && len(data) > 0 {
		debugPrint("debug.intraday.tencentSuccess", stockCode, len(data))
		return data, nil
	}
	if err != nil {
		lastErr = err
		debugPrint("debug.intraday.tencentFail", stockCode, err)
	} else {
		debugPrint("debug.intraday.tencentNoData", stockCode)
	}

	// Try EastMoney API (secondary)
	data, err = tryGetIntradayFromEastMoney(stockCode)
	if err == nil && len(data) > 0 {
		debugPrint("debug.intraday.eastMoneySuccess", stockCode, len(data))
		return data, nil
	}
	if err != nil {
		lastErr = err
		debugPrint("debug.intraday.eastMoneyFail", stockCode, err)
	} else {
		debugPrint("debug.intraday.eastMoneyNoData", stockCode)
	}

	// Try Sina Finance API (last fallback - K-line data, may not have today's data)
	data, err = tryGetIntradayFromSina(stockCode)
	if err == nil && len(data) > 0 {
		debugPrint("debug.intraday.sinaSuccess", stockCode, len(data))
		return data, nil
	}
	if err != nil {
		lastErr = err
		debugPrint("debug.intraday.sinaFail", stockCode, err)
	} else {
		debugPrint("debug.intraday.sinaNoData", stockCode)
	}

	return nil, fmt.Errorf("所有A股API失败, 最后错误: %w", lastErr)
}

// tryGetIntradayFromSina fetches intraday data from Sina Finance API
func tryGetIntradayFromSina(stockCode string) ([]IntradayDataPoint, error) {
	// Convert stock code for Sina API
	sinaCode := convertStockCodeForSina(stockCode)

	// Build URL
	url := fmt.Sprintf(
		"http://money.finance.sina.com.cn/quotes_service/api/json_v2.php/CN_MarketData.getKLineData?symbol=%s&scale=1&datalen=250",
		sinaCode,
	)

	// Create HTTP client with timeout
	client := &http.Client{Timeout: 8 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	// Set headers
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://finance.sina.com.cn")

	// Send request with retry
	resp, err := fetchWithRetry(client, req, 2)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Parse JSON response
	var sinaData []struct {
		Day    string `json:"day"`    // "2025-11-26 09:31:00"
		Open   string `json:"open"`   // "8.52"
		High   string `json:"high"`   // "8.53"
		Low    string `json:"low"`    // "8.51"
		Close  string `json:"close"`  // "8.52"
		Volume string `json:"volume"` // "12000"
	}

	if err := json.NewDecoder(resp.Body).Decode(&sinaData); err != nil {
		return nil, err
	}

	// Convert to IntradayDataPoint
	result := make([]IntradayDataPoint, 0, len(sinaData))
	for _, item := range sinaData {
		price, err := strconv.ParseFloat(item.Close, 64)
		if err != nil {
			continue
		}

		timeStr := formatIntradayTime(item.Day)
		if timeStr == "" {
			continue
		}

		result = append(result, IntradayDataPoint{
			Time:  timeStr,
			Price: price,
		})
	}

	return result, nil
}

// tryGetIntradayFromEastMoney fetches intraday data from EastMoney API
func tryGetIntradayFromEastMoney(stockCode string) ([]IntradayDataPoint, error) {
	// Convert stock code for EastMoney API
	emCode := convertStockCodeForEastMoney(stockCode)

	// Build URL
	url := fmt.Sprintf(
		"https://push2.eastmoney.com/api/qt/stock/trends2/get?secid=%s&fields1=f1,f2,f3&fields2=f51,f52,f53,f54,f55&iscr=0",
		emCode,
	)

	// Create HTTP client with timeout
	client := &http.Client{Timeout: 8 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	// Set headers to avoid being blocked
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://www.eastmoney.com")

	resp, err := fetchWithRetry(client, req, 2) // Retry up to 2 times
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP status %d", resp.StatusCode)
	}

	// Parse response
	var emData struct {
		Data struct {
			Trends []string `json:"trends"` // ["2025-11-26 09:31,8.52,12000,...]
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&emData); err != nil {
		return nil, err
	}

	if emData.Data.Trends == nil {
		return nil, fmt.Errorf("no trends data")
	}

	// Parse each trend data
	result := make([]IntradayDataPoint, 0, len(emData.Data.Trends))
	for _, trend := range emData.Data.Trends {
		parts := strings.Split(trend, ",")
		if len(parts) < 2 {
			continue
		}

		timeStr := formatIntradayTime(parts[0])
		if timeStr == "" {
			continue
		}

		price, err := strconv.ParseFloat(parts[1], 64)
		if err != nil {
			continue
		}

		result = append(result, IntradayDataPoint{
			Time:  timeStr,
			Price: price,
		})
	}

	return result, nil
}

// mergeDatapoints combines existing and new datapoints, deduplicating by time
func mergeDatapoints(existing, new []IntradayDataPoint) []IntradayDataPoint {
	// Create map of datapoints by time
	dataMap := make(map[string]IntradayDataPoint)

	// Add existing datapoints
	for _, dp := range existing {
		dataMap[dp.Time] = dp
	}

	// Overlay new datapoints (overwrites duplicates)
	for _, dp := range new {
		dataMap[dp.Time] = dp
	}

	// Convert back to sorted slice
	result := make([]IntradayDataPoint, 0, len(dataMap))
	for _, dp := range dataMap {
		result = append(result, dp)
	}

	// Sort by time
	sort.Slice(result, func(i, j int) bool {
		return result[i].Time < result[j].Time
	})

	return result
}

// ensureIntradayDirectory creates the directory structure for a stock if needed
func ensureIntradayDirectory(stockCode string) error {
	dirPath := filepath.Join("data", "intraday", stockCode)
	return os.MkdirAll(dirPath, 0755)
}

// getMarketDirectory returns market subdirectory (CN/HK/US) based on stock code
func getMarketDirectory(code string) string {
	market := getMarketType(code)
	switch market {
	case MarketChina:
		return "CN"
	case MarketHongKong:
		return "HK"
	case MarketUS:
		return "US"
	default:
		return "US"
	}
}

// getIntradayFilePath returns file path with backward compatibility fallback
// Priority: new market-based structure (data/intraday/CN/SH600058/20251211.json)
//
//	→ old flat structure (data/intraday/SH600058/20251211.json)
func getIntradayFilePath(stockCode, date string) string {
	// Try new market-based structure first
	marketDir := getMarketDirectory(stockCode)
	newPath := filepath.Join("data", "intraday", marketDir, stockCode, date+".json")
	if fileExists(newPath) {
		return newPath
	}

	// Fallback to old flat structure for backward compatibility
	return filepath.Join("data", "intraday", stockCode, date+".json")
}

// ensureIntradayDirectoryWithMarket creates market-based directory structure
// New implementation that organizes stocks by market (CN/HK/US)
func ensureIntradayDirectoryWithMarket(stockCode string) error {
	marketDir := getMarketDirectory(stockCode)
	dirPath := filepath.Join("data", "intraday", marketDir, stockCode)
	return os.MkdirAll(dirPath, 0755)
}

// saveIntradayData writes IntradayData to JSON file with thread-safe locking
func saveIntradayData(filePath string, data *IntradayData) error {
	lock := getFileLock(filePath)
	lock.Lock()
	defer lock.Unlock()

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	// Atomic write: write to temp file, then rename
	tempPath := filePath + ".tmp"
	if err := os.WriteFile(tempPath, jsonData, 0644); err != nil {
		return err
	}

	return os.Rename(tempPath, filePath)
}

// getFileLock returns a mutex for the given file path
func getFileLock(filePath string) *sync.Mutex {
	lock, _ := intradayFileLocks.LoadOrStore(filePath, &sync.Mutex{})
	return lock.(*sync.Mutex)
}

// isMarketOpen 检查当前是否在交易时间内（支持多市场）
func isMarketOpen(stockCode string, m *Model) bool {
	market := getMarketType(stockCode)

	var marketConfig MarketConfig
	switch market {
	case MarketChina:
		marketConfig = m.config.Markets.China
	case MarketUS:
		marketConfig = m.config.Markets.US
	case MarketHongKong:
		marketConfig = m.config.Markets.HongKong
	default:
		debugPrint("debug.market.unknownType", stockCode, market)
		return false
	}

	// 检查配置是否有效（向后兼容降级）
	if len(marketConfig.TradingSessions) == 0 {
		if market == MarketChina {
			return isMarketOpenHardcoded() // 保留老版本硬编码逻辑
		}
		return false
	}

	now := time.Now()
	return isMarketOpenForConfig(now, marketConfig)
}

// isMarketOpenHardcoded 硬编码的A股交易时间判断（降级方案）
func isMarketOpenHardcoded() bool {
	now := time.Now()

	// Check if weekday
	weekday := now.Weekday()
	if weekday == time.Saturday || weekday == time.Sunday {
		return false
	}

	// Market hours: 09:30 - 11:30, 13:00 - 15:00 (China timezone)
	hour := now.Hour()
	minute := now.Minute()
	currentTime := hour*100 + minute

	// Morning session: 09:30 - 11:30
	if currentTime >= 930 && currentTime <= 1130 {
		return true
	}

	// Afternoon session: 13:00 - 15:00
	if currentTime >= 1300 && currentTime <= 1500 {
		return true
	}

	return false
}

// formatIntradayTime converts "2025-11-26 09:31:00" to "09:31"
func formatIntradayTime(fullTime string) string {
	// Try to parse various formats
	parts := strings.Fields(fullTime)
	if len(parts) < 2 {
		return ""
	}

	// Extract time part (e.g., "09:31:00")
	timePart := parts[1]
	timeComponents := strings.Split(timePart, ":")
	if len(timeComponents) < 2 {
		return ""
	}

	// Return "HH:MM"
	return timeComponents[0] + ":" + timeComponents[1]
}

// fileExists checks if a file path exists
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// convertStockCodeForSina converts "SH600000" to "sh600000" for Sina API
// Also handles HK stocks: "HK2020" -> "hk02020" (pads to 5 digits)
func convertStockCodeForSina(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))

	if strings.HasPrefix(code, "SH") {
		return "sh" + strings.TrimPrefix(code, "SH")
	} else if strings.HasPrefix(code, "SZ") {
		return "sz" + strings.TrimPrefix(code, "SZ")
	} else if strings.HasPrefix(code, "HK") {
		// 港股格式: HK00700 -> hk00700, HK2020 -> hk02020
		// 港股代码需要补齐5位数字
		stockNum := strings.TrimPrefix(code, "HK")
		return "hk" + padHKStockCodeIntraday(stockNum)
	} else if strings.HasSuffix(code, ".HK") {
		// 港股格式: 0700.HK -> hk00700, 2020.HK -> hk02020
		stockNum := strings.TrimSuffix(code, ".HK")
		return "hk" + padHKStockCodeIntraday(stockNum)
	}

	// 检查是否为纯数字的6位A股代码
	if len(code) == 6 {
		// 根据首位数字判断市场
		if strings.HasPrefix(code, "6") {
			return "sh" + code // 上海
		} else if strings.HasPrefix(code, "0") || strings.HasPrefix(code, "3") {
			return "sz" + code // 深圳
		}
	}

	// 其他格式，直接转小写
	return strings.ToLower(code)
}

// convertStockCodeForEastMoney converts "SH600000" to "1.600000" for EastMoney API
// Also handles HK stocks: "HK00700" -> "116.00700" (Hong Kong market code is 116)
func convertStockCodeForEastMoney(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))

	if strings.HasPrefix(code, "SH") {
		// Shanghai market: "1." prefix
		return "1." + code[2:]
	} else if strings.HasPrefix(code, "SZ") {
		// Shenzhen market: "0." prefix
		return "0." + code[2:]
	} else if strings.HasPrefix(code, "HK") {
		// Hong Kong market: "116." prefix
		// 港股格式: HK00700 -> 116.00700, HK2020 -> 116.02020
		stockNum := strings.TrimPrefix(code, "HK")
		return "116." + padHKStockCodeIntraday(stockNum)
	} else if strings.HasSuffix(code, ".HK") {
		// Hong Kong market: "116." prefix
		// 港股格式: 0700.HK -> 116.00700, 2020.HK -> 116.02020
		stockNum := strings.TrimSuffix(code, ".HK")
		return "116." + padHKStockCodeIntraday(stockNum)
	}

	// 检查是否为纯数字的6位A股代码
	if len(code) == 6 {
		// 根据首位数字判断市场
		if strings.HasPrefix(code, "6") {
			return "1." + code // 上海
		} else if strings.HasPrefix(code, "0") || strings.HasPrefix(code, "3") {
			return "0." + code // 深圳
		}
	}

	// Default: 其他情况不处理（可能是美股等不支持的市场）
	return code
}

// convertStockCodeForTencent converts "SH600000" to "sh600000" for Tencent API
// Also handles HK stocks: "HK2020" -> "hk02020" (pads to 5 digits)
func convertStockCodeForTencent(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))

	if strings.HasPrefix(code, "SH") {
		return "sh" + strings.TrimPrefix(code, "SH")
	} else if strings.HasPrefix(code, "SZ") {
		return "sz" + strings.TrimPrefix(code, "SZ")
	} else if strings.HasPrefix(code, "HK") {
		// 港股格式: HK00700 -> hk00700, HK2020 -> hk02020
		// 港股代码需要补齐5位数字
		stockNum := strings.TrimPrefix(code, "HK")
		return "hk" + padHKStockCodeIntraday(stockNum)
	} else if strings.HasSuffix(code, ".HK") {
		// 港股格式: 0700.HK -> hk00700, 2020.HK -> hk02020
		stockNum := strings.TrimSuffix(code, ".HK")
		return "hk" + padHKStockCodeIntraday(stockNum)
	}

	// 美股或其他格式，直接转小写
	return strings.ToLower(code)
}

// padHKStockCodeIntraday 将港股代码补齐为5位数字
// 例如: "700" -> "00700", "2020" -> "02020", "00700" -> "00700"
func padHKStockCodeIntraday(code string) string {
	code = strings.TrimSpace(code)
	if len(code) >= 5 {
		return code
	}
	// 补齐到5位
	return fmt.Sprintf("%05s", code)
}

// fetchWithRetry performs HTTP request with retry mechanism
func fetchWithRetry(client *http.Client, req *http.Request, maxRetries int) (*http.Response, error) {
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		// Clone the request for retry (request body can only be read once)
		reqClone := req.Clone(req.Context())
		resp, err := client.Do(reqClone)
		if err == nil && resp.StatusCode == http.StatusOK {
			return resp, nil
		}
		if err != nil {
			lastErr = err
		} else {
			lastErr = fmt.Errorf("HTTP status %d", resp.StatusCode)
			resp.Body.Close()
		}
		// Wait before retry (exponential backoff: 500ms, 1000ms, ...)
		if i < maxRetries-1 {
			time.Sleep(time.Duration(i+1) * 500 * time.Millisecond)
		}
	}
	return nil, lastErr
}

// tryGetIntradayFromTencent fetches intraday data from Tencent API (primary source)
func tryGetIntradayFromTencent(stockCode string) ([]IntradayDataPoint, error) {
	// Convert stock code for Tencent API
	tencentCode := convertStockCodeForTencent(stockCode)

	// Build URL - Tencent minute data API (JSONP format)
	// Response format: min_data_sh601138={"code":0,"data":{"sh601138":{"data":{"data":["0930 60.88 10989 66901032.00",...]}}}}
	url := fmt.Sprintf(
		"http://ifzq.gtimg.cn/appstock/app/minute/query?_var=min_data_%s&code=%s",
		tencentCode, tencentCode,
	)

	// Create HTTP client with timeout
	client := &http.Client{Timeout: 8 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	// Set headers
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://gu.qq.com")

	resp, err := fetchWithRetry(client, req, 2)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Parse JSONP response - strip "min_data_XXX=" prefix
	bodyStr := string(body)
	eqIdx := strings.Index(bodyStr, "=")
	if eqIdx == -1 {
		return nil, fmt.Errorf("invalid JSONP response format")
	}
	jsonStr := bodyStr[eqIdx+1:]

	// Parse JSON response
	// Format: {"code":0,"data":{"sh601138":{"data":{"data":["0930 60.88 10989 66901032.00",...]}}}}
	var tencentResp struct {
		Code int `json:"code"`
		Data map[string]struct {
			Data struct {
				Data []string `json:"data"` // Array of "HHMM price volume amount"
			} `json:"data"`
		} `json:"data"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &tencentResp); err != nil {
		return nil, err
	}

	if tencentResp.Code != 0 {
		return nil, fmt.Errorf("API error code: %d", tencentResp.Code)
	}

	// Parse data points from all stocks in response
	result := make([]IntradayDataPoint, 0)

	for _, stockData := range tencentResp.Data {
		for _, dataStr := range stockData.Data.Data {
			// Format: "0930 60.88 10989 66901032.00" (time price volume amount)
			parts := strings.Split(dataStr, " ")
			if len(parts) < 2 {
				continue
			}

			// Parse time (format: "0930" -> "09:30")
			timeStr := parts[0]
			if len(timeStr) == 4 {
				timeStr = timeStr[:2] + ":" + timeStr[2:]
			}

			// Parse price
			price, err := strconv.ParseFloat(parts[1], 64)
			if err != nil {
				continue
			}

			result = append(result, IntradayDataPoint{
				Time:  timeStr,
				Price: price,
			})
		}
	}

	return result, nil
}

// tryGetIntradayFromYahoo fetches intraday data from Yahoo Finance API (for US and HK stocks)
// Yahoo Finance provides free, unlimited intraday data for global stocks
func tryGetIntradayFromYahoo(stockCode string) ([]IntradayDataPoint, error) {
	// Convert stock code for Yahoo Finance API
	yahooSymbol := convertStockCodeForYahoo(stockCode)

	// Build URL - Yahoo Finance chart API
	// interval=1m (1 minute), range=1d (1 day)
	url := fmt.Sprintf(
		"https://query1.finance.yahoo.com/v8/finance/chart/%s?interval=1m&range=1d",
		yahooSymbol,
	)

	// Create HTTP client with timeout
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	// Set headers to mimic browser
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "application/json")

	resp, err := fetchWithRetry(client, req, 2)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Parse JSON response
	var yahooResp struct {
		Chart struct {
			Result []struct {
				Meta struct {
					Symbol string `json:"symbol"`
				} `json:"meta"`
				Timestamp  []int64 `json:"timestamp"` // Unix timestamps
				Indicators struct {
					Quote []struct {
						Close []float64 `json:"close"` // Closing prices
					} `json:"quote"`
				} `json:"indicators"`
			} `json:"result"`
			Error *struct {
				Code        string `json:"code"`
				Description string `json:"description"`
			} `json:"error"`
		} `json:"chart"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&yahooResp); err != nil {
		return nil, err
	}

	// Check for API errors
	if yahooResp.Chart.Error != nil {
		return nil, fmt.Errorf("Yahoo API error: %s - %s",
			yahooResp.Chart.Error.Code,
			yahooResp.Chart.Error.Description)
	}

	// Check if we have data
	if len(yahooResp.Chart.Result) == 0 {
		return nil, fmt.Errorf("no data in Yahoo response")
	}

	result := yahooResp.Chart.Result[0]
	timestamps := result.Timestamp
	quotes := result.Indicators.Quote

	if len(quotes) == 0 || len(quotes[0].Close) == 0 {
		return nil, fmt.Errorf("no price data in Yahoo response")
	}

	closePrices := quotes[0].Close

	// Convert timestamps and prices to IntradayDataPoint
	datapoints := make([]IntradayDataPoint, 0, len(timestamps))

	for i, timestamp := range timestamps {
		// Skip if we don't have a price for this timestamp
		if i >= len(closePrices) {
			break
		}

		price := closePrices[i]
		// Skip null/zero prices
		if price == 0 {
			continue
		}

		// Convert Unix timestamp to time
		t := time.Unix(timestamp, 0)

		// Format time as "HH:MM" in local market timezone
		// Yahoo returns timestamps in UTC, need to convert to market time
		market := getMarketType(stockCode)
		var location *time.Location

		switch market {
		case MarketUS:
			location, _ = time.LoadLocation("America/New_York")
		case MarketHongKong:
			location, _ = time.LoadLocation("Asia/Hong_Kong")
		default:
			location = time.Local
		}

		if location != nil {
			t = t.In(location)
		}

		timeStr := t.Format("15:04") // HH:MM format

		datapoints = append(datapoints, IntradayDataPoint{
			Time:  timeStr,
			Price: price,
		})
	}

	return datapoints, nil
}

// convertStockCodeForYahoo converts stock code to Yahoo Finance format
// Examples:
//   - AAPL -> AAPL (US stocks keep as-is)
//   - HK00700 -> 0700.HK (Hong Kong stocks)
//   - HK2020 -> 2020.HK (Hong Kong stocks, no need to pad for Yahoo)
func convertStockCodeForYahoo(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))

	// Hong Kong stocks: HK00700 -> 0700.HK, HK2020 -> 2020.HK
	if strings.HasPrefix(code, "HK") {
		stockNum := strings.TrimPrefix(code, "HK")
		// Remove leading zeros for Yahoo format
		stockNum = strings.TrimLeft(stockNum, "0")
		if stockNum == "" {
			stockNum = "0"
		}
		return stockNum + ".HK"
	}

	// Already in .HK format
	if strings.HasSuffix(code, ".HK") {
		return code
	}

	// US stocks and others: return as-is
	return code
}
