// Package tray shows the app's icon in the system tray (the top bar on
// Linux, through StatusNotifierItem; the notification area by the clock
// on Windows), with a small menu, so a desktop app running in the
// background stays visible and can be closed. It does NOT decide what the
// items do: the desktop app and app mode pass their own actions.
package tray

import (
	"runtime"
	"sync"

	"fyne.io/systray"
)

// Item is a menu entry.
type Item struct {
	Label   string
	OnClick func()
}

// Options configures the icon.
type Options struct {
	Tooltip string
	OnOpen  func() // a click on the icon itself
	Items   []Item
}

var once sync.Once

// Start shows the icon until stop is called (once per process). The tray
// runs its own loop on a locked OS thread, beside the app's window loop.
func Start(opts Options) (stop func()) {
	started := false
	once.Do(func() {
		started = true
		go func() { // owned by stop: systray.Quit ends Run
			runtime.LockOSThread()
			systray.Run(func() { setUp(opts) }, nil)
		}()
	})
	if !started {
		return func() {}
	}
	return systray.Quit
}

func setUp(opts Options) {
	systray.SetIcon(icon)
	systray.SetTitle("LeftOff")
	systray.SetTooltip(opts.Tooltip)
	if opts.OnOpen != nil {
		systray.SetOnTapped(opts.OnOpen)
	}
	for _, it := range opts.Items {
		mi := systray.AddMenuItem(it.Label, "")
		go func() { // ends with the tray: ClickedCh is never closed, the process exits
			for range mi.ClickedCh {
				it.OnClick()
			}
		}()
	}
}
