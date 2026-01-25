package notifications

import (
	"Koukyo_discord_bot/internal/monitor"
	"math"
)

// Tier 通知段階
type Tier int

const (
	TierNone Tier = iota
	Tier10        // 10%以上
	Tier20        // 20%以上
	Tier30        // 30%以上
	Tier40        // 40%以上
	Tier50        // 50%以上（メンション閾値）
)

// getDiffValue 指標に応じた差分値を取得
func getDiffValue(data *monitor.MonitorData, metric string) float64 {
	if metric == "weighted" && data.WeightedDiffPercentage != nil {
		return *data.WeightedDiffPercentage
	}
	return data.DiffPercentage
}

// calculateTier 差分率からTierを計算
func calculateTier(diffValue, threshold float64) Tier {
	if diffValue < threshold {
		return TierNone
	}
	if diffValue >= 50 {
		return Tier50
	}
	if diffValue >= 40 {
		return Tier40
	}
	if diffValue >= 30 {
		return Tier30
	}
	if diffValue >= 20 {
		return Tier20
	}
	return Tier10
}

func isZeroDiff(value float64) bool {
	const zeroDiffEpsilon = 0.005
	return math.Abs(value) <= zeroDiffEpsilon
}

// getTierColor Tierに応じた色を取得
func getTierColor(tier Tier) int {
	switch tier {
	case Tier50:
		return 0xFF0000 // 赤
	case Tier40:
		return 0xFF4500 // オレンジレッド
	case Tier30:
		return 0xFFA500 // オレンジ
	case Tier20:
		return 0xFFD700 // ゴールド
	case Tier10:
		return 0xFFFF00 // 黄色
	default:
		return 0x808080 // グレー
	}
}
