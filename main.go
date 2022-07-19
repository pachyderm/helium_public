package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime/debug"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"

	"github.com/pachyderm/helium/backend"
	"github.com/pachyderm/helium/handlers"
	"github.com/pachyderm/helium/pulumi_backends"
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

	env := os.Getenv("HELIUM_ENV")
	if env == "PROD" {
		// TODO: // HACK:
		cmd := exec.Command("gcloud", "auth", "login", "--cred-file=/var/secrets/google/key.json")
		err := cmd.Run()
		if err != nil {
			log.Fatal(err)
		}

		// TODO: HACK
		cmd = exec.Command("aws", "configure", "set", "region", "us-west-2")
		err = cmd.Run()
		if err != nil {
			log.Fatal(err)
		}
		cmd = exec.Command("aws", "configure", "set", "aws_access_key_id", os.Getenv("AWS_ACCESS_KEY_ID"), "--profile", "default")
		err = cmd.Run()
		if err != nil {
			log.Fatal(err)
		}
		cmd = exec.Command("aws", "configure", "set", "aws_secret_access_key", os.Getenv("AWS_SECRET_ACCESS_KEY"), "--profile", "default")
		err = cmd.Run()
		if err != nil {
			log.Fatal(err)
		}
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

var (
	// Version will be the version tag if the binary is built with "go install url/tool@version".
	// If the binary is built some other way, it will be "(devel)".
	Version = "unknown"
	// Revision is taken from the vcs.revision tag in Go 1.18+.
	Revision = "unknown"
	// LastCommit is taken from the vcs.time tag in Go 1.18+.
	LastCommit time.Time
	// DirtyBuild is taken from the vcs.modified tag in Go 1.18+.
	DirtyBuild = true
)

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
	info, ok := debug.ReadBuildInfo()
	if !ok {
		log.Error("unable to read build info")
	} else {
		for _, kv := range info.Settings {
			switch kv.Key {
			case "vcs.revision":
				Revision = kv.Value
			case "vcs.time":
				LastCommit, _ = time.Parse(time.RFC3339, kv.Value)
			case "vcs.modified":
				DirtyBuild = kv.Value == "true"
			}
		}
	}
	log.Infof("version sha: %s", Revision)
	log.Infof("version last commit: %s", LastCommit)
	log.Infof("version dirty: %v", DirtyBuild)
	log.Info("starting server on :2323")
	log.Fatal(s.ListenAndServe())
}

func RunControlplane() {
	for {
		ctx := context.Background()
		gnp := &pulumi_backends.Runner{}
		err := backend.RunDeletionController(ctx, gnp)
		if err != nil {
			log.Errorf("deletion controller: %v", err)
		}
		time.Sleep(1800 * time.Second)
	}
}
