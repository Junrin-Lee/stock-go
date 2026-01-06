package main

import (
	"testing"
)

// TestConvertStockCodeForTencent 测试腾讯API的股票代码转换
func TestConvertStockCodeForTencent(t *testing.T) {
	tests := []struct {
		input    string
		expected string
		desc     string
	}{
		// A股测试
		{"SH600000", "sh600000", "上海A股标准格式"},
		{"SZ000001", "sz000001", "深圳A股标准格式"},

		// 港股测试（关键：补齐到5位）
		{"HK00700", "hk00700", "港股标准格式（已经5位）"},
		{"HK9626", "hk09626", "港股4位代码应补齐到5位"},
		{"HK2020", "hk02020", "港股4位代码应补齐到5位"},
		{"HK700", "hk00700", "港股3位代码应补齐到5位"},
		{"0700.HK", "hk00700", "港股.HK格式应转换并补齐"},
		{"2020.HK", "hk02020", "港股.HK格式应转换并补齐"},

		// 美股测试
		{"AAPL", "aapl", "美股保持原样并转小写"},
		{"AMD", "amd", "美股保持原样并转小写"},
	}

	for _, tt := range tests {
		result := convertStockCodeForTencent(tt.input)
		if result != tt.expected {
			t.Errorf("%s: convertStockCodeForTencent(%q) = %q, expected %q",
				tt.desc, tt.input, result, tt.expected)
		}
	}
}

// TestConvertStockCodeForYahoo 测试Yahoo Finance API的股票代码转换
func TestConvertStockCodeForYahoo(t *testing.T) {
	tests := []struct {
		input    string
		expected string
		desc     string
	}{
		// 港股测试（关键：去除前导零，添加.HK）
		{"HK00700", "700.HK", "港股5位代码去除前导零"},
		{"HK9626", "9626.HK", "港股4位代码保持"},
		{"HK2020", "2020.HK", "港股4位代码保持"},
		{"HK700", "700.HK", "港股3位代码去除前导零"},
		{"0700.HK", "0700.HK", "港股已是.HK格式保持原样"},

		// 美股测试（保持原样）
		{"AAPL", "AAPL", "美股保持原样"},
		{"AMD", "AMD", "美股保持原样"},
		{"MSFT", "MSFT", "美股保持原样"},
	}

	for _, tt := range tests {
		result := convertStockCodeForYahoo(tt.input)
		if result != tt.expected {
			t.Errorf("%s: convertStockCodeForYahoo(%q) = %q, expected %q",
				tt.desc, tt.input, result, tt.expected)
		}
	}
}

// TestConvertStockCodeForEastMoney 测试东方财富API的股票代码转换
func TestConvertStockCodeForEastMoney(t *testing.T) {
	tests := []struct {
		input    string
		expected string
		desc     string
	}{
		// A股测试
		{"SH600000", "1.600000", "上海A股"},
		{"SZ000001", "0.000001", "深圳A股"},

		// 港股测试（市场代码116）
		{"HK00700", "116.00700", "港股5位代码"},
		{"HK9626", "116.09626", "港股4位代码补齐到5位"},
		{"HK2020", "116.02020", "港股4位代码补齐到5位"},
	}

	for _, tt := range tests {
		result := convertStockCodeForEastMoney(tt.input)
		if result != tt.expected {
			t.Errorf("%s: convertStockCodeForEastMoney(%q) = %q, expected %q",
				tt.desc, tt.input, result, tt.expected)
		}
	}
}

// TestPadHKStockCodeIntraday 测试港股代码补齐函数
func TestPadHKStockCodeIntraday(t *testing.T) {
	tests := []struct {
		input    string
		expected string
		desc     string
	}{
		{"700", "00700", "3位代码补齐到5位"},
		{"2020", "02020", "4位代码补齐到5位"},
		{"9626", "09626", "4位代码补齐到5位"},
		{"00700", "00700", "已经5位保持不变"},
		{"123456", "123456", "超过5位保持不变"},
	}

	for _, tt := range tests {
		result := padHKStockCodeIntraday(tt.input)
		if result != tt.expected {
			t.Errorf("%s: padHKStockCodeIntraday(%q) = %q, expected %q",
				tt.desc, tt.input, result, tt.expected)
		}
	}
}

