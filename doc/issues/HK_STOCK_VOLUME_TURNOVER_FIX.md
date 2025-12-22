# 港股成交量和换手率显示问题修复方案

**创建日期**: 2025-12-22  
**问题报告人**: 用户  
**分析人**: AI Assistant  
**优先级**: 🔴 高 (影响港股用户体验)  
**状态**: 📝 待实施

---

## 📊 问题总结

### 用户报告
在自选列表和持股列表中，港股的**成交量（Volume）**和**换手率（TurnoverRate）**字段**没有数据显示**。

### 影响范围
- ✅ **A股**: 成交量和换手率正常显示
- ❌ **港股**: 成交量和换手率缺失
- ⚠️ **美股**: 未测试（可能存在类似问题）

---

## 🔍 根因分析

### 1. 配置检查结果 ✅

**当前配置文件** (`cmd/conf/config.yml`):

```yaml
watchlist_columns:
    - cursor
    - tag
    - code
    - name
    - price
    - prev_close
    - open
    - high
    - low
    - today_change
    - turnover      # ✅ 已配置
    - volume        # ✅ 已配置
```

**结论**: 配置文件正确，列定义存在且顺序合理。

---

### 2. API数据源测试结果

#### 🔴 问题1：腾讯API港股换手率缺失

**测试数据**（2025-12-22）:

| 股票 | 代码 | fields[36] (成交量) | fields[38] (换手率) |
|------|------|-------------------|-------------------|
| **A股** | SH600000 (浦发银行) | 678151 | **0.20** ✅ |
| **港股** | HK00700 (腾讯控股) | 17765762.0 | **0** ❌ |
| **港股** | HK09626 (哔哩哔哩) | 2722898.0 | **0** ❌ |
| **港股** | HK02020 (安踏体育) | 5298265.0 | **0** ❌ |
| **港股** | HK00388 (港交所) | 3281541.0 | **0** ❌ |

**API响应示例** (腾讯API):
```
v_hk00700="100~腾讯控股~00700~614.000~605.000~...~17765762.0~...~0~..."
          [0]  [1]      [2]   [3]      [4]         [36]           [38]
          市场 名称     代码  现价     昨收        成交量         换手率(恒为0)
```

**根本原因**:
- 腾讯财经API对**港股**的 `fields[38]` (换手率) **始终返回 0**
- 这是**API数据源的限制**，非代码bug
- 成交量 `fields[36]` 有正常数据

---

#### ✅ 解决方案：使用备用API

经过详细测试，找到了**两个可用的备用API**:

##### 方案A：东方财富API（推荐）⭐

**优势**:
- ✅ **有完整的港股换手率数据**
- ✅ 免费、无需API key
- ✅ 无明显限流（测试5次连续请求全部成功）
- ✅ 响应速度快（<1秒）
- ✅ 返回JSON格式，易于解析

**API详情**:
```
URL: https://push2.eastmoney.com/api/qt/stock/get
参数: 
  - secid=116.{code}  # 港股市场代码116
  - fields=f43,f46,f47,f168,f170  # 只请求必要字段
```

**字段映射**:
```go
f43:  现价 (需除以100)      // 614000 → 614.00
f46:  昨收价 (需除以100)    // 605000 → 605.00
f47:  成交量 (手)           // 17765762
f168: 换手率 (需除以100)    // 19 → 0.19%
f170: 涨跌幅 (需除以100)    // 149 → 1.49%
```

**实际测试数据**:
| 股票 | 成交量 (f47) | 换手率 (f168) |
|------|-------------|-------------|
| 腾讯控股 (00700) | 17765762 | 19 (0.19%) ✅ |
| 哔哩哔哩 (09626) | 2722898 | 80 (0.80%) ✅ |
| 安踏体育 (02020) | 5298265 | 19 (0.19%) ✅ |

**限流测试**: 连续5次请求全部成功，HTTP 200

---

##### 方案B：Yahoo Finance API

**优势**:
- ✅ 免费、无需API key
- ✅ 稳定性高（已在项目中使用）
- ✅ 有成交量数据

**缺陷**:
- ❌ **没有换手率字段**
- ⚠️ 只能作为成交量的备用数据源

**API详情**:
```
URL: https://query1.finance.yahoo.com/v8/finance/chart/{symbol}
参数: interval=1d&range=1d
```

**测试数据**:
| 股票 | 成交量 | 换手率 |
|------|-------|-------|
| 腾讯 (0700.HK) | 17765862 ✅ | N/A ❌ |
| 哔哩哔哩 (9626.HK) | 2723358 ✅ | N/A ❌ |

---

### 3. 代码逻辑分析

#### 当前数据流

