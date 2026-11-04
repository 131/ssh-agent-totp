package tray

import (
	"golang.org/x/sys/windows"
	"time"
)

var getDoubleClickTime = windows.NewLazySystemDLL("user32.dll").NewProc("GetDoubleClickTime")

func doubleClickDelay() time.Duration {
	milliseconds, _, _ := getDoubleClickTime.Call()
	return time.Duration(milliseconds) * time.Millisecond
}
