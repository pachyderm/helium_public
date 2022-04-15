package api

import "time"

type ID string

type ApiDefaultRequest struct {
	Version string
	Backend string
}
type CreateRequest struct {
	ApiDefaultRequest
	Spec Spec
}

type CreateResponse struct {
	ID ID
}

type Spec struct {
	//	Backend          Backend
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
//type Backend struct {
//	Target   string
//	Type     string
//	Provider string
//}

type GetConnectionInfoRequest struct {
	ApiDefaultRequest
	ID ID
}

type GetConnectionInfoResponse struct {
	ConnectionInfo ConnectionInfo
}

type ListRequest struct {
	ApiDefaultRequest
}

type ListResponse struct {
	IDs []ID
}

type ConnectionInfo struct {
	K8s          string
	ConsoleURL   string
	NotebooksURL string
	Pachctl      string
}
