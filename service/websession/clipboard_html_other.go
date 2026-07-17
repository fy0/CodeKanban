//go:build !windows

package websession

import "context"

func readLocalClipboardHTML(context.Context) (string, error) {
	return "", ErrLocalClipboardUnavailable
}
