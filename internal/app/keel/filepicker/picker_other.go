//go:build !darwin || !cgo

package filepicker

import "fmt"

func Choose(_ Options, done func(Result)) {
	if done != nil {
		go done(Result{Err: fmt.Errorf("此平台暂不支持系统文件选择，请输入文件路径")})
	}
}
