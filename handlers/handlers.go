package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/pachyderm/helium/api"
	"github.com/pachyderm/helium/gcp_namespace_pulumi"
	log "github.com/sirupsen/logrus"
)

func ListRequest(w http.ResponseWriter, r *http.Request) {
	gnp := &gcp_namespace_pulumi.Runner{}
	w.Header().Set("Content-Type", "application/json")
	var res *api.ListResponse
	res, err := gnp.List()
	if err != nil {
		w.WriteHeader(500)
		fmt.Fprintf(w, "error listing stack")
		log.Errorf("list handler: %v", err)
		return
	}
	json.NewEncoder(w).Encode(&res)
}

func GetConnInfoRequest(w http.ResponseWriter, r *http.Request) {
	gnp := &gcp_namespace_pulumi.Runner{}
	w.Header().Set("Content-Type", "application/json")
	vars := mux.Vars(r)
	id := api.ID(vars["workspaceId"])
	log.Debugf("GetConn ID: %v", id)
	var res *api.GetConnectionInfoResponse
	res, err := gnp.GetConnectionInfo(id)
	if err != nil {
		w.WriteHeader(500)
		fmt.Fprintf(w, "error getting connection info for stack")
		log.Errorf("getConnInfo handler: %v", err)
		return
	}
	log.Debugf("getConnInfo res: %v", res)
	json.NewEncoder(w).Encode(&res)
}

func CreateRequest(w http.ResponseWriter, r *http.Request) {
	gnp := &gcp_namespace_pulumi.Runner{}
	log.Debug("create handler")
	w.Header().Set("Content-Type", "application/json")

	var req *api.Spec
	var res *api.CreateResponse

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		log.Errorf("failed to parse create request: %v", err)
		w.WriteHeader(400)
		fmt.Fprintf(w, "failed to parse create request")
		return
	}

	res, err = gnp.Create(req)
	if err != nil {
		w.WriteHeader(500)
		fmt.Fprintf(w, "error creating stack")
		log.Errorf("create handler: %v", err)
		return
	}

	log.Debugf("create req: %v", req)
	json.NewEncoder(w).Encode(&res)
}

func IsExpiredRequest(w http.ResponseWriter, r *http.Request) {
	gnp := &gcp_namespace_pulumi.Runner{}
	w.Header().Set("Content-Type", "application/json")
	vars := mux.Vars(r)
	id := api.ID(vars["workspaceId"])
	log.Debugf("GetConn ID: %v", id)
	var val bool
	val, err := gnp.IsExpired(id)
	if err != nil {
		w.WriteHeader(500)
		fmt.Fprintf(w, "error getting expiry for stack")
		log.Errorf("IsExpired handler: %v", err)
		return

	}
	json.NewEncoder(w).Encode(&api.IsExpiredResponse{Expired: val})
}

// TODO: pick delete or destroy, not both
func DeleteRequest(w http.ResponseWriter, r *http.Request) {
	gnp := &gcp_namespace_pulumi.Runner{}
	w.Header().Set("Content-Type", "application/json")
	vars := mux.Vars(r)
	id := api.ID(vars["workspaceId"])
	log.Debugf("GetConn ID: %v", id)

	err := gnp.Destroy(id)
	if err != nil {
		w.WriteHeader(500)
		fmt.Fprintf(w, "error destroying stack")
		log.Errorf("delete handler: %v", err)
		return
	}
	w.WriteHeader(200)
}
