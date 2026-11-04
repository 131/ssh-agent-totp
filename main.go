package main

import (
	"log"
	"os"
	"slices"

	"github.com/131/ssh-agent-totp/internal/update"
)

// updatedFlag marks the first start after an update, while the previous
// instance is still closing.
const updatedFlag = "--updated"

func main() {
	if slices.Contains(os.Args[1:], updatedFlag) {
		waitParent()
	}
	update.Cleanup()
	if err := NewRuntime(NewApp()).Run(); err != nil {
		log.Fatal(err)
	}
}
