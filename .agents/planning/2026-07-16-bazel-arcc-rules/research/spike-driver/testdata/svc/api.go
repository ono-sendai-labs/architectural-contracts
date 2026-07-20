// Package svc is a fixture component whose public function mints filesystem
// ambient authority (os.Open takes an ambient path), so Capslock should
// attribute CAPABILITY_FILES to it.
package svc

import "os"

// ReadFirst opens path and returns its first bytes. os.Open is the FILES
// authority-minting site.
func ReadFirst(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	buf := make([]byte, 64)
	n, _ := f.Read(buf)
	return buf[:n], nil
}
