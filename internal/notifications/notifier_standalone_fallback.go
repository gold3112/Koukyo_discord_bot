package notifications

import (
	"Koukyo_discord_bot/internal/monitor"
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

const (
	standaloneTriggerAfter = 1 * time.Minute
	standaloneBaseInterval = 15 * time.Second
	standaloneMaxInterval  = 5 * time.Minute
)

func (n *Notifier) maybeRunStandaloneFallback(now time.Time) {
	if n == nil || n.monitor == nil || n.watchTargetsState == nil {
		return
	}

	if !n.monitor.IsWSUnavailableFor(standaloneTriggerAfter) {
		n.resetStandaloneSchedule()
		return
	}

	n.standaloneMu.Lock()
	if !n.standaloneNextRun.IsZero() && now.Before(n.standaloneNextRun) {
		n.standaloneMu.Unlock()
		return
	}
	n.standaloneMu.Unlock()

	cfg, err := n.resolveStandaloneTarget()
	if err != nil {
		n.scheduleStandaloneFailure(now)
		log.Printf("standalone fallback: target resolve failed: %v", err)
		return
	}

	result, err := n.buildWatchTargetResult(cfg)
	if err != nil {
		n.scheduleStandaloneFailure(now)
		log.Printf("standalone fallback: build failed target=%s err=%v", cfg.ID, err)
		return
	}

	n.monitor.State.UpdateData(&monitor.MonitorData{
		Type:           "standalone",
		DiffPercentage: result.percent,
		DiffPixels:     result.diffPixels,
		TotalPixels:    result.template.OpaqueCount,
	})
	n.monitor.State.UpdateImages(&monitor.ImageData{
		LiveImage: result.livePNG,
		DiffImage: result.diffPNG,
		Timestamp: now,
	})
	n.scheduleStandaloneSuccess(now)
}

func (n *Notifier) resolveStandaloneTarget() (watchTargetConfig, error) {
	targetID := strings.TrimSpace(os.Getenv("MONITOR_STANDALONE_TARGET_ID"))
	if targetID != "" && n.watchTargetsState != nil {
		targets, err := n.watchTargetsState.loadConfigs()
		if err == nil && len(targets) > 0 {
			for _, t := range targets {
				if targetIDMatches(t, targetID) {
					return t, nil
				}
			}
			return watchTargetConfig{}, fmt.Errorf("MONITOR_STANDALONE_TARGET_ID not found: %s", targetID)
		}
		return watchTargetConfig{}, fmt.Errorf("failed to load watch targets for MONITOR_STANDALONE_TARGET_ID")
	}

	origin := strings.TrimSpace(os.Getenv("MONITOR_STANDALONE_ORIGIN"))
	template := strings.TrimSpace(os.Getenv("MONITOR_STANDALONE_TEMPLATE"))
	if origin == "" {
		origin = "1818-806-989-358"
	}
	if template == "" {
		template = "1818-806-989-358.png"
	}
	return watchTargetConfig{
		ID:       "standalone-default",
		Label:    "Standalone Default",
		Origin:   origin,
		Template: template,
		Interval: standaloneBaseInterval,
	}, nil
}

func (n *Notifier) resetStandaloneSchedule() {
	n.standaloneMu.Lock()
	n.standaloneAttempts = 0
	n.standaloneNextRun = time.Time{}
	n.standaloneMu.Unlock()
}

func (n *Notifier) scheduleStandaloneFailure(now time.Time) {
	n.standaloneMu.Lock()
	attempt := n.standaloneAttempts
	if attempt > 5 {
		attempt = 5
	}
	delay := standaloneBaseInterval * time.Duration(1<<uint(attempt))
	if delay > standaloneMaxInterval {
		delay = standaloneMaxInterval
	}
	n.standaloneNextRun = now.Add(delay)
	if n.standaloneAttempts < 5 {
		n.standaloneAttempts++
	}
	n.standaloneMu.Unlock()
}

func (n *Notifier) scheduleStandaloneSuccess(now time.Time) {
	n.standaloneMu.Lock()
	n.standaloneAttempts = 0
	n.standaloneNextRun = now.Add(standaloneBaseInterval)
	n.standaloneMu.Unlock()
}
