package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	dryRun := false // 设置为 true 进行预览，设置为 false 执行迁移

	fmt.Println("=== Intraday 目录重构迁移工具 ===")
	fmt.Printf("模式: %s\n\n", map[bool]string{true: "预览模式（不会修改文件）", false: "执行模式（将移动文件）"}[dryRun])

	oldRoot := filepath.Join("data", "intraday")
	entries, err := os.ReadDir(oldRoot)
	if err != nil {
		fmt.Printf("❌ 读取目录失败: %v\n", err)
		return
	}

	stats := make(map[string]int)      // 市场 -> 文件数量
	stockCount := make(map[string]int) // 市场 -> 股票数量
	errorLog := []string{}

	fmt.Println("扫描现有股票目录...\n")

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		stockCode := entry.Name()

		// 跳过已经是市场目录的
		if stockCode == "CN" || stockCode == "HK" || stockCode == "US" {
			fmt.Printf("⏭️  跳过市场目录: %s\n", stockCode)
			continue
		}

		market := getMarketDirectory(stockCode)
		oldPath := filepath.Join(oldRoot, stockCode)
		newPath := filepath.Join(oldRoot, market, stockCode)

		// 统计文件数
		fileCount, err := countJSONFiles(oldPath)
		if err != nil {
			errMsg := fmt.Sprintf("统计文件失败 %s: %v", stockCode, err)
			errorLog = append(errorLog, errMsg)
			fmt.Printf("⚠️  %s\n", errMsg)
			continue
		}

		fmt.Printf("📦 %s → %s/%s (%d 文件)\n", stockCode, market, stockCode, fileCount)

		if !dryRun {
			// 创建市场目录
			marketDir := filepath.Join(oldRoot, market)
			if err := os.MkdirAll(marketDir, 0755); err != nil {
				errMsg := fmt.Sprintf("创建市场目录失败 %s: %v", market, err)
				errorLog = append(errorLog, errMsg)
				fmt.Printf("❌ %s\n", errMsg)
				continue
			}

			// 移动股票目录
			if err := moveDirectory(oldPath, newPath); err != nil {
				errMsg := fmt.Sprintf("移动目录失败 %s: %v", stockCode, err)
				errorLog = append(errorLog, errMsg)
				fmt.Printf("❌ %s\n", errMsg)
				continue
			}

			fmt.Printf("✅ 已迁移 %s\n", stockCode)
		}

		stats[market] += fileCount
		stockCount[market]++
	}

	// 输出汇总
	fmt.Println("\n" + strings.Repeat("=", 50))
	fmt.Println("迁移汇总")
	fmt.Println(strings.Repeat("=", 50))

	totalFiles := 0
	totalStocks := 0
	for market, count := range stats {
		stockNum := stockCount[market]
		fmt.Printf("%-8s: %3d 股票, %4d 文件\n", market, stockNum, count)
		totalFiles += count
		totalStocks += stockNum
	}
	fmt.Println(strings.Repeat("-", 50))
	fmt.Printf("总计:     %3d 股票, %4d 文件\n", totalStocks, totalFiles)

	if len(errorLog) > 0 {
		fmt.Println("\n" + strings.Repeat("=", 50))
		fmt.Println("错误日志")
		fmt.Println(strings.Repeat("=", 50))
		for i, err := range errorLog {
			fmt.Printf("%d. %s\n", i+1, err)
		}
	}

	if dryRun {
		fmt.Println("\n" + strings.Repeat("=", 50))
		fmt.Println("💡 这是预览模式，未做任何修改")
		fmt.Println("   要执行迁移，请编辑脚本将 dryRun 改为 false")
		fmt.Println(strings.Repeat("=", 50))
	} else {
		fmt.Println("\n" + strings.Repeat("=", 50))
		fmt.Println("✅ 迁移完成！")
		fmt.Println(strings.Repeat("=", 50))
	}
}

// getMarketDirectory 根据股票代码返回市场目录名
func getMarketDirectory(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))

	// A股识别 (上海、深圳)
	if strings.HasPrefix(code, "SH") || strings.HasPrefix(code, "SZ") ||
		(len(code) == 6 && (strings.HasPrefix(code, "0") ||
			strings.HasPrefix(code, "3") ||
			strings.HasPrefix(code, "6"))) {
		return "CN"
	}

	// 港股识别
	if strings.HasPrefix(code, "HK") || strings.HasSuffix(code, ".HK") {
		return "HK"
	}

	// 默认为美股
	return "US"
}

// moveDirectory 安全地移动目录（原子操作或复制+删除）
func moveDirectory(src, dst string) error {
	// 检查源目录是否存在
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return fmt.Errorf("源目录不存在: %s", src)
	}

	// 检查目标目录是否已存在
	if _, err := os.Stat(dst); err == nil {
		return fmt.Errorf("目标目录已存在: %s", dst)
	}

	// 尝试直接重命名（同文件系统时是原子操作）
	if err := os.Rename(src, dst); err == nil {
		return nil
	}

	// 重命名失败，使用复制+删除方案
	if err := copyDirectory(src, dst); err != nil {
		return fmt.Errorf("复制失败: %w", err)
	}

	// 验证复制是否成功（比较文件数）
	srcCount, _ := countJSONFiles(src)
	dstCount, _ := countJSONFiles(dst)
	if srcCount != dstCount {
		return fmt.Errorf("文件数不匹配: 源=%d, 目标=%d", srcCount, dstCount)
	}

	// 删除源目录（仅在验证成功后）
	return os.RemoveAll(src)
}

// copyDirectory 递归复制目录
func copyDirectory(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, _ := filepath.Rel(src, path)
		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		return copyFile(path, dstPath)
	})
}

// copyFile 复制单个文件
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}

	return dstFile.Sync()
}

// countJSONFiles 统计目录中的 JSON 文件数量
func countJSONFiles(dir string) (int, error) {
	count := 0
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(info.Name(), ".json") {
			count++
		}
		return nil
	})
	return count, err
}
