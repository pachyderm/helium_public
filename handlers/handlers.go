package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"text/template"

	"github.com/gorilla/mux"
	"github.com/gorilla/schema"
	"github.com/pachyderm/helium/api"
	"github.com/pachyderm/helium/gcp_namespace_pulumi"
	"github.com/pachyderm/helium/util"
	log "github.com/sirupsen/logrus"
)

const (
	SECRET_PASSWORD        = "Bearer ***REMOVED***"
	SECRET_PASSWORD_HEADER = "Authorization"
)

var decoder = schema.NewDecoder()

// Middleware function, which will be called for each request
func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get(SECRET_PASSWORD_HEADER)
		if token == SECRET_PASSWORD {
			next.ServeHTTP(w, r)
		} else {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
		}
	})
}

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

// TODO: deduplicate param handling logic between sync and async
func CreateRequest(w http.ResponseWriter, r *http.Request) {
	gnp := &gcp_namespace_pulumi.Runner{}

	log.SetReportCaller(true)
	log.SetLevel(log.DebugLevel)
	log.Info("testing handler")

	var spec api.Spec
	var res *api.CreateResponse

	err := r.ParseMultipartForm(32 << 20)
	if err != nil {
		log.Errorf("Error parsing form: %v", err)
	}
	err = decoder.Decode(&spec, r.PostForm)
	if err != nil {
		log.Errorf("Error decoding form: %v", err)
	}

	file, _, err := r.FormFile("valuesYaml")
	if err != nil {
		if strings.Contains(err.Error(), "no such file") {
			log.Debug("no file param")
		} else {
			log.Errorf("Error reading FormFile: %v", err)
		}
	}
	if spec.Name == "" {
		spec.Name = util.Name()
	}

	if file != nil {
		f, err := os.CreateTemp("", "temp-values")
		if err != nil {
			w.WriteHeader(500)
			fmt.Fprintf(w, "failed to upload file: %v", err)
			log.Errorf("error creating temp file: %v", err)
			return
		}
		defer os.Remove(f.Name())
		defer f.Close()
		_, err = io.Copy(f, file)
		if err != nil {
			w.WriteHeader(500)
			fmt.Fprintf(w, "failed to upload file: %v", err)
			log.Errorf("error copying file: %v", err)
			return
		}
		content, err := ioutil.ReadFile(f.Name())
		if err != nil {
			w.WriteHeader(500)
			fmt.Fprintf(w, "failed to upload file: %v", err)
			log.Errorf("error copying file: %v", err)
			return
		}
		log.Debugf("Content:\n%s", content)
		spec.ValuesYAML = f.Name()
	}
	res, err = gnp.Create(&spec)
	if err != nil {
		w.WriteHeader(500)
		fmt.Fprintf(w, "error creating stack")
		log.Errorf("create handler: %v", err)
		return
	}

	log.Debugf("Returning Response: %v", res)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(&res)
}

func UIListWorkspace(w http.ResponseWriter, r *http.Request) {
	gnp := &gcp_namespace_pulumi.Runner{}
	var res *api.ListResponse
	res, err := gnp.List()
	if err != nil {
		w.WriteHeader(500)
		fmt.Fprintf(w, "error listing stacks")
		log.Errorf("ui list handler: %v", err)
		return
	}

	//{
	//  "Workspace": {
	//    "ID": "sean-named-this-99",
	//    "Status": "ready",
	//    "PulumiURL": "https://app.pulumi.com/pachyderm/helium/sean-named-this-99/updates/1",
	//    "K8s": "gcloud container clusters get-credentials ***REMOVED*** --zone us-east1-b --project ***REMOVED***",
	//    "K8sNamespace": "sean-named-this-99",
	//    "ConsoleURL": "https://sean-named-this-99.***REMOVED***",
	//    "NotebooksURL": "https://jh-sean-named-this-99.***REMOVED***",
	//    "GCSBucket": "pach-bucket-8b939a9",
	//    "Pachctl": "echo '{\"pachd_address\": \"grpc://34.148.152.146:30651\", \"source\": 2}' | tr -d \\ | pachctl config set context sean-named-this-99 --overwrite && pachctl config set active-context sean-named-this-99"
	//  }
	//}
	//var ids []string
	//for _, y := range res.IDs {
	//	ids = append(ids, string(y))
	//}
	tmpl := template.Must(template.ParseFiles("templates/list.tmpl"))
	if err := tmpl.Execute(w, res); err != nil {
		panic(err)
	}
}

func UIRootHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	tmpl := template.Must(template.ParseFiles("templates/create.html"))
	if err := tmpl.Execute(w, nil); err != nil {
		panic(err)
	}
}

func UIGetWorkspace(w http.ResponseWriter, r *http.Request) {
	gnp := &gcp_namespace_pulumi.Runner{}
	//	w.Header().Set("Content-Type", "application/json")
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

	//{
	//  "Workspace": {
	//    "ID": "sean-named-this-99",
	//    "Status": "ready",
	//    "PulumiURL": "https://app.pulumi.com/pachyderm/helium/sean-named-this-99/updates/1",
	//    "K8s": "gcloud container clusters get-credentials ***REMOVED*** --zone us-east1-b --project ***REMOVED***",
	//    "K8sNamespace": "sean-named-this-99",
	//    "ConsoleURL": "https://sean-named-this-99.***REMOVED***",
	//    "NotebooksURL": "https://jh-sean-named-this-99.***REMOVED***",
	//    "GCSBucket": "pach-bucket-8b939a9",
	//    "Pachctl": "echo '{\"pachd_address\": \"grpc://34.148.152.146:30651\", \"source\": 2}' | tr -d \\ | pachctl config set context sean-named-this-99 --overwrite && pachctl config set active-context sean-named-this-99"
	//  }
	//}

	tmpl := template.Must(template.ParseFiles("templates/get.tmpl"))
	if err := tmpl.Execute(w, res.Workspace); err != nil {
		panic(err)
	}
}

func UICreation(w http.ResponseWriter, r *http.Request) {
	gnp := &gcp_namespace_pulumi.Runner{}

	log.SetReportCaller(true)
	log.SetLevel(log.DebugLevel)
	log.Info("testing handler")

	var spec api.Spec
	var err error

	err = r.ParseMultipartForm(32 << 20)
	if err != nil {
		log.Errorf("Error parsing form: %v", err)
	}
	err = decoder.Decode(&spec, r.PostForm)
	if err != nil {
		log.Errorf("Error decoding form: %v", err)
	}

	file, _, err := r.FormFile("valuesYaml")
	if err != nil {
		if strings.Contains(err.Error(), "no such file") {
			log.Debug("no file param")
		} else {
			w.WriteHeader(500)
			fmt.Fprintf(w, "failed to upload file: %v", err)
			log.Errorf("Error reading FormFile: %v", err)
		}
	}
	if spec.Name == "" {
		spec.Name = util.Name()
	}
	var f *os.File
	if file != nil {
		f, err = os.CreateTemp("", "temp-values")
		if err != nil {
			w.WriteHeader(500)
			fmt.Fprintf(w, "failed to upload file: %v", err)
			log.Errorf("error creating temp file: %v", err)
			return
		}
		_, err = io.Copy(f, file)
		if err != nil {
			w.WriteHeader(500)
			fmt.Fprintf(w, "failed to upload file: %v", err)
			log.Errorf("error copying file: %v", err)
			return
		}
		content, err := ioutil.ReadFile(f.Name())
		if err != nil {
			w.WriteHeader(500)
			fmt.Fprintf(w, "failed to upload file: %v", err)
			log.Errorf("error copying file: %v", err)
			return
		}
		log.Debugf("Content:\n%s", content)
		spec.ValuesYAML = f.Name()
	}

	// TODO: This is a bit of a hack
	// TODO: need to handle temp file being deleted before this is called because of the defers
	go func(spec api.Spec, f *os.File) {
		_, err = gnp.Create(&spec)
		if err != nil {
			log.Errorf("create handler: %v", err)
			return
		}
		if f != nil {
			defer os.Remove(f.Name())
			defer f.Close()
		}
	}(spec, f)
	res2 := &api.ConnectionInfo{
		ID:     api.ID(spec.Name),
		Status: "creating",
	}
	//{
	//  "Workspace": {
	//    "ID": "sean-named-this-99",
	//    "Status": "ready",
	//    "PulumiURL": "https://app.pulumi.com/pachyderm/helium/sean-named-this-99/updates/1",
	//    "K8s": "gcloud container clusters get-credentials ***REMOVED*** --zone us-east1-b --project ***REMOVED***",
	//    "K8sNamespace": "sean-named-this-99",
	//    "ConsoleURL": "https://sean-named-this-99.***REMOVED***",
	//    "NotebooksURL": "https://jh-sean-named-this-99.***REMOVED***",
	//    "GCSBucket": "pach-bucket-8b939a9",
	//    "Pachctl": "echo '{\"pachd_address\": \"grpc://34.148.152.146:30651\", \"source\": 2}' | tr -d \\ | pachctl config set context sean-named-this-99 --overwrite && pachctl config set active-context sean-named-this-99"
	//  }
	//}

	tmpl := template.Must(template.ParseFiles("templates/get.tmpl"))
	if err := tmpl.Execute(w, res2); err != nil {
		panic(err)
	}
}

