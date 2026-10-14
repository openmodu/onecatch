package keeldesktop

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/dyike/keel/native/clipboard"
	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/theme"
	uiassets "github.com/openmodu/onecatch/internal/app/keel/assets"
	"github.com/openmodu/onecatch/internal/app/keel/filepicker"
	desktopservice "github.com/openmodu/onecatch/internal/service/desktop"
	_ "golang.org/x/image/bmp"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

const attachmentCount = 8
const attachmentSize = 20 << 20
const attachmentTotal = 50 << 20
const attachmentPixels = 16 << 20

type selectedAttachment struct {
	Path, Name string
	Size       int64
	Staged     bool
	Image      image.Image
}
type attachmentState struct {
	Items            []selectedAttachment
	Images           map[string]*kit.ImageView
	Version          uint64
	Loading, Picking bool
	FocusPrompt      bool
}

func validateAttachmentFiles(paths []string, existing []selectedAttachment) ([]selectedAttachment, error) {
	result := append([]selectedAttachment(nil), existing...)
	total := int64(0)
	seen := map[string]bool{}
	for _, item := range existing {
		seen[item.Path] = true
		total += item.Size
	}
	for _, path := range paths {
		path, err := expandProjectPath(strings.TrimSpace(path))
		if err != nil {
			return nil, err
		}
		path, err = filepath.Abs(path)
		if err != nil {
			return nil, err
		}
		if seen[path] {
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("附件必须是文件：%s", filepath.Base(path))
		}
		if len(result) >= attachmentCount {
			return nil, fmt.Errorf("最多添加 %d 个附件", attachmentCount)
		}
		if info.Size() > attachmentSize {
			return nil, fmt.Errorf("附件不能超过 20 MiB：%s", filepath.Base(path))
		}
		total += info.Size()
		if total > attachmentTotal {
			return nil, fmt.Errorf("附件总大小不能超过 50 MiB")
		}
		result = append(result, selectedAttachment{Path: path, Name: filepath.Base(path), Size: info.Size(), Image: attachmentThumbnail(path)})
		seen[path] = true
	}
	return result, nil
}
func attachmentThumbnail(path string) image.Image {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()
	config, _, err := image.DecodeConfig(io.LimitReader(file, attachmentSize+1))
	if err != nil || config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > attachmentPixels {
		return nil
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return nil
	}
	decoded, _, err := image.Decode(io.LimitReader(file, attachmentSize+1))
	if err != nil {
		return nil
	}
	ratio := min(float64(96)/float64(config.Width), float64(96)/float64(config.Height))
	width, height := max(1, int(float64(config.Width)*ratio)), max(1, int(float64(config.Height)*ratio))
	thumb := image.NewNRGBA(image.Rect(0, 0, width, height))
	draw.CatmullRom.Scale(thumb, thumb.Bounds(), decoded, decoded.Bounds(), draw.Src, nil)
	return thumb
}
func readClipboard(done func(core.ClipboardData, error)) {
	clipboard.Read(func(data clipboard.Data, err error) {
		result := core.ClipboardData{Text: data.Text, Files: data.Files}
		for _, item := range data.Images {
			result.Images = append(result.Images, core.ClipboardImage{MIME: item.MIME, Data: item.Data})
		}
		done(result, err)
	})
}
func normalizedClipboardImage(data core.ClipboardImage) ([]byte, string, error) {
	if len(data.Data) == 0 || len(data.Data) > attachmentSize {
		return nil, "", fmt.Errorf("剪贴板图片过大或为空")
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data.Data))
	if err != nil || config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > attachmentPixels {
		return nil, "", fmt.Errorf("无法读取剪贴板图片，或图片尺寸过大")
	}
	switch data.MIME {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return data.Data, data.MIME, nil
	}
	// AppKit commonly supplies TIFF screenshots; normalize them to the same PNG
	// format already accepted by every existing Agent attachment pipeline.
	img, _, err := image.Decode(bytes.NewReader(data.Data))
	if err != nil {
		return nil, "", err
	}
	var buffer bytes.Buffer
	if err = png.Encode(&buffer, img); err != nil {
		return nil, "", err
	}
	if buffer.Len() > attachmentSize {
		return nil, "", fmt.Errorf("图片转换后不能超过 20 MiB")
	}
	return buffer.Bytes(), "image/png", nil
}
func stageClipboardAttachments(ctx context.Context, service *desktopservice.Service, data core.ClipboardData, existing []selectedAttachment) (result []selectedAttachment, err error) {
	var staged []string
	defer func() {
		if err != nil {
			for _, path := range staged {
				_ = service.DiscardStagedAttachment(path)
			}
		}
	}()
	result, err = validateAttachmentFiles(data.Files, existing)
	if err != nil {
		return nil, err
	}
	for _, item := range data.Images {
		bytes, mime, normalizeErr := normalizedClipboardImage(item)
		if normalizeErr != nil {
			return nil, normalizeErr
		}
		path, stageErr := service.StagePastedImage(ctx, desktopservice.PastedImageInput{MIMEType: mime, DataBase64: base64.StdEncoding.EncodeToString(bytes)})
		if stageErr != nil {
			return nil, stageErr
		}
		staged = append(staged, path)
		result, err = validateAttachmentFiles([]string{path}, result)
		if err != nil {
			return nil, err
		}
		result[len(result)-1].Staged = true
	}
	return result, nil
}
func (a *app) attachmentsEnabled() bool {
	return !a.remoteConversation() && !a.busy && !a.loading && !a.attachments.Loading && !(a.task != nil && a.runID == "")
}
func (a *app) chooseAttachments() {
	if !a.attachmentsEnabled() || a.attachments.Picking {
		return
	}
	a.attachments.Picking = true
	version := a.selection
	filepicker.Choose(filepicker.Options{Title: "选择任务附件", Multiple: true}, func(result filepicker.Result) {
		core.Update(func() {
			if a.closed {
				return
			}
			a.attachments.Picking = false
			if a.selection != version {
				return
			}
			if result.Err != nil {
				a.fail(result.Err)
				return
			}
			if len(result.Paths) > 0 {
				a.addAttachmentPaths(result.Paths)
			}
		})
	})
}
func (a *app) addAttachmentPaths(paths []string) {
	if !a.attachmentsEnabled() {
		return
	}
	selection, version := a.selection, a.attachments.Version+1
	a.attachments.Version = version
	existing := append([]selectedAttachment(nil), a.attachments.Items...)
	a.attachments.Loading = true
	go func() {
		items, err := validateAttachmentFiles(paths, existing)
		core.Update(func() {
			if a.closed || selection != a.selection || version != a.attachments.Version {
				return
			}
			a.attachments.Loading = false
			a.fail(err)
			if err == nil {
				a.attachments.Items = items
			}
		})
	}()
}
func (a *app) pasteTaskAttachments(data core.ClipboardData) bool {
	if len(data.Images) == 0 && len(data.Files) == 0 {
		return false
	}
	if !a.attachmentsEnabled() {
		return true
	}
	selection, version := a.selection, a.attachments.Version+1
	a.attachments.Version = version
	existing := append([]selectedAttachment(nil), a.attachments.Items...)
	a.attachments.Loading = true
	go func() {
		items, err := stageClipboardAttachments(a.ctx, a.backend.service, data, existing)
		if a.ctx.Err() != nil {
			discardNewAttachments(a.backend.service, items, existing)
			return
		}
		applied := make(chan struct{})
		core.Update(func() {
			defer close(applied)
			a.applyPastedAttachments(selection, version, data, items, existing, err)
		})
		select {
		case <-a.ctx.Done():
			discardNewAttachments(a.backend.service, items, existing)
		case <-applied:
		}
	}()
	return true
}

