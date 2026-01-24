package eastereggs

import (
	"math/rand"
	"time"
)

var kaniResponses = []string{
	"エッチガニ",
	"毛蟹",
	"どこにも居場所がない蟹",
	"スケベガ二",
}

// Handlekani returns a random response when cmdName is "kani".
func Handlekani(cmdName string) (string, bool) {
	if cmdName != "エッチなカニ4選" {
		return "", false
	}
	rand.Seed(time.Now().UnixNano())
	return kaniResponses[rand.Intn(len(kaniResponses))], true
}
