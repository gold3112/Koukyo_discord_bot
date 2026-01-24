package eastereggs

import (
	"math/rand"
	"time"
)

var endlessResponses = []string{
	"あ゛っ゛…ちゃかぁい！",
	"ごｌｄ(さん)",
	"元幼女",
	"Placerだぁ！",
}

// HandleEndless returns a random response when cmdName is "endless".
func HandleEndless(cmdName string) (string, bool) {
	if cmdName != "endless" {
		return "", false
	}
	rand.Seed(time.Now().UnixNano())
	return endlessResponses[rand.Intn(len(endlessResponses))], true
}
