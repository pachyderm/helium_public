package gcp_vm_replicated

import (
	"github.com/pachyderm/helium/api"
	"github.com/pachyderm/helium/backend"
)

const BackendName = "gcp-vm-replicated"

type Runner struct {
	a string
}

func (r *Runner) GetConnectionInfo(i api.ID) (string, error) {
	return "", nil
}

func (r *Runner) List() ([]api.ID, error) {
	return nil, nil
}

func (r *Runner) IsExpired(i api.ID) (bool, error) {
	return false, nil
}

func (r *Runner) Destroy(i api.ID) error {
	return nil
}

func (r *Runner) Controller() []backend.Controller { //[]Somethings
	return []backend.Controller{}
}

func (r *Runner) Register() *api.CreateRequest {
	return &api.CreateRequest{Backend: BackendName}
}

//
//
//
