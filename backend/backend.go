package backend

import (
	"context"
	"os"

	log "github.com/sirupsen/logrus"

	"github.com/pachyderm/helium/api"
)

const (
	InfraPrewarmCount     = 2
	WorkspacePrewarmCount = 2
)

var deletionControllerMode string = os.Getenv("HELIUM_CONTROLPLANE_DELETE_ALL")

type Name string

type Controller func(context.Context) error

type Lister interface {
	// TODO: Maybe should take a list.request
	List() (*api.ListResponse, error)
}
type GetConnInfoer interface {
	GetConnectionInfo(api.ID) (*api.GetConnectionInfoResponse, error)
}
type Destroyer interface {
	Destroy(api.ID) error
}
type Creator interface {
	Create(*api.Spec) (*api.CreateResponse, error)
}

type Backend interface {
	Lister
	GetConnInfoer
	Creator
	Destroyer
	// TODO: Expiry probably doesn't need to live in the API
	IsExpirer
	Register() *api.CreateRequest
	Controller(context.Context) []Controller
}

// Register() *api.Backend
//	RestoreSeedData(string) error

type IsExpirer interface {
	IsExpired(api.ID) (bool, error)
}

type DeletionController interface {
	Lister
	IsExpirer
	Destroyer
	//	DeletionController()
}

func RunDeletionController(ctx context.Context, br DeletionController) error {
	//For each Pach, check Expiry. If true, call Delete
	id, err := br.List()
	if err != nil {
		return err
	}
	for _, v := range id.IDs {
		b, err := br.IsExpired(v)
		if err != nil {
			//	if err strings.Contains("expected stack output 'helium-expiry' not found for stack") {
			//		br.Remove(v)
			//	}
			log.Errorf("expiry error: %v", err)
			continue
		}
		if b || deletionControllerMode == "True" {
			log.Debugf("deletion controller destroying: %v", v)
			br.Destroy(v)
		}
	}
	return nil
}
