package backend

import (
	"context"
	"os"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/pachyderm/helium/api"
	"github.com/pachyderm/helium/pulumi_backends"
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
			if strings.Contains(err.Error(), "expected stack output 'helium-expiry' not found for stack") {
				log.Debugf("deletion controller destroying because expiry not found: %v", v)
				err := br.Destroy(v)
				if err != nil {
					log.Errorf("deletion controller error destroying: %v", err)
				}
			} else {
				log.Errorf("expiry error: %v", err)
				continue
			}
		}
		if b || deletionControllerMode == "True" {
			log.Debugf("deletion controller destroying: %v", v)
			err := br.Destroy(v)
			if err != nil {
				log.Errorf("deletion controller error destroying: %v", err)
			}
			// TODO: This is a bit of a hack for feeddog. Will cause a circular import if anything in pulumi_backends needs this package
			time.Sleep(time.Minute * 5)
			if v == "public-sandbox" {
				spec := &api.Spec{
					Name:    "public-sandbox",
					Backend: "gcp_cluster",
				}
				gnp := &pulumi_backends.Runner{}
				_, err = gnp.Create(spec)
				if err != nil {
					log.Errorf("create handler: %v", err)
				}
			}
		}
	}
	return nil
}
