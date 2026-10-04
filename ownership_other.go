//go:build !unix

package config

import (
	"fmt"
	"os"
)

const openNonBlock = 0

func checkFileOwner(os.FileInfo) error {
	return wrapError(ErrFileAccess, fmt.Errorf("file ownership checks are unsupported on this platform"))
}
