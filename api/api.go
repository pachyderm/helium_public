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
	CleanupOnFail    string `schema:"cleanupOnFail"`
	Backend          string `schema:"name"`
	// This should be an actual file upload
	ValuesYAML string //schema:"valuesYaml" This field isn't handled by schema directly
	// This should be an actual file upload
	// TODO: This needs to actually be wired up yet
	InfraJSON string //schema:"infraJson" This field isn't handled by schema directly
	// TODO: A bit of a hack
	InfraJSONContent InfraJson

	// This is populated automatically by a header
	CreatedBy string
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
	LastUpdated  string
	PulumiURL    string
	K8s          string
	K8sNamespace string
	ConsoleURL   string
	NotebooksURL string
	GCSBucket    string
	PachdIp      string
	Pachctl      string
	Expiry       string
	CreatedBy    string
	Backend      string
}

type InfraJson struct {
	K8S `json:"k8s"`
	RDS `json:"rds"`
}

type RDS struct {
	NodeType string `json:"nodeType"`
	DiskType string `json:"diskType"`
	DiskSize int    `json:"diskSize"`
	DiskIOPS int    `json:"diskIOPS"`
}

type K8S struct {
	Nodepools []Nodepool `json:"nodepools"`
}

type Nodepool struct {
	NodeType         string `json:"nodeType"`
	NodeNumInstances int    `json:"nodeNumInstances"`
	NodeDiskType     string `json:"nodeDiskType"`
	NodeDiskSize     int    `json:"nodeDiskSize"`
	NodeDiskIOPS     int    `json:"nodeDiskIOPS"`
}