```
[腾讯API] → api.go:tryTencentAPI()
             ↓
         StockData{
             Volume: fields[36],        // ✅ 有数据
             TurnoverRate: fields[38]   // ❌ 港股恒为0
         }
             ↓
         columns.go:GenerateWatchlistRow()
             ↓
         界面显示: formatVolume(stockData.Volume)
                  fmt.Sprintf("%.2f%%", stockData.TurnoverRate)
```

**分析**:
- ✅ 代码逻辑正确，按预期从 `stockData.Volume` 和 `stockData.TurnoverRate` 读取
- ✅ `formatVolume()` 函数正常工作（A股测试通过）
- ❌ 问题在于数据源：港股 `TurnoverRate` 始终为 0

---

## 💡 修复方案

### 方案设计原则

1. **最小侵入性**: 优先在现有架构上扩展，避免大规模重构
2. **向后兼容**: 不影响A股和美股的现有功能
3. **API降级链**: 主API失败时自动尝试备用API
4. **数据准确性**: 确保港股换手率数据来源可靠

---

### 推荐实施方案：三级API降级策略

#### 架构设计

```
港股实时数据获取流程:

1. 主数据源 (腾讯API)
   ├─ 获取: 价格、昨收、开盘、最高、最低、成交量
   └─ 缺失: 换手率

2. 换手率补充 (东方财富API)
   └─ 仅在港股 && 换手率=0 时调用
   └─ 获取: 换手率 (f168)

3. 成交量备用 (Yahoo Finance)
   └─ 腾讯和东方财富都失败时使用
```

#### 实施步骤

##### Step 1: 新增东方财富API集成函数

**位置**: `api.go`

```go
// tryEastMoneyHKTurnover 从东方财富获取港股换手率
// 仅用于补充腾讯API缺失的港股换手率数据
func tryEastMoneyHKTurnover(symbol string) (float64, int64, error) {
	// 转换股票代码为东方财富格式 (HK00700 → 116.00700)
	emCode := convertStockCodeForEastMoneyAPI(symbol)
	if emCode == "" {
		return 0, 0, fmt.Errorf("invalid HK stock code: %s", symbol)
	}

	// 构建API URL (只请求必要字段以减少流量)
	url := fmt.Sprintf(
		"https://push2.eastmoney.com/api/qt/stock/get?secid=%s&fields=f47,f168",
		emCode,
	)
	debugPrint("debug.api.eastmoneyTurnoverUrl", url)

	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		debugPrint("debug.api.eastmoneyTurnoverHttpFail", err)
		return 0, 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		debugPrint("debug.api.eastmoneyTurnoverReadFail", err)
		return 0, 0, err
	}

	var result struct {
		Data struct {
			F47  int64 `json:"f47"`  // 成交量
			F168 int   `json:"f168"` // 换手率 (需除以100)
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		debugPrint("debug.api.eastmoneyTurnoverJsonFail", err)
		return 0, 0, err
	}

	// 换手率需要除以100 (19 → 0.19%)
	turnover := float64(result.Data.F168) / 100.0
	volume := result.Data.F47

	debugPrint("debug.api.eastmoneyTurnoverSuccess", symbol, turnover, volume)
	return turnover, volume, nil
}

// convertStockCodeForEastMoneyAPI 转换股票代码为东方财富API格式
func convertStockCodeForEastMoneyAPI(symbol string) string {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))

	if strings.HasPrefix(symbol, "HK") {
		// HK00700 → 116.00700, HK9626 → 116.09626
		code := strings.TrimPrefix(symbol, "HK")
		return "116." + padHKStockCode(code)
	} else if strings.HasSuffix(symbol, ".HK") {
		// 0700.HK → 116.00700
		code := strings.TrimSuffix(symbol, ".HK")
		return "116." + padHKStockCode(code)
	}

	return "" // 非港股返回空字符串
}
```

---

##### Step 2: 修改主获取函数，增加港股换手率补充逻辑

**位置**: `api.go:getStockPrice()`

```go
// getStockPrice 获取股票价格（带多API降级策略）
func getStockPrice(symbol string) *StockData {
	// A股和港股都优先使用腾讯API
	if isChinaStock(symbol) || isHKStock(symbol) {
		data := tryTencentAPI(symbol)
		if data.Price > 0 {
			// 🆕 港股换手率补充逻辑
			if isHKStock(symbol) && data.TurnoverRate == 0 {
				debugPrint("debug.api.hkTurnoverMissing", symbol)
				
				// 尝试从东方财富获取换手率和成交量
				turnover, volume, err := tryEastMoneyHKTurnover(symbol)
				if err == nil {
					data.TurnoverRate = turnover
					// 如果东方财富的成交量更新，也使用它
					if volume > 0 {
						data.Volume = volume
					}
					debugPrint("debug.api.hkTurnoverEnhanced", symbol, turnover)
				} else {
					debugPrint("debug.api.hkTurnoverFallbackFail", err)
				}
			}
			return data
		}
		debugPrint("debug.api.tencentFail")
	}

	// 其他市场的降级逻辑保持不变
	data := tryFinnhubAPI(symbol)
	if data.Price > 0 {
		return data
	}

	debugPrint("debug.api.allApiFail")
	return nil
}
```

