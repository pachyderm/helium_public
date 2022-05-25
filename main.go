package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"

	"github.com/pachyderm/helium/backend"
	"github.com/pachyderm/helium/gcp_namespace_pulumi"
	"github.com/pachyderm/helium/handlers"
	psentry "github.com/pachyderm/helium/sentry"
)

const (
	DEMO_PROJECT_ID       = "fancy-elephant-demos"
	DEMO_GCP_ZONE         = "us-central1-a"
	MINIMUM_PREWARM_COUNT = 2
	SENTRY_DSN            = "***REMOVED***"
)

func main() {
	// Send error logs to Sentry.
	if err := sentry.Init(sentry.ClientOptions{
		Dsn:              SENTRY_DSN,
		AttachStacktrace: false, // We do this ourselves.
		Environment:      "dev",
		Release:          "v0.1",

		// It would be nice to provide a Logrus logger (with 'log.WithField("source",
		// "sentry").WriterLevel(log.DebugLevel)'), but because we call into Sentry from a
		// logrus hook, that would deadlock.
		Debug: false,
	}); err != nil {
		log.Panicf("failed to initialize sentry: %v", err)
	}
	defer func() {
		// Flush sentry async buffers during panic.
		log.Info("waiting up to 5 seconds for sentry to send final alerts")
		ok := sentry.Flush(5 * time.Second)
		if ok {
			log.Info("flushed sentry ok")
		} else {
			log.Info("flush to sentry timed out")
		}
	}()
	defer func() {
		// Send panics not caused by logrus to Sentry.
		if err := recover(); err != nil {
			switch x := err.(type) {
			case *log.Entry:
				log.Panic(x)
			case error:
				log.WithError(x).Panic("recovered panic")
			default:
				log.WithError(fmt.Errorf("%v", x)).Panic("recovered panic")
			}
		}
	}()

	log.AddHook(&psentry.Logrus{
		EnabledLevels: []log.Level{
			log.WarnLevel,
			log.ErrorLevel,
			log.PanicLevel,
			log.FatalLevel,
		},
	})
	// TODO: make local dev pretty
	// make logs look nice for stackdriver
	log.SetFormatter(&log.JSONFormatter{
		FieldMap: log.FieldMap{
			log.FieldKeyTime:  "time",
			log.FieldKeyLevel: "severity",
			log.FieldKeyMsg:   "message",
		},
		// https://github.com/sirupsen/logrus/pull/162/files
		TimestampFormat: time.RFC3339Nano,
	})
	log.SetReportCaller(true)

	mode := os.Getenv("HELIUM_MODE")
	// TODO: // HACK:
	cmd := exec.Command("gcloud", "auth", "login", "--cred-file=/var/secrets/google/key.json")
	err := cmd.Run()
	if err != nil {
		log.Fatal(err)
	}
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
	a.Router.Use(handlers.SentryMiddleware)
	a.Router.Use(handlers.LoggingMiddleware)
	a.Router.HandleFunc("/", handlers.UIRootHandler)
	a.Router.HandleFunc("/healthz", handlers.HealthCheck)
	a.Router.HandleFunc("/get/{workspaceId}", handlers.UIGetWorkspace)
	a.Router.HandleFunc("/create", handlers.UICreation)
	a.Router.HandleFunc("/list", handlers.UIListWorkspace)

	restRouter := a.Router.PathPrefix("/v1/api").Subrouter()
	restRouter.Use(handlers.AuthMiddleware)
	restRouter.HandleFunc("/workspaces", handlers.ListRequest).Methods("GET")
	restRouter.HandleFunc("/workspace", handlers.AsyncCreationRequest).Methods("POST")
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
		ctx := context.Background()
		gnp := &gcp_namespace_pulumi.Runner{}
		err := backend.RunDeletionController(ctx, gnp)
		if err != nil {
			log.Errorf("deletion controller: %v", err)
		}
		time.Sleep(1800 * time.Second)
	}
}
