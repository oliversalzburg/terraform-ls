package datadir

import (
	"path/filepath"
	"strings"

	"github.com/hashicorp/terraform-ls/internal/filesystem"
)

const DataDirName = ".terraform"

type DataDir struct {
	ModuleManifestPath string
	PluginLockFilePath string
}

// ModulePath strips known lock file paths to get the path
// to the (closest) module these files belong to
func ModulePath(filePath string) (string, bool) {
	manifestSuffix := filepath.Join(manifestPathElements...)
	if strings.HasSuffix(filePath, manifestSuffix) {
		return strings.TrimSuffix(filePath, manifestSuffix), true
	}

	for _, pathElems := range pluginLockFilePathElements {
		suffix := filepath.Join(pathElems...)
		if strings.HasSuffix(filePath, suffix) {
			return strings.TrimSuffix(filePath, suffix), true
		}
	}

	return "", false
}

func WalkDataDirOfModule(fs filesystem.Filesystem, modPath string) *DataDir {
	dir := &DataDir{}

	path, ok := ModuleManifestFilePath(fs, modPath)
	if ok {
		dir.ModuleManifestPath = path
	}

	path, ok = PluginLockFilePath(fs, modPath)
	if ok {
		dir.PluginLockFilePath = path
	}

	return dir
}