// TestCompareDatapoints 测试数据点比较函数
func TestCompareDatapoints(t *testing.T) {
	tests := []struct {
		name               string
		oldDatapoints      []IntradayDataPoint
		newDatapoints      []IntradayDataPoint
		expectedNewCount   int
		expectedPriceCount int
		desc               string
	}{
		{
			name: "完全相同的数据",
			oldDatapoints: []IntradayDataPoint{
				{Time: "09:30", Price: 100.0},
				{Time: "09:31", Price: 100.5},
			},
			newDatapoints: []IntradayDataPoint{
				{Time: "09:30", Price: 100.0},
				{Time: "09:31", Price: 100.5},
			},
			expectedNewCount:   0,
			expectedPriceCount: 0,
			desc:               "数据完全相同应该返回0个新增和0个价格变化",
		},
		{
			name: "仅有新增时间点",
			oldDatapoints: []IntradayDataPoint{
				{Time: "09:30", Price: 100.0},
				{Time: "09:31", Price: 100.5},
			},
			newDatapoints: []IntradayDataPoint{
				{Time: "09:30", Price: 100.0},
				{Time: "09:31", Price: 100.5},
				{Time: "09:32", Price: 101.0},
				{Time: "09:33", Price: 101.5},
			},
			expectedNewCount:   2,
			expectedPriceCount: 0,
			desc:               "仅追加新时间点，价格变化计数应为0",
		},
		{
			name: "仅有价格变化",
			oldDatapoints: []IntradayDataPoint{
				{Time: "09:30", Price: 100.0},
				{Time: "09:31", Price: 100.5},
			},
			newDatapoints: []IntradayDataPoint{
				{Time: "09:30", Price: 100.2}, // 价格变化
				{Time: "09:31", Price: 100.8}, // 价格变化
			},
			expectedNewCount:   0,
			expectedPriceCount: 2,
			desc:               "价格变化应该被正确识别",
		},
		{
			name: "同时有新增和价格变化",
			oldDatapoints: []IntradayDataPoint{
				{Time: "09:30", Price: 100.0},
				{Time: "09:31", Price: 100.5},
			},
			newDatapoints: []IntradayDataPoint{
				{Time: "09:30", Price: 100.2}, // 价格变化
				{Time: "09:31", Price: 100.5}, // 价格不变
				{Time: "09:32", Price: 101.0}, // 新增时间点
			},
			expectedNewCount:   1,
			expectedPriceCount: 1,
			desc:               "应该同时识别新增时间点和价格变化",
		},
		{
			name:          "空数据对比",
			oldDatapoints: []IntradayDataPoint{},
			newDatapoints: []IntradayDataPoint{
				{Time: "09:30", Price: 100.0},
			},
			expectedNewCount:   1,
			expectedPriceCount: 0,
			desc:               "旧数据为空，所有新数据应视为新增",
		},
		{
			name: "价格微小差异（浮点数精度）",
			oldDatapoints: []IntradayDataPoint{
				{Time: "09:30", Price: 100.123},
			},
			newDatapoints: []IntradayDataPoint{
				{Time: "09:30", Price: 100.123001}, // 微小差异
			},
			expectedNewCount:   0,
			expectedPriceCount: 1,
			desc:               "即使微小的价格差异也应该被识别",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := compareDatapoints(tt.oldDatapoints, tt.newDatapoints)

			if result.NewEntryCount != tt.expectedNewCount {
				t.Errorf("%s: NewEntryCount = %d, expected %d",
					tt.desc, result.NewEntryCount, tt.expectedNewCount)
			}

			if result.PriceChangeCount != tt.expectedPriceCount {
				t.Errorf("%s: PriceChangeCount = %d, expected %d",
					tt.desc, result.PriceChangeCount, tt.expectedPriceCount)
			}
		})
	}
}

