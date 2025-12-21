package main

import (
	"testing"
)

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
		{"SZ000001", "", "深圳A股返回空字符串"},
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

func TestTryEastMoneyHKTurnover(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过集成测试（需要网络）")
	}

	testCases := []struct {
		code string
		name string
	}{
		{"HK00700", "腾讯控股"},
		{"HK09626", "哔哩哔哩"},
		{"HK02020", "安踏体育"},
	}

	for _, tc := range testCases {
		turnover, volume, err := tryEastMoneyHKTurnover(tc.code)
		if err != nil {
			t.Errorf("tryEastMoneyHKTurnover(%s) 返回错误: %v", tc.code, err)
			continue
		}

		if turnover < 0 || turnover > 100 {
			t.Errorf("tryEastMoneyHKTurnover(%s) 换手率异常: %.2f%%", tc.code, turnover)
		}

		if volume <= 0 {
			t.Logf("警告: %s (%s) 成交量为0，可能是非交易时间", tc.name, tc.code)
		}

		t.Logf("✅ %s (%s): 换手率=%.2f%%, 成交量=%d", tc.name, tc.code, turnover, volume)
	}
}

func TestIsHKStock(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
		desc     string
	}{
		{"HK00700", true, "HK前缀"},
		{"HK9626", true, "HK前缀短代码"},
		{"0700.HK", true, ".HK后缀"},
		{"SH600000", false, "上海A股"},
		{"SZ000001", false, "深圳A股"},
		{"AAPL", false, "美股"},
	}

	for _, tt := range tests {
		result := isHKStock(tt.input)
		if result != tt.expected {
			t.Errorf("%s: isHKStock(%q) = %v, expected %v",
				tt.desc, tt.input, result, tt.expected)
		}
	}
}
