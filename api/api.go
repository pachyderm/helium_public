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
	Name             string
	Expiry           string
	PachdVersion     string
	ConsoleVersion   string
	NotebooksVersion string
	HelmVersion      string
	//	ValuesYAML       string
}

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

type ConnectionInfo struct {
	K8s          string
	K8sNamespace string
	ConsoleURL   string
	NotebooksURL string
	Pachctl      string
}
