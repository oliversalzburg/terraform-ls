package rootmodule

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/go-multierror"
	"github.com/hashicorp/go-version"
	"github.com/hashicorp/hcl-lang/decoder"
	"github.com/hashicorp/hcl-lang/schema"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	tfjson "github.com/hashicorp/terraform-json"
	"github.com/hashicorp/terraform-ls/internal/filesystem"
	"github.com/hashicorp/terraform-ls/internal/schemas"
	"github.com/hashicorp/terraform-ls/internal/terraform/datadir"
	"github.com/hashicorp/terraform-ls/internal/terraform/discovery"
	"github.com/hashicorp/terraform-ls/internal/terraform/exec"
	tfschema "github.com/hashicorp/terraform-schema/schema"
)

type rootModule struct {
	path   string
	logger *log.Logger

	// loading
	isLoading     bool
	isLoadingMu   *sync.RWMutex
	loadingDone   <-chan struct{}
	cancelLoading context.CancelFunc
	loadErr       error
	loadErrMu     *sync.RWMutex

	// module cache
	moduleMu       *sync.RWMutex
	moduleManifest *datadir.ModuleManifest

	// plugin (provider schema) cache
	pluginMu         *sync.RWMutex
	providerSchema   *tfjson.ProviderSchemas
	providerSchemaMu *sync.RWMutex
	providerVersions map[string]*version.Version

	// terraform executor
	tfLoadingDone bool
	tfLoadingMu   *sync.RWMutex
	tfExec        exec.TerraformExecutor
	tfNewExecutor exec.ExecutorFactory
	tfExecPath    string
	tfExecTimeout time.Duration
	tfExecLogPath string

	// terraform discovery
	tfDiscoFunc  discovery.DiscoveryFunc
	tfDiscoErr   error
	tfVersion    *version.Version
	tfVersionErr error

	// core schema
	coreSchema   *schema.BodySchema
	coreSchemaMu *sync.RWMutex

	// decoder
	isParsed    bool
	isParsedMu  *sync.RWMutex
	pFilesMap   map[string]*hcl.File
	parsedDiags map[string]hcl.Diagnostics
	parserMu    *sync.RWMutex
	filesystem  filesystem.Filesystem
}

func newRootModule(fs filesystem.Filesystem, dir string) *rootModule {
	return &rootModule{
		path:             dir,
		filesystem:       fs,
		logger:           defaultLogger,
		isLoadingMu:      &sync.RWMutex{},
		loadErrMu:        &sync.RWMutex{},
		moduleMu:         &sync.RWMutex{},
		pluginMu:         &sync.RWMutex{},
		providerSchemaMu: &sync.RWMutex{},
		tfLoadingMu:      &sync.RWMutex{},
		coreSchema:       tfschema.UniversalCoreModuleSchema(),
		coreSchemaMu:     &sync.RWMutex{},
		isParsedMu:       &sync.RWMutex{},
		pFilesMap:        make(map[string]*hcl.File, 0),
		providerVersions: make(map[string]*version.Version, 0),
		parserMu:         &sync.RWMutex{},
	}
}

var defaultLogger = log.New(ioutil.Discard, "", 0)

func NewRootModule(ctx context.Context, fs filesystem.Filesystem, dir string) (RootModule, error) {
	rm := newRootModule(fs, dir)

	d := &discovery.Discovery{}
	rm.tfDiscoFunc = d.LookPath

	rm.tfNewExecutor = exec.NewExecutor

	err := rm.parseDataDir()
	if err != nil {
		return rm, err
	}

	return rm, rm.load(ctx)
}

func (rm *rootModule) parseDataDir() error {
	return rm.ParseInstalledModules()
}

func (rm *rootModule) WasInitialized() (bool, error) {
	tfDirPath := filepath.Join(rm.Path(), datadir.DirName)

	f, err := rm.filesystem.Open(tfDirPath)
	if err != nil {
		return false, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return false, err
	}
	if !fi.IsDir() {
		return false, fmt.Errorf("%s is not a directory", tfDirPath)
	}

	return true, nil
}

func (rm *rootModule) SetLogger(logger *log.Logger) {
	rm.logger = logger
}

func (rm *rootModule) StartLoading() error {
	if !rm.IsLoadingDone() {
		return fmt.Errorf("root module is already being loaded")
	}
	ctx, cancelFunc := context.WithCancel(context.Background())
	rm.cancelLoading = cancelFunc
	rm.loadingDone = ctx.Done()

	go func(ctx context.Context) {
		rm.setLoadErr(rm.load(ctx))
	}(ctx)
	return nil
}

func (rm *rootModule) CancelLoading() {
	if !rm.IsLoadingDone() && rm.cancelLoading != nil {
		rm.cancelLoading()
	}
	rm.setLoadingState(false)
}

