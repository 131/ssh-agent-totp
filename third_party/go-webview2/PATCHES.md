# Local lifecycle patch

Source: `github.com/jchv/go-webview2`, version
`v0.0.0-20260205173254-56598839c808` (MIT; see LICENSE).
The demo is omitted; runtime sources and embedded loader assets are retained.

The app owns a persistent native message loop. This copy adds:

- `WebView.Close()`: explicit controller Close, release WebView/controller/
  environment references, destroy the GUI HWND without posting WM_QUIT,
  remove its context and discard queued binding completions.
- `WebView.ProcessDispatch()`: drain the private binding queue from that loop
  and after native modal loops.
- Balanced COM ownership: GetCoreWebView2 already returns a reference, and
  the settings reference is released after configuration.
- `WindowOptions.Hidden` hides both HWND and controller during initialization.
  Publication shows the parent frame, then explicitly reveals the controller
  through `WebView.Show()`; showing the parent alone leaves a blank surface.
- The environment-creation pump propagates WM_QUIT instead of reporting an
  interrupted initialization as successful.
- Initialization HRESULTs use signed 32-bit decoding. Failed/null completions
  return to the host; cancelled instances ignore late environment/controller
  callbacks instead of dereferencing them or terminating the resident app.
- Closed views reject late dispatch/evaluation and Close is idempotent.

DLLs stay loaded. Closing the last controller lets WebView2 terminate its
browser processes normally; there is no process killing or IPC.

Windows tests in this module cover lifecycle and COM call ordering. The
application's Windows integration test additionally verifies browser-process
exit and recreation.

Run module vet with `-unsafeptr=false`: upstream's native HWND return and
WM_GETMINMAXINFO LPARAM conversion intentionally cross the Win32 ABI as
uintptr/pointer values. Application vet keeps its default analyzers enabled.
