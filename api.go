package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ============================================================================
// 股票信息获取入口
// ============================================================================

// getStockInfo 获取股票信息（支持中英文搜索）
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

	return stockData
}

// containsChineseChars 检查字符串是否包含中文字符
func containsChineseChars(s string) bool {
	for _, r := range s {
		if r >= 0x4e00 && r <= 0x9fff {
			return true
		}
	}
	return false
}

// isChinaStock 判断是否为中国A股
func isChinaStock(symbol string) bool {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	return strings.HasPrefix(symbol, "SH") || strings.HasPrefix(symbol, "SZ") ||
		(len(symbol) == 6 && (strings.HasPrefix(symbol, "0") || strings.HasPrefix(symbol, "3") || strings.HasPrefix(symbol, "6")))
}

// ============================================================================
// 股票搜索函数
// ============================================================================

// searchStockBySymbol 通过符号搜索股票（支持美股等国际股票）
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

// searchChineseStock 通过API搜索中文股票名称
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

// ============================================================================
// TwelveData API
// ============================================================================

// searchStockByTwelveDataAPI 使用TwelveData搜索API查找股票
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

// tryTwelveDataAPI 使用TwelveData API获取股票报价
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

// ============================================================================
// 腾讯 API
// ============================================================================

// searchStockByTencentAPI 使用腾讯搜索API查找股票
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

// tryTencentAPI 使用腾讯API获取股票价格
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

// convertStockSymbolForTencent 转换股票代码为腾讯API格式
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

// ============================================================================
// 腾讯搜索结果解析
// ============================================================================

// parseSearchResults 解析腾讯搜索结果
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

// parseTencentHintFormat 解析腾讯Hint格式的搜索结果
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

// parseJSONSearchResults 解析JSON格式的搜索结果
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

// parseLegacySearchResults 解析旧格式的搜索结果
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

// convertJSONCodeToStandard 转换JSON格式的股票代码为标准格式
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

// convertToStandardCode 将腾讯的股票代码转换为标准格式
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

// ============================================================================
// 新浪 API
// ============================================================================

// searchStockBySinaAPI 使用新浪财经搜索API查找股票
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

// parseSinaSearchResults 解析新浪搜索结果
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

// convertSinaCodeToStandard 转换新浪的股票代码为标准格式
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

// ============================================================================
// 高级搜索
// ============================================================================

// tryAdvancedSearch 高级搜索策略：尝试多种关键词变形
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

// generateSearchKeywords 生成搜索关键词变形
func generateSearchKeywords(name string) []string {
	var keywords []string

	// 原始关键词
	keywords = append(keywords, name)

	// 如果名称包含"股份"、"集团"等后缀，尝试去掉
	suffixes := []string{"股份", "集团", "公司", "有限公司", "科技", "实业"}
	for _, suffix := range suffixes {
		if strings.HasSuffix(name, suffix) {
			shortName := strings.TrimSuffix(name, suffix)
			if len(shortName) > 1 {
				keywords = append(keywords, shortName)
			}
		}
	}

	// 如果名称包含"中国"、"上海"等前缀，尝试去掉
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

// ============================================================================
// FMP API (Financial Modeling Prep)
// ============================================================================

// tryFMPFreeAPI 使用免费的Financial Modeling Prep API (不需要API key的基础功能)
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

// ============================================================================
// Yahoo Finance API
// ============================================================================

// tryYahooFinanceAPI 使用Yahoo Finance API作为备用方案
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

// ============================================================================
// 主要价格获取入口
// ============================================================================

// getStockPrice 获取股票价格（带多API降级策略）
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

// tryFinnhubAPI 尝试美股API（实际上是多API降级策略）
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

// ============================================================================
// 辅助函数
// ============================================================================

// min 返回两个整数中的较小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
