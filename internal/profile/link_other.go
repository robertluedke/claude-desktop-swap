//go:build !windows

package profile

import "os"

func makeLink(target, path string) error {
	return os.Symlink(target, path)
}
