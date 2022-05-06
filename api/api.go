package api

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
	Name             string `schema:"name"`
	Expiry           string `schema:"expiry"`
	PachdVersion     string `schema:"pachdVersion"`
	ConsoleVersion   string `schema:"consoleVersion"`
	NotebooksVersion string `schema:"notebooksVersion"`
	HelmVersion      string `schema:"helmVersion"`
	// This should be an actual file upload
	ValuesYAML string //schema:"valuesYaml" This one isn't handled by a schema directly
}

type GetConnectionInfoRequest struct {
	ApiDefaultRequest
	ID ID
}

type GetConnectionInfoResponse struct {
	Workspace ConnectionInfo
}

type ListRequest struct {
	ApiDefaultRequest
}

type ListResponse struct {
	IDs []ID
}

type IsExpiredRequest struct {
	ApiDefaultRequest
	ID ID
}

type IsExpiredResponse struct {
	Expired bool
}

type DeleteRequest struct {
	ApiDefaultRequest
	ID ID
}

// TODO: Rename Workspace
type ConnectionInfo struct {
	ID           ID
	Status       string
	PulumiURL    string
	K8s          string
	K8sNamespace string
	ConsoleURL   string
	NotebooksURL string
	GCSBucket    string
	Pachctl      string
}
