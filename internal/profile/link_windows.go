//go:build windows

package profile

import (
	"fmt"
	"os/exec"
)

// makeLink creates a directory junction: unlike a symlink it needs neither
// elevation nor Developer Mode.
func makeLink(target, path string) error {
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", path, target).CombinedOutput(); err != nil {
		return fmt.Errorf("%w: %s", err, out)
	}
	return nil
}
