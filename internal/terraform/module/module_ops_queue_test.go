package module

import (
	"container/heap"
	"path/filepath"
	"testing"

	"github.com/hashicorp/terraform-ls/internal/filesystem"
	ilsp "github.com/hashicorp/terraform-ls/internal/lsp"
	"github.com/hashicorp/terraform-ls/internal/protocol"
)

func TestModuleOpsQueue_basic(t *testing.T) {
	mq := newModuleOpsQueue()

	fs := filesystem.NewFilesystem()

	heap.Push(&mq, ModuleOperation{
		Module: closedModAtPath(t, fs, "beta"),
		Type:   OpTypeGetTerraformVersion,
	})
	heap.Push(&mq, ModuleOperation{
		Module: openModAtPath(t, fs, "alpha"),
		Type:   OpTypeGetTerraformVersion,
	})

	nextItem := heap.Pop(&mq)
	nextModOp := nextItem.(ModuleOperation)
	t.Fatalf("next: %q", nextModOp.Module.HumanReadablePath(t.TempDir()))
}

func closedModAtPath(t *testing.T, fs filesystem.Filesystem, name string) Module {
	dh := ilsp.FileHandlerFromDocumentURI(protocol.DocumentURI(t.TempDir()))
	err := fs.CreateDocument(dh, []byte{})
	if err != nil {
		t.Fatal(err)
	}
	return newModule(fs, filepath.Join(t.TempDir(), "alpha"))
}

func openModAtPath(t *testing.T, fs filesystem.Filesystem, name string) Module {
	dh := ilsp.FileHandlerFromDocumentURI(protocol.DocumentURI(t.TempDir()))
	err := fs.CreateAndOpenDocument(dh, []byte{})
	if err != nil {
		t.Fatal(err)
	}
	return newModule(fs, filepath.Join(t.TempDir(), "alpha"))
}
