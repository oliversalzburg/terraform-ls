package path

import (
	"path/filepath"
	"strings"
)

func Equals(path1, path2 string) bool {
	volume1 := filepath.VolumeName(path1)
	volume2 := filepath.VolumeName(path2)
	return strings.EqualFold(volume1, volume2) && path1[len(volume1):] == path2[len(volume2):]
}
