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
	mode := os.Getenv("HELIUM_MODE")
	if mode == "API" {
		RunAPI()
	} else if mode == "CONTROLPLANE" {
		RunControlplane()
	} else {
		log.Fatal("unknown mode of operation, please set the env var HELIUM_MODE")
	}
}

type App struct {
	Router *mux.Router
}

func (a *App) Initialize() {
	a.Router = mux.NewRouter()

	a.Router.HandleFunc("/healthz", HealthCheckHandler)
	a.Router.PathPrefix("/templates/").Handler(http.StripPrefix("/templates/", http.FileServer(http.Dir("templates"))))
	// TODO: Fix auth for this
	a.Router.HandleFunc("/testing", handlers.AsyncCreationRequest).Methods("POST")

	restRouter := a.Router.PathPrefix("/v1/api").Subrouter()
	restRouter.Use(handlers.AuthMiddleware)
	restRouter.HandleFunc("/workspaces", handlers.ListRequest).Methods("GET")
	restRouter.HandleFunc("/workspace", handlers.CreateRequest).Methods("POST")
	restRouter.HandleFunc("/workspace/{workspaceId}", handlers.GetConnInfoRequest).Methods("GET")
	restRouter.HandleFunc("/workspace/{workspaceId}", handlers.DeleteRequest).Methods("DELETE")
	restRouter.HandleFunc("/workspace/{workspaceId}/expired", handlers.IsExpiredRequest).Methods("GET")
}

func RunAPI() {
	log.SetReportCaller(true)
	log.SetLevel(log.DebugLevel)

	app := App{}
	app.Initialize()
	s := &http.Server{
		Addr:    ":2323",
		Handler: app.Router,
	}
	//
	log.Info("starting server on :2323")
	log.Fatal(s.ListenAndServe())
}

func RunControlplane() {
	for {
		time.Sleep(60 * time.Second)
		ctx := context.Background()
		gnp := &gcp_namespace_pulumi.Runner{}
		err := backend.RunDeletionController(ctx, gnp)
		if err != nil {
			log.Errorf("deletion controller: %v", err)
		}
	}
}
func HealthCheckHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
}
