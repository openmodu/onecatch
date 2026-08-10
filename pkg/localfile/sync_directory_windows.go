//go:build windows

package localfile

// Windows does not support syncing a directory through os.File.Sync. The file
// itself is flushed before every rename, append, or removal that calls this
// helper, so there is no additional portable directory flush to perform here.
func syncDirectory(string) error {
	return nil
}
