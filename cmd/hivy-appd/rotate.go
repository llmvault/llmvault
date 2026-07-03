package main

import (
	"fmt"
	"io"
	"os"
)

// rotateIfNeeded size-rotates path when it exceeds maxBytes, keeping `keep`
// generations (path.1 newest .. path.keep oldest). Rotation is
// copy-then-truncate so processes holding the file open (the app, systemd
// append mode, appd's own logger) keep writing to the live file without
// reopening.
func rotateIfNeeded(path string, maxBytes int64, keep int) (bool, error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if info.Size() < maxBytes {
		return false, nil
	}

	// Shift older generations: path.(keep-1) -> path.keep, ...
	for i := keep - 1; i >= 1; i-- {
		src := fmt.Sprintf("%s.%d", path, i)
		if _, err := os.Stat(src); err != nil {
			continue
		}
		if err := os.Rename(src, fmt.Sprintf("%s.%d", path, i+1)); err != nil {
			return false, err
		}
	}
	if err := copyFile(path, path+".1"); err != nil {
		return false, err
	}
	if err := os.Truncate(path, 0); err != nil {
		return false, err
	}
	return true, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