func (rm *rootModule) LoadingDone() <-chan struct{} {
	return rm.loadingDone
}

func (rm *rootModule) load(ctx context.Context) error {
	var errs *multierror.Error
	defer rm.CancelLoading()

	// reset internal loading state
	rm.setLoadingState(true)

	// The following operations have to happen in a particular order
	// as they depend on the internal state as mutated by each operation

	err := rm.discoverTerraformExecutor(ctx)
	rm.tfDiscoErr = err
	errs = multierror.Append(errs, err)

	err = rm.discoverTerraformVersion(ctx)
	rm.tfVersionErr = err
	errs = multierror.Append(errs, err)

	err = rm.findAndSetCoreSchema()
	if err != nil {
		rm.logger.Printf("%s: %s - falling back to universal schema",
			rm.Path(), err)
	}

	err = rm.UpdateProviderSchemaCache(ctx)
	errs = multierror.Append(errs, err)

	rm.logger.Printf("loading of root module %s finished: %s",
		rm.Path(), errs)
	return errs.ErrorOrNil()
}

func (rm *rootModule) setLoadingState(isLoading bool) {
	rm.isLoadingMu.Lock()
	defer rm.isLoadingMu.Unlock()
	rm.isLoading = isLoading
}

func (rm *rootModule) IsLoadingDone() bool {
	rm.isLoadingMu.RLock()
	defer rm.isLoadingMu.RUnlock()
	return !rm.isLoading
}

func (rm *rootModule) discoverTerraformExecutor(ctx context.Context) error {
	defer func() {
		rm.setTfDiscoveryFinished(true)
	}()

	tfPath := rm.tfExecPath
	if tfPath == "" {
		var err error
		tfPath, err = rm.tfDiscoFunc()
		if err != nil {
			return err
		}
	}

	tf, err := rm.tfNewExecutor(rm.path, tfPath)
	if err != nil {
		return err
	}

	tf.SetLogger(rm.logger)

	if rm.tfExecLogPath != "" {
		tf.SetExecLogPath(rm.tfExecLogPath)
	}

	if rm.tfExecTimeout != 0 {
		tf.SetTimeout(rm.tfExecTimeout)
	}

	rm.tfExec = tf

	return nil
}

func (rm *rootModule) ExecuteTerraformInit(ctx context.Context) error {
	if !rm.IsTerraformAvailable() {
		if err := rm.discoverTerraformExecutor(ctx); err != nil {
			return err
		}
	}

	return rm.tfExec.Init(ctx)
}

func (rm *rootModule) ExecuteTerraformValidate(ctx context.Context) (map[string]hcl.Diagnostics, error) {
	diagsMap := make(map[string]hcl.Diagnostics)

	if !rm.IsTerraformAvailable() {
		if err := rm.discoverTerraformExecutor(ctx); err != nil {
			return diagsMap, err
		}
	}

	if !rm.IsParsed() {
		if err := rm.ParseFiles(); err != nil {
			return diagsMap, err
		}
	}

	// an entry for each file should exist, even if there are no diags
	for filename := range rm.parsedFiles() {
		diagsMap[filename] = make(hcl.Diagnostics, 0)
	}
	// since validation applies to linked modules, create an entry for all
	// files of linked modules
	for _, m := range rm.moduleManifest.Records {
		if m.IsRoot() {
			// skip root modules
			continue
		}
		if m.IsExternal() {
			// skip external modules
			continue
		}

		absPath := filepath.Join(rm.moduleManifest.RootDir(), m.Dir)
		infos, err := rm.filesystem.ReadDir(absPath)
		if err != nil {
			return diagsMap, fmt.Errorf("failed to read module at %q: %w", absPath, err)
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

			// map entries are relative to the parent module path
			filename := filepath.Join(m.Dir, name)

			diagsMap[filename] = make(hcl.Diagnostics, 0)
		}
	}

	validationDiags, err := rm.tfExec.Validate(ctx)
	if err != nil {
		return diagsMap, err
	}

	// tfjson.Diagnostic is a conversion of an internal diag to terraform core,
	// tfdiags, which is effectively based on hcl.Diagnostic.
	// This process is really just converting it back to hcl.Diagnotic
	// since it is the defacto diagnostic type for our codebase currently
	// https://github.com/hashicorp/terraform/blob/ae025248cc0712bf53c675dc2fe77af4276dd5cc/command/validate.go#L138
	for _, d := range validationDiags {
		// the diagnostic must be tied to a file to exist in the map
		if d.Range == nil || d.Range.Filename == "" {
			continue
		}

		diags := diagsMap[d.Range.Filename]

		var severity hcl.DiagnosticSeverity
		if d.Severity == "error" {
			severity = hcl.DiagError
		} else if d.Severity == "warning" {
			severity = hcl.DiagWarning
		}

		diags = append(diags, &hcl.Diagnostic{
			Severity: severity,
			Summary:  d.Summary,
			Detail:   d.Detail,
			Subject: &hcl.Range{
				Filename: d.Range.Filename,
				Start:    hcl.Pos(d.Range.Start),
				End:      hcl.Pos(d.Range.End),
			},
		})
		diagsMap[d.Range.Filename] = diags
	}

	return diagsMap, nil
}

