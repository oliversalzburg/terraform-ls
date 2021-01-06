package moduleloader

var _ heap.Interface = &ModuleQueue{}

type ModuleQueue []rootmodule.RootModule

func (mq ModuleQueue) Len() int {
	return len(mq)
}

func (mq ModuleQueue) Less(i, j int) bool {
	return mq[i].HasOpenFiles() < mq[j].HasOpenFiles()
}

func (mq ModuleQueue) Swap(i, j int) {
	mq[i], mq[j] = mq[j], mq[i]
}

func (mq *ModuleQueue) Push(x interface{}) {
	m := x.(rootmodule.RootModule)
	*mq = append(*mq, m)
}

func (mq *ModuleQueue) Pop() interface{} {
	old := *mq
	oldLength := len(old)
	module := old[oldLength-1]
	old[oldLength-1] = nil // avoid memory leak
	*mq = old[0 : oldLength-1]
	return module
}