---

##### Step 3: 添加调试日志国际化

**位置**: `i18n/zh.json` 和 `i18n/en.json`

```json
// zh.json
{
  "debug.api.eastmoneyTurnoverUrl": "[东方财富] 请求URL: %s",
  "debug.api.eastmoneyTurnoverHttpFail": "[东方财富] HTTP请求失败: %v",
  "debug.api.eastmoneyTurnoverReadFail": "[东方财富] 读取响应失败: %v",
  "debug.api.eastmoneyTurnoverJsonFail": "[东方财富] JSON解析失败: %v",
  "debug.api.eastmoneyTurnoverSuccess": "[东方财富] 港股换手率获取成功: %s, 换手率=%.2f%%, 成交量=%d",
  "debug.api.hkTurnoverMissing": "[港股] 检测到换手率缺失: %s, 尝试东方财富API补充",
  "debug.api.hkTurnoverEnhanced": "[港股] 换手率补充成功: %s, 换手率=%.2f%%",
  "debug.api.hkTurnoverFallbackFail": "[港股] 换手率备用API失败: %v"
}

// en.json
{
  "debug.api.eastmoneyTurnoverUrl": "[EastMoney] Request URL: %s",
  "debug.api.eastmoneyTurnoverHttpFail": "[EastMoney] HTTP request failed: %v",
  "debug.api.eastmoneyTurnoverReadFail": "[EastMoney] Read response failed: %v",
  "debug.api.eastmoneyTurnoverJsonFail": "[EastMoney] JSON parse failed: %v",
  "debug.api.eastmoneyTurnoverSuccess": "[EastMoney] HK turnover fetched: %s, turnover=%.2f%%, volume=%d",
  "debug.api.hkTurnoverMissing": "[HK Stock] Turnover missing: %s, trying EastMoney API",
  "debug.api.hkTurnoverEnhanced": "[HK Stock] Turnover enhanced: %s, turnover=%.2f%%",
  "debug.api.hkTurnoverFallbackFail": "[HK Stock] Turnover fallback failed: %v"
}
```

---

##### Step 4: 更新单元测试

**位置**: `api_test.go` (新建文件)

```go
package main

import (
	"testing"
)

// TestConvertStockCodeForEastMoneyAPI 测试东方财富API代码转换
func TestConvertStockCodeForEastMoneyAPI(t *testing.T) {
	tests := []struct {
		input    string
		expected string
		desc     string
	}{
		{"HK00700", "116.00700", "港股5位代码"},
		{"HK9626", "116.09626", "港股4位代码补齐到5位"},
		{"HK2020", "116.02020", "港股4位代码补齐到5位"},
		{"HK700", "116.00700", "港股3位代码补齐到5位"},
		{"0700.HK", "116.00700", "港股.HK格式转换并补齐"},
		{"2020.HK", "116.02020", "港股.HK格式转换并补齐"},
		{"SH600000", "", "A股返回空字符串"},
		{"AAPL", "", "美股返回空字符串"},
	}

	for _, tt := range tests {
		result := convertStockCodeForEastMoneyAPI(tt.input)
		if result != tt.expected {
			t.Errorf("%s: convertStockCodeForEastMoneyAPI(%q) = %q, expected %q",
				tt.desc, tt.input, result, tt.expected)
		}
	}
}

// TestTryEastMoneyHKTurnover 测试东方财富换手率获取（集成测试）
func TestTryEastMoneyHKTurnover(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过集成测试（需要网络）")
	}

	testCases := []string{
		"HK00700", // 腾讯控股
		"HK09626", // 哔哩哔哩
		"HK02020", // 安踏体育
	}

	for _, code := range testCases {
		turnover, volume, err := tryEastMoneyHKTurnover(code)
		if err != nil {
			t.Errorf("tryEastMoneyHKTurnover(%s) 返回错误: %v", code, err)
			continue
		}

		if turnover < 0 || turnover > 100 {
			t.Errorf("tryEastMoneyHKTurnover(%s) 换手率异常: %.2f%%", code, turnover)
		}

		if volume <= 0 {
			t.Errorf("tryEastMoneyHKTurnover(%s) 成交量异常: %d", code, volume)
		}

		t.Logf("✅ %s: 换手率=%.2f%%, 成交量=%d", code, turnover, volume)
	}
}
```

