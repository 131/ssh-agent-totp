package main

import (
	"log"
	"os"
	"os/exec"
	"slices"
	"syscall"

	"github.com/131/ssh-agent-totp/internal/desktop"
	"github.com/131/ssh-agent-totp/internal/tray"
	"github.com/131/ssh-agent-totp/internal/update"
)

func openBrowser(link string) error {
	command := exec.Command("rundll32", "url.dll,FileProtocolHandler", link)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return command.Start()
}

// updatedFlag marks the first start after an update, while the previous
// instance is still closing.
const updatedFlag = "--updated"

func main() {
	if desktop.ActivateExisting(slices.Contains(os.Args[1:], updatedFlag), tray.WindowTitle) {
		return
	}
	update.Cleanup()
	runtime, err := NewRuntime(NewApp(openBrowser))
	if err != nil {
		log.Fatal(err)
	}
	runtime.Run()
}
