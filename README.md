This is still under active development, please expect breaking changes.

At a very high level - this is a standardized API around provisioning pachyderm instances - using a variety of backends.  Right now there is only a single default backend - GCP_Namespace_Pulumi.  This backend provisons new workspaces in a single cluster in GCP, sharding by namespace.  Auth is enabled by default and setup with Auth0. By default all workspaces are given an expiration of 2 days, and it's configurable up to 90 days. So any workspaces will be deleted the following day at midnight. The DeletionController which runs as part of the controlplane takes care of automatically deleting those environments which are expired.  

The different backends might be used to point at different clouds etc. (WIP note, backend and runner are still used somewhat interchangably throughout the codebase, switching to backend)


#Running Instructions:

When running locally, helium uses port 2323, like so: http://localhost:2323/v1/api/workspaces

You will need the HELIUM_CLIENT_SECRET and HELIUM_CLIENT_ID environment variables set, both of which can be found in the Prod Auth0 tenant for Hub, with the name `Auth0 for Helium`: https://manage.auth0.com/dashboard/us/***REMOVED***/applications/cwqe6eu76gLVLvmcKsnJRE3tkJrwDIsG/settings

In order to run the API, in a terminal tab run:
```shell
HELIUM_MODE=API HELIUM_CLIENT_SECRET="XXXXXXXXXXXX" HELIUM_CLIENT_ID="XXXXXXXXXX"   go run main.go
```

#Usage


The following command may be used to list workspaces:
```shell
curl -H "Authorization: Bearer ***REMOVED***"  https://0f52-172-98-132-18.ngrok.io/v1/api/workspaces
```
Getting workspace info (substituting your own workspace id on all of these commands):
```shell
curl -H "Authorization: Bearer ***REMOVED***"  https://0f52-172-98-132-18.ngrok.io/v1/api/workspace/example-workspace-id | jq .
```
The response should look something like:
```shell
{
  "Workspace": {
    "ID": "example-workspace-id",
    "Status": "ready",
    "PulumiURL": "https://app.pulumi.com/pachyderm/helium/example-workspace-id/updates/1",
    "K8s": "gcloud container clusters get-credentials ***REMOVED*** --zone us-east1-b --project ***REMOVED***",
    "K8sNamespace": "example-workspace-id",
    "ConsoleURL": "https://example-workspace-id.***REMOVED***",
    "NotebooksURL": "https://jh-example-workspace-id.***REMOVED***",
    "GCSBucket": "pach-bucket-ec496ed",
    "Pachctl": "echo '{\"pachd_address\": \"grpc://34.138.177.35:30651\", \"source\": 2}' | tr -d \\ | pachctl config set context example-workspace-id --overwrite && pachctl config set active-context example-workspace-id"
  }
}
```
Status should either be "creating", "ready", or "failed".  If failed, following the pulumiURL will provide more info. Credentials are in 1password.  
K8s is a link to the GKE cluster.

Setting up Pachctl and then running `pachctl auth login` will follow the Auth0 auth flow.

Checking expiry:
```shell
curl -H "Authorization: Bearer ***REMOVED***"  https://0f52-172-98-132-18.ngrok.io/v1/api/workspace/example-workspace-id/expired
```

To quickly create a new workspace with the default options, run:
```shell
curl -X POST -H "Authorization: Bearer ***REMOVED***" https://0f52-172-98-132-18.ngrok.io/v1/api/workspace
```
This command is a synchronous request, and may take up to 10min. On average, it should take less than 2 minutes. If using the synchronous request, it's recommended to supply a name parameter incase the connection times out before the request is completed.  Further info can then be given by just getting that workspaces (command repeated for clarity:
```shell
  curl -H "Authorization: Bearer ***REMOVED***"  https://0f52-172-98-132-18.ngrok.io/v1/api/workspace/example-workspace-id | jq .  
```
).

Workspace creation also takes a bunch of optional parameters:
```golang
type Spec struct {
	Name             string `schema:"name"`
	Expiry           string `schema:"expiry"`
	PachdVersion     string `schema:"pachdVersion"`
	ConsoleVersion   string `schema:"consoleVersion"`
	NotebooksVersion string `schema:"notebooksVersion"`
	HelmVersion      string `schema:"helmVersion"`
  // ValuesYAML should be a path to your file locally
	ValuesYAML string //schema:"valuesYaml" This one isn't handled by a schema directly
}
```
None of the fields are required. ValuesYAML should be a path to your values.yaml file locally. However, it doesn't take precedence over the values Helium supplies, which could be a source of confusion. Future work is planned to eliminate this. These params can be used in a request like so:

```shell
curl -X POST -H "Authorization: Bearer ***REMOVED***" -F name=example-workspace-id -F helmVersion=2.2.0-rc.1 -F valuesYaml=@testval.yml https://0f52-172-98-132-18.ngrok.io/v1/api/workspace
```
Where `testval.yml` is a values.yaml file in my current directory.


Deleting a workspace manually:
```shell
curl -X DELETE -H "Authorization: Bearer ***REMOVED***"  https://0f52-172-98-132-18.ngrok.io/v1/api/workspace/example-workspace-id
```



In another tab in order to run the DeletionController:
```shell
HELIUM_MODE=CONTROLPLANE HELIUM_CLIENT_SECRET="XXXXXXXXXX" HELIUM_CLIENT_ID="XXXXXX"   go run main.go
```
The deletionController automatically queries every environment to check it's expiry, and if it's expired, it automatically deletes it.

# Known Issues:

The tests are broken, but shouldn't affect anything other than controllers
The interface for backend is likely to change slightly, with controller() and register() most likely to shift.
