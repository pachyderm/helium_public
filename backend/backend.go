package backend

import (
	"context"

	"github.com/pachyderm/helium/api"
)

const (
	InfraPrewarmCount     = 2
	WorkspacePrewarmCount = 2
)

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

//type IsPrewarmer interface {
//	IsPrewarm(api.ID) (bool, error)
//}
//
//type PrewarmProvisioner interface {
//	ProvisionPrewarm() (api.ID, error)
//}
//type PrewarmController interface {
//	Lister
//	IsPrewarmer
//	PrewarmProvisioner
//	//	PrewarmController()
//}

//type Controller interface {
//	Run(ctx context.Context) error
//}
// Call a sleep or something outside of this?
func RunDeletionController(ctx context.Context, br DeletionController) error {
	// For each registered Runner
	//List()
	//For each Pach, check Expiry. If true, call Delete
	id, err := br.List()
	if err != nil {
		return err
	}
	for _, v := range id.IDs {
		b, err := br.IsExpired(v)
		if err != nil {
			return err
		}
		if b {
			br.Destroy(v)
		}
	}
	return nil
}

//
//

//func CreateHandler(br *BackendRunner, r api.CreateRequest) error {
//	// Grab an existing prewarm if request doesn't include ForceNew
//	// Compute the difference - if possible do a helm upgrade to the desired versions
//	// Or create an entirely new pach
//	//
//	return nil
//}

// TODO: Need a way to list all Pachs so that Expiry can be called

// Can enviornments leak without a record of state for api / controlplane too?
// ProvisonInfra called - fails partway through.

// Transactions and central state store maybe able to be avoided if following constraints are true?
// DELETION if made up of multipe calls, all objects need to be deleted, or labeled object deleted last
// TTL should always be run on last object deleted, same with Get?
// ANY PARTIAL RESOURCES CREATED MUST BE ABLE TO BE DISCOVERED BY GET() - ie GCP labels created first, then non-labeled objects, if any

// TTL Must always return until Destroy is fully completed

///api/create

///api/prewarm

// version: v1
// kind: EphemeralEnv
// spec:
//   backend:
//     target: GCP
//     type: Namespace
//     provider: Pulumi
//   expiry: 30days #not allowed longer than 30 days && default 3 days?
//   pachdVersion:
//   consoleVersion:
//   auth.enabled: true
//   Infra.yaml:  // Backend specific overrides
//     namespace: foo
//   Values.yaml:
//     deploymentTarget: bar
//
// Different languages:
// API - Pulumi is syncronous
//
// Golang Interfaces:
// Pach - An ephemeral env referring to a single instance of pachyderm.
// Querier - Returns state?, and if ready - connection info for console, k8s, notebooks, etc
// TTLer - Needs to know how long cluster has been alive for
// Destroyer
// InfraProvisioner - (Null in the case of Namespaces)
// Helm(Workspace?)Provisioner - (Null in the case of Replicated)

// Let's see how pulumi works - maybe

//
