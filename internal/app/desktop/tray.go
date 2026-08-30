package desktop

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	domaintasks "github.com/openmodu/onecatch/internal/domain/tasks"
	domainworkspaces "github.com/openmodu/onecatch/internal/domain/workspaces"
	desktopservice "github.com/openmodu/onecatch/internal/service/desktop"
	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	trayActionEvent      = "onecatch:tray-navigate"
	languageChangedEvent = "onecatch:language-changed"
)

type trayAction struct {
	Action      string `json:"action"`
	TaskID      string `json:"taskId,omitempty"`
	WorkspaceID string `json:"workspaceId,omitempty"`
}

type trayConversation struct {
	TaskID      string
	WorkspaceID string
	Label       string
}

type macTrayController struct {
	service *desktopservice.Service
	tray    *application.SystemTray
	items   []*application.MenuItem
	openItem,
	newItem,
	recentItem,
	quitItem *application.MenuItem
	open    func(trayAction)
	onError func(error)

	mu            sync.RWMutex
	conversations []trayConversation
	language      string
}

func newMacTrayController(app *application.App, service *desktopservice.Service, icon []byte, open func(trayAction), onError func(error)) *macTrayController {
	controller := &macTrayController{service: service, open: open, onError: onError, language: "zh-CN"}
	menu := app.NewMenu()
	controller.openItem = menu.Add("").OnClick(func(*application.Context) {
		controller.openAction(trayAction{Action: "show"})
	})
	controller.newItem = menu.Add("").SetAccelerator("CmdOrCtrl+N").OnClick(func(*application.Context) {
		controller.openAction(trayAction{Action: "new"})
	})
	menu.AddSeparator()
	controller.recentItem = menu.Add("").SetEnabled(false)
	for index := range 3 {
		slot := menu.Add("")
		slotIndex := index
		slot.OnClick(func(*application.Context) {
			controller.openConversation(slotIndex)
		})
		controller.items = append(controller.items, slot)
	}
	menu.AddSeparator()
	controller.quitItem = menu.Add("").OnClick(func(*application.Context) {
		app.Quit()
	})
	controller.applyLanguage()

	controller.tray = app.SystemTray.New()
	if templateIcon, err := macTrayTemplateIcon(icon); err == nil {
		controller.tray.SetTemplateIcon(templateIcon)
	} else {
		controller.reportError(err)
		controller.tray.SetIcon(icon)
	}
	controller.tray.SetMenu(menu)
	controller.refresh()
	return controller
}

type trayLabels struct {
	open, new, recent, empty, quit string
}

func labelsForLanguage(language string) trayLabels {
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(language)), "zh") {
		return trayLabels{open: "Open OneCatch", new: "New Conversation", recent: "Recent Conversations", empty: "No Conversations", quit: "Quit OneCatch"}
	}
	return trayLabels{open: "打开 OneCatch", new: "新建会话", recent: "最近会话", empty: "暂无会话", quit: "退出 OneCatch"}
}

func (c *macTrayController) setLanguage(language string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(language)), "zh") {
		c.language = "zh-CN"
	} else {
		c.language = "en"
	}
	c.mu.Unlock()
	c.applyLanguage()
}

func (c *macTrayController) applyLanguage() {
	if c == nil {
		return
	}
	c.mu.RLock()
	labels := labelsForLanguage(c.language)
	hasConversations := len(c.conversations) > 0
	c.mu.RUnlock()
	c.openItem.SetLabel(labels.open)
	c.newItem.SetLabel(labels.new)
	c.recentItem.SetLabel(labels.recent)
	c.quitItem.SetLabel(labels.quit)
	if !hasConversations && len(c.items) > 0 {
		c.items[0].SetLabel(labels.empty)
	}
}

func (c *macTrayController) openAction(action trayAction) {
	if c != nil && c.open != nil {
		c.open(action)
	}
}

func (c *macTrayController) openConversation(index int) {
	if c == nil {
		return
	}
	c.mu.RLock()
	if index < 0 || index >= len(c.conversations) {
		c.mu.RUnlock()
		return
	}
	conversation := c.conversations[index]
	c.mu.RUnlock()
	c.openAction(trayAction{Action: "open", TaskID: conversation.TaskID, WorkspaceID: conversation.WorkspaceID})
}

