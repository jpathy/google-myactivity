// +build darwin

package myactivity

const (
	defaultChromePath = `/Applications/Google Chrome.app/Contents/MacOS/Google Chrome`
)

func findChromePath() string {
	return defaultChromePath
}
