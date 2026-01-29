package eastereggs

import (
	"math/rand"
	"sync"
	"time"
)

type randomRegistry struct {
	mu       sync.Mutex
	rnd      *rand.Rand
	commands map[string]randomCommand
}

var registry = randomRegistry{
	rnd:      rand.New(rand.NewSource(time.Now().UnixNano())),
	commands: make(map[string]randomCommand),
}

func RegisterRandomCommand(name string, responses []string) {
	RegisterRandomCommandWithChance(name, 100, responses)
}

func RegisterRandomCommandWithChance(name string, chancePercent float64, responses []string) {
	if name == "" || len(responses) == 0 {
		return
	}
	copied := make([]string, 0, len(responses))
	for _, resp := range responses {
		if resp == "" {
			continue
		}
		copied = append(copied, resp)
	}
	if len(copied) == 0 {
		return
	}
	registry.mu.Lock()
	registry.commands[name] = randomCommand{
		responses:     copied,
		chancePercent: chancePercent,
	}
	registry.mu.Unlock()
}

func RegisterWeightedCommand(name string, choices []WeightedReply) {
	RegisterWeightedCommandWithChance(name, 100, choices)
}

func RegisterWeightedCommandWithChance(name string, chancePercent float64, choices []WeightedReply) {
	if name == "" || len(choices) == 0 {
		return
	}
	copied := make([]WeightedReply, 0, len(choices))
	for _, choice := range choices {
		if choice.Reply == "" || choice.WeightPercent <= 0 {
			continue
		}
		copied = append(copied, choice)
	}
	if len(copied) == 0 {
		return
	}
	registry.mu.Lock()
	registry.commands[name] = randomCommand{
		weighted:      copied,
		chancePercent: chancePercent,
	}
	registry.mu.Unlock()
}

func RandomReply(cmdName string) (string, bool) {
	registry.mu.Lock()
	command, ok := registry.commands[cmdName]
	if !ok || (len(command.responses) == 0 && len(command.weighted) == 0) {
		registry.mu.Unlock()
		return "", false
	}
	if !shouldTriggerChanceLocked(command.chancePercent) {
		registry.mu.Unlock()
		return "", false
	}
	var reply string
	if len(command.weighted) > 0 {
		reply, ok = weightedChoiceLocked(command.weighted)
	} else {
		reply = command.responses[registry.rnd.Intn(len(command.responses))]
		ok = true
	}
	registry.mu.Unlock()
	return reply, ok
}

func ShouldTriggerChance(chancePercent float64) bool {
	registry.mu.Lock()
	ok := shouldTriggerChanceLocked(chancePercent)
	registry.mu.Unlock()
	return ok
}

func WeightedChoice(choices []WeightedReply) (string, bool) {
	registry.mu.Lock()
	reply, ok := weightedChoiceLocked(choices)
	registry.mu.Unlock()
	return reply, ok
}

type randomCommand struct {
	responses     []string
	weighted      []WeightedReply
	chancePercent float64
}

type WeightedReply struct {
	Reply         string  `json:"reply"`
	WeightPercent float64 `json:"weight"`
}

func shouldTriggerChanceLocked(chancePercent float64) bool {
	switch {
	case chancePercent >= 100:
		return true
	case chancePercent <= 0:
		return false
	default:
		return registry.rnd.Float64()*100 < chancePercent
	}
}

func weightedChoiceLocked(choices []WeightedReply) (string, bool) {
	total := 0.0
	for _, choice := range choices {
		if choice.Reply == "" || choice.WeightPercent <= 0 {
			continue
		}
		total += choice.WeightPercent
	}
	if total <= 0 {
		return "", false
	}
	target := registry.rnd.Float64() * total
	acc := 0.0
	for _, choice := range choices {
		if choice.Reply == "" || choice.WeightPercent <= 0 {
			continue
		}
		acc += choice.WeightPercent
		if target < acc {
			return choice.Reply, true
		}
	}
	return "", false
}
