package main

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"

	"github.com/pachyderm/helium/backend"
	"github.com/pachyderm/helium/gcp_namespace_pulumi"
	"github.com/pachyderm/helium/handlers"
)

const (
	DEMO_PROJECT_ID       = "fancy-elephant-demos"
	DEMO_GCP_ZONE         = "us-central1-a"
	MINIMUM_PREWARM_COUNT = 2
)

func main() {
	log.SetReportCaller(true)
	log.SetLevel(log.DebugLevel)

	// Mode handles whether or not we run as a controlplane or api server
	mode := os.Getenv("HELIUM_MODE")
	if mode == "API" {
		router := mux.NewRouter()
		router.HandleFunc("/healthz", HealthCheckHandler)

		restRouter := router.PathPrefix("/v1/api").Subrouter()
		restRouter.Use(handlers.AuthMiddleware)
		restRouter.HandleFunc("/workspaces", handlers.ListRequest).Methods("GET")
		restRouter.HandleFunc("/workspace", handlers.CreateRequest).Methods("POST")
		restRouter.HandleFunc("/workspace/{workspaceId}", handlers.GetConnInfoRequest).Methods("GET")
		restRouter.HandleFunc("/workspace/{workspaceId}", handlers.DeleteRequest).Methods("DELETE")
		restRouter.HandleFunc("/workspace/{workspaceId}/expired", handlers.IsExpiredRequest).Methods("GET")
		//
		s := &http.Server{
			Addr:    ":2323",
			Handler: router,
		}

		log.Info("starting server on :2323")
		log.Fatal(s.ListenAndServe())
	} else if mode == "CONTROLPLANE" {
		// TODO: This should split into goroutines
		for {
			// TODO: set to 60 seconds
			time.Sleep(5 * time.Second)
			ctx := context.Background()
			gnp := &gcp_namespace_pulumi.Runner{}
			err := backend.RunDeletionController(ctx, gnp)
			if err != nil {
				log.Errorf("deletion controller: %v", err)
			}
		}
	} else {
		log.Fatal("unknown mode of operation, please set the env var HELIUM_MODE")
	}
}

func HealthCheckHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
}
