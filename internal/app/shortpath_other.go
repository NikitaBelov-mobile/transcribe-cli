//go:build !windows

package app

import "errors"

func tryWindowsShortPath(path string) (string, error) {
	return "", errors.New("short path conversion is only available on windows")
}
