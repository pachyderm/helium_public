package backend

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/pachyderm/helium/api"
)

func TestDeletionControllerLoop(t *testing.T) {
	want := &TestBackendRunner{
		ResourceIDs: &api.ListResponse{[]api.ID{"B", "C", "D"}},
	}
	got := &TestBackendRunner{
		ResourceIDs: &api.ListResponse{[]api.ID{"A", "B", "C", "D"}},
	}
	err := RunDeletionController(context.Background(), got)
	if err != nil {
		t.Errorf("error: %s", err)
	}
	if !cmp.Equal(got, want) {
		t.Errorf(fmt.Sprintf("diff: %v", cmp.Diff(want, got)))
	}
}

type TestBackendRunner struct {
	ResourceIDs *api.ListResponse
}

func (r *TestBackendRunner) Get(i api.ID) (string, error) {
	return "", nil
}

// how to mutate data?
func (r *TestBackendRunner) List() (*api.ListResponse, error) {
	return r.ResourceIDs, nil
}

func (r *TestBackendRunner) IsExpired(i api.ID) (bool, error) {
	if i == "A" {
		return true, nil
	}
	return false, nil
}

func (r *TestBackendRunner) Destroy(i api.ID) error {
	index := 100
	for z, x := range r.ResourceIDs.IDs {
		if x == i {
			index = z
		}
	}
	r.ResourceIDs.IDs = append(r.ResourceIDs.IDs[:index], r.ResourceIDs.IDs[index+1:]...)
	return nil
}

func (r *TestBackendRunner) Create(api.CreateRequest) (api.CreateResponse, error) {
	return api.CreateResponse{}, nil
}
