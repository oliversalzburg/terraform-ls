package datadir

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	version "github.com/hashicorp/go-version"
	"github.com/hashicorp/terraform-ls/internal/filesystem"
	"github.com/hashicorp/terraform-ls/internal/path"
)

func ModuleManifestFilePath(modulePath string) string {
	return filepath.Join(
		modulePath,
		DirName,
		"modules",
		"modules.json")
}

func IsModuleManifestFile(modulePath, givenPath string) bool {
	manifestPath := ModuleManifestFilePath(modulePath)
	return path.Equals(manifestPath, givenPath)
}

// The following structs were copied from terraform's
// internal/modsdir/manifest.go

// ModuleRecord represents some metadata about an installed module, as part
// of a ModuleManifest.
type ModuleRecord struct {
	// Key is a unique identifier for this particular module, based on its
	// position within the static module tree.
	Key string `json:"Key"`

	// SourceAddr is the source address given for this module in configuration.
	// This is used only to detect if the source was changed in configuration
	// since the module was last installed, which means that the installer
	// must re-install it.
	SourceAddr string `json:"Source"`

	// Version is the exact version of the module, which results from parsing
	// VersionStr. nil for un-versioned modules.
	Version *version.Version `json:"-"`

	// VersionStr is the version specifier string. This is used only for
	// serialization in snapshots and should not be accessed or updated
	// by any other codepaths; use "Version" instead.
	VersionStr string `json:"Version,omitempty"`

	// Dir is the path to the local directory where the module is installed.
	Dir string `json:"Dir"`
}

func (r *ModuleRecord) UnmarshalJSON(b []byte) error {
	type rawRecord ModuleRecord
	var record rawRecord

	err := json.Unmarshal(b, &record)
	if err != nil {
		return err
	}
	if record.VersionStr != "" {
		record.Version, err = version.NewVersion(record.VersionStr)
		if err != nil {
			return fmt.Errorf("invalid version %q for %s: %s", record.VersionStr, record.Key, err)
		}
	}

	// Ensure Windows is using the proper modules path format after
	// reading the modules manifest Dir records
	record.Dir = filepath.FromSlash(record.Dir)

	// Terraform should be persisting clean paths already
	// but it doesn't hurt to clean them for sanity
	record.Dir = filepath.Clean(record.Dir)

	// TODO: Follow symlinks (requires proper test data)

	*r = (ModuleRecord)(record)

	return nil
}

func (r *ModuleRecord) IsRoot() bool {
	return r.Key == ""
}

func (r *ModuleRecord) IsExternal() bool {
	modCacheDir := filepath.Join(".terraform", "modules")
	if strings.HasPrefix(r.Dir, modCacheDir) {
		return true
	}

	return false
}

// ModuleManifest is an internal struct used only to assist in our JSON
// serialization of manifest snapshots. It should not be used for any other
// purpose.
type ModuleManifest struct {
	rootDir string
	Records []ModuleRecord `json:"Modules"`
}

func (mm *ModuleManifest) RootDir() string {
	return mm.rootDir
}

func (mm *ModuleManifest) ReferencesModule(modPath string) bool {
	for _, m := range mm.Records {
		if m.IsRoot() {
			// skip root module, as that's tracked separately
			continue
		}
		if m.IsExternal() {
			// skip external modules as these shouldn't be modified from cache
			continue
		}
		absPath := filepath.Join(mm.RootDir(), m.Dir)
		if path.Equals(absPath, modPath) {
			return true
		}
	}
	return false
}

func ParseInstalledModules(fs filesystem.Filesystem, modulePath string) (*ModuleManifest, error) {
	manifestPath := ModuleManifestFilePath(modulePath)
	return parseModuleManifestFromFile(fs, manifestPath)
}

func parseModuleManifestFromFile(fs filesystem.Filesystem, path string) (*ModuleManifest, error) {
	b, err := fs.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(b) == 0 {
		// mimic Terraform's own behavior by treating empty file
		// as if it was empty JSON object
		b = []byte("{}")
	}

	mm, err := parseModuleManifest(b)
	if err != nil {
		return nil, err
	}

	mm.rootDir = TrimLockFilePath(path)

	return mm, nil
}

func parseModuleManifest(b []byte) (*ModuleManifest, error) {
	mm := ModuleManifest{}
	err := json.Unmarshal(b, &mm)
	if err != nil {
		return nil, err
	}

	if mm.Records == nil {
		mm.Records = make([]ModuleRecord, 0)
	}

	return &mm, nil
}
