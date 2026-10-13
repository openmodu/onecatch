package keeldesktop

import "github.com/dyike/keel/gio/io/key"

// Keep preedit text local until CompositionEvent ends. Sending every EditEvent
// directly to the PTY would print the Latin spelling and then the Chinese result.
type terminalIME struct {
	Text                   []rune
	Selection, Composition key.Range
}

func (i *terminalIME) reset() {
	i.Text = nil
	i.Selection = key.Range{}
	i.Composition = key.Range{Start: -1, End: -1}
}
func (i *terminalIME) composing() bool {
	return i.Composition.Start >= 0 && i.Composition.End >= i.Composition.Start
}
func (i *terminalIME) edit(e key.EditEvent) {
	start := max(0, min(len(i.Text), e.Range.Start))
	end := max(start, min(len(i.Text), e.Range.End))
	value := []rune(e.Text)
	if len(i.Text)-(end-start)+len(value) > 1<<16 {
		return
	}
	next := make([]rune, 0, len(i.Text)-(end-start)+len(value))
	next = append(next, i.Text[:start]...)
	next = append(next, value...)
	next = append(next, i.Text[end:]...)
	i.Text = next
	i.Selection = key.Range{Start: start + len(value), End: start + len(value)}
}
func (i *terminalIME) commit() string {
	if i.composing() {
		return ""
	}
	v := string(i.Text)
	i.reset()
	return v
}
