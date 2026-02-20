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