func AsyncCreationRequest(w http.ResponseWriter, r *http.Request) {
	gnp := &gcp_namespace_pulumi.Runner{}

	log.SetReportCaller(true)
	log.SetLevel(log.DebugLevel)
	log.Info("testing handler")

	var spec api.Spec
	var res *api.CreateResponse
	var err error

	err = r.ParseMultipartForm(32 << 20)
	if err != nil {
		log.Errorf("Error parsing form: %v", err)
	}
	err = decoder.Decode(&spec, r.PostForm)
	if err != nil {
		log.Errorf("Error decoding form: %v", err)
	}

	file, _, err := r.FormFile("valuesYaml")
	if err != nil {
		if strings.Contains(err.Error(), "no such file") {
			log.Debug("no file param")
		} else {
			w.WriteHeader(500)
			fmt.Fprintf(w, "failed to upload file: %v", err)
			log.Errorf("Error reading FormFile: %v", err)
		}
	}
	if spec.Name == "" {
		spec.Name = util.Name()
	}
	res = &api.CreateResponse{api.ID(spec.Name)}
	log.Debugf("Returning Response: %v", res)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(&res)

	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	var f *os.File
	if file != nil {
		f, err = os.CreateTemp("", "temp-values")
		if err != nil {
			w.WriteHeader(500)
			fmt.Fprintf(w, "failed to upload file: %v", err)
			log.Errorf("error creating temp file: %v", err)
			return
		}
		_, err = io.Copy(f, file)
		if err != nil {
			w.WriteHeader(500)
			fmt.Fprintf(w, "failed to upload file: %v", err)
			log.Errorf("error copying file: %v", err)
			return
		}
		content, err := ioutil.ReadFile(f.Name())
		if err != nil {
			w.WriteHeader(500)
			fmt.Fprintf(w, "failed to upload file: %v", err)
			log.Errorf("error copying file: %v", err)
			return
		}
		log.Debugf("Content:\n%s", content)
		spec.ValuesYAML = f.Name()
	}

	// TODO: This is a bit of a hack
	// TODO: need to handle temp file being deleted before this is called because of the defers
	go func(spec api.Spec, f *os.File) {
		_, err = gnp.Create(&spec)
		if err != nil {
			log.Errorf("create handler: %v", err)
			return
		}
		if f != nil {
			defer os.Remove(f.Name())
			defer f.Close()
		}
	}(spec, f)

	//// TODO Delete comment
	////res = &api.CreateResponse{api.ID(spec.Name)}
	//
	//log.Debugf("Returning Response: %v", res)
	//w.Header().Set("Content-Type", "application/json")
	//json.NewEncoder(w).Encode(&res)
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
