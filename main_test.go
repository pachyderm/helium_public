package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
	"regexp"

	"github.com/pachyderm/helium/api"
	"github.com/pachyderm/helium/util"
	"github.com/stretchr/testify/assert"
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
	//	if testing.Short() {
	t.Skip("skipping broken e2e test in short mode")
	//}
	// List
	req, _ := http.NewRequest("GET", "/v1/api/workspaces", nil)
	req.Header.Set("Authorization", "Bearer ***REMOVED***")
	response := executeRequest(req)
	actual := response.Code
	expected := http.StatusOK
	if expected != actual {
		t.Errorf("Expected response code %d. Got %d\n", expected, actual)
	}
	// Create
	req, _ = http.NewRequest("POST", "/v1/api/workspace", nil)
	response = executeRequest(req)
	req.Header.Set("Authorization", "Bearer ***REMOVED***")
	actual = response.Code
	expected = http.StatusOK
	if expected != actual {
		t.Errorf("Expected response code %d. Got %d\n", expected, actual)
	}

	id := &api.CreateResponse{}
	err := json.NewDecoder(response.Body).Decode(&id)
	if err != nil {
		t.Errorf("Unabled to decode create response. Got %s", response.Body)
	}
	// Poll with Get for 11min
	for i := 0; i < 66; i++ {
		time.Sleep(10 * time.Second)
		req, _ = http.NewRequest("Get", fmt.Sprintf("/v1/api/workspace/%s", id.ID), nil)
		response = executeRequest(req)
		req.Header.Set("Authorization", "Bearer ***REMOVED***")
		actual = response.Code
		expected = http.StatusOK
		if expected != actual {
			t.Errorf("Expected response code %d. Got %d\n", expected, actual)
		}

		res := &api.GetConnectionInfoResponse{}
		err := json.NewDecoder(response.Body).Decode(&res)
		if err != nil {
			t.Errorf("Unabled to decode get response. Got %s", response.Body)
		}
		if res.Workspace.Status != "creating" {
			break
		}
	}
	// Delete

	t.Logf("Response Body: %v", response.Body.String())
	if body := response.Body.String(); body != "" {
		t.Errorf("Expected an empty array. Got %s", body)
	}
}

func TestParseInfraJSON(t *testing.T) {
	infraJson := `
{
	"k8s": {
		"nodepools": [
		{
			"nodeType": "m1",
			"nodeNumInstances": 2,
			"nodeDiskType": "gp2",
			"nodeDiskSize": 100,
			"nodeDiskIOPS": 10000
		}
	]
},
	"rds": {
			"nodeType": "m1",
			"diskType": "gp2",
			"diskSize": 100,
			"diskIOPS": 10000
		}
}`

	var infra api.InfraJson
	err := json.Unmarshal([]byte(infraJson), &infra)
	if err != nil {
		t.Errorf("couldn't unmarshal: %v", err)
	}
	t.Logf("Infra: %#v", infra)
}

func TestUtilName(t *testing.T) {
	log.Println("TestUtilName running")
	actual := util.Name()
	expected := regexp.MustCompile(`^[a-z0-9]([-a-z0-9]{1,61}[a-z0-9]{1})$`)
	assert.Regexp(t,expected,actual)
}

func executeRequest(req *http.Request) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()
	a.Initialize()
	a.Router.ServeHTTP(rr, req)
	return rr
}
