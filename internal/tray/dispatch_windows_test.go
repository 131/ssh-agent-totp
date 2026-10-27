package tray

import (
	"sync"
	"testing"
)

func TestDispatchQueue(t *testing.T) {
	tray := &Tray{}
	var sent sync.WaitGroup
	count := 0
	for i := 0; i < 20; i++ {
		sent.Add(1)
		go func() {
			defer sent.Done()
			tray.Dispatch(func() { count++ })
		}()
	}
	sent.Wait()
	tray.runDispatched()
	tray.runDispatched()
	if count != 20 {
		t.Fatalf("callbacks lost or repeated: %d", count)
	}
	tray.Dispatch(func() { tray.Dispatch(func() { count++ }) })
	tray.runDispatched()
	if count != 20 {
		t.Fatal("nested dispatch ran inline")
	}
	tray.runDispatched()
	if count != 21 {
		t.Fatal("nested callback lost")
	}
}
