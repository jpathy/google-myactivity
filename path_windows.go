// +build windows

package myactivity

import "os/exec"

const (
	defaultChromePath = `C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`
)

func findChromePath() string {
	path, err := exec.LookPath(`chrome.exe`)
	if err == nil {
		return path
	}

	return defaultChromePath
}