func (rm *rootModule) discoverTerraformVersion(ctx context.Context) error {
	if rm.tfExec == nil {
		return errors.New("no terraform executor - unable to read version")
	}

	version, providerVersions, err := rm.tfExec.Version(ctx)
	if err != nil {
		return err
	}
	rm.logger.Printf("Terraform version %s found at %s for %s", version,
		rm.tfExec.GetExecPath(), rm.Path())
	rm.tfVersion = version

	rm.providerVersions = providerVersions

	return nil
}

func (rm *rootModule) findAndSetCoreSchema() error {
	if rm.tfVersion == nil {
		return errors.New("unable to find core schema without version")
	}

	coreSchema, err := tfschema.CoreModuleSchemaForVersion(rm.tfVersion)
	if err != nil {
		return err
	}

	rm.coreSchemaMu.Lock()
	rm.coreSchema = coreSchema
	rm.coreSchemaMu.Unlock()

	return nil
}

func (rm *rootModule) LoadError() error {
	rm.loadErrMu.RLock()
	defer rm.loadErrMu.RUnlock()
	return rm.loadErr
}

func (rm *rootModule) setLoadErr(err error) {
	rm.loadErrMu.Lock()
	defer rm.loadErrMu.Unlock()
	rm.loadErr = err
}

func (rm *rootModule) Path() string {
	return rm.path
}

func (rm *rootModule) MatchesPath(path string) bool {
	return filepath.Clean(rm.path) == filepath.Clean(path)
}

// HumanReadablePath helps display shorter, but still relevant paths
func (rm *rootModule) HumanReadablePath(rootDir string) string {
	if rootDir == "" {
		return rm.path
	}

	// absolute paths can be too long for UI/messages,
	// so we just display relative to root dir
	relDir, err := filepath.Rel(rootDir, rm.path)
	if err != nil {
		return rm.path
	}

	if relDir == "." {
		// Name of the root dir is more helpful than "."
		return filepath.Base(rootDir)
	}

	return relDir
}

func (rm *rootModule) ParseInstalledModules() error {
	rm.moduleMu.Lock()
	defer rm.moduleMu.Unlock()

	mm, err := datadir.ParseInstalledModules(rm.filesystem, rm.Path())
	if err != nil {
		if os.IsNotExist(err) {
			rm.logger.Printf("no module manifest for %s", rm.Path())
			return nil
		}
		return fmt.Errorf("failed to update module manifest: %w", err)
	}

	rm.moduleManifest = mm
	rm.logger.Printf("updated module manifest - %d references parsed for %s",
		len(mm.Records), rm.Path())
	return nil
}

func (rm *rootModule) DecoderWithSchema(schema *schema.BodySchema) (*decoder.Decoder, error) {
	d, err := rm.Decoder()
	if err != nil {
		return nil, err
	}

	d.SetSchema(schema)

	return d, nil
}

func (rm *rootModule) Decoder() (*decoder.Decoder, error) {
	d := decoder.NewDecoder()

	for name, f := range rm.parsedFiles() {
		err := d.LoadFile(name, f)
		if err != nil {
			return nil, fmt.Errorf("failed to load a file: %w", err)
		}
	}
	return d, nil
}

func (rm *rootModule) IsProviderSchemaLoaded() bool {
	rm.providerSchemaMu.RLock()
	defer rm.providerSchemaMu.RUnlock()
	return rm.providerSchema != nil
}

func (rm *rootModule) IsParsed() bool {
	rm.isParsedMu.RLock()
	defer rm.isParsedMu.RUnlock()
	return rm.isParsed
}

func (rm *rootModule) setIsParsed(parsed bool) {
	rm.isParsedMu.Lock()
	defer rm.isParsedMu.Unlock()
	rm.isParsed = parsed
}

