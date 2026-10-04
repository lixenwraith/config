//go:build unix

package config

import (
	"fmt"
	"os"
	"syscall"
)

const openNonBlock = syscall.O_NONBLOCK

func checkFileOwner(info os.FileInfo) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Geteuid()) {
		return wrapError(ErrFileAccess, fmt.Errorf("config file is not owned by current user"))
	}
	return nil
}
