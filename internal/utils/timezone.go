package utils

import (
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
	return t.In(loc).Format("2006-01-02 15:04:05 MST")
}
