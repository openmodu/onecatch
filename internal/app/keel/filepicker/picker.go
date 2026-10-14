// Package filepicker supplies native user-initiated file selection. It is kept
// separate from the Go-rendered UI and never links a browser or Wails runtime.
package filepicker

type Options struct {
	Title               string
	Directory, Multiple bool
}
type Result struct {
	Paths []string
	Err   error
}
