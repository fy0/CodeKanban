package api

import (
	"fmt"
	"net/url"
	"strings"

	"go-template/utils"
)

type wsMessage struct {
	Type string `json:"type"`
	Data string `json:"data,omitempty"`
	Cols int    `json:"cols,omitempty"`
	Rows int    `json:"rows,omitempty"`
}

func buildWSURL(cfg *utils.AppConfig, path string) string {
	if path == "" {
		return ""
	}
	if cfg == nil {
		return path
	}

	domain := strings.TrimSpace(cfg.Domain)
	if domain == "" {
		return path
	}

	if strings.HasPrefix(domain, "http://") ||
		strings.HasPrefix(domain, "https://") ||
		strings.HasPrefix(domain, "ws://") ||
		strings.HasPrefix(domain, "wss://") {
		u, err := url.Parse(domain)
		if err == nil {
			host := u.Host
			if host == "" {
				host = u.Path
			}
			scheme := "ws"
			switch u.Scheme {
			case "https", "wss":
				scheme = "wss"
			case "ws":
				scheme = "ws"
			case "http":
				scheme = "ws"
			}
			return fmt.Sprintf("%s://%s%s", scheme, host, path)
		}
	}

	return fmt.Sprintf("ws://%s%s", domain, path)
}
