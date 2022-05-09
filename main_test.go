package main

import (
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestA(t *testing.T) {
	log.Println("TestA running")
}

var a App

func TestHealthz(t *testing.T) {
	req, _ := http.NewRequest("GET", "/healthz", nil)
	response := executeRequest(req)
	actual := response.Code
	expected := http.StatusOK
	if expected != actual {
		t.Errorf("Expected response code %d. Got %d\n", expected, actual)
	}

	if body := response.Body.String(); body != "" {
		t.Errorf("Expected an empty array. Got %s", body)
	}
}

//

func TestAuthList(t *testing.T) {
	req, _ := http.NewRequest("GET", "/v1/api/workspaces", nil)
	response := executeRequest(req)
	actual := response.Code
	expected := http.StatusUnauthorized
	if expected != actual {
		t.Errorf("Expected response code %d. Got %d\n", expected, actual)
	}
}

func TestList(t *testing.T) {
	req, _ := http.NewRequest("GET", "/v1/api/workspaces", nil)
	req.Header.Set("Authorization", "Bearer ***REMOVED***")
	response := executeRequest(req)
	actual := response.Code
	expected := http.StatusOK
	if expected != actual {
		t.Errorf("Expected response code %d. Got %d\n", expected, actual)
	}
}

//

func TestE2E(t *testing.T) {
	req, _ := http.NewRequest("GET", "/v1/api/workspaces", nil)
	req.Header.Set("Authorization", "Bearer ***REMOVED***")
	response := executeRequest(req)
	actual := response.Code
	expected := http.StatusOK
	if expected != actual {
		t.Errorf("Expected response code %d. Got %d\n", expected, actual)
	}
	//if body := response.Body.String(); body != "" {
	//	t.Errorf("Expected an empty array. Got %s", body)
	//}
}

func executeRequest(req *http.Request) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()
	a.Initialize()
	a.Router.ServeHTTP(rr, req)
	return rr
}
