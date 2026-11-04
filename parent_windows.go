package main

import (
	"os"

	"golang.org/x/sys/windows"
)

// waitParent lets the updated executable start once the previous instance,
// its parent, has released the single-instance lock.
func waitParent() {
	process, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(os.Getppid()))
	if err != nil {
		return
	}
	defer windows.CloseHandle(process)
	windows.WaitForSingleObject(process, 15000)
}