func (a *app) applyPastedAttachments(selection, version uint64, data core.ClipboardData, items, existing []selectedAttachment, err error) {
	if a.closed || selection != a.selection || version != a.attachments.Version {
		discardNewAttachments(a.backend.service, items, existing)
		return
	}
	a.attachments.Loading = false
	a.fail(err)
	if err == nil {
		a.attachments.Items = items
		if data.Text != "" {
			a.prompt += data.Text
		}
		a.attachments.FocusPrompt = true
	}
}
func discardNewAttachments(service *desktopservice.Service, items, existing []selectedAttachment) {
	seen := map[string]bool{}
	for _, item := range existing {
		seen[item.Path] = true
	}
	for _, item := range items {
		if item.Staged && !seen[item.Path] {
			_ = service.DiscardStagedAttachment(item.Path)
		}
	}
}
func (a *app) attachmentPaths() []string {
	paths := make([]string, len(a.attachments.Items))
	for i, item := range a.attachments.Items {
		paths[i] = item.Path
	}
	return paths
}
func (a *app) clearAttachments() {
	a.attachments.Version++
	for _, item := range a.attachments.Items {
		if item.Staged {
			_ = a.backend.service.DiscardStagedAttachment(item.Path)
		}
	}
	a.attachments.Items = nil
	a.attachments.Loading = false
	a.attachments.Images = nil
	a.attachments.FocusPrompt = false
}
func (a *app) removeAttachment(path string) {
	if !a.attachmentsEnabled() {
		return
	}
	for i, item := range a.attachments.Items {
		if item.Path == path {
			if item.Staged {
				_ = a.backend.service.DiscardStagedAttachment(path)
			}
			a.attachments.Items = append(a.attachments.Items[:i], a.attachments.Items[i+1:]...)
			delete(a.attachments.Images, path)
			break
		}
	}
}
func (a *app) attachmentStrip(cx *el.Context) el.Element {
	row := el.Div().Row().Wrap().Gap(6).Px(4)
	for _, item := range a.attachments.Items {
		entry := item
		remove := button("", func() { a.removeAttachment(entry.Path) }, kit.ButtonGhost).ID("remove-attachment/" + item.Path).Name("移除附件 " + item.Name).Size(18).Compact(true).Content(el.ViewFunc(func(cx *el.Context) el.Element { return uiassets.Lucide(cx, "x", 13, theme.Muted) }))
		removeControl := remove.Render(cx)
		if buttonBox, ok := removeControl.(*el.DivEl); ok {
			buttonBox.W(el.Dp(18)).Px(0).Rounded(5)
		}
		if item.Image != nil {
			if a.attachments.Images == nil {
				a.attachments.Images = map[string]*kit.ImageView{}
			}
			view := a.attachments.Images[item.Path]
			if view == nil {
				view = kit.Image(item.Image, item.Name).Size(72, 72).Rounded(10)
				a.attachments.Images[item.Path] = view
			}
			row.Child(el.Div().W(el.Dp(72)).H(el.Dp(72)).Rounded(10).Bg(theme.Subtle).Child(view.Render(cx), el.Div().Absolute().Right(4).Top(4).Child(removeControl)))
		} else {
			row.Child(el.Div().Row().Items(el.Center).Gap(5).Bg(theme.SubtleHover).Rounded(8).Pl(9).Pr(6).Py(4).MaxW(el.Dp(240)).Child(el.Text(item.Name).TextSize(11).TextColor(theme.Muted).MaxLines(1).MaxW(el.Dp(195)), removeControl))
		}
	}
	if a.attachments.Loading {
		row.Child(el.Text("正在添加附件…").TextSize(11).TextColor(theme.Muted))
	}
	return row
}

func (a *app) continuationHarness() string {
	if a.detail != nil && a.task != nil && a.task.WorkflowID != "single_agent" {
		id := a.detail.Run.CurrentStepID
		if len(a.detail.StepRuns) > 0 {
			id = a.detail.StepRuns[len(a.detail.StepRuns)-1].StepID
		}
		for _, step := range a.detail.Workflow.Steps {
			if step.ID == id {
				return string(step.Runtime)
			}
		}
	}
	return a.runtime.Value()
}
