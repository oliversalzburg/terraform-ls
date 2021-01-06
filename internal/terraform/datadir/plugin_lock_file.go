package datadir

import (
	"path/filepath"
	"runtime"

	"github.com/hashicorp/terraform-ls/internal/path"
)

func PluginLockFilePaths(modulePath string) []string {
	return []string{
		// Terraform >= 0.14
		filepath.Join(modulePath,
			".terraform.lock.hcl",
		),
		// Terraform >= v0.13
		filepath.Join(modulePath,
			DirName,
			"plugins",
			"selections.json"),
		// Terraform <= v0.12
		filepath.Join(modulePath,
			DirName,
			"plugins",
			runtime.GOOS+"_"+runtime.GOARCH,
			"lock.json"),
	}
}

func IsPluginLockFile(modulePath, givenPath string) bool {
	paths := PluginLockFilePaths(modulePath)
	for _, p := range paths {
		if path.Equals(p, givenPath) {
			return true
		}
	}
	return false
}
