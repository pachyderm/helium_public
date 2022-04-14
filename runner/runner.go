package runner

import (
	"fmt"

	"github.com/pachyderm/helium/api"
)

const (
	InfraPrewarmCount     = 2
	WorkspacePrewarmCount = 2
)

type ControlLoops func(BackendRunner) error

type BackendRunner interface {
	Get(api.ID) (string, error)
	List() ([]api.ID, error)
	// Does pulumi support labels there?
	IsPrewarmInfra(api.ID) (bool, error)
	IsPrewarmWorkspace(api.ID) (bool, error)
	// Does a comparison of expiry vs current time and returns if it's expired or not
	IsExpired(api.ID) (bool, error)
	Destroy(api.ID) error

	// ProvisionInfra should create an ID - attach as an annotation or label where appropriate
	ProvisionInfra() (api.ID, error)
	// What sorts of data needs to be passed into ProvisionWorkspace vs ProvisionInfra?
	// Does downscoping help at all? Optionally pass through the create request? Where should validation happen?
	ProvisionWorkspace() (api.ID, error)

	Create(api.CreateRequest) (api.CreateResponse, error)

	RestoreSeedData(string) error

	Register() *api.Backend

	Controller() []ControlLoops
}

// Call a sleep or something outside of this?
func DeletionControllerLoop(br BackendRunner) error {
	// For each registered Runner
	//List()
	//For each Pach, check Expiry. If true, call Delete
	ids, err := br.List()
	if err != nil {
		return err
	}
	for _, v := range ids {
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
func PrewarmInfraLoop(br BackendRunner) error {
	ids, err := br.List()
	if err != nil {
		return err
	}
	var count int
	for _, v := range ids {
		b, err := br.IsPrewarmInfra(v)
		if err != nil {
			return err
		}
		if b {
			count += 1
		}
	}
	if count < InfraPrewarmCount {
		i, err := br.ProvisionInfra()
		if err != nil {
			return err
		}
		fmt.Printf("Prewarming Infra ID: %v\n", i)
	}
	return nil
}

func PrewarmWorkspaceLoop(br BackendRunner) error {
	ids, err := br.List()
	if err != nil {
		return err
	}
	var count int
	for _, v := range ids {
		b, err := br.IsPrewarmWorkspace(v)
		if err != nil {
			return err
		}
		if b {
			count += 1
		}
	}
	if count < WorkspacePrewarmCount {
		i, err := br.ProvisionWorkspace()
		if err != nil {
			return err
		}
		fmt.Printf("Prewarming Worskpace ID: %v\n", i)
	}
	return nil
}

func CreateHandler(br *BackendRunner, r api.CreateRequest) error {
	// Grab an existing prewarm if request doesn't include ForceNew
	// Compute the difference - if possible do a helm upgrade to the desired versions
	// Or create an entirely new pach
	//
	return nil
}

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
//   Infra.yaml:
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
