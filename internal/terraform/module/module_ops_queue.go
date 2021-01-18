package module

import (
	"container/heap"
	"sync"
)

type moduleOpsQueue struct {
	modOps []ModuleOperation
	mu     *sync.RWMutex
}

func newModuleOpsQueue() moduleOpsQueue {
	q := moduleOpsQueue{
		mu: &sync.RWMutex{},
	}
	heap.Init(&q)
	return q
}

var _ heap.Interface = &moduleOpsQueue{}

func (q *moduleOpsQueue) Len() int {
	q.mu.RLock()
	defer q.mu.RUnlock()

	return len(q.modOps)
}

func (q *moduleOpsQueue) Less(i, j int) bool {
	q.mu.RLock()
	defer q.mu.RUnlock()

	return moduleOperationLess(q.modOps[i], q.modOps[j])
}

func moduleOperationLess(aModOp, bModOp ModuleOperation) bool {
	leftOpen, rightOpen := 0, 0

	if aModOp.Module.HasOpenFiles() {
		leftOpen = 1
	}
	if bModOp.Module.HasOpenFiles() {
		rightOpen = 1
	}

	return leftOpen < rightOpen
}

func (q *moduleOpsQueue) Swap(i, j int) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.modOps[i], q.modOps[j] = q.modOps[j], q.modOps[i]
}

func (q *moduleOpsQueue) Pop() interface{} {
	q.mu.Lock()
	defer q.mu.Unlock()

	n := len(q.modOps)
	item := q.modOps[n-1]
	q.modOps = q.modOps[0 : n-1]
	return item
}

func (q *moduleOpsQueue) Peek() interface{} {
	q.mu.RLock()
	defer q.mu.RUnlock()

	n := len(q.modOps)
	return q.modOps[n-1]
}

func (q *moduleOpsQueue) Push(x interface{}) {
	q.mu.Lock()
	defer q.mu.Unlock()

	modOp := x.(ModuleOperation)
	q.modOps = append(q.modOps, modOp)
}
