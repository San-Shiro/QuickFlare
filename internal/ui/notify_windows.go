//go:build windows

package ui

import (
	"context"
	"encoding/base64"
	"fmt"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

var (
	notifyMu       sync.Mutex
	lastNotifyTime time.Time
	lastNotifyMsg  string
	notifyActive   bool
)

// notify displays a native Windows Action Center toast notification without flashing a console window.
func notify(title, message string) {
	notifyMu.Lock()
	now := time.Now()
	// Suppress rapid flapping: at least 3 seconds between any notifications, and 10s for the exact same message.
	if now.Sub(lastNotifyTime) < 3*time.Second || (message == lastNotifyMsg && now.Sub(lastNotifyTime) < 10*time.Second) {
		notifyMu.Unlock()
		return
	}
	if notifyActive {
		notifyMu.Unlock()
		return
	}
	notifyActive = true
	lastNotifyTime = now
	lastNotifyMsg = message
	notifyMu.Unlock()

	go func() {
		defer func() {
			notifyMu.Lock()
			notifyActive = false
			notifyMu.Unlock()
		}()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		tB64 := base64.StdEncoding.EncodeToString([]byte(title))
		mB64 := base64.StdEncoding.EncodeToString([]byte(message))

		script := fmt.Sprintf(`$t = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String("%s")); $m = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String("%s")); [Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null; $template = [Windows.UI.Notifications.ToastNotificationManager]::GetTemplateContent([Windows.UI.Notifications.ToastTemplateType]::ToastText02); $strings = $template.GetElementsByTagName("text"); $null = $strings.Item(0).AppendChild($template.CreateTextNode($t)); $null = $strings.Item(1).AppendChild($template.CreateTextNode($m)); $toast = [Windows.UI.Notifications.ToastNotification]::new($template); [Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier("{1AC14E77-02E7-4E5D-B744-2EB1AE5198B7}\WindowsPowerShell\v1.0\powershell.exe").Show($toast)`, tB64, mB64)

		cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
		cmd.SysProcAttr = &syscall.SysProcAttr{
			HideWindow:    true,
			CreationFlags: 0x08000000,
		}
		_ = cmd.Run()
	}()
}
