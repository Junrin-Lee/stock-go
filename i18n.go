package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// texts i18n 配置 - 存储各语言的文本映射
var texts map[Language]TextMap

// loadI18nFiles 加载 i18n 文件
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

// getText 获取本地化文本的辅助函数
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

// getDebugText 全局调试文本获取函数（供 debugPrint 使用）
// 通过 globalModel 获取当前语言设置，如果 globalModel 未初始化则默认返回英文
func getDebugText(key string) string {
	if globalModel == nil {
		// globalModel 未初始化时，默认返回英文
		if text, exists := texts[English][key]; exists {
			return text
		}
		return key
	}
	return globalModel.getText(key)
}
