package webview2

import (
	"errors"
	"testing"
)

type fakeBrowser struct {
	closes, evals int
	err           error
}

func (*fakeBrowser) Show() error                              { return nil }
func (b *fakeBrowser) Close() error                           { b.closes++; return b.err }
func (b *fakeBrowser) Eval(string)                            { b.evals++ }
func (*fakeBrowser) Embed(uintptr) bool                       { return true }
func (*fakeBrowser) Resize()                                  {}
func (*fakeBrowser) Navigate(string)                          {}
func (*fakeBrowser) NavigateToString(string)                  {}
func (*fakeBrowser) Init(string)                              {}
func (*fakeBrowser) NotifyParentWindowPositionChanged() error { return nil }
func (*fakeBrowser) Focus()                                   {}

func TestCloseDropsLateWork(t *testing.T) {
	b := &fakeBrowser{}
	w := &webview{browser: b, bindings: map[string]interface{}{"call": func() {}}}
	called := 0
	w.Dispatch(func() { called++ })
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	w.Dispatch(func() { called++ })
	w.ProcessDispatch()
	w.Eval("late reply")
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if b.closes != 1 || b.evals != 0 || called != 0 || len(w.bindings) != 0 {
		t.Fatalf("closed view retained work: close=%d eval=%d called=%d", b.closes, b.evals, called)
	}
}

func TestCloseDuringDispatch(t *testing.T) {
	w := &webview{browser: &fakeBrowser{}}
	w.Dispatch(func() {
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
	})
	w.Dispatch(func() { t.Fatal("callback ran after Close") })
	w.ProcessDispatch()
}

func TestFailedCloseCanRetry(t *testing.T) {
	b := &fakeBrowser{err: errors.New("close failed")}
	w := &webview{browser: b}
	if w.Close() == nil || w.isClosed() {
		t.Fatal("failed Close discarded live view")
	}
	b.err = nil
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
}
