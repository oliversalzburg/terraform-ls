package moduleloader

import (
	"container/heap"

	"github.com/hashicorp/terraform-ls/internal/terraform/rootmodule"
)

type ModuleLoader struct {
	ModuleQueue ModuleQueue
}

func NewModuleLoader() *ModuleLoader {
	return &ModuleLoader{
		ModuleQueue: make(ModuleQueue, 0),
	}
}

func (l ModuleLoader) Enqueue(m rootmodule.RootModule) {
	// TODO
	heap.Push(&l.ModuleQueue, m)
}

func (l ModuleLoader) StartLoading() {
	// TODO
	heap.Init(&l.ModuleQueue)
}

func (l ModuleLoader) CancelLoading() {
	// TODO
}
