package module

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/terraform-ls/internal/terraform/datadir"
)

type OpState uint

const (
	OpStateUnknown OpState = iota
	OpStateQueued
	OpStateLoading
	OpStateLoaded
)

type OpType uint

const (
	OpTypeUnknown OpType = iota
	OpTypeGetTerraformVersion
	OpTypeObtainSchema
	OpTypeParseConfiguration
	OpTypeParseModuleManifest
)

type ModuleOperation struct {
	Module Module
	Type   OpType
}

func GetTerraformVersion(mod Module) error {
	// TODO
	return nil
}

func ObtainSchema(mod Module) error {
	// TODO
	return nil
}

func ParseConfiguration(mod Module) error {
	m := mod.(*module)
	m.SetConfigParsingState(OpStateLoading)
	defer m.SetConfigParsingState(OpStateLoaded)

	files := make(map[string]*hcl.File, 0)
	diags := make(map[string]hcl.Diagnostics, 0)

	infos, err := m.fs.ReadDir(m.Path())
	if err != nil {
		return fmt.Errorf("failed to read module at %q: %w", m.Path(), err)
	}

	for _, info := range infos {
		if info.IsDir() {
			// We only care about files
			continue
		}

		name := info.Name()
		if !strings.HasSuffix(name, ".tf") || IsIgnoredFile(name) {
			continue
		}

		// TODO: overrides

		fullPath := filepath.Join(m.Path(), name)

		src, err := m.fs.ReadFile(fullPath)
		if err != nil {
			return fmt.Errorf("failed to read %q: %s", name, err)
		}

		f, pDiags := hclsyntax.ParseConfig(src, name, hcl.InitialPos)
		diags[name] = pDiags
		if f != nil {
			files[name] = f
		}
	}

	m.SetParsedFiles(files)
	m.SetDiagnostics(diags)
	return nil
}

// IsIgnoredFile returns true if the given filename (which must not have a
// directory path ahead of it) should be ignored as e.g. an editor swap file.
func IsIgnoredFile(name string) bool {
	return strings.HasPrefix(name, ".") || // Unix-like hidden files
		strings.HasSuffix(name, "~") || // vim
		strings.HasPrefix(name, "#") && strings.HasSuffix(name, "#") // emacs
}

func ParseModuleManifest(mod Module) error {
	m := mod.(*module)
	m.SetModuleManifestParsingState(OpStateLoading)
	defer m.SetModuleManifestParsingState(OpStateLoaded)

	mm, err := datadir.ParseModuleManifestFromFile()
	if err != nil {
		return fmt.Errorf("failed to update module manifest: %w", err)
	}

	m.moduleManifest = mm
	m.logger.Printf("updated module manifest - %d references parsed for %s",
		len(mm.Records), m.Path())
	return nil
}
