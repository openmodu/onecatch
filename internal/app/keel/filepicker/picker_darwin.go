//go:build darwin && cgo

package filepicker

/*
#cgo CFLAGS: -x objective-c -fblocks -fobjc-arc
#cgo LDFLAGS: -framework AppKit
#include <stdint.h>
#include <stdlib.h>
void onecatch_choose_files(uintptr_t token, const char *title, int directory, int multiple);
*/
import "C"

import (
	"encoding/json"
	"fmt"
	"runtime/cgo"
	"unsafe"
)

// Choose starts a sheet on the active application window. Cancellation returns
// an empty result. The callback runs once on a background goroutine.
func Choose(options Options, done func(Result)) {
	if done == nil {
		return
	}
	title := C.CString(options.Title)
	defer C.free(unsafe.Pointer(title))
	directory, multiple := C.int(0), C.int(0)
	if options.Directory {
		directory = 1
	}
	if options.Multiple {
		multiple = 1
	}
	C.onecatch_choose_files(C.uintptr_t(cgo.NewHandle(done)), title, directory, multiple)
}

//export onecatch_files_chosen
func onecatch_files_chosen(token C.uintptr_t, raw *C.char, failed C.int) {
	handle := cgo.Handle(token)
	done := handle.Value().(func(Result))
	handle.Delete()
	result := Result{}
	if failed != 0 {
		result.Err = fmt.Errorf("无法打开系统文件选择窗口")
	} else {
		result.Err = json.Unmarshal([]byte(C.GoString(raw)), &result.Paths)
	}
	go done(result)
}
