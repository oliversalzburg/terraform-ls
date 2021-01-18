package watcher

import (
	"log"

	"github.com/hashicorp/terraform-ls/internal/terraform/datadir"
)

type Watcher interface {
	Start() error
	Stop() error
	SetLogger(*log.Logger)
	AddModule(string, *datadir.DataDir) error
}
