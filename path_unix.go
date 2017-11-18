// +build linux freebsd netbsd openbsd

package myactivity

import "os/exec"

const defaultChromePath = "/usr/bin/google-chrome"

// taken from https://github.com/knq/chromedp/blob/master/runner/path_unix.go
// chromeNames are the Chrome executable names to search for in the path.
var chromeNames = []string{
	"google-chrome",
	"google-chrome-stable",
	"chromium-browser",
	"chromium",
	"google-chrome-beta",
	"google-chrome-unstable",
}

func findChromePath() string {
	for _, p := range chromeNames {
		path, err := exec.LookPath(p)
		if err == nil {
			return path
		}
	}

	return defaultChromePath
}
