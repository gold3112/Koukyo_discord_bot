package utils

import (
	"fmt"
	"time"
)

// TimezoneInfo タイムゾーン情報
type TimezoneInfo struct {
	Name     string
	Location *time.Location
	Flag     string
	Label    string
}

// GetCommonTimezones よく使うタイムゾーン一覧を返す
func GetCommonTimezones() []*TimezoneInfo {
	locations := []struct {
		name  string
		flag  string
		label string
		tz    string
	}{
		{"UTC", "🌐", "協定世界時 (UTC)", "UTC"},
		{"America/Los_Angeles", "🇺🇸", "サンタクララ (PST/PDT)", "America/Los_Angeles"},
		{"Europe/Paris", "🇫🇷", "フランス (CET/CEST)", "Europe/Paris"},
		{"Asia/Tokyo", "🇯🇵", "日本標準時 (JST)", "Asia/Tokyo"},
	}

	var timezones []*TimezoneInfo
	for _, l := range locations {
		loc, err := time.LoadLocation(l.tz)
		if err != nil {
			continue
		}
		timezones = append(timezones, &TimezoneInfo{
			Name:     l.name,
			Location: loc,
			Flag:     l.flag,
			Label:    l.label,
		})
	}
	return timezones
}

// ParseTimezone タイムゾーン名から Location を取得
func ParseTimezone(tzName string) (*time.Location, error) {
	// 短縮形のマッピング
	shortNames := map[string]string{
		"pst":  "America/Los_Angeles",
		"pdt":  "America/Los_Angeles",
		"jst":  "Asia/Tokyo",
		"cet":  "Europe/Paris",
		"cest": "Europe/Paris",
		"utc":  "UTC",
	}

	// 短縮形をチェック
	if fullName, ok := shortNames[tzName]; ok {
		return time.LoadLocation(fullName)
	}

	// そのまま試す
	return time.LoadLocation(tzName)
}

// FormatTimeInTimezone 指定タイムゾーンで時刻をフォーマット
func FormatTimeInTimezone(t time.Time, loc *time.Location) string {
	tt := t.In(loc)
	weekdays := []string{"日", "月", "火", "水", "木", "金", "土"}
	wd := weekdays[int(tt.Weekday())]
	return fmt.Sprintf("%s (%s) %s", tt.Format("2006-01-02"), wd, tt.Format("15:04:05 MST"))
}

// ConvertTime タイムゾーン間で時刻を変換
// fromTz: 元のタイムゾーン (例: "JST", "PST")
// toTz: 変換先タイムゾーン (例: "JST", "PST")
// timeStr: 時刻文字列 (例: "23:20" または空文字列で現在時刻を使用)
func ConvertTime(fromTz, toTz, timeStr string) (string, error) {
	// タイムゾーンを解析
	fromLoc, err := ParseTimezone(fromTz)
	if err != nil {
		return "", fmt.Errorf("無効な元のタイムゾーン: %s", fromTz)
	}

	toLoc, err := ParseTimezone(toTz)
	if err != nil {
		return "", fmt.Errorf("無効な変換先タイムゾーン: %s", toTz)
	}

	var sourceTime time.Time

	// 時刻文字列がある場合、パースする
	if timeStr != "" {
		// "HH:MM" または "HH:MM:SS" 形式をサポート
		now := time.Now()
		parsed, err := time.Parse("15:04:05", timeStr+":00")
		if err != nil {
			// ":00" を追加してリトライ
			parsed, err = time.Parse("15:04", timeStr)
			if err != nil {
				return "", fmt.Errorf("無効な時刻形式: %s (HH:MM または HH:MM:SS 形式で入力してください)", timeStr)
			}
		}
		// 本日の日付で時刻を作成（元のタイムゾーンで）
		sourceTime = time.Date(now.Year(), now.Month(), now.Day(),
			parsed.Hour(), parsed.Minute(), parsed.Second(), 0, fromLoc)
	} else {
		// 時刻指定がない場合は現在時刻を使用
		sourceTime = time.Now().In(fromLoc)
	}

	// 変換先タイムゾーンに変換
	convertedTime := sourceTime.In(toLoc)

	// 結果をフォーマット
	weekdays := []string{"日", "月", "火", "水", "木", "金", "土"}
	wd := weekdays[int(convertedTime.Weekday())]

	result := fmt.Sprintf("%s (%s) %s",
		convertedTime.Format("2006-01-02"), wd, convertedTime.Format("15:04:05 MST"))

	// 元の時刻と変換先の時刻を両方表示
	return fmt.Sprintf("**[元] %s (%s)**: %s\n**[先] %s (%s)**: %s",
		GetTimezoneLabel(fromTz), fromTz,
		sourceTime.Format("2006-01-02 (Mon) 15:04:05 MST"),
		GetTimezoneLabel(toTz), toTz,
		result), nil
}

// GetTimezoneLabel タイムゾーン名からラベルを取得
func GetTimezoneLabel(tzName string) string {
	labels := map[string]string{
		"UTC":                 "協定世界時",
		"America/Los_Angeles": "サンタクララ",
		"PST":                 "サンタクララ",
		"PDT":                 "サンタクララ",
		"Europe/Paris":        "フランス",
		"CET":                 "フランス",
		"CEST":                "フランス",
		"Asia/Tokyo":          "日本標準時",
		"JST":                 "日本標準時",
	}

	if label, ok := labels[tzName]; ok {
		return label
	}
	return tzName
}
