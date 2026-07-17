//go:build windows

package websession

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const maxClipboardHTMLBytes = 32 * 1024 * 1024

var (
	clipboardFormatOnce sync.Once
	clipboardHTMLFormat uint32
	clipboardFormatErr  error

	user32               = windows.NewLazySystemDLL("user32.dll")
	kernel32             = windows.NewLazySystemDLL("kernel32.dll")
	procOpenClipboard    = user32.NewProc("OpenClipboard")
	procCloseClipboard   = user32.NewProc("CloseClipboard")
	procGetClipboardData = user32.NewProc("GetClipboardData")
	procRegisterFormat   = user32.NewProc("RegisterClipboardFormatW")
	procGlobalLock       = kernel32.NewProc("GlobalLock")
	procGlobalUnlock     = kernel32.NewProc("GlobalUnlock")
	procGlobalSize       = kernel32.NewProc("GlobalSize")
	procRtlMoveMemory    = kernel32.NewProc("RtlMoveMemory")
)

func registerClipboardHTMLFormat() (uint32, error) {
	clipboardFormatOnce.Do(func() {
		name, err := windows.UTF16PtrFromString("HTML Format")
		if err != nil {
			clipboardFormatErr = err
			return
		}
		result, _, callErr := procRegisterFormat.Call(uintptr(unsafe.Pointer(name)))
		if result == 0 {
			clipboardFormatErr = fmt.Errorf("RegisterClipboardFormatW failed: %w", callErr)
			return
		}
		clipboardHTMLFormat = uint32(result)
	})
	return clipboardHTMLFormat, clipboardFormatErr
}

func openClipboardWithRetry(ctx context.Context) error {
	for attempt := 0; attempt < 12; attempt++ {
		result, _, callErr := procOpenClipboard.Call(0)
		if result != 0 {
			return nil
		}
		if attempt == 11 {
			return fmt.Errorf("OpenClipboard failed: %w", callErr)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(15 * time.Millisecond):
		}
	}
	return ErrLocalClipboardUnavailable
}

func readLocalClipboardHTML(ctx context.Context) (string, error) {
	format, err := registerClipboardHTMLFormat()
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrLocalClipboardUnavailable, err)
	}

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := openClipboardWithRetry(ctx); err != nil {
		return "", fmt.Errorf("%w: %v", ErrLocalClipboardUnavailable, err)
	}
	defer procCloseClipboard.Call()

	handle, _, callErr := procGetClipboardData.Call(uintptr(format))
	if handle == 0 {
		return "", fmt.Errorf("%w: clipboard does not contain HTML: %v", ErrLocalClipboardUnavailable, callErr)
	}
	size, _, callErr := procGlobalSize.Call(handle)
	if size == 0 || size > maxClipboardHTMLBytes {
		return "", fmt.Errorf("%w: invalid clipboard HTML size: %v", ErrLocalClipboardUnavailable, callErr)
	}
	pointer, _, callErr := procGlobalLock.Call(handle)
	if pointer == 0 {
		return "", fmt.Errorf("%w: GlobalLock failed: %v", ErrLocalClipboardUnavailable, callErr)
	}
	defer procGlobalUnlock.Call(handle)

	copyOfData := make([]byte, int(size))
	procRtlMoveMemory.Call(uintptr(unsafe.Pointer(&copyOfData[0])), pointer, size)
	runtime.KeepAlive(copyOfData)
	for len(copyOfData) > 0 && copyOfData[len(copyOfData)-1] == 0 {
		copyOfData = copyOfData[:len(copyOfData)-1]
	}
	if len(copyOfData) == 0 {
		return "", fmt.Errorf("%w: clipboard HTML is empty", ErrLocalClipboardUnavailable)
	}
	return string(copyOfData), nil
}
