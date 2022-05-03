This is still under active development, please expect breaking changes.

At a very high level - this is a standardized API around provisioning pachyderm instances - using a variety of backends.  Right now there is only a single default backend - GCP_Namespace_Pulumi.  This backend provisons new workspaces in a single cluster in GCP, sharding by namespace.  Auth is enabled by default and setup with Auth0. By default all clusters are given an expiration of 3 days, and it's configurable up to 90 days.  The DeletionController which runs as part of the controlplane takes care of automatically deleting those environments which are expired.  

The different backends might be used to point at different clouds etc. (WIP note, backend and runner are still used somewhat interchangably throughout the codebase, switching to backend)


#Running Instructions:

You will need the HELIUM_CLIENT_SECRET and HELIUM_CLIENT_ID environment variables set, both of which can be found in the Prod Auth0 tenant for Hub, with the name `Auth0 for Helium`: https://manage.auth0.com/dashboard/us/***REMOVED***/applications/cwqe6eu76gLVLvmcKsnJRE3tkJrwDIsG/settings

In order to run the API, in a terminal tab run:
```shell
HELIUM_MODE=API HELIUM_CLIENT_SECRET="XXXXXXXXXXXX" HELIUM_CLIENT_ID="XXXXXXXXXX"   go run main.go
```
Then in another terminal tab the following command may be used to list workspaces:
```shell
curl -H "X-Pach: NotAGoodSecret"  localhost:2323/v1/api/workspaces
```
Getting connection info (substituting your own workspace id on all of these commands):
```shell
curl -H "X-Pach: NotAGoodSecret"  localhost:2323/v1/api/workspace/example-workspace-id | jq .
```
The response should look something like:
```shell
{
  "ConnectionInfo": {
    "K8s": "gcloud container clusters get-credentials ci-cluster-b9c3629 --zone us-east1-b --project ***REMOVED***",
    "K8sNamespace": "example-workspace-id",
    "ConsoleURL": "https://example-workspace-id.***REMOVED***",
    "NotebooksURL": "https://example-workspace-id.***REMOVED***",
    "Pachctl": "echo '{\"pachd_address\": \"grpc://34.74.249.170:30651\", \"source\": 2}' | tr -d \\ | pachctl config set context example-workspace-id --overwrite && pachctl config set active-context example-workspace-id"
  }
}
```


Checking expiry:
```shell
curl -H "X-Pach: NotAGoodSecret"  localhost:2323/v1/api/workspace/example-workspace-id/expired
```
Creating a new workspace, the available options are:
```golang
type Spec struct {
	Name             string
	Expiry           string
	PachdVersion     string
	ConsoleVersion   string
	NotebooksVersion string
	HelmVersion      string
	//	ValuesYAML       string
}
```
Which can be used in a request like so:
```shell
curl -X POST -H "X-Pach: NotAGoodSecret" -d '{"name": "example-workspace-02", "expiry": "2023-01-01"}'  localhost:2323/v1/api/workspace
```
Deleting a workspace manually:
```shell
curl -X DELETE -H "X-Pach: NotAGoodSecret"  localhost:2323/v1/api/workspace/example-workspace-id
```



In another tab in order to run the DeletionController:
```shell
HELIUM_MODE=CONTROLPLANE HELIUM_CLIENT_SECRET="XXXXXXXXXX" HELIUM_CLIENT_ID="XXXXXX"   go run main.go
```
The deletionController automatically queries every environment to check it's expiry, and if it's expired, it automatically deletes it.

# Known Issues:

The tests are broken, but shouldn't affect anything other than controllers
The interface for backend is likely to change slightly, with controller() and register() most likely to shift.
