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
