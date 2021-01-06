package datadir

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/go-version"
	"github.com/hashicorp/terraform-ls/internal/filesystem"
)

func TestParseInstalledModules_notExists(t *testing.T) {
	fs := filesystem.NewFilesystem()

	modPath := filepath.Join(t.TempDir(), "blablah")
	_, err := ParseInstalledModules(fs, modPath)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		t.Fatal(err)
	}
	t.Fatal("expected failure")
}

func TestParseInstalledModules_empty(t *testing.T) {
	fs := filesystem.NewFilesystem()

	modPath := createTmpManifest(t, []byte{})
	manifest, err := ParseInstalledModules(fs, modPath)
	if err != nil {
		t.Fatal(err)
	}
	expectedManifest := &ModuleManifest{
		rootDir: modPath,
		Records: make([]ModuleRecord, 0),
	}

	opts := cmp.AllowUnexported(ModuleManifest{})
	if diff := cmp.Diff(expectedManifest, manifest, opts); diff != "" {
		t.Fatalf("manifest does not match: %s", diff)
	}
}

func TestParseInstalledModules_noModules(t *testing.T) {
	fs := filesystem.NewFilesystem()

	modPath := createTmpManifest(t, []byte(`{}`))
	manifest, err := ParseInstalledModules(fs, modPath)
	if err != nil {
		t.Fatal(err)
	}
	expectedManifest := &ModuleManifest{
		rootDir: modPath,
		Records: make([]ModuleRecord, 0),
	}

	opts := cmp.AllowUnexported(ModuleManifest{})
	if diff := cmp.Diff(expectedManifest, manifest, opts); diff != "" {
		t.Fatalf("manifest does not match: %s", diff)
	}
}

func TestParseInstalledModules_basic(t *testing.T) {
	fs := filesystem.NewFilesystem()

	modPath := createTmpManifest(t, []byte(moduleManifestData))
	manifest, err := ParseInstalledModules(fs, modPath)
	if err != nil {
		t.Fatal(err)
	}
	expectedManifest := &ModuleManifest{
		rootDir: modPath,
		Records: []ModuleRecord{
			{
				Key:        "external_module",
				SourceAddr: "terraform-aws-modules/security-group/aws//modules/http-80",
				Version:    version.Must(version.NewVersion("3.10.0")),
				VersionStr: "3.10.0",
				Dir:        ".terraform/modules/web_server_sg/terraform-aws-security-group-3.10.0/modules/http-80",
			},
			{
				Key:        "external_module_dirty_path",
				SourceAddr: "terraform-aws-modules/security-group/aws//modules/http-80",
				Version:    version.Must(version.NewVersion("3.10.0")),
				VersionStr: "3.10.0",
				Dir:        ".terraform/modules/web_server_sg/terraform-aws-security-group-3.10.0/modules/http-80",
			},
			{
				Key:        "local",
				SourceAddr: "./nested/path",
				Dir:        "nested/path",
			},
			{
				Dir: ".",
			},
		},
	}

	opts := cmp.AllowUnexported(ModuleManifest{})
	if diff := cmp.Diff(expectedManifest, manifest, opts); diff != "" {
		t.Fatalf("manifest does not match: %s", diff)
	}
}

func createTmpManifest(t *testing.T, data []byte) string {
	modPath := filepath.Join(t.TempDir(), t.Name())

	manifestDir := filepath.Join(modPath, ".terraform", "modules")
	err := os.MkdirAll(manifestDir, 0o755)
	if err != nil {
		t.Fatal(err)
	}
	err = ioutil.WriteFile(filepath.Join(manifestDir, "modules.json"), data, 0o755)
	if err != nil {
		t.Fatal(err)
	}

	return modPath
}

const moduleManifestData = `{
	"Modules": [
		{
			"Key": "external_module",
			"Source": "terraform-aws-modules/security-group/aws//modules/http-80",
			"Version": "3.10.0",
			"Dir": ".terraform/modules/web_server_sg/terraform-aws-security-group-3.10.0/modules/http-80"
		},
		{
			"Key": "external_module_dirty_path",
			"Source": "terraform-aws-modules/security-group/aws//modules/http-80",
			"Version": "3.10.0",
			"Dir": ".terraform/modules/web_server_sg/terraform-aws-security-group-3.10.0/modules/something/../http-80"
		},
		{
			"Key": "local",
			"Source": "./nested/path",
			"Dir": "nested/path"
		},
		{
			"Key": "",
			"Source": "",
			"Dir": "."
		}
	]
}`

const moduleManifestRecord_external = `{
    "Key": "web_server_sg",
    "Source": "terraform-aws-modules/security-group/aws//modules/http-80",
    "Version": "3.10.0",
    "Dir": ".terraform/modules/web_server_sg/terraform-aws-security-group-3.10.0/modules/http-80"
}`

const moduleManifestRecord_externalDirtyPath = `{
    "Key": "web_server_sg",
    "Source": "terraform-aws-modules/security-group/aws//modules/http-80",
    "Version": "3.10.0",
    "Dir": ".terraform/modules/web_server_sg/terraform-aws-security-group-3.10.0/modules/something/../http-80"
}`

const moduleManifestRecord_local = `{
    "Key": "local",
    "Source": "./nested/path",
    "Dir": "nested/path"
}`

const moduleManifestRecord_root = `{
    "Key": "",
    "Source": "",
    "Dir": "."
}`
