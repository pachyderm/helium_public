package api

import "time"

type ID string

type CreateRequest struct {
	Version string
	Kind    string
	Spec    Spec
}

type CreateResponse struct {
	ID ID
}

type Spec struct {
	Backend          Backend
	Expiry           time.Time
	PachdVersion     string
	ConsoleVersion   string
	NotebooksVersion string
	AuthEnabled      bool
	InfraYAML        string
	ValuesYAML       string
	SeedDataTarget   string // TODO: remove before deploy
	// ForceNew         bool // Don't use a prewarm
}

// TODO: Should backend be where flavors of auth are supported? Is it implementation dependent?
type Backend struct {
	Target   string
	Type     string
	Provider string
}

type GetRequest struct {
	ID ID
}

type GetRequestReturn struct {
	ConnectionInfo ConnectionInfo
}

type ConnectionInfo struct {
	K8s          string
	ConsoleURL   string
	NotebooksURL string
	Pachctl      string
}