func (c *macTrayController) refresh() {
	if c == nil || c.service == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tasks, err := c.service.ListTasks(ctx, "")
	if err != nil {
		c.reportError(err)
		return
	}
	workspaces, err := c.service.ListWorkspaces(ctx)
	if err != nil {
		c.reportError(err)
		return
	}
	conversations := recentTrayConversations(tasks, workspaces, len(c.items))
	c.mu.Lock()
	c.conversations = conversations
	labels := labelsForLanguage(c.language)
	c.mu.Unlock()

	for index, item := range c.items {
		if index < len(conversations) {
			item.SetLabel(conversations[index].Label).SetEnabled(true).SetHidden(false)
			continue
		}
		if index == 0 {
			item.SetLabel(labels.empty).SetEnabled(false).SetHidden(false)
			continue
		}
		item.SetHidden(true)
	}
}

func (c *macTrayController) reportError(err error) {
	if c != nil && c.onError != nil {
		c.onError(err)
	}
}

func recentTrayConversations(tasks []domaintasks.Task, workspaces []domainworkspaces.Workspace, limit int) []trayConversation {
	if limit <= 0 {
		return nil
	}
	workspaceNames := make(map[string]string, len(workspaces))
	for _, workspace := range workspaces {
		workspaceNames[workspace.ID] = workspace.Name
	}
	ordered := make([]domaintasks.Task, 0, len(tasks))
	for _, task := range tasks {
		if _, visible := workspaceNames[task.WorkspaceID]; visible {
			ordered = append(ordered, task)
		}
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].UpdatedAt.Equal(ordered[j].UpdatedAt) {
			return ordered[i].ID < ordered[j].ID
		}
		return ordered[i].UpdatedAt.After(ordered[j].UpdatedAt)
	})
	if len(ordered) > limit {
		ordered = ordered[:limit]
	}
	result := make([]trayConversation, 0, len(ordered))
	for _, task := range ordered {
		result = append(result, trayConversation{
			TaskID:      task.ID,
			WorkspaceID: task.WorkspaceID,
			Label:       trayConversationLabel(task.Title),
		})
	}
	return result
}

func trayConversationLabel(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		title = "未命名会话"
	}
	const maxWidth = 28
	width := 0
	value := []rune(title)
	for index, current := range value {
		currentWidth := trayRuneWidth(current)
		if width+currentWidth > maxWidth {
			return string(value[:index]) + "…"
		}
		width += currentWidth
	}
	return title
}

func trayRuneWidth(value rune) int {
	if unicode.Is(unicode.Han, value) || unicode.Is(unicode.Hangul, value) || unicode.Is(unicode.Hiragana, value) || unicode.Is(unicode.Katakana, value) || value >= 0x1F000 {
		return 2
	}
	return 1
}

// macTrayTemplateIcon removes the opaque app-icon tile and keeps only the
// bright OneCatch mark as alpha. AppKit then supplies white or black according
// to the menu-bar appearance, just like a native template image.
func macTrayTemplateIcon(source []byte) ([]byte, error) {
	input, err := png.Decode(bytes.NewReader(source))
	if err != nil {
		return nil, err
	}
	bounds := input.Bounds()
	mask := image.NewAlpha(bounds)
	logoBounds := image.Rectangle{}
	found := false
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			red, green, blue, sourceAlpha := input.At(x, y).RGBA()
			brightest := max(red, green, blue) >> 8
			var alpha uint32
			switch {
			case brightest <= 55:
				alpha = 0
			case brightest >= 145:
				alpha = sourceAlpha >> 8
			default:
				alpha = (sourceAlpha >> 8) * (brightest - 55) / 90
			}
			mask.SetAlpha(x, y, color.Alpha{A: uint8(alpha)})
			if alpha <= 8 {
				continue
			}
			point := image.Rect(x, y, x+1, y+1)
			if !found {
				logoBounds = point
				found = true
			} else {
				logoBounds = logoBounds.Union(point)
			}
		}
	}
	if !found {
		return nil, errors.New("app icon contains no template mark")
	}
	logoSize := max(logoBounds.Dx(), logoBounds.Dy())
	// Leave a little more breathing room than the app icon crop. macOS renders
	// template images at the menu-bar slot size, so this transparent inset keeps
	// the mark visually aligned with the smaller system status icons.
	padding := max(2, logoSize/7)
	side := min(logoSize+2*padding, bounds.Dx(), bounds.Dy())
	centerX := (logoBounds.Min.X + logoBounds.Max.X) / 2
	centerY := (logoBounds.Min.Y + logoBounds.Max.Y) / 2
	cropX := min(max(centerX-side/2, bounds.Min.X), bounds.Max.X-side)
	cropY := min(max(centerY-side/2, bounds.Min.Y), bounds.Max.Y-side)
	output := image.NewNRGBA(image.Rect(0, 0, side, side))
	for y := range side {
		for x := range side {
			output.SetNRGBA(x, y, color.NRGBA{A: mask.AlphaAt(cropX+x, cropY+y).A})
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, output); err != nil {
		return nil, err
	}
	return encoded.Bytes(), nil
}
