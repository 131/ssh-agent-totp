package edge

import (
	"reflect"
	"testing"

	"golang.org/x/sys/windows"
)

func TestCloseReleasesOwnedReferences(t *testing.T) {
	var calls []string
	proc := func(name string) ComProc {
		return ComProc(windows.NewCallback(func(this uintptr) uintptr { calls = append(calls, name); return 0 }))
	}
	e := &Chromium{
		controller:  &ICoreWebView2Controller{vtbl: &_ICoreWebView2ControllerVtbl{_IUnknownVtbl: _IUnknownVtbl{Release: proc("controller.release")}, Close: proc("controller.close")}},
		webview:     &ICoreWebView2{vtbl: &iCoreWebView2Vtbl{_IUnknownVtbl: _IUnknownVtbl{Release: proc("webview.release")}}},
		environment: &ICoreWebView2Environment{vtbl: &iCoreWebView2EnvironmentVtbl{_IUnknownVtbl: _IUnknownVtbl{Release: proc("environment.release")}}},
	}
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	want := []string{"controller.close", "webview.release", "controller.release", "environment.release"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("COM calls = %v, want %v", calls, want)
	}
	if e.controller != nil || e.webview != nil || e.environment != nil {
		t.Fatal("COM references retained")
	}
}

func TestCloseFailurePreservesReferences(t *testing.T) {
	released := false
	e := &Chromium{controller: &ICoreWebView2Controller{vtbl: &_ICoreWebView2ControllerVtbl{
		_IUnknownVtbl: _IUnknownVtbl{Release: ComProc(windows.NewCallback(func(this uintptr) uintptr { released = true; return 0 }))},
		Close:         ComProc(windows.NewCallback(func(this uintptr) uintptr { return 0x80004005 })),
	}}}
	if e.Close() == nil || released || e.controller == nil {
		t.Fatal("failed Close released references")
	}
}

func TestInitializationFailureDoesNotDereferenceNullCallbacks(t *testing.T) {
	for _, complete := range []func(*Chromium){
		func(e *Chromium) { e.EnvironmentCompleted(0x80004005, nil) },
		func(e *Chromium) { e.CreateCoreWebView2ControllerCompleted(0x80004005, nil) },
		func(e *Chromium) { e.EnvironmentCompleted(0, nil) },
		func(e *Chromium) { e.CreateCoreWebView2ControllerCompleted(0, nil) },
	} {
		e := &Chromium{}
		complete(e)
		if e.initErr == nil || e.inited == 0 {
			t.Fatal("failed completion reported success or remained pending")
		}
	}
}

func TestCancelledInitializationIgnoresLateCallbacks(t *testing.T) {
	e := &Chromium{}
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	e.EnvironmentCompleted(0, nil)
	e.CreateCoreWebView2ControllerCompleted(0, nil)
	if e.initErr != nil || e.controller != nil || e.environment != nil {
		t.Fatal("cancelled instance accepted late callbacks")
	}
}
