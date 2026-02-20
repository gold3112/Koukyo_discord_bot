package achievements

import (
	"testing"
	"time"
)

func TestEvaluateRuleConditions(t *testing.T) {
	rules := &RuleSet{
		Version: 1,
		Rules: []Rule{
			{
				ID:      "restore_10",
				Name:    "Restore 10",
				Enabled: boolPtr(true),
				Conditions: RuleConditions{
					RestoredCountGTE: intPtr(10),
				},
			},
			{
				ID:      "score_plus",
				Name:    "Score +20",
				Enabled: boolPtr(true),
				Conditions: RuleConditions{
					ActivityScoreGTE: intPtr(20),
				},
			},
			{
				ID:      "disabled_rule",
				Name:    "Disabled",
				Enabled: boolPtr(false),
				Conditions: RuleConditions{
					TotalActionsGTE: intPtr(1),
				},
			},
		},
	}

	snapshot := UserSnapshot{
		DiscordID:          "123",
		RestoredCount:      15,
		VandalCount:        3,
		ActivityScore:      12,
		DailyVandalCounts:  map[string]int{"2026-02-20": 3},
		DailyRestoreCounts: map[string]int{"2026-02-20": 15},
	}

	awards := Evaluate(snapshot, rules)
	if len(awards) != 1 {
		t.Fatalf("unexpected awards length: got=%d want=1", len(awards))
	}
	if awards[0].ID != "restore_10" {
		t.Fatalf("unexpected award id: got=%s want=restore_10", awards[0].ID)
	}
}

func TestEvaluateActiveDaysAndDailyMax(t *testing.T) {
	rules := &RuleSet{
		Version: 1,
		Rules: []Rule{
			{
				ID:   "daily_vandal",
				Name: "Daily Vandal",
				Conditions: RuleConditions{
					MaxDailyVandalGTE: intPtr(5),
				},
			},
			{
				ID:   "active_3_days",
				Name: "Active 3 Days",
				Conditions: RuleConditions{
					ActiveDaysGTE: intPtr(3),
				},
			},
		},
	}
	snapshot := UserSnapshot{
		DiscordID: "abc",
		DailyVandalCounts: map[string]int{
			"2026-02-18": 2,
			"2026-02-19": 5,
		},
		DailyRestoreCounts: map[string]int{
			"2026-02-20": 1,
		},
	}

	awards := Evaluate(snapshot, rules)
	if len(awards) != 2 {
		t.Fatalf("unexpected awards length: got=%d want=2", len(awards))
	}
}

func TestEvaluateInactiveDaysForVandal(t *testing.T) {
	rules := &RuleSet{
		Version: 1,
		Rules: []Rule{
			{
				ID:   "three_day_dropout_vandal",
				Name: "3日坊主(荒らし)",
				Conditions: RuleConditions{
					VandalCountGTE:   intPtr(1),
					ActivityScoreLTE: intPtr(-1),
					InactiveDaysGTE:  intPtr(4),
				},
			},
		},
	}

	snapshot := UserSnapshot{
		DiscordID:     "abc",
		VandalCount:   10,
		RestoredCount: 1,
		ActivityScore: -9,
		LastSeenAt:    time.Now().Add(-4 * 24 * time.Hour).Add(-1 * time.Minute),
	}
	awards := Evaluate(snapshot, rules)
	if len(awards) != 1 {
		t.Fatalf("unexpected awards length: got=%d want=1", len(awards))
	}

	// Recent activity should fail the inactivity condition.
	snapshot.LastSeenAt = time.Now().Add(-2 * 24 * time.Hour)
	awards = Evaluate(snapshot, rules)
	if len(awards) != 0 {
		t.Fatalf("unexpected awards length: got=%d want=0", len(awards))
	}
}

func boolPtr(v bool) *bool {
	return &v
}
