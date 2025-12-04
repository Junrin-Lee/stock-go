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
	Code       string              `json:"code"`       // e.g., "SH600000"
	Name       string              `json:"name"`       // e.g., "浦发银行"
	Date       string              `json:"date"`       // Format: "20251126"
	Datapoints []IntradayDataPoint `json:"datapoints"` // Minute-by-minute data
	UpdatedAt  string              `json:"updated_at"` // Format: "2025-11-26 15:00:00"
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
			debugPrint("[分时数据] Worker停止: %s\n", stockCode)
		}()

		debugPrint("[分时数据] Worker启动: %s (%s)\n", stockCode, stockName)

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
				if !isMarketOpen() {
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
	if checkMarketHours && !isMarketOpen() {
		return
	}

	// Fetch from API
	datapoints, err := fetchIntradayDataFromAPI(stockCode)
	if err != nil {
		debugPrint("[分时数据] 获取失败 %s: %v\n", stockCode, err)
		return
	}

	if len(datapoints) == 0 {
		debugPrint("[分时数据] 无数据返回 %s\n", stockCode)
		return
	}

	// Prepare file path
	today := getCurrentDate()
	filePath := filepath.Join("data", "intraday", stockCode, today+".json")

	// Ensure directory exists
	if err := ensureIntradayDirectory(stockCode); err != nil {
		debugPrint("[分时数据] 创建目录失败 %s: %v\n", stockCode, err)
		return
	}

	// Read existing data (if any)
	existingData := &IntradayData{
		Code:       stockCode,
		Name:       stockName,
		Date:       today,
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

	// Write back to file
	if err := saveIntradayData(filePath, existingData); err != nil {
		debugPrint("[分时数据] 保存失败 %s: %v\n", stockCode, err)
		return
	}

	debugPrint("[分时数据] 更新成功 %s: %d 个数据点\n", stockCode, len(existingData.Datapoints))
}

// fetchIntradayDataFromAPI tries all APIs in fallback order
func fetchIntradayDataFromAPI(stockCode string) ([]IntradayDataPoint, error) {
	var lastErr error

	// Try Tencent API (primary - most reliable for intraday data)
	data, err := tryGetIntradayFromTencent(stockCode)
	if err == nil && len(data) > 0 {
		debugPrint("[分时数据] Tencent API 成功: %s (%d points)\n", stockCode, len(data))
		return data, nil
	}
	if err != nil {
		lastErr = err
		debugPrint("[分时数据] Tencent API 失败: %s - %v\n", stockCode, err)
	} else {
		debugPrint("[分时数据] Tencent API 无数据: %s\n", stockCode)
	}

	// Try EastMoney API (secondary)
	data, err = tryGetIntradayFromEastMoney(stockCode)
	if err == nil && len(data) > 0 {
		debugPrint("[分时数据] EastMoney API 成功: %s (%d points)\n", stockCode, len(data))
		return data, nil
	}
	if err != nil {
		lastErr = err
		debugPrint("[分时数据] EastMoney API 失败: %s - %v\n", stockCode, err)
	} else {
		debugPrint("[分时数据] EastMoney API 无数据: %s\n", stockCode)
	}

	// Try Sina Finance API (last fallback - K-line data, may not have today's data)
	data, err = tryGetIntradayFromSina(stockCode)
	if err == nil && len(data) > 0 {
		debugPrint("[分时数据] Sina API 成功: %s (%d points)\n", stockCode, len(data))
		return data, nil
	}
	if err != nil {
		lastErr = err
		debugPrint("[分时数据] Sina API 失败: %s - %v\n", stockCode, err)
	} else {
		debugPrint("[分时数据] Sina API 无数据: %s\n", stockCode)
	}

	return nil, fmt.Errorf("所有API失败, 最后错误: %w", lastErr)
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

// isMarketOpen checks if current time is within trading hours
func isMarketOpen() bool {
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

// getCurrentDate returns today's date in YYYYMMDD format
func getCurrentDate() string {
	return time.Now().Format("20060102")
}

// fileExists checks if a file path exists
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// convertStockCodeForSina converts "SH600000" to "sh600000" for Sina API
func convertStockCodeForSina(code string) string {
	return strings.ToLower(code)
}

// convertStockCodeForEastMoney converts "SH600000" to "1.600000" for EastMoney API
func convertStockCodeForEastMoney(code string) string {
	code = strings.ToUpper(code)

	if strings.HasPrefix(code, "SH") {
		// Shanghai market: "1." prefix
		return "1." + code[2:]
	} else if strings.HasPrefix(code, "SZ") {
		// Shenzhen market: "0." prefix
		return "0." + code[2:]
	}

	// Default: assume Shanghai if no prefix
	return "1." + code
}

// convertStockCodeForTencent converts "SH600000" to "sh600000" for Tencent API
func convertStockCodeForTencent(code string) string {
	return strings.ToLower(code)
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
