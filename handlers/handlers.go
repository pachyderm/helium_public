package handlers

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"strings"
	"text/template"
	"time"

	sentryhttp "github.com/getsentry/sentry-go/http"
	"github.com/gorilla/mux"
	"github.com/gorilla/schema"
	"github.com/pachyderm/helium/api"
	"github.com/pachyderm/helium/pulumi_backends"
	"github.com/pachyderm/helium/util"

	log "github.com/sirupsen/logrus"
)

const (
	SECRET_PASSWORD        = "Bearer ***REMOVED***"
	SECRET_PASSWORD_HEADER = "Authorization"
	USER_HEADER            = "X-Forwarded-Email"
)

var decoder = schema.NewDecoder()
var validNameCharacters = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]{1,61}[a-z0-9]{1})$`)

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

func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		url := r.URL
		method := r.Method
		user := r.Header.Get(USER_HEADER)
		next.ServeHTTP(w, r)
		duration := time.Since(start)
		log.WithFields(log.Fields{
			"url":      url,
			"method":   method,
			"duration": duration,
			"user":     user,
		}).Info("request completed")
	})
}

func SentryMiddleware(next http.Handler) http.Handler {
	sentryHandler := sentryhttp.New(sentryhttp.Options{
		Repanic: true,
	})
	return sentryHandler.HandleFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
	})
}

func HealthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
}

func ListRequest(w http.ResponseWriter, r *http.Request) {
	gnp := &pulumi_backends.Runner{}
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
	gnp := &pulumi_backends.Runner{}
	w.Header().Set("Content-Type", "application/json")
	vars := mux.Vars(r)
	id := api.ID(vars["workspaceId"])
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

func AsyncCreationRequest(w http.ResponseWriter, r *http.Request) {
	gnp := &pulumi_backends.Runner{}

	log.SetReportCaller(true)
	log.SetLevel(log.DebugLevel)

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

	var content []byte
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
		content, err = ioutil.ReadFile(f.Name())
		if err != nil {
			w.WriteHeader(500)
			fmt.Fprintf(w, "failed to upload file: %v", err)
			log.Errorf("error copying file: %v", err)
			return
		}

		spec.ValuesYAML = f.Name()
		spec.ValuesYAMLContent = content
	}

	fileInfra, _, err := r.FormFile("infraJson")
	if err != nil {
		if strings.Contains(err.Error(), "no such file") {
			log.Debug("no file param")
		} else {
			w.WriteHeader(500)
			fmt.Fprintf(w, "failed to upload file: %v", err)
			log.Errorf("Error reading FormFile: %v", err)
		}
	}

	var contentInfra []byte
	var fInfra *os.File
	if fileInfra != nil {
		fInfra, err = os.CreateTemp("", "temp-values")
		if err != nil {
			w.WriteHeader(500)
			fmt.Fprintf(w, "failed to upload file: %v", err)
			log.Errorf("error creating temp file: %v", err)
			return
		}
		_, err = io.Copy(fInfra, fileInfra)
		if err != nil {
			w.WriteHeader(500)
			fmt.Fprintf(w, "failed to upload file: %v", err)
			log.Errorf("error copying file: %v", err)
			return
		}
		contentInfra, err = ioutil.ReadFile(fInfra.Name())
		if err != nil {
			w.WriteHeader(500)
			fmt.Fprintf(w, "failed to upload file: %v", err)
			log.Errorf("error copying file: %v", err)
			return
		}

		spec.InfraJSON = fInfra.Name()
		spec.InfraJSONContent = contentInfra
	}

	spec.CreatedBy = r.Header.Get(USER_HEADER)

	if spec.Name == "" {
		spec.Name = util.Name()
	}
	spec.Name = strings.ToLower(spec.Name)
	if badChar := validNameCharacters.FindString(spec.Name); badChar == "" {
		w.WriteHeader(500)
		fmt.Fprintf(w, html.EscapeString("contains invalid character or is too long, must fit this regex ^[a-z0-9]([-a-z0-9]{1,61}[a-z0-9]{1})$"))
		log.Errorf("invalid name: %v", spec.Name)
		return
	}

	log.WithFields(log.Fields{
		"canonical":         "true",
		"request":           "create-api",
		"name":              spec.Name,
		"createdBy":         spec.CreatedBy,
		"expiry":            spec.Expiry,
		"pachdVersion":      spec.PachdVersion,
		"consoleVersion":    spec.ConsoleVersion,
		"notebooksVersion":  spec.NotebooksVersion,
		"helmVersion":       spec.HelmVersion,
		"cleanupOnFail":     spec.CleanupOnFail,
		"disableNotebooks":  spec.DisableNotebooks,
		"clusterStack":      spec.ClusterStack,
		"valuesYAML":        spec.ValuesYAML,
		"valuesYAMLContent": spec.ValuesYAMLContent,
		"infraJSON":         spec.ValuesYAML,
		"infraJSONContent":  contentInfra,
		"backend":           spec.Backend,
	}).Infof("create parameters")

	// TODO: This is a bit of a hack
	go func(spec api.Spec, f *os.File, fInfra *os.File) {
		_, err = gnp.Create(&spec)
		if err != nil {
			log.Errorf("create handler: %v", err)
			return
		}
		if f != nil {
			defer os.Remove(f.Name())
			defer f.Close()
		}
		if fInfra != nil {
			defer os.Remove(fInfra.Name())
			defer fInfra.Close()
		}
	}(spec, f, fInfra)
}

func IsExpiredRequest(w http.ResponseWriter, r *http.Request) {
	gnp := &pulumi_backends.Runner{}
	w.Header().Set("Content-Type", "application/json")
	vars := mux.Vars(r)
	id := api.ID(vars["workspaceId"])
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
	gnp := &pulumi_backends.Runner{}
	w.Header().Set("Content-Type", "application/json")
	vars := mux.Vars(r)
	id := api.ID(vars["workspaceId"])

	err := gnp.Destroy(id)
	if err != nil {
		w.WriteHeader(500)
		fmt.Fprintf(w, "error destroying stack")
		log.Errorf("delete handler: %v", err)
		return
	}
	w.WriteHeader(200)
}

func UIListWorkspace(w http.ResponseWriter, r *http.Request) {
	gnp := &pulumi_backends.Runner{}
	var res *api.ListResponse
	res, err := gnp.List()
	if err != nil {
		w.WriteHeader(500)
		fmt.Fprintf(w, "error listing stacks")
		log.Errorf("ui list handler: %v", err)
		return
	}
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
	gnp := &pulumi_backends.Runner{}
	vars := mux.Vars(r)
	id := api.ID(vars["workspaceId"])
	var res *api.GetConnectionInfoResponse
	res, err := gnp.GetConnectionInfo(id)
	if err != nil {
		w.WriteHeader(500)
		fmt.Fprintf(w, "error getting connection info for stack")
		log.Errorf("getConnInfo handler: %v", err)
		return
	}

	tmpl := template.Must(template.ParseFiles("templates/get.tmpl"))
	if err := tmpl.Execute(w, res.Workspace); err != nil {
		panic(err)
	}
}

func UICreation(w http.ResponseWriter, r *http.Request) {
	gnp := &pulumi_backends.Runner{}

	log.SetReportCaller(true)
	log.SetLevel(log.DebugLevel)

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

	fileInfra, _, err := r.FormFile("infraJson")
	if err != nil {
		if strings.Contains(err.Error(), "no such file") {
			log.Debug("no file param")
		} else {
			w.WriteHeader(500)
			fmt.Fprintf(w, "failed to upload file: %v", err)
			log.Errorf("Error reading FormFile: %v", err)
		}
	}
	var content []byte
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
		content, err = ioutil.ReadFile(f.Name())
		if err != nil {
			w.WriteHeader(500)
			fmt.Fprintf(w, "failed to upload file: %v", err)
			log.Errorf("error copying file: %v", err)
			return
		}
		spec.ValuesYAML = f.Name()
		spec.ValuesYAMLContent = content
	}

	var contentInfra []byte
	var fInfra *os.File
	if fileInfra != nil {
		fInfra, err = os.CreateTemp("", "temp-values")
		if err != nil {
			w.WriteHeader(500)
			fmt.Fprintf(w, "failed to upload file: %v", err)
			log.Errorf("error creating temp file: %v", err)
			return
		}
		_, err = io.Copy(fInfra, fileInfra)
		if err != nil {
			w.WriteHeader(500)
			fmt.Fprintf(w, "failed to upload file: %v", err)
			log.Errorf("error copying file: %v", err)
			return
		}
		contentInfra, err = ioutil.ReadFile(fInfra.Name())
		if err != nil {
			w.WriteHeader(500)
			fmt.Fprintf(w, "failed to upload file: %v", err)
			log.Errorf("error copying file: %v", err)
			return
		}

		spec.InfraJSON = fInfra.Name()
		spec.InfraJSONContent = contentInfra
	}
	spec.CreatedBy = r.Header.Get(USER_HEADER)

	if spec.Name == "" {
		spec.Name = util.Name()
	}
	spec.Name = strings.ToLower(spec.Name)
	if badChar := validNameCharacters.FindString(spec.Name); badChar == "" {
		w.WriteHeader(500)
		fmt.Fprintf(w, html.EscapeString("contains invalid character or is too long, must fit this regex ^[a-z0-9]([-a-z0-9]{1,61}[a-z0-9]{1})$"))
		log.Errorf("invalid name: %v", spec.Name)
		return
	}

	log.WithFields(log.Fields{
		"canonical":         "true",
		"request":           "create-ui",
		"name":              spec.Name,
		"createdBy":         spec.CreatedBy,
		"expiry":            spec.Expiry,
		"pachdVersion":      spec.PachdVersion,
		"consoleVersion":    spec.ConsoleVersion,
		"notebooksVersion":  spec.NotebooksVersion,
		"helmVersion":       spec.HelmVersion,
		"cleanupOnFail":     spec.CleanupOnFail,
		"disableNotebooks":  spec.DisableNotebooks,
		"clusterStack":      spec.ClusterStack,
		"valuesYAML":        spec.ValuesYAML,
		"valuesYAMLContent": spec.ValuesYAMLContent,
		"infraJSON":         spec.ValuesYAML,
		"infraJSONContent":  contentInfra,
		"backend":           spec.Backend,
	}).Infof("create parameters")

	// TODO: This is a bit of a hack
	go func(spec api.Spec, f *os.File, fInfra *os.File) {
		_, err = gnp.Create(&spec)
		if err != nil {
			log.Errorf("create handler: %v", err)
			return
		}
		if f != nil {
			defer os.Remove(f.Name())
			defer f.Close()
		}
		if fInfra != nil {
			defer os.Remove(fInfra.Name())
			defer fInfra.Close()
		}
	}(spec, f, fInfra)
	// Set the first requests data to creating, because a list lookup will race condition and fail.
	// Meta refresh on template of ~10 seconds is plenty of time to make next list condition work.
	res2 := &api.ConnectionInfo{
		ID:     api.ID(spec.Name),
		Status: "creating",
	}

	tmpl := template.Must(template.ParseFiles("templates/get.tmpl"))
	if err := tmpl.Execute(w, res2); err != nil {
		panic(err)
	}
}
