package module

import (
	"context"
	"io/ioutil"
	"log"
	"path/filepath"
	"sync"

	"github.com/hashicorp/go-version"
	"github.com/hashicorp/hcl/v2"
	tfjson "github.com/hashicorp/terraform-json"
	"github.com/hashicorp/terraform-ls/internal/filesystem"
	"github.com/hashicorp/terraform-ls/internal/terraform/datadir"
	"github.com/hashicorp/terraform-ls/internal/terraform/exec"
)

type module struct {
	path   string
	fs     filesystem.Filesystem
	logger *log.Logger

	// module manifest
	modManifest      *datadir.ModuleManifest
	modManigestState OpState
	modManifestErr   error
	modManifestMu    *sync.RWMutex

	// provider schema
	providerSchema      *tfjson.ProviderSchemas
	providerSchemaState OpState
	providerSchemaErr   error
	providerSchemaMu    *sync.RWMutex

	// terraform exec path
	tfExecPath      string
	tfExecPathState OpState
	tfExecPathMu    *sync.RWMutex
	tfExecPathErr   error

	// terraform version
	tfVersion      *version.Version
	tfVersionState OpState
	tfVersionMu    *sync.RWMutex
	tfVersionErr   error

	// provider versions
	providerVersions   map[string]*version.Version
	providerVersionsMu *sync.RWMutex

	// config (HCL) parser
	parserState OpState
	parsedFiles map[string]*hcl.File
	parserMu    *sync.RWMutex

	// module diagnostics
	diags   map[string]hcl.Diagnostics
	diagsMu *sync.RWMutex
}

func newModule(fs filesystem.Filesystem, dir string) *module {
	return &module{
		path:   dir,
		fs:     fs,
		logger: defaultLogger,

		modManifestMu:      &sync.RWMutex{},
		providerSchemaMu:   &sync.RWMutex{},
		providerVersions:   make(map[string]*version.Version, 0),
		providerVersionsMu: &sync.RWMutex{},
		tfVersionMu:        &sync.RWMutex{},
		parsedFiles:        make(map[string]*hcl.File, 0),
		parserMu:           &sync.RWMutex{},
		diagsMu:            &sync.RWMutex{},
	}
}

var defaultLogger = log.New(ioutil.Discard, "", 0)

func NewModule(fs filesystem.Filesystem, dir string) Module {
	return newModule(fs, dir)
}

func (m *module) HasOpenFiles() bool {
	openFiles, err := m.fs.HasOpenFiles(m.Path())
	if err != nil {
		m.logger.Printf("%s: failed to check whether module has open files: %s",
			m.Path(), err)
	}
	return openFiles
}

func (m *module) TerraformVersion() *version.Version {
	m.tfVersionMu.RLock()
	defer m.tfVersionMu.RUnlock()
	return m.tfVersion
}

func (m *module) ProviderVersions() map[string]*version.Version {
	m.providerVersionsMu.RLock()
	defer m.providerVersionsMu.RUnlock()
	return m.providerVersions
}

func (m *module) TerraformExecutor(ctx context.Context) exec.TerraformExecutor {
	// TODO: read log path, exec path and timeout from ctx
	return nil
}

func (m *module) TerraformVersionState() OpState {
	m.tfVersionMu.RLock()
	defer m.tfVersionMu.RUnlock()
	return m.tfVersionState
}

func (m *module) SetTerraformVersionState(state OpState) {
	m.tfVersionMu.Lock()
	defer m.tfVersionMu.Unlock()
	m.tfVersionState = state
}

func (m *module) ModuleManifestState() OpState {
	m.modManifestMu.RLock()
	defer m.modManifestMu.RUnlock()
	return m.modManigestState
}

func (m *module) SetModuleManifestParsingState(state OpState) {
	m.modManifestMu.Lock()
	defer m.modManifestMu.Unlock()
	m.modManigestState = state
}

func (m *module) ProviderSchema() *tfjson.ProviderSchemas {
	m.providerSchemaMu.RLock()
	defer m.providerSchemaMu.RUnlock()
	return m.providerSchema
}

func (m *module) ProviderSchemaState() OpState {
	m.providerSchemaMu.RLock()
	defer m.providerSchemaMu.RUnlock()
	return m.providerSchemaState
}

func (m *module) SetProviderSchemaObtainingState(state OpState) {
	m.providerSchemaMu.Lock()
	defer m.providerSchemaMu.Unlock()
	m.providerSchemaState = state
}

func (m *module) ParsedFiles() map[string]*hcl.File {
	m.parserMu.RLock()
	defer m.parserMu.RUnlock()
	return m.parsedFiles
}

func (m *module) SetParsedFiles(files map[string]*hcl.File) {
	m.parserMu.Lock()
	defer m.parserMu.Unlock()
	m.parsedFiles = files
}

func (m *module) SetDiagnostics(diags map[string]hcl.Diagnostics) {
	m.diagsMu.Lock()
	defer m.diagsMu.Unlock()
	m.diags = diags
}

func (m *module) ConfigParsingState() OpState {
	m.parserMu.RLock()
	defer m.parserMu.RUnlock()
	return m.parserState
}

func (m *module) SetConfigParsingState(state OpState) {
	m.parserMu.Lock()
	defer m.parserMu.Unlock()
	m.parserState = state
}

func (m *module) ModuleCalls() []datadir.ModuleRecord {
	m.modManifestMu.RLock()
	defer m.modManifestMu.RUnlock()
	if m.modManifest == nil {
		return []datadir.ModuleRecord{}
	}

	return m.modManifest.Records
}

func (m *module) CallsModule(path string) bool {
	m.modManifestMu.RLock()
	defer m.modManifestMu.RUnlock()
	if m.modManifest == nil {
		return false
	}

	for _, mod := range m.modManifest.Records {
		if mod.IsRoot() {
			// skip root module, as that's tracked separately
			continue
		}
		if mod.IsExternal() {
			// skip external modules as these shouldn't be modified from cache
			continue
		}
		absPath := filepath.Join(m.modManifest.RootDir(), mod.Dir)
		if pathEquals(absPath, path) {
			return true
		}
	}

	return false
}

func (m *module) SetLogger(logger *log.Logger) {
	m.logger = logger
}

func (m *module) Path() string {
	return m.path
}

func (m *module) MatchesPath(path string) bool {
	return pathEquals(m.path, path)
}

// HumanReadablePath helps display shorter, but still relevant paths
func (m *module) HumanReadablePath(rootDir string) string {
	if rootDir == "" {
		return m.path
	}

	// absolute paths can be too long for UI/messages,
	// so we just display relative to root dir
	relDir, err := filepath.Rel(rootDir, m.path)
	if err != nil {
		return m.path
	}

	if relDir == "." {
		// Name of the root dir is more helpful than "."
		return filepath.Base(rootDir)
	}

	return relDir
}

func (m *module) TerraformExecPath() string {
	m.tfExecPathMu.RLock()
	defer m.tfExecPathMu.RUnlock()
	return m.tfExecPath
}

func (m *module) Diagnostics() map[string]hcl.Diagnostics {
	m.diagsMu.RLock()
	defer m.diagsMu.RUnlock()
	return m.diags
}
