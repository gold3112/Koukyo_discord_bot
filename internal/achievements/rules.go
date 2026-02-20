package achievements

import (
	"Koukyo_discord_bot/internal/utils"
	"encoding/json"
	"os"
	"time"
)

type RuleSet struct {
	Version int    `json:"version"`
	Rules   []Rule `json:"rules"`
}

type Rule struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Enabled     *bool          `json:"enabled,omitempty"`
	Conditions  RuleConditions `json:"conditions"`
}

type RuleConditions struct {
	VandalCountGTE       *int  `json:"vandal_count_gte,omitempty"`
	RestoredCountGTE     *int  `json:"restored_count_gte,omitempty"`
	ActivityScoreGTE     *int  `json:"activity_score_gte,omitempty"`
	ActivityScoreLTE     *int  `json:"activity_score_lte,omitempty"`
	TotalActionsGTE      *int  `json:"total_actions_gte,omitempty"`
	MaxDailyVandalGTE    *int  `json:"max_daily_vandal_gte,omitempty"`
	MaxDailyRestoredGTE  *int  `json:"max_daily_restored_gte,omitempty"`
	ActiveDaysGTE        *int  `json:"active_days_gte,omitempty"`
	InactiveDaysGTE      *int  `json:"inactive_days_gte,omitempty"`
	DiscordLinkedRequire *bool `json:"discord_linked_required,omitempty"`
}

type UserSnapshot struct {
	DiscordID          string
	DiscordName        string
	WplaceID           string
	WplaceName         string
	VandalCount        int
	RestoredCount      int
	ActivityScore      int
	LastSeenAt         time.Time
	DailyVandalCounts  map[string]int
	DailyRestoreCounts map[string]int
}

func DefaultRuleSet() *RuleSet {
	return &RuleSet{
		Version: 1,
		Rules: []Rule{
			{
				ID:          "first_steps",
				Name:        "First Steps",
				Description: "総アクションが10回に到達",
				Conditions: RuleConditions{
					TotalActionsGTE: intPtr(10),
				},
			},
			{
				ID:          "restorer_50",
				Name:        "Restorer 50",
				Description: "修復数が50回に到達",
				Conditions: RuleConditions{
					RestoredCountGTE: intPtr(50),
				},
			},
			{
				ID:          "vandal_watcher_50",
				Name:        "Vandal Watcher 50",
				Description: "荒らし検知数が50回に到達",
				Conditions: RuleConditions{
					VandalCountGTE: intPtr(50),
				},
			},
			{
				ID:          "daily_restorer_25",
				Name:        "Daily Restorer",
				Description: "1日で修復25回以上",
				Conditions: RuleConditions{
					MaxDailyRestoredGTE: intPtr(25),
				},
			},
			{
				ID:          "score_guardian_100",
				Name:        "Guardian +100",
				Description: "活動スコアが+100以上",
				Conditions: RuleConditions{
					ActivityScoreGTE: intPtr(100),
				},
			},
			{
				ID:          "score_guardian_1000",
				Name:        "Guardian +1000",
				Description: "活動スコアが+1000以上",
				Conditions: RuleConditions{
					ActivityScoreGTE: intPtr(1000),
				},
			},
			{
				ID:          "score_guardian_5000",
				Name:        "Guardian +5000",
				Description: "活動スコアが+5000以上",
				Conditions: RuleConditions{
					ActivityScoreGTE: intPtr(5000),
				},
			},
			{
				ID:          "score_guardian_10354",
				Name:        "Guardian +10354(1皇居)",
				Description: "活動スコアが+10354以上",
				Conditions: RuleConditions{
					ActivityScoreGTE: intPtr(10354),
				},
			},
			{
				ID:          "score_destroyer_500",
				Name:        "Destroyer -500",
				Description: "活動スコアが-500以下",
				Conditions: RuleConditions{
					ActivityScoreLTE: intPtr(-500),
				},
			},
			{
				ID:          "score_destroyer_1000",
				Name:        "Destroyer -1000",
				Description: "活動スコアが-1000以下",
				Conditions: RuleConditions{
					ActivityScoreLTE: intPtr(-1000),
				},
			},
			{
				ID:          "score_destroyer_5000",
				Name:        "Destroyer -5000",
				Description: "活動スコアが-5000以下",
				Conditions: RuleConditions{
					ActivityScoreLTE: intPtr(-5000),
				},
			},
			{
				ID:          "score_destroyer_10354",
				Name:        "Destroyer -10354",
				Description: "活動スコアが-10354以下",
				Conditions: RuleConditions{
					ActivityScoreLTE: intPtr(-10354),
				},
			},
			{
				ID:          "are_you_sleepy_244070",
				Name:        "AreYouSleepy?",
				Description: "総荒らし数が244070px以上",
				Conditions: RuleConditions{
					VandalCountGTE: intPtr(244070),
				},
			},
			{
				ID:          "three_day_dropout_vandal",
				Name:        "3日坊主(荒らし)",
				Description: "荒らし寄りユーザーで4日以上活動が見られない",
				Conditions: RuleConditions{
					VandalCountGTE:   intPtr(1),
					ActivityScoreLTE: intPtr(-1),
					InactiveDaysGTE:  intPtr(4),
				},
			},
		},
	}
}