---

## 🧪 测试计划

### 测试用例

| 测试ID | 场景 | 股票代码 | 预期结果 |
|-------|------|---------|---------|
| TC-01 | A股成交量和换手率 | SH600000 | ✅ 正常显示（腾讯API） |
| TC-02 | 港股成交量 | HK00700 | ✅ 正常显示（腾讯API） |
| TC-03 | 港股换手率 | HK00700 | ✅ 正常显示（东方财富API补充） |
| TC-04 | 港股换手率（备用） | HK09626 | ✅ 正常显示（东方财富API） |
| TC-05 | 东方财富API失败降级 | HK00700 | ⚠️ 换手率显示0或"-" |
| TC-06 | 腾讯API和东方财富都失败 | HK00700 | ⚠️ 价格也为"-"（预期行为） |

### 回归测试

- [ ] A股持股列表显示正常
- [ ] A股自选列表显示正常
- [ ] 美股持股列表显示正常（如有）
- [ ] 美股自选列表显示正常（如有）
- [ ] 排序功能正常（按成交量、换手率）
- [ ] 分页功能正常
- [ ] 配置文件加载正常

---

## 📦 交付物

### 代码变更

1. **api.go**
   - 新增: `tryEastMoneyHKTurnover()` 函数
   - 新增: `convertStockCodeForEastMoneyAPI()` 函数
   - 修改: `getStockPrice()` 添加港股换手率补充逻辑

2. **i18n/zh.json** 和 **i18n/en.json**
   - 新增: 8个调试日志键值对

3. **api_test.go** (新建)
   - 新增: `TestConvertStockCodeForEastMoneyAPI()`
   - 新增: `TestTryEastMoneyHKTurnover()`

### 文档更新

1. **README.md** 和 **README_EN.md**
   - 更新API支持列表，添加东方财富API
   - 更新港股数据源说明

2. **doc/version/v5.4.md** (新建版本文档)
   - 记录此次修复的详细信息

---

## ⏱️ 实施时间估算

| 任务 | 预计时间 |
|------|---------|
| 编写代码 | 1小时 |
| 单元测试 | 30分钟 |
| 集成测试 | 30分钟 |
| 文档更新 | 30分钟 |
| **总计** | **2.5小时** |

---

## 🚀 部署计划

### 开发环境

1. 创建特性分支: `feature/hk-stock-turnover-fix`
2. 实施代码变更
3. 运行单元测试: `go test -v ./...`
4. 本地手动测试（添加真实港股到自选列表）
5. 提交代码并创建Pull Request

### 生产环境

1. 合并到主分支
2. 更新版本号为 v5.4
3. 编译新版本: `go build -o cmd/stock-monitor`
4. 发布Release Notes

---

## 🔮 未来优化建议

### 1. 美股换手率支持

**问题**: 美股可能也缺少换手率数据  
**方案**: 调研并集成美股换手率数据源（如Alpha Vantage、Polygon.io）

### 2. API性能监控

**目标**: 监控各API的响应时间和成功率  
**实现**: 添加Prometheus metrics或简单的日志统计

### 3. API Key管理

**目标**: 为可能需要API Key的数据源预留配置  
**实现**: 在 `config.yml` 添加 `api_keys` 配置块

### 4. 数据缓存优化

**目标**: 减少API调用频率  
**实现**: 对换手率数据也应用30秒TTL缓存

---

## 📚 参考资料

### API文档

1. **东方财富API**
   - Endpoint: `https://push2.eastmoney.com/api/qt/stock/get`
   - 无官方文档，通过逆向工程获得

2. **腾讯财经API**
   - Endpoint: `https://qt.gtimg.cn/q=<code>`
   - 现有实现: `api.go:tryTencentAPI()`

3. **Yahoo Finance API**
   - Endpoint: `https://query1.finance.yahoo.com/v8/finance/chart/<symbol>`
   - 现有实现: `api.go:tryYahooFinanceAPI()`

### 相关Issues

- v5.1: 修复美股和港股分时数据采集问题
- v5.3: 智能Worker状态追踪和多市场时区增强

---

## ✅ 验收标准

1. ✅ 港股换手率在自选列表中正常显示（非0值）
2. ✅ 港股成交量在自选列表中正常显示
3. ✅ A股功能不受影响
4. ✅ 美股功能不受影响（如有）
5. ✅ 单元测试全部通过
6. ✅ 调试模式下能看到API调用日志
7. ✅ 东方财富API失败时优雅降级（显示0或"-"）

---

**文档版本**: 1.0  
**最后更新**: 2025-12-22
