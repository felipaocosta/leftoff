package main

import (
	"os"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/menu/keys"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// menuTexts are the native menu labels (the web UI has its own i18n).
var menuTexts = map[string]map[string]string{
	"pt-BR": {"app": "LeftOff", "local": "Biblioteca deste computador", "server": "Servidor conectado",
		"reload": "Recarregar", "browser": "Abrir no navegador", "quit": "Sair"},
	"en": {"app": "LeftOff", "local": "This computer’s library", "server": "Connected server",
		"reload": "Reload", "browser": "Open in browser", "quit": "Quit"},
}

// menuLanguage follows the system language: Portuguese or English.
func menuLanguage() string {
	for _, v := range []string{os.Getenv("LC_ALL"), os.Getenv("LC_MESSAGES"), os.Getenv("LANG")} {
		if v != "" {
			if strings.HasPrefix(strings.ToLower(v), "pt") {
				return "pt-BR"
			}
			return "en"
		}
	}
	return "pt-BR"
}

func (w *window) menu(lang, serverURL string) *menu.Menu {
	t := menuTexts[lang]
	m := menu.NewMenu()
	sub := m.AddSubmenu(t["app"])
	sub.AddText(t["local"], keys.CmdOrCtrl("l"), func(*menu.CallbackData) { w.navigate(w.local) })
	if serverURL != "" {
		sub.AddText(t["server"], nil, func(*menu.CallbackData) { w.navigate(serverURL + "/") })
	}
	sub.AddSeparator()
	sub.AddText(t["reload"], keys.CmdOrCtrl("r"), func(*menu.CallbackData) { runtime.WindowReload(w.wails()) })
	sub.AddText(t["browser"], nil, func(*menu.CallbackData) { runtime.BrowserOpenURL(w.wails(), w.local) })
	sub.AddSeparator()
	sub.AddText(t["quit"], keys.CmdOrCtrl("q"), func(*menu.CallbackData) { runtime.Quit(w.wails()) })
	return m
}