func (rm *rootModule) ParseFiles() error {
	rm.parserMu.Lock()
	defer rm.parserMu.Unlock()

	files := make(map[string]*hcl.File, 0)
	diags := make(map[string]hcl.Diagnostics, 0)

	infos, err := rm.filesystem.ReadDir(rm.Path())
	if err != nil {
		return fmt.Errorf("failed to read module at %q: %w", rm.Path(), err)
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

		fullPath := filepath.Join(rm.Path(), name)

		src, err := rm.filesystem.ReadFile(fullPath)
		if err != nil {
			return fmt.Errorf("failed to read %q: %s", name, err)
		}

		rm.logger.Printf("parsing file %q", name)
		f, pDiags := hclsyntax.ParseConfig(src, name, hcl.InitialPos)
		diags[name] = pDiags
		if f != nil {
			files[name] = f
		}
	}

	rm.pFilesMap = files
	rm.parsedDiags = diags
	rm.setIsParsed(true)

	return nil
}

func (rm *rootModule) ParsedDiagnostics() map[string]hcl.Diagnostics {
	rm.parserMu.Lock()
	defer rm.parserMu.Unlock()
	return rm.parsedDiags
}

func (rm *rootModule) parsedFiles() map[string]*hcl.File {
	rm.parserMu.RLock()
	defer rm.parserMu.RUnlock()

	return rm.pFilesMap
}

func (rm *rootModule) MergedSchema() (*schema.BodySchema, error) {
	rm.coreSchemaMu.RLock()
	defer rm.coreSchemaMu.RUnlock()

	if !rm.IsParsed() {
		err := rm.ParseFiles()
		if err != nil {
			return nil, err
		}
	}

	ps, vOut, err := schemas.PreloadedProviderSchemas()
	if err != nil {
		return nil, err
	}
	providerVersions := vOut.Providers
	tfVersion := vOut.Core

	if rm.IsProviderSchemaLoaded() {
		rm.providerSchemaMu.RLock()
		defer rm.providerSchemaMu.RUnlock()
		ps = rm.providerSchema
		providerVersions = rm.providerVersions
		tfVersion = rm.tfVersion
	}

	if ps == nil {
		rm.logger.Print("provider schemas is nil... skipping merge with core schema")
		return rm.coreSchema, nil
	}

	sm := tfschema.NewSchemaMerger(rm.coreSchema)
	sm.SetCoreVersion(tfVersion)
	sm.SetParsedFiles(rm.parsedFiles())

	err = sm.SetProviderVersions(providerVersions)
	if err != nil {
		return nil, err
	}

	return sm.MergeWithJsonProviderSchemas(ps)
}

// IsIgnoredFile returns true if the given filename (which must not have a
// directory path ahead of it) should be ignored as e.g. an editor swap file.
func IsIgnoredFile(name string) bool {
	return strings.HasPrefix(name, ".") || // Unix-like hidden files
		strings.HasSuffix(name, "~") || // vim
		strings.HasPrefix(name, "#") && strings.HasSuffix(name, "#") // emacs
}

func (rm *rootModule) ReferencesModulePath(path string) bool {
	rm.moduleMu.Lock()
	defer rm.moduleMu.Unlock()

	if rm.moduleManifest == nil {
		return false
	}

	return rm.moduleManifest.ReferencesModule(path)
}

func (rm *rootModule) TerraformFormatter() (exec.Formatter, error) {
	if !rm.HasTerraformDiscoveryFinished() {
		return nil, fmt.Errorf("terraform is not loaded yet")
	}

	if !rm.IsTerraformAvailable() {
		return nil, fmt.Errorf("terraform is not available")
	}

	return rm.tfExec.Format, nil
}

func (rm *rootModule) HasTerraformDiscoveryFinished() bool {
	rm.tfLoadingMu.RLock()
	defer rm.tfLoadingMu.RUnlock()
	return rm.tfLoadingDone
}

func (rm *rootModule) setTfDiscoveryFinished(isLoaded bool) {
	rm.tfLoadingMu.Lock()
	defer rm.tfLoadingMu.Unlock()
	rm.tfLoadingDone = isLoaded
}

func (rm *rootModule) IsTerraformAvailable() bool {
	return rm.HasTerraformDiscoveryFinished() && rm.tfExec != nil
}

func (rm *rootModule) UpdateProviderSchemaCache(ctx context.Context) error {
	rm.pluginMu.Lock()
	defer rm.pluginMu.Unlock()

	if !rm.IsTerraformAvailable() {
		return fmt.Errorf("cannot update provider schema as terraform is unavailable")
	}

	schemas, err := rm.tfExec.ProviderSchemas(ctx)
	if err != nil {
		return err
	}

	rm.providerSchemaMu.Lock()
	rm.providerSchema = schemas
	rm.providerSchemaMu.Unlock()

	return nil
}

func (rm *rootModule) InstalledModules() []datadir.ModuleRecord {
	rm.moduleMu.Lock()
	defer rm.moduleMu.Unlock()
	if rm.moduleManifest == nil {
		return []datadir.ModuleRecord{}
	}

	return rm.moduleManifest.Records
}
