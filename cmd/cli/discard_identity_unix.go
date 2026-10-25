//go:build darwin || linux

package cli

import (
	"os"
	"syscall"
)

func discardFilesystemIdentity(_ string, info os.FileInfo) (discardFileIdentity, error) {
	stat := info.Sys().(*syscall.Stat_t)
	return discardFileIdentity{device: uint64(stat.Dev), inode: uint64(stat.Ino)}, nil
}
