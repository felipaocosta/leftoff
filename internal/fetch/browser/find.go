//go:build rod

package browser

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// flatpakBrowsers are Chromium-family Flatpak apps, in order of preference.
var flatpakBrowsers = []string{"com.google.Chrome", "org.chromium.Chromium", "com.microsoft.Edge", "com.brave.Browser"}

// binary is a browser to launch and the profile directory it can write.
type binary struct {
	path    string
	profile string
}

// findFlatpak returns a wrapper script that runs an installed Flatpak
// browser. Flatpak sandboxes usually cannot write outside the app's own
// directory, so the profile lives under ~/.var/app/<id>/.
func findFlatpak(ctx context.Context, wrapperDir string) (binary, error) {
	if runtime.GOOS != "linux" {
		return binary{}, fmt.Errorf("flatpak is linux-only")
	}
	if _, err := exec.LookPath("flatpak"); err != nil {
		return binary{}, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return binary{}, err
	}
	for _, id := range flatpakBrowsers {
		if exec.CommandContext(ctx, "flatpak", "info", id).Run() != nil {
			continue
		}
		if err := os.MkdirAll(wrapperDir, 0o755); err != nil {
			return binary{}, err
		}
		wrapper := filepath.Join(wrapperDir, "flatpak-"+id+".sh")
		script := "#!/bin/sh\nexec flatpak run --command=" + flatpakCommand(id) + " " + id + " \"$@\"\n"
		if err := os.WriteFile(wrapper, []byte(script), 0o755); err != nil {
			return binary{}, err
		}
		return binary{path: wrapper, profile: filepath.Join(home, ".var", "app", id, "data", "leftoff-profile")}, nil
	}
	return binary{}, fmt.Errorf("no Chromium-family Flatpak browser installed")
}

// flatpakCommand is the browser executable inside each Flatpak.
func flatpakCommand(id string) string {
	switch id {
	case "com.google.Chrome":
		return "/app/extra/chrome"
	case "com.brave.Browser":
		return "brave"
	case "com.microsoft.Edge":
		return "/app/extra/msedge"
	default:
		return "chromium"
	}
}
