package main

import (
	
	"os"
	"strings"
)

func main() {
	b, _ := os.ReadFile("internal/world/state/state.go")
	s := string(b)
	s = strings.Replace(s, "mu                sync.RWMutex", `mu                sync.RWMutex // Legacy global lock
	roomsMu           sync.RWMutex
	playersMu         sync.RWMutex
	creaturesMu       sync.RWMutex
	objectsMu         sync.RWMutex
	banksMu           sync.RWMutex
	familiesMu        sync.RWMutex
	miscMu            sync.RWMutex`, 1)
	os.WriteFile("internal/world/state/state.go", []byte(s), 0644)
}
