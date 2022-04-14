package runner

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/pachyderm/helium/api"
)

func Test(t *testing.T) {
	testData := []struct {
		name string
		//  simpleTestRunner TestBackendRunner
		//  want

	}{
		{
			"hello",
		},
		{
			"goodbye",
		},
	}

	for _, z := range testData {
		t.Run(z.name, func(t *testing.T) {

		})
	}
}

//
func TestDeletionControllerLoop(t *testing.T) {
	want := &TestBackendRunner{
		ResourceIDs: []api.ID{"B", "C", "D"},
	}
	got := &TestBackendRunner{
		ResourceIDs: []api.ID{"A", "B", "C", "D"},
	}
	DeletionControllerLoop(got)
	if !cmp.Equal(got, want) {
		t.Errorf(fmt.Sprintf("diff: %v", cmp.Diff(got, want)))
	}
}

func TestPrewarmInfraLoop(t *testing.T) {
	want := &TestBackendRunner{
		ResourceIDs: []api.ID{"A", "B", "C", "D", "E"},
	}
	got := &TestBackendRunner{
		ResourceIDs: []api.ID{"A", "B", "C", "D"},
	}
	PrewarmInfraLoop(got)
	if !cmp.Equal(got, want) {
		t.Errorf(fmt.Sprintf("diff: %v", cmp.Diff(got, want)))
	}
}

func TestPrewarmWorkspaceLoop(t *testing.T) {
	want := &TestBackendRunner{
		ResourceIDs: []api.ID{"A", "B", "C", "D", "F"},
	}
	got := &TestBackendRunner{
		ResourceIDs: []api.ID{"A", "B", "C", "D"},
	}
	PrewarmWorkspaceLoop(got)
	if !cmp.Equal(got, want) {
		t.Errorf(fmt.Sprintf("diff: %v", cmp.Diff(got, want)))
	}
}

type TestBackendRunner struct {
	ResourceIDs []api.ID
}

func (r *TestBackendRunner) Get(i api.ID) (string, error) {
	return "", nil
}

// how to mutate data?
func (r *TestBackendRunner) List() ([]api.ID, error) {
	return r.ResourceIDs, nil
}

func (r *TestBackendRunner) IsExpired(i api.ID) (bool, error) {
	if i == "A" {
		return true, nil
	}
	return false, nil
}

func (r *TestBackendRunner) Destroy(i api.ID) error {
	// b := r.ResourceIDs[:0]
	index := 100
	for z, x := range r.ResourceIDs {
		if x == i {
			index = z
		}
	}
	r.ResourceIDs = append(r.ResourceIDs[:index], r.ResourceIDs[index+1:]...)
	return nil
}

func (r *TestBackendRunner) ProvisionInfra() (api.ID, error) {
	r.ResourceIDs = append(r.ResourceIDs, "E")
	return "E", nil
}

func (r *TestBackendRunner) ProvisionWorkspace() (api.ID, error) {
	r.ResourceIDs = append(r.ResourceIDs, "F")
	return "F", nil
}

func (r *TestBackendRunner) IsPrewarmInfra(i api.ID) (bool, error) {
	if i == "A" || i == "E" {
		return true, nil
	}
	return false, nil
}

// Does this need an ID as a param?
func (r *TestBackendRunner) IsPrewarmWorkspace(i api.ID) (bool, error) {
	if i == "B" || i == "F" {
		return true, nil
	}
	return false, nil
}

func (r *TestBackendRunner) Create(api.CreateRequest) (api.CreateResponse, error) {
	return api.CreateResponse{}, nil
}

func (r *TestBackendRunner) RestoreSeedData(bucket string) error {
	return nil
}
func (r *TestBackendRunner) Register() *api.Backend {
	return nil
}

//
func (r *TestBackendRunner) Controller() []ControlLoops {
	return []ControlLoops{
		PrewarmWorkspaceLoop,
		PrewarmInfraLoop,
		DeletionControllerLoop,
	}
}
