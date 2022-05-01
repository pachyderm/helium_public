package main

import (
	"net/http"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"

	//	"os/exec"
	"bufio"
	"io"
	"os"
	"time"

	"github.com/pachyderm/helium/handlers"
)

const (
	DEMO_PROJECT_ID        = "fancy-elephant-demos"
	DEMO_GCP_ZONE          = "us-central1-a"
	MINIMUM_PREWARM_COUNT  = 2
	SECRET_PASSWORD        = "NotAGoodSecret"
	SECRET_PASSWORD_HEADER = "X-Pach"
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
		restRouter.Use(authMiddleware)
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
		//	for {
		//		for _, v := range supportedBackends {
		//			for _, j := range v.Controller(context.Background()) {
		//				if err := j(v); err != nil {
		//					log.Errorf("controller error: %v", err)
		//				}
		//			}
		//		}
		//	log.Debug("getting prewarms")
		//	err := HandlePrewarms()
		//	if err != nil {
		//		log.Error(err)
		//	}
		log.Debug("done handling controllers, sleeping for 60s")
		time.Sleep(60 * time.Second)
		//	}
	} else {
		log.Fatal("unknown mode of operation, please set the env var HELIUM_MODE")
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
//
