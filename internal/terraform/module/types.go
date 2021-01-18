package module

import (
	"context"
	"log"

	"github.com/hashicorp/go-version"
	"github.com/hashicorp/hcl-lang/schema"
	"github.com/hashicorp/hcl/v2"
	tfjson "github.com/hashicorp/terraform-json"
	"github.com/hashicorp/terraform-ls/internal/filesystem"
	"github.com/hashicorp/terraform-ls/internal/terraform/datadir"
	"github.com/hashicorp/terraform-ls/internal/terraform/exec"
)

type File interface {
	Path() string
}

type ModuleFinder interface {
	ModuleCandidatesByPath(path string) Modules
	ModuleByPath(path string) (Module, error)
	SchemaForPath(path string) (*schema.BodySchema, error)
}

type ModuleLoader func(dir string) (Module, error)

type ModuleManager interface {
	ModuleFinder

	SetLogger(logger *log.Logger)
	AddModuleAtPath(modPath string) error
	ListModules() Modules
	CancelLoading()
}

type Modules []Module

func (mods Modules) Paths() []string {
	paths := make([]string, len(mods))
	for i, mod := range mods {
		paths[i] = mod.Path()
	}
	return paths
}

type Module interface {
	Path() string
	HumanReadablePath(string) string
	MatchesPath(path string) bool
	HasOpenFiles() bool

	TerraformExecPath() string
	TerraformVersion() *version.Version
	ProviderVersions() map[string]*version.Version

	ProviderSchema() *tfjson.ProviderSchemas

	ParsedFiles() map[string]*hcl.File
	Diagnostics() map[string]hcl.Diagnostics
	ModuleCalls() []datadir.ModuleRecord
}

type ModuleFactory func(filesystem.Filesystem, string) (*module, error)

type ModuleManagerFactory func(filesystem.Filesystem) ModuleManager

type WalkerFactory func() *Walker
