//go:build !unix

package config

import (
	"fmt"
	"os"
)

func checkFileOwner(os.FileInfo) error {
	return wrapError(ErrFileAccess, fmt.Errorf("file ownership checks are unsupported on this platform"))
}
