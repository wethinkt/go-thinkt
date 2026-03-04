package tui

import "github.com/atotto/clipboard"

func copyTextToClipboard(text string) error {
	return clipboard.WriteAll(text)
}
