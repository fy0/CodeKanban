package websession

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strings"
	"time"
)

const (
	remoteAttachmentMaxURLLength = 4096
	remoteAttachmentMaxRedirects = 4
	remoteAttachmentTimeout      = 15 * time.Second
)

var carrierGradeNAT = mustParseCIDR("100.64.0.0/10")

func mustParseCIDR(value string) *net.IPNet {
	_, network, err := net.ParseCIDR(value)
	if err != nil {
		panic(err)
	}
	return network
}

func validateRemoteAttachmentURL(rawURL string) (*url.URL, error) {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return nil, fmt.Errorf("remote image URL is required")
	}
	if len(trimmed) > remoteAttachmentMaxURLLength {
		return nil, fmt.Errorf("remote image URL is too long")
	}

	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Hostname() == "" {
		return nil, fmt.Errorf("invalid remote image URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("only HTTP and HTTPS image URLs are supported")
	}
	if parsed.User != nil {
		return nil, fmt.Errorf("remote image URL must not contain credentials")
	}
	if literalIP := net.ParseIP(parsed.Hostname()); literalIP != nil && !isAllowedRemoteAttachmentIP(literalIP) {
		return nil, fmt.Errorf("remote image URL points to a blocked address")
	}
	parsed.Fragment = ""
	return parsed, nil
}

func isAllowedRemoteAttachmentIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	return !ip.IsLoopback() &&
		!ip.IsPrivate() &&
		!ip.IsLinkLocalUnicast() &&
		!ip.IsLinkLocalMulticast() &&
		!ip.IsUnspecified() &&
		!ip.IsMulticast() &&
		!carrierGradeNAT.Contains(ip)
}

func newRemoteAttachmentHTTPClient() *http.Client {
	dialer := &net.Dialer{Timeout: 8 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, fmt.Errorf("invalid remote image address: %w", err)
			}
			addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve remote image host: %w", err)
			}
			for _, address := range addresses {
				if !isAllowedRemoteAttachmentIP(address.IP) {
					continue
				}
				return dialer.DialContext(ctx, network, net.JoinHostPort(address.IP.String(), port))
			}
			return nil, fmt.Errorf("remote image host resolves only to blocked addresses")
		},
		ForceAttemptHTTP2:     true,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   8 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   remoteAttachmentTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= remoteAttachmentMaxRedirects {
				return fmt.Errorf("too many remote image redirects")
			}
			_, err := validateRemoteAttachmentURL(req.URL.String())
			return err
		},
	}
}

func remoteAttachmentMimeType(headerValue string, data []byte) (string, error) {
	declared, _, _ := mime.ParseMediaType(strings.TrimSpace(headerValue))
	detected := http.DetectContentType(data)
	if strings.HasPrefix(strings.ToLower(detected), "image/") {
		return detected, nil
	}
	if strings.EqualFold(declared, "image/svg+xml") {
		trimmed := strings.TrimSpace(string(data))
		if strings.HasPrefix(trimmed, "<svg") ||
			(strings.HasPrefix(trimmed, "<?xml") && strings.Contains(trimmed, "<svg")) {
			return "image/svg+xml", nil
		}
	}
	return "", fmt.Errorf("remote resource is not a supported image")
}

func remoteAttachmentExtension(mimeType string) string {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "image/jpeg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "image/bmp":
		return ".bmp"
	case "image/tiff":
		return ".tiff"
	case "image/svg+xml":
		return ".svg"
	default:
		return ".png"
	}
}

func remoteAttachmentFileName(resp *http.Response, sourceURL *url.URL, mimeType string) string {
	fileName := ""
	if _, params, err := mime.ParseMediaType(resp.Header.Get("Content-Disposition")); err == nil {
		fileName = params["filename"]
	}
	if fileName == "" {
		fileName = path.Base(sourceURL.Path)
	}
	if decoded, err := url.PathUnescape(fileName); err == nil {
		fileName = decoded
	}
	fileName = filepath.Base(strings.TrimSpace(fileName))
	extension := strings.ToLower(filepath.Ext(fileName))
	allowedExtension := map[string]bool{
		".bmp": true, ".gif": true, ".jpeg": true, ".jpg": true, ".png": true,
		".svg": true, ".tif": true, ".tiff": true, ".webp": true,
	}
	if fileName == "" || fileName == "." {
		return "remote-image" + remoteAttachmentExtension(mimeType)
	}
	if !allowedExtension[extension] {
		fileName = strings.TrimSuffix(fileName, filepath.Ext(fileName)) + remoteAttachmentExtension(mimeType)
	}
	return fileName
}

func (m *Manager) ImportRemoteAttachment(ctx context.Context, rawURL string) (Attachment, error) {
	sourceURL, err := validateRemoteAttachmentURL(rawURL)
	if err != nil {
		return Attachment{}, err
	}

	client := m.cfg.RemoteAttachmentClient
	if client == nil {
		client = newRemoteAttachmentHTTPClient()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL.String(), nil)
	if err != nil {
		return Attachment{}, fmt.Errorf("failed to create remote image request: %w", err)
	}
	req.Header.Set("Accept", "image/*")
	req.Header.Set("User-Agent", "CodeKanban/remote-image-import")

	resp, err := client.Do(req)
	if err != nil {
		return Attachment{}, fmt.Errorf("failed to download remote image: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return Attachment{}, fmt.Errorf("remote image returned HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > m.cfg.AttachmentSizeLimit {
		return Attachment{}, fmt.Errorf("attachment too large")
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, m.cfg.AttachmentSizeLimit+1))
	if err != nil {
		return Attachment{}, fmt.Errorf("failed to read remote image: %w", err)
	}
	if int64(len(data)) > m.cfg.AttachmentSizeLimit {
		return Attachment{}, fmt.Errorf("attachment too large")
	}
	mimeType, err := remoteAttachmentMimeType(resp.Header.Get("Content-Type"), data)
	if err != nil {
		return Attachment{}, err
	}
	fileName := remoteAttachmentFileName(resp, sourceURL, mimeType)
	return m.saveAttachmentBytes(fileName, mimeType, data)
}
