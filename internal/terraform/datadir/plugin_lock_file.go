package datadir

import (
	"path/filepath"
	"runtime"

	"github.com/hashicorp/terraform-ls/internal/filesystem"
)

var pluginLockFilePathElements = [][]string{
	// Terraform >= 0.14
	{".terraform.lock.hcl"},
	// Terraform >= v0.13
	{DataDirName, "plugins", "selections.json"},
	// Terraform >= v0.12
	{DataDirName, "plugins", runtime.GOOS + "_" + runtime.GOARCH, "lock.json"},
}

func PluginLockFilePath(fs filesystem.Filesystem, modPath string) (string, bool) {
	for _, pathElems := range pluginLockFilePathElements {
		fullPath := filepath.Join(append([]string{modPath}, pathElems...)...)
		_, err := fs.Open(fullPath)
		if err == nil {
			return fullPath, true
		}
	}

	return "", false
}
