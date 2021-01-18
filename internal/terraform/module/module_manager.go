package module

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"time"

	"github.com/hashicorp/hcl-lang/schema"
	"github.com/hashicorp/terraform-ls/internal/filesystem"
	"github.com/hashicorp/terraform-ls/internal/terraform/discovery"
	"github.com/hashicorp/terraform-ls/internal/terraform/exec"
)

type moduleManager struct {
	modules    []*module
	newModule  ModuleFactory
	filesystem filesystem.Filesystem

	loader      *moduleLoader
	syncLoading bool
	logger      *log.Logger

	// terraform discovery
	tfDiscoFunc discovery.DiscoveryFunc

	// terraform executor
	tfNewExecutor exec.ExecutorFactory
	tfExecPath    string
	tfExecTimeout time.Duration
	tfExecLogPath string
}

func NewModuleManager(ctx context.Context, fs filesystem.Filesystem) ModuleManager {
	return newModuleManager(fs)
}

func newModuleManager(ctx context.Context, fs filesystem.Filesystem) *moduleManager {
	d := &discovery.Discovery{}

	mm := &moduleManager{
		modules:       make([]*module, 0),
		filesystem:    fs,
		loader:        newModuleLoader(),
		logger:        defaultLogger,
		tfDiscoFunc:   d.LookPath,
		tfNewExecutor: exec.NewExecutor,
	}
	mm.newModule = mm.defaultModuleFactory
	return mm
}

func (mm *moduleManager) defaultModuleFactory(ctx context.Context, dir string) (*module, error) {
	mod := newModule(mm.filesystem, dir)

	mod.SetLogger(mm.logger)

	d := &discovery.Discovery{}
	mod.tfDiscoFunc = d.LookPath
	mod.tfNewExecutor = exec.NewExecutor

	mod.tfExecPath = mm.tfExecPath
	mod.tfExecTimeout = mm.tfExecTimeout
	mod.tfExecLogPath = mm.tfExecLogPath

	return mod, mod.discoverCaches(ctx, dir)
}

func (mm *moduleManager) SetLogger(logger *log.Logger) {
	mm.logger = logger
	mm.loader.SetLogger(logger)
}

func (mm *moduleManager) AddModuleAtPath(modPath string) error {
	modPath = filepath.Clean(modPath)

	// TODO: Follow symlinks (requires proper test data)

	if _, ok := mm.moduleByPath(modPath); ok {
		return fmt.Errorf("module %s was already added", modPath)
	}

	mod, err := mm.newModule(modPath)
	if err != nil {
		return err
	}

	mm.modules = append(mm.modules, mod)

	return nil
}

func (mm *moduleManager) SchemaForPath(path string) (*schema.BodySchema, error) {
	// TODO
	return nil, nil
}

func (mm *moduleManager) moduleByPath(dir string) (*module, bool) {
	for _, mod := range mm.modules {
		if pathEquals(mod.Path(), dir) {
			return mod, true
		}
	}
	return nil, false
}

func (mm *moduleManager) ListModules() Modules {
	modules := make([]Module, 0)
	for _, mod := range mm.modules {
		modules = append(modules, mod)
	}
	return modules
}
func (mm *moduleManager) ModuleByPath(path string) (Module, error) {
	path = filepath.Clean(path)

	if mod, ok := mm.moduleByPath(path); ok {
		return mod, nil
	}

	return nil, &ModuleNotFoundErr{path}
}

func (mm *moduleManager) CancelLoading() {
	// TODO: All loading should be done inside loader, so this shouldn't be necessary
	mm.loader.CancelLoading()
}

// NewModuleLoader allows adding & loading modules
// with a given context. This can be passed down to any handler
// which itself will have short-lived context
// therefore couldn't finish loading the module asynchronously
// after it responds to the client
func NewModuleLoader(ctx context.Context, mm ModuleManager) ModuleLoader {
	return func(dir string) (Module, error) {
		return mm.AddAndStartLoadingModule(ctx, dir)
	}
}
