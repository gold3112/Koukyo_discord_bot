package eastereggs

import (
	"math/rand"
	"sync"
	"time"
)

type randomRegistry struct {
	mu       sync.Mutex
	rnd      *rand.Rand
	commands map[string][]string
}

var registry = randomRegistry{
	rnd:      rand.New(rand.NewSource(time.Now().UnixNano())),
	commands: make(map[string][]string),
}

func RegisterRandomCommand(name string, responses []string) {
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
	registry.commands[name] = copied
	registry.mu.Unlock()
}

func RandomReply(cmdName string) (string, bool) {
	registry.mu.Lock()
	responses, ok := registry.commands[cmdName]
	if !ok || len(responses) == 0 {
		registry.mu.Unlock()
		return "", false
	}
	reply := responses[registry.rnd.Intn(len(responses))]
	registry.mu.Unlock()
	return reply, true
}
