//go:build windows

package ui

import (
	"context"
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
)

// notify displays a native Windows Action Center toast notification without flashing a console window.
func notify(title, message string) {
	notifyMu.Lock()
	now := time.Now()
	if message == lastNotifyMsg && now.Sub(lastNotifyTime) < 5*time.Second {
		notifyMu.Unlock()
		return
	}
	lastNotifyTime = now
	lastNotifyMsg = message
	notifyMu.Unlock()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		script := fmt.Sprintf(`[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null; $template = [Windows.UI.Notifications.ToastNotificationManager]::GetTemplateContent([Windows.UI.Notifications.ToastTemplateType]::ToastText02); $strings = $template.GetElementsByTagName("text"); $null = $strings.Item(0).AppendChild($template.CreateTextNode(%q)); $null = $strings.Item(1).AppendChild($template.CreateTextNode(%q)); $toast = [Windows.UI.Notifications.ToastNotification]::new($template); [Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier("{1AC14E77-02E7-4E5D-B744-2EB1AE5198B7}\WindowsPowerShell\v1.0\powershell.exe").Show($toast)`, title, message)

		cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
		cmd.SysProcAttr = &syscall.SysProcAttr{
			HideWindow:    true,
			CreationFlags: 0x08000000,
		}
		_ = cmd.Run()
	}()
}
