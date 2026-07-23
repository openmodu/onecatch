package bindings

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

// NotifyBinding sends a macOS system notification for a standby edge (all tasks
// finished, or a run now needs the user). It is deliberately best-effort: the
// notifier is nil unless the app runs from a signed .app bundle, in which case
// Send is a no-op. The frontend already reflects the same change on the standby
// screen, so a missing notification is never a functional loss.
type NotifyBinding struct {
	notifier *notifications.NotificationService
	authOnce sync.Once
	seq      atomic.Uint64
}

func NewNotifyBinding(notifier *notifications.NotificationService) *NotifyBinding {
	return &NotifyBinding{notifier: notifier}
}

// Send delivers one notification. Authorization is requested lazily on the first
// call so the OS prompt only appears once the feature is actually used.
func (b *NotifyBinding) Send(title, body string) error {
	if b == nil || b.notifier == nil {
		return nil
	}
	b.authOnce.Do(func() { _, _ = b.notifier.RequestNotificationAuthorization() })
	return b.notifier.SendNotification(notifications.NotificationOptions{
		ID:    fmt.Sprintf("oneshot-standby-%d", b.seq.Add(1)),
		Title: title,
		Body:  body,
	})
}
