package main

import (
	"fmt"
	"muhan/internal/krtext"
)

func main() {
	fmt.Printf("Bob: %s\n", krtext.Particle("Bob", '1'))
	fmt.Printf("Bob(1): %s\n", krtext.Particle("Bob", '1'))
	fmt.Printf("clean: %t\n", krtext.HasFinalConsonant("Bob"))
}