// TestShouldSaveIntradayData 测试保存决策函数
func TestShouldSaveIntradayData(t *testing.T) {
	tests := []struct {
		name               string
		existingDatapoints []IntradayDataPoint
		newDatapoints      []IntradayDataPoint
		expectedDecision   SaveDecision
		desc               string
	}{
		{
			name:               "首次写入（空数据）",
			existingDatapoints: []IntradayDataPoint{},
			newDatapoints: []IntradayDataPoint{
				{Time: "09:30", Price: 100.0},
			},
			expectedDecision: SaveDecisionUpdate,
			desc:             "首次写入应该返回Update决策",
		},
		{
			name: "数据完全相同",
			existingDatapoints: []IntradayDataPoint{
				{Time: "09:30", Price: 100.0},
				{Time: "09:31", Price: 100.5},
			},
			newDatapoints: []IntradayDataPoint{
				{Time: "09:30", Price: 100.0},
				{Time: "09:31", Price: 100.5},
			},
			expectedDecision: SaveDecisionSkip,
			desc:             "数据完全相同应该跳过保存",
		},
		{
			name: "仅有新增时间点（无价格变化）",
			existingDatapoints: []IntradayDataPoint{
				{Time: "09:30", Price: 100.0},
				{Time: "09:31", Price: 100.5},
			},
			newDatapoints: []IntradayDataPoint{
				{Time: "09:30", Price: 100.0},
				{Time: "09:31", Price: 100.5},
				{Time: "09:32", Price: 101.0},
				{Time: "09:33", Price: 101.5},
			},
			expectedDecision: SaveDecisionAppend,
			desc:             "仅追加新数据点应该返回Append决策",
		},
		{
			name: "有价格变化（无新增）",
			existingDatapoints: []IntradayDataPoint{
				{Time: "09:30", Price: 100.0},
				{Time: "09:31", Price: 100.5},
			},
			newDatapoints: []IntradayDataPoint{
				{Time: "09:30", Price: 100.2}, // 价格变化
				{Time: "09:31", Price: 100.8}, // 价格变化
			},
			expectedDecision: SaveDecisionUpdate,
			desc:             "有价格变化应该返回Update决策",
		},
		{
			name: "同时有新增和价格变化",
			existingDatapoints: []IntradayDataPoint{
				{Time: "09:30", Price: 100.0},
				{Time: "09:31", Price: 100.5},
			},
			newDatapoints: []IntradayDataPoint{
				{Time: "09:30", Price: 100.2}, // 价格变化
				{Time: "09:31", Price: 100.5}, // 价格不变
				{Time: "09:32", Price: 101.0}, // 新增时间点
			},
			expectedDecision: SaveDecisionUpdate,
			desc:             "有价格变化时应该优先返回Update决策",
		},
		{
			name: "多个新增时间点（无价格变化）",
			existingDatapoints: []IntradayDataPoint{
				{Time: "09:30", Price: 100.0},
			},
			newDatapoints: []IntradayDataPoint{
				{Time: "09:30", Price: 100.0},
				{Time: "09:31", Price: 100.5},
				{Time: "09:32", Price: 101.0},
				{Time: "09:33", Price: 101.5},
				{Time: "09:34", Price: 102.0},
			},
			expectedDecision: SaveDecisionAppend,
			desc:             "多个新增时间点但无价格变化应该返回Append",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shouldSaveIntradayData(tt.existingDatapoints, tt.newDatapoints)

			if result != tt.expectedDecision {
				t.Errorf("%s: shouldSaveIntradayData() = %v, expected %v",
					tt.desc, result, tt.expectedDecision)
			}
		})
	}
}
