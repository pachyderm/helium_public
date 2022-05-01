package api

import (
	"time"
)

// Create a new workspace in response to a valid POST request at /workspace,
// Delete a workspace in response to a valid DELETE request at /workspace/{id},
// Fetch a workspace in response to a valid GET request at /workspace/{id}, and
// Fetch a list of workspaces in response to a valid GET request at /workspaces.
// Get expiry info in response to a valid GET request at /workspace/{id}/expiry.
// /api/v1/backend/{id}/workspaces

//type App struct {
//	Router *mux.Router
//}
//
//func Initialize() *App {
//	app := &App{}
//	app.Router = mux.NewRouter()
//	app.Router.HandleFunc("/healthz", HealthCheckHandler)
//
//	oldRouter := app.Router.PathPrefix("/api").Subrouter()
//	oldRouter.Use(authMiddleware)
//	// TODO: actually implement a REST api
//	oldRouter.HandleFunc("/create", HandleCreateRequest).Methods("POST")
//	oldRouter.HandleFunc("/list", HandleListRequest).Methods("POST")
//	oldRouter.HandleFunc("/get", HandleGetConnInfoRequest).Methods("POST")
//	oldRouter.HandleFunc("/delete", HandleDeleteRequest).Methods("POST")
//	oldRouter.HandleFunc("/isexpired", HandleIsExpiredRequest).Methods("POST")
//
//	restRouter := app.Router.PathPrefix("/v1/api").Subrouter()
//	restRouter.Use(authMiddleware)
//	restRouter.HandlerFunc("/workspaces", HandleListRequest).Methods("GET")
//	restRouter.HandlerFunc("/workspace", HandleCreateRequest).Methods("POST")
//	restRouter.HandleFunc("/workspace/{workspaceId}", HandleGetConnInfoRequest).Methods("GET")
//	restRouter.HandleFunc("/workspace/{workspaceId}", HandleDeleteRequest).Methods("DELETE")
//	restRouter.HandleFunc("/workspace/{workspaceId}/expired", HandleIsExpiredRequest).Methods("GET")
//
//	return app
//}

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
	Expiry           time.Time
	PachdVersion     string
	ConsoleVersion   string
	NotebooksVersion string
	ValuesYAML       string
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
