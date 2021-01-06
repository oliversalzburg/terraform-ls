package datadir

import (
	"os"
	"strings"
)

const DirName = ".terraform"

func PathsToWatch(modulePath string) []string {
	paths := []string{
		ModuleManifestFilePath(modulePath),
	}
	paths = append(paths, PluginLockFilePaths(modulePath)...)

	return paths
}

// TrimLockFilePath strips known lock file paths and filenames
// to get the directory path of the relevant rootModule
func TrimLockFilePath(filePath string) string {
	pluginLockFileSuffixes := PluginLockFilePaths(string(os.PathSeparator))
	for _, s := range pluginLockFileSuffixes {
		if strings.HasSuffix(filePath, s) {
			return strings.TrimSuffix(filePath, s)
		}
	}

	moduleManifestSuffix := ModuleManifestFilePath(string(os.PathSeparator))
	if strings.HasSuffix(filePath, moduleManifestSuffix) {
		return strings.TrimSuffix(filePath, moduleManifestSuffix)
	}

	return filePath
}
