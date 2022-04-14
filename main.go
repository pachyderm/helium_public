package main

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"

	//	"os/exec"
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/pachyderm/helium/api"
	"github.com/pachyderm/helium/gcp_namespace_pulumi"
	"github.com/pachyderm/helium/runner"
	compute "google.golang.org/api/compute/v1"
)

const (
	DEMO_PROJECT_ID        = "fancy-elephant-demos"
	DEMO_GCP_ZONE          = "us-central1-a"
	MINIMUM_PREWARM_COUNT  = 2
	SECRET_PASSWORD        = "NotAGoodSecret"
	SECRET_PASSWORD_HEADER = "X-Pach"
)

var supportedBackends []runner.BackendRunner

func main() {
	log.SetReportCaller(true)
	log.SetLevel(log.DebugLevel)
	gnp := &gcp_namespace_pulumi.Runner{}
	gnp.Setup()
	supportedBackends = append(supportedBackends, gnp)

	// Mode handles whether or not we run as a controlplane or api server
	mode := os.Getenv("HELIUM_MODE")
	if mode == "API" {
		router := mux.NewRouter()
		router.HandleFunc("/healthz", HealthCheckHandler)

		authRouter := router.PathPrefix("/api").Subrouter()
		authRouter.Use(authMiddleware)
		authRouter.HandleFunc("/create", HandleCreateRequest).Methods("POST")

		s := &http.Server{
			Addr:    ":2323",
			Handler: router,
		}

		log.Info("starting server on :2323")
		log.Fatal(s.ListenAndServe())
	} else if mode == "CONTROLPLANE" {
		// TODO: This should split into goroutines
		for {
			for _, v := range supportedBackends {
				for _, j := range v.Controller() {
					if err := j(v); err != nil {
						log.Errorf("controller error: %v", err)
					}
				}
			}
			//	log.Debug("getting prewarms")
			//	err := HandlePrewarms()
			//	if err != nil {
			//		log.Error(err)
			//	}
			log.Debug("done handling controllers, sleeping for 60s")
			time.Sleep(60 * time.Second)
		}
	} else {
		log.Fatal("unknown mode of operation, please set the env var HELIUM_MODE")
	}
}

//

func HandleCreateRequest(w http.ResponseWriter, r *http.Request) {
	log.Debug("create handler")
	w.Header().Set("Content-Type", "application/json")

	var createReq api.CreateRequest
	var createResponse api.CreateResponse

	err := json.NewDecoder(r.Body).Decode(&createReq)
	if err != nil {
		w.WriteHeader(400)
		fmt.Fprintf(w, "failed to parse create request")
		return
	}

	for _, v := range supportedBackends {
		log.Debugf("Backend Found, using: %v", v.Register())
		log.Debugf("Backend Requested, using: %v", &createReq.Spec.Backend)
		if createReq.Spec.Backend.Type == v.Register().Type && createReq.Spec.Backend.Provider == v.Register().Provider && createReq.Spec.Backend.Target == v.Register().Target {
			log.Debugf("Supported Backend Found, using: %v", createReq.Spec.Backend)
			createResponse, err = v.Create(createReq)
			if err != nil {
				w.WriteHeader(500)
				fmt.Fprintf(w, "error creating stack")
				return
			}
		}
	}

	log.Debugf("create req: %v", createReq)
	log.Debugf("auth enabled: %v", createReq.Spec.AuthEnabled)
	json.NewEncoder(w).Encode(&createResponse)
}

// Handle prewarm ensures that there is the minimum prewarm count, and if not, creates another one.
func HandlePrewarms() error {
	computeService, err := compute.NewService(context.Background())
	if err != nil {
		return fmt.Errorf("create compute client: %w", err)
	}
	instanceService := compute.NewInstancesService(computeService)
	instances, err := instanceService.List(DEMO_PROJECT_ID, DEMO_GCP_ZONE).Filter("labels.prewarm:t AND labels.assigned-at=none").Do()
	if err != nil {
		return fmt.Errorf("list instances: %w", err)
	}
	if len(instances.Items) < MINIMUM_PREWARM_COUNT {
		log.Infof("Not enough Prewarms, making another one... Current Count: %v", len(instances.Items))
		// the report caller is annoying here
		log.SetReportCaller(false)
		cmd := exec.Command("./prewarms.sh")
		cmd.Stdout = newLogWriter(log.WithFields(log.Fields{"command": cmd, "stream": "stdout"}))
		cmd.Stderr = newLogWriter(log.WithFields(log.Fields{"command": cmd, "stream": "stderr"}))
		if err := cmd.Run(); err != nil {
			log.SetReportCaller(true)
			log.WithError(err).Error("examples.sh failed; see above for output")
			return err
		}
		log.SetReportCaller(true)
	} else {
		log.Infof("Sufficient Prewarms. Current Count: %v", len(instances.Items))
	}
	return nil
}

func createHandler(w http.ResponseWriter, r *http.Request) {
	log.Debug("create handler")
	computeService, err := compute.NewService(context.Background())
	if err != nil {
		log.Errorf("create compute client: %v", err)
	}
	instanceService := compute.NewInstancesService(computeService)
	instances, err := instanceService.List(DEMO_PROJECT_ID, DEMO_GCP_ZONE).Filter("labels.prewarm:t AND labels.assigned-at=none").Do()
	if err != nil {
		log.Errorf("error listing instances: %v", err)
	}
	// Grab the first unassigned prewarm
	if len(instances.Items) > 0 {
		v := instances.Items[0]
		log.Debugf("Name: %s\n", v.Name)
		log.Debugf("Lables: %s\n", v.Labels)
		// Mon Jan 2 15:04:05 -0700 MST 2006
		v.Labels["assigned-at"] = time.Now().Format("2006-01-02")
		addLabels := &compute.InstancesSetLabelsRequest{
			Labels:           v.Labels,
			LabelFingerprint: v.LabelFingerprint,
		}
		_, err = instanceService.SetLabels(DEMO_PROJECT_ID, DEMO_GCP_ZONE, v.Name, addLabels).Do()
		if err != nil {
			log.Errorf("error setting label: %v", err)
		}
		i, err := instanceService.Get(DEMO_PROJECT_ID, DEMO_GCP_ZONE, v.Name).Do()
		if err != nil {
			log.Errorf("error getting instance: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(fmt.Sprintf("{\"name\": \"%s\", \"ip_addr\": \"%s\", \"command\": \"gcloud beta compute ssh --zone %s %s  --project %s\"}", i.Name, i.NetworkInterfaces[0].NetworkIP, DEMO_GCP_ZONE, i.Name, DEMO_PROJECT_ID)))
	} else {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(fmt.Sprintf("{\"error\": \"no prewarms available, please try again\"}")))
	}
}

// Middleware function, which will be called for each request
func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get(SECRET_PASSWORD_HEADER)
		if token == SECRET_PASSWORD {
			next.ServeHTTP(w, r)
		} else {
			http.Error(w, "Forbidden", http.StatusForbidden)
		}
	})
}

//
//
func HealthCheckHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
}

// newLogWriter returns an io.Writer that logs each full line written to it to the provided logrus
// Entry.
func newLogWriter(l *log.Entry) io.Writer {
	r, w := io.Pipe()
	s := bufio.NewScanner(r)
	go func() {
		for s.Scan() {
			l.Info(s.Text())
		}
		if err := s.Err(); err != nil {
			l.WithError(err).Error("error scanning lines")
		}
	}()
	return w
}

//