func EnsureRuleSetFile(path string) error {
	_, err := os.Stat(path)
	if err == nil {
		return nil
	}
	if !os.IsNotExist(err) {
		return err
	}
	return SaveRuleSet(path, DefaultRuleSet())
}

func LoadRuleSet(path string) (*RuleSet, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return DefaultRuleSet(), nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return DefaultRuleSet(), nil
	}
	var ruleSet RuleSet
	if err := json.Unmarshal(data, &ruleSet); err != nil {
		return nil, err
	}
	if ruleSet.Version == 0 {
		ruleSet.Version = 1
	}
	if ruleSet.Rules == nil {
		ruleSet.Rules = []Rule{}
	}
	return &ruleSet, nil
}

func SaveRuleSet(path string, rules *RuleSet) error {
	if rules == nil {
		return nil
	}
	data, err := json.MarshalIndent(rules, "", "  ")
	if err != nil {
		return err
	}
	return utils.WriteFileAtomic(path, data)
}

func Evaluate(snapshot UserSnapshot, rules *RuleSet) []Achievement {
	if rules == nil {
		return nil
	}
	out := make([]Achievement, 0, len(rules.Rules))
	for _, rule := range rules.Rules {
		if !ruleEnabled(rule.Enabled) {
			continue
		}
		if rule.ID == "" || rule.Name == "" {
			continue
		}
		if !matchConditions(snapshot, rule.Conditions) {
			continue
		}
		out = append(out, Achievement{
			ID:          rule.ID,
			Name:        rule.Name,
			Description: rule.Description,
		})
	}
	return out
}

func matchConditions(snapshot UserSnapshot, c RuleConditions) bool {
	totalActions := snapshot.VandalCount + snapshot.RestoredCount
	maxDailyVandal := maxCount(snapshot.DailyVandalCounts)
	maxDailyRestore := maxCount(snapshot.DailyRestoreCounts)
	activeDays := activeDaysCount(snapshot.DailyVandalCounts, snapshot.DailyRestoreCounts)

	if c.VandalCountGTE != nil && snapshot.VandalCount < *c.VandalCountGTE {
		return false
	}
	if c.RestoredCountGTE != nil && snapshot.RestoredCount < *c.RestoredCountGTE {
		return false
	}
	if c.ActivityScoreGTE != nil && snapshot.ActivityScore < *c.ActivityScoreGTE {
		return false
	}
	if c.ActivityScoreLTE != nil && snapshot.ActivityScore > *c.ActivityScoreLTE {
		return false
	}
	if c.TotalActionsGTE != nil && totalActions < *c.TotalActionsGTE {
		return false
	}
	if c.MaxDailyVandalGTE != nil && maxDailyVandal < *c.MaxDailyVandalGTE {
		return false
	}
	if c.MaxDailyRestoredGTE != nil && maxDailyRestore < *c.MaxDailyRestoredGTE {
		return false
	}
	if c.ActiveDaysGTE != nil && activeDays < *c.ActiveDaysGTE {
		return false
	}
	if c.InactiveDaysGTE != nil {
		if snapshot.LastSeenAt.IsZero() {
			return false
		}
		inactiveDays := int(time.Since(snapshot.LastSeenAt).Hours() / 24)
		if inactiveDays < *c.InactiveDaysGTE {
			return false
		}
	}
	if c.DiscordLinkedRequire != nil {
		linked := snapshot.DiscordID != ""
		if *c.DiscordLinkedRequire != linked {
			return false
		}
	}
	return true
}

func activeDaysCount(vandal map[string]int, restore map[string]int) int {
	seen := make(map[string]struct{})
	for day, count := range vandal {
		if count > 0 {
			seen[day] = struct{}{}
		}
	}
	for day, count := range restore {
		if count > 0 {
			seen[day] = struct{}{}
		}
	}
	return len(seen)
}

func maxCount(m map[string]int) int {
	max := 0
	for _, count := range m {
		if count > max {
			max = count
		}
	}
	return max
}

func ruleEnabled(v *bool) bool {
	if v == nil {
		return true
	}
	return *v
}

func intPtr(v int) *int {
	return &v
}
