package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// parseTimeInMarket 在指定市场时区中解析时间字符串
// date: "20251210", timeStr: "09:30", marketConfig: 市场配置
// 返回：该市场的本地时间（time.Time）
func parseTimeInMarket(date string, timeStr string, marketConfig MarketConfig) (time.Time, error) {
	location, err := time.LoadLocation(marketConfig.Timezone)
	if err != nil {
		// 降级到本地时区
		debugPrint("debug.timezone.loadFail", marketConfig.Timezone, err)
		location = time.Local
	}

	// 解析日期
	if len(date) != 8 {
		return time.Time{}, fmt.Errorf("invalid date format: %s (expected YYYYMMDD)", date)
	}

	year, err := strconv.Atoi(date[:4])
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid year in date %s: %w", date, err)
	}
	month, err := strconv.Atoi(date[4:6])
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid month in date %s: %w", date, err)
	}
	day, err := strconv.Atoi(date[6:8])
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid day in date %s: %w", date, err)
	}

	// 解析时间
	parts := strings.Split(timeStr, ":")
	if len(parts) != 2 {
		return time.Time{}, fmt.Errorf("invalid time format: %s (expected HH:MM)", timeStr)
	}

	hour, err := strconv.Atoi(parts[0])
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid hour in time %s: %w", timeStr, err)
	}
	minute, err := strconv.Atoi(parts[1])
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid minute in time %s: %w", timeStr, err)
	}

	// 在市场时区创建时间
	return time.Date(year, time.Month(month), day, hour, minute, 0, 0, location), nil
}

// isMarketOpenForConfig 检查指定市场在指定时间是否开市
// checkTime: 要检查的时间
// marketConfig: 市场配置
// 返回：true 表示开市，false 表示休市
func isMarketOpenForConfig(checkTime time.Time, marketConfig MarketConfig) bool {
	// 转换检查时间到市场时区
	location, err := time.LoadLocation(marketConfig.Timezone)
	if err != nil {
		debugPrint("debug.timezone.invalidLocation", marketConfig.Timezone, err)
		return false
	}

	marketTime := checkTime.In(location)

	// 检查是否为工作日
	weekday := int(marketTime.Weekday())
	if weekday == 0 { // Sunday = 0 in Go, convert to 7
		weekday = 7
	}

	isWeekday := false
	for _, wd := range marketConfig.Weekdays {
		if wd == weekday {
			isWeekday = true
			break
		}
	}

	if !isWeekday {
		return false
	}

	// 检查是否在交易时段内
	currentMinutes := marketTime.Hour()*60 + marketTime.Minute()

	for _, session := range marketConfig.TradingSessions {
		startParts := strings.Split(session.StartTime, ":")
		endParts := strings.Split(session.EndTime, ":")

		if len(startParts) != 2 || len(endParts) != 2 {
			debugPrint("debug.timezone.invalidSessionTime", session.StartTime, session.EndTime)
			continue
		}

		startHour, err1 := strconv.Atoi(startParts[0])
		startMin, err2 := strconv.Atoi(startParts[1])
		endHour, err3 := strconv.Atoi(endParts[0])
		endMin, err4 := strconv.Atoi(endParts[1])

		if err1 != nil || err2 != nil || err3 != nil || err4 != nil {
			debugPrint("debug.timezone.parseSessionTimeFail", session.StartTime, session.EndTime)
			continue
		}

		startMinutes := startHour*60 + startMin
		endMinutes := endHour*60 + endMin

		if currentMinutes >= startMinutes && currentMinutes <= endMinutes {
			return true
		}
	}

	return false
}

// getCurrentDateForMarket 获取指定市场的当前日期（考虑时区）
// market: 市场类型
// m: Model 指针（用于访问配置）
// 返回：当前日期字符串 (YYYYMMDD)
func getCurrentDateForMarket(market MarketType, m *Model) string {
	var timezone string
	switch market {
	case MarketChina:
		timezone = m.config.Markets.China.Timezone
	case MarketUS:
		timezone = m.config.Markets.US.Timezone
	case MarketHongKong:
		timezone = m.config.Markets.HongKong.Timezone
	default:
		return time.Now().Format("20060102")
	}

	location, err := time.LoadLocation(timezone)
	if err != nil {
		debugPrint("debug.timezone.loadFail", timezone, err)
		return time.Now().Format("20060102")
	}

	return time.Now().In(location).Format("20060102")
}

// getMarketLocation 根据市场类型返回对应的时区Location
// marketType: 市场类型字符串 (例如 "CN", "US", "HK")
// 返回：时区Location和可能的错误
func getMarketLocation(marketType MarketType) (*time.Location, error) {
	var timezone string

	switch marketType {
	case MarketChina:
		timezone = "Asia/Shanghai"
	case MarketUS:
		timezone = "America/New_York"
	case MarketHongKong:
		timezone = "Asia/Hong_Kong"
	default:
		return nil, fmt.Errorf("unknown market type: %s", marketType)
	}

	location, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, fmt.Errorf("failed to load timezone %s: %w", timezone, err)
	}

	return location, nil
}
