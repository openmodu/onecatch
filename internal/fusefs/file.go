package fusefs

import (
	"context"
	"errors"
	"io"
	"sync"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/openmodu/oneshot/internal/remotefs"
)

type fileHandle struct {
	remote remotefs.File
	once   sync.Once
}

func (f *fileHandle) Read(
	_ context.Context,
	buffer []byte,
	offset int64,
) (fuse.ReadResult, syscall.Errno) {
	count, err := f.remote.ReadAt(buffer, offset)
	if err != nil && !errorsAreEOF(err, count) {
		return nil, toErrno(err)
	}
	return fuse.ReadResultData(buffer[:count]), 0
}

func (f *fileHandle) Write(
	_ context.Context,
	data []byte,
	offset int64,
) (uint32, syscall.Errno) {
	count, err := f.remote.WriteAt(data, offset)
	if err != nil {
		return uint32(count), toErrno(err)
	}
	return uint32(count), 0
}

func (f *fileHandle) Flush(_ context.Context) syscall.Errno {
	return toErrno(f.remote.Sync())
}

func (f *fileHandle) Fsync(_ context.Context, _ uint32) syscall.Errno {
	return toErrno(f.remote.Sync())
}

func (f *fileHandle) Release(_ context.Context) syscall.Errno {
	var err error
	f.once.Do(func() {
		err = f.remote.Close()
	})
	return toErrno(err)
}

func errorsAreEOF(err error, count int) bool {
	return errors.Is(err, io.EOF) && count >= 0
}
