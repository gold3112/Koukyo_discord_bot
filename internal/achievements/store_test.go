package achievements

import "testing"

func TestStoreGetByIdentity(t *testing.T) {
	store := &Store{Users: map[string]*UserAchievements{}}

	store.UpsertUserProfile("discord-1", "discord-user", "100", "wplace-user")
	if !store.AwardByIdentity("discord-1", "100", Achievement{ID: "a1", Name: "A1"}) {
		t.Fatalf("expected award for linked identity")
	}

	linked := store.GetByIdentity("discord-1", "100")
	if linked == nil {
		t.Fatalf("linked user should be found by identity")
	}
	if linked.DiscordID != "discord-1" {
		t.Fatalf("unexpected discord id: got=%s", linked.DiscordID)
	}

	store.UpsertUserProfile("", "", "200", "wplace-only")
	if !store.AwardByIdentity("", "200", Achievement{ID: "b1", Name: "B1"}) {
		t.Fatalf("expected award for wplace-only identity")
	}

	unlinked := store.GetByIdentity("", "200")
	if unlinked == nil {
		t.Fatalf("unlinked user should be found by wplace identity")
	}
	if unlinked.WplaceID != "200" {
		t.Fatalf("unexpected wplace id: got=%s", unlinked.WplaceID)
	}
}

func TestStoreMergesWplaceRecordWhenDiscordIsLinked(t *testing.T) {
	store := &Store{Users: map[string]*UserAchievements{}}

	store.UpsertUserProfile("", "", "300", "wplace-only")
	if !store.AwardByIdentity("", "300", Achievement{ID: "wp-a", Name: "WP"}) {
		t.Fatalf("expected initial wplace award")
	}

	store.UpsertUserProfile("discord-300", "discord-user", "300", "wplace-only")

	user := store.GetByIdentity("discord-300", "300")
	if user == nil {
		t.Fatalf("expected merged user")
	}
	if user.DiscordID != "discord-300" {
		t.Fatalf("expected discord id to be updated: got=%s", user.DiscordID)
	}
	if len(user.Achievements) != 1 || user.Achievements[0].ID != "wp-a" {
		t.Fatalf("expected wplace achievement to be preserved: %+v", user.Achievements)
	}
	if _, ok := store.Users["wplace:300"]; ok {
		t.Fatalf("expected wplace key to be cleaned up after link")
	}
}

func TestStoreMergesLegacySplitRecordsOnAward(t *testing.T) {
	store := &Store{
		Users: map[string]*UserAchievements{
			"discord-400": {
				DiscordID: "discord-400",
				WplaceID:  "400",
				Achievements: []Achievement{
					{ID: "d1", Name: "Discord Side"},
				},
			},
			"wplace:400": {
				WplaceID: "400",
				Achievements: []Achievement{
					{ID: "w1", Name: "Wplace Side"},
				},
			},
		},
	}

	if !store.AwardByIdentity("discord-400", "400", Achievement{ID: "new", Name: "New"}) {
		t.Fatalf("expected new award after merge")
	}

	user := store.GetByIdentity("discord-400", "400")
	if user == nil {
		t.Fatalf("expected merged user")
	}
	if len(user.Achievements) != 3 {
		t.Fatalf("expected merged achievements, got=%d", len(user.Achievements))
	}
	if _, ok := store.Users["wplace:400"]; ok {
		t.Fatalf("expected stale wplace key to be removed")
	}
}

func TestStoreFindsByFieldsWhenKeyIsLegacy(t *testing.T) {
	user := &UserAchievements{
		DiscordID: "discord-500",
		WplaceID:  "500",
	}
	store := &Store{
		Users: map[string]*UserAchievements{
			"legacy-key": user,
		},
	}

	if got := store.GetByDiscordID("discord-500"); got != user {
		t.Fatalf("expected discord lookup to find legacy-key record")
	}
	if got := store.GetByWplaceID("500"); got != user {
		t.Fatalf("expected wplace lookup to find legacy-key record")
	}
	if got := store.GetByIdentity("discord-500", "500"); got != user {
		t.Fatalf("expected identity lookup to find legacy-key record")
	}
}
