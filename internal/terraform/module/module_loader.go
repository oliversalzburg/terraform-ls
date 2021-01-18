package module

import (
	"container/heap"
	"context"
	"log"
	"runtime"
	"sync/atomic"

	"github.com/hashicorp/terraform-ls/internal/terraform/exec"
)

type moduleLoader struct {
	queue              moduleOpsQueue
	nonPrioParallelism int64
	prioParallelism    int64
	logger             *log.Logger
	tfExecOpts         *exec.ExecutorOpts

	loadingCount     int64
	prioLoadingCount int64
}

func newModuleLoader(ctx context.Context) *moduleLoader {
	nonPrioParallelism := 2 * runtime.NumCPU()
	prioParallelism := 1 * runtime.NumCPU()

	ml := &moduleLoader{
		queue:              newModuleOpsQueue(),
		logger:             defaultLogger,
		nonPrioParallelism: int64(nonPrioParallelism),
		prioParallelism:    int64(prioParallelism),
	}
	// TODO: new cancellable child context
	ml.start(ctx)
}

func (ml *moduleLoader) SetLogger(logger *log.Logger) {
	ml.logger = logger
}

func (ml *moduleLoader) start(ctx context.Context) {
	for {
		nonPrioCapacity := ml.nonPrioParallelism - ml.loadingCount
		prioCapacity := ml.prioParallelism - ml.prioLoadingCount
		totalCapacity := nonPrioCapacity + prioCapacity

		// Keep scheduling work from queue if we have capacity
		if ml.queue.Len() > 0 && totalCapacity > 0 {
			item := ml.queue.Peek()
			nextModOp := item.(ModuleOperation)

			if nextModOp.Module.HasOpenFiles() && prioCapacity > 0 {
				atomic.AddInt64(&ml.prioLoadingCount, 1)
				item := heap.Pop(&ml.queue)
				mod := item.(ModuleOperation)
				go func(ml *moduleLoader) {
					defer atomic.AddInt64(&ml.prioLoadingCount, -1)
					ml.executeModuleOp(ctx, mod)
				}(ml)
			} else if nonPrioCapacity > 0 {
				atomic.AddInt64(&ml.loadingCount, 1)
				item := heap.Pop(&ml.queue)
				mod := item.(ModuleOperation)
				go func(ml *moduleLoader) {
					defer atomic.AddInt64(&ml.loadingCount, -1)
					ml.executeModuleOp(ctx, mod)
				}(ml)
			}
		}
	}
}

func (ml *moduleLoader) executeModuleOp(ctx context.Context, modOp ModuleOperation) {
	// TODO: Report progress for each operation

	switch modOp.Type {
	case OpTypeGetTerraformVersion:
		GetTerraformVersion(ctx, modOp.Module)
		return
	case OpTypeObtainSchema:
		ObtainSchema(ctx, modOp.Module)
		return
	case OpTypeParseConfiguration:
		ParseConfiguration(modOp.Module)
		return
	case OpTypeParseModuleManifest:
		ParseModuleManifest(modOp.Module)
		return
	}

	ml.logger.Printf("%s: unknown operation (%#v) for module operation", modOp.Module.Path())
}

// TODO: Allow queueing individual stages separately
// e.g. just TF version, just schema update, module manifest update etc.
func (ml *moduleLoader) EnqueueModuleOp(modOp ModuleOperation) {
	m := modOp.Module
	mod := m.(*module)

	switch modOp.Type {
	case OpTypeGetTerraformVersion:
		if mod.TerraformVersionState() == OpStateQueued {
			// avoid enqueuing duplicate operation
			return
		}
		mod.SetTerraformVersionState(OpStateQueued)
		return
	case OpTypeObtainSchema:
		if mod.ProviderSchemaState() == OpStateQueued {
			// avoid enqueuing duplicate operation
			return
		}
		mod.SetProviderSchemaObtainingState(OpStateQueued)
		return
	case OpTypeParseConfiguration:
		if mod.ConfigParsingState() == OpStateQueued {
			// avoid enqueuing duplicate operation
			return
		}
		mod.SetConfigParsingState(OpStateQueued)
		return
	case OpTypeParseModuleManifest:
		if mod.ModuleManifestState() == OpStateQueued {
			// avoid enqueuing duplicate operation
			return
		}
		mod.SetModuleManifestParsingState(OpStateQueued)
		return
	}

	heap.Push(&ml.queue, modOp)
}

func (ml *moduleLoader) CancelLoading() {
	// TODO
}
