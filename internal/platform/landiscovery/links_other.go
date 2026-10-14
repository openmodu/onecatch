//go:build !android

package landiscovery

func systemLinks() ([]link, error) { return netLinks() }
