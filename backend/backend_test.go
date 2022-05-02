package backend

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/pachyderm/helium/api"
)

//func Test(t *testing.T) {
//	testData := []struct {
//		name string
//		//  simpleTestRunner TestBackendRunner
//		//  want
//
//	}{
//		{
//			"hello",
//		},
//		{
//			"goodbye",
//		},
//	}
//
//	for _, z := range testData {
//		t.Run(z.name, func(t *testing.T) {
//
//		})
//	}
//}

//
func TestDeletionControllerLoop(t *testing.T) {
	want := &TestBackendRunner{
		ResourceIDs: api.ListResponse{[]api.ID{"B", "C", "D"}},
	}
	got := &TestBackendRunner{
		ResourceIDs: api.ListResponse{[]api.ID{"A", "B", "C", "D"}},
	}
	err := DeletionController(got)
	if err != nil {
		t.Errorf("error: %v", err)
	}
	if !cmp.Equal(got, want) {
		t.Errorf(fmt.Sprintf("diff: %v", cmp.Diff(want, got)))
	}
}

//
//TODO: Setup proper pointers at some point
//func TestPrewarm(t *testing.T) {
//	want := &TestBackendRunner{
//		ResourceIDs: api.ListResponse{[]api.ID{"A", "B", "C", "D", "F"}},
//	}
//	got := &TestBackendRunner{
//		ResourceIDs: api.ListResponse{[]api.ID{"A", "B", "C", "D"}},
//	}
//	err := PrewarmController(got)
//	if err != nil {
//		t.Errorf("error: %v", err)
//	}
//	if !cmp.Equal(got, want) {
//		t.Errorf(fmt.Sprintf("diff: %v", cmp.Diff(got, want)))
//	}
//}

//
//
type TestBackendRunner struct {
	ResourceIDs api.ListResponse
}

func (r *TestBackendRunner) Get(i api.ID) (string, error) {
	return "", nil
}

// how to mutate data?
func (r *TestBackendRunner) List() (*api.ListResponse, error) {
	return &r.ResourceIDs, nil
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
	for z, x := range r.ResourceIDs.IDs {
		if x == i {
			index = z
		}
	}
	r.ResourceIDs.IDs = append(r.ResourceIDs.IDs[:index], r.ResourceIDs.IDs[index+1:]...)
	return nil
}

func (r *TestBackendRunner) ProvisionPrewarm() (api.ID, error) {
	r.ResourceIDs.IDs = append(r.ResourceIDs.IDs, "F")
	return "F", nil
}

// Does this need an ID as a param?
func (r *TestBackendRunner) IsPrewarm(i api.ID) (bool, error) {
	if i == "B" || i == "F" {
		return true, nil
	}
	return false, nil
}

func (r *TestBackendRunner) Create(api.CreateRequest) (api.CreateResponse, error) {
	return api.CreateResponse{}, nil
}

//
func (r *TestBackendRunner) Controller(ctx context.Context) []Controller {
	return []Controller{
		//r.PrewarmController,
		r.DeletionController,
	}
}

//func (r *TestBackendRunner) PrewarmController(ctx context.Context) error {
//	return RunPrewarmController(ctx, r)
//} //TODO ctx

func (r *TestBackendRunner) DeletionController(ctx context.Context) error {
	return RunDeletionController(ctx, r)
} //TODO ctx
