package startup

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

func DefaultStore() *Store {
	return &Store{
		Folder: func() (string, error) {
			return windows.KnownFolderPath(windows.FOLDERID_Startup, windows.KF_FLAG_DONT_VERIFY)
		},
		Executable:     os.Executable,
		CreateShortcut: createShortcut,
	}
}

func quote(value string) string { return "'" + strings.ReplaceAll(value, "'", "''") + "'" }

func createShortcut(path, target string) error {
	script := "$ErrorActionPreference='Stop'; $shell=New-Object -ComObject WScript.Shell; " +
		"$link=$shell.CreateShortcut(" + quote(path) + "); " +
		"$link.TargetPath=" + quote(target) + "; " +
		"$link.WorkingDirectory=" + quote(filepath.Dir(target)) + "; $link.Save()"
	command := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("create startup shortcut: %w: %s", err, strings.TrimSpace(string(output)))
	}

	return nil
}
