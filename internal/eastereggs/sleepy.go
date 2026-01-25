package eastereggs

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

type sleepyCounter struct {
	mu    sync.Mutex
	count map[string]int
}

var sleepyState = sleepyCounter{
	count: make(map[string]int),
}

func HandleSleepyboard(command string, guildID string, userID string) ([]string, bool) {
	if command != "sleepyboard" && command != "syb" {
		return nil, false
	}
	userKey := fmt.Sprintf("%s:%s", guildID, userID)

	sleepyState.mu.Lock()
	count := sleepyState.count[userKey]
	if count > 0 {
		delete(sleepyState.count, userKey)
	}
	sleepyState.mu.Unlock()

	if count >= 3 {
		return []string{
			"SyB様、我々をお救いなさってください。",
			"あなたはsybによって許されました。",
		}, true
	}
	return []string{"SyB様、我々をお救いなさってください。"}, true
}

func HandleSleepyHeresy(raw string, guildID string, userID string) (string, bool) {
	if !strings.HasPrefix(strings.ToLower(raw), "!sleepyは神ではない") {
		return "", false
	}
	userKey := fmt.Sprintf("%s:%s", guildID, userID)

	sleepyState.mu.Lock()
	count := sleepyState.count[userKey] + 1
	sleepyState.count[userKey] = count
	sleepyState.mu.Unlock()

	switch count {
	case 1:
		return "改心しなさい", true
	case 2:
		return "３度目はありません", true
	case 3:
		return "このbotはSyBによって制限されました。", true
	default:
		return "[検閲済み]", true
	}
}

func init() {
	go func() {
		ticker := time.NewTicker(2 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			sleepyState.mu.Lock()
			sleepyState.count = make(map[string]int)
			sleepyState.mu.Unlock()
		}
	}()
}
