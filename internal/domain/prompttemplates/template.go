// Package prompttemplates defines reusable prompts independently of their UI
// and persistence format.
package prompttemplates

import "errors"

var ErrConflict = errors.New("模板库已被其他窗口修改，请重新打开后保存")

type Template struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Body    string `json:"body"`
	Builtin bool   `json:"builtin,omitempty"`
}

// Revision is an opaque persistence token. Callers return it unchanged on save.
type Library struct {
	Templates []Template
	Revision  string
}
