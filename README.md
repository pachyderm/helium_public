# Helium

## NOTE: THIS PUBLIC REPO IS FOR EXPERIMENTAL USE ONLY


A standardized interface for provisioning pachyderm instances, on both AWS and GCP. Helium provides a UI and API at https://helium.***REMOVED***, as well as a controlplane that handles automatically cleaning up workspaces when they have expired.  Helium runs pulumi programs that are defined at https://github.com/pachyderm/poc-pulumi. In the default use case, helium spins up workspaces in a single cluster in GCP, each isolated in it's own namespace. These are closer to real "production" pachyderm instances, as Console, Notebooks, Auth0, TLS, DNS, Ingress, GPUs, and Autoscaling is all correctly wired up. Helium itself is stateless, pushing that concern onto the responsibility of pulumi.

By default all workspaces are deleted at midnight of the day they are created. However, expiration is configurable for up to 90 days. The DeletionController which runs as part of the controlplane takes care of automatically deleting those environments which are expired.  

Auth is enabled by default and setup with Auth0.

Requirements for this project can be found here: `https://docs.google.com/document/d/1qFMMBQOS_KwHpRAiiwFwzRkvd5d5BKfSuxkchq6ra9U/edit`

## Using Helium UI

A frontend interface for Helium lives at `https://helium.***REMOVED***/`.

It currently doesn't allow you to delete a workspace through the UI. You have to utilize curl or wait for the DeletionController to clean it up after it expires.

On average, it takes 90 seconds to provision a new workspace.

## Using the API directly with curl

The following command may be used to list workspaces:
```shell
curl -H "Authorization: Bearer ***REMOVED***"  https://helium.***REMOVED***/v1/api/workspaces
```
Getting workspace info (substituting your own workspace id on all of these commands):
```shell
curl -H "Authorization: Bearer ***REMOVED***"  https://helium.***REMOVED***/v1/api/workspace/example-workspace-id | jq .
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
curl -H "Authorization: Bearer ***REMOVED***"  https://helium.***REMOVED***/v1/api/workspace/example-workspace-id/expired
```

To quickly create a new workspace with the default options, run:
```shell
curl -X POST -H "Authorization: Bearer ***REMOVED***" https://helium.***REMOVED***/v1/api/workspace
```
This command is an asynchronous request, and should return quickly. Polling is then necessary. On average, it should take less than 2 minutes. If using the synchronous request, it's recommended to supply a name parameter incase the connection times out before the request is completed.  Further info can then be given by just getting that workspaces (command repeated for clarity:
```shell
  curl -H "Authorization: Bearer ***REMOVED***"  https://helium.***REMOVED***/v1/api/workspace/example-workspace-id | jq .  
```
).

Workspace creation also takes a bunch of optional parameters:
```golang
type Spec struct {
	Name               string `schema:"name"`
	Expiry             string `schema:"expiry"`
	PachdVersion       string `schema:"pachdVersion"`
	ConsoleVersion     string `schema:"consoleVersion"`
	NotebooksVersion   string `schema:"notebooksVersion"`
	MountServerVersion string `schema:"mountServerVersion"`
	HelmVersion        string `schema:"helmVersion"`
	DisableNotebooks   string `schema:"disableNotebooks"`
	Backend            string `schema:"backend"`
	ClusterStack       string `schema:"clusterStack"`
	// This should be an actual file upload
	ValuesYAML        string //schema:"valuesYaml" This field isn't handled by schema directly
	ValuesYAMLContent []byte
	InfraJSON         string //schema:"infraJson" This field isn't handled by schema directly
	InfraJSONContent  []byte
	// This is populated automatically by a header
	CreatedBy string
}
```
None of the fields are required. ValuesYAML should be a path to your values.yaml file locally. However, it doesn't take precedence over the values Helium supplies, which could be a source of confusion. Future work is planned to eliminate this. These params can be used in a request like so:

```shell
curl -X POST -H "Authorization: Bearer ***REMOVED***" -F name=example-workspace-id -F helmVersion=2.2.0-rc.1 -F valuesYaml=@testval.yml https://helium.***REMOVED***/v1/api/workspace
```
Where `testval.yml` is a values.yaml file in my current directory.


#### Deleting a workspace manually:
```shell
curl -X DELETE -H "Authorization: Bearer ***REMOVED***"  https://helium.***REMOVED***/v1/api/workspace/example-workspace-id
```

If needing to implement a polling mechanism in bash for automation purposes, the following might help:

```shell
for _ in $(seq 36); do
  STATUS=$(curl -s -H "Authorization: Bearer ***REMOVED***"  https://helium.***REMOVED***/v1/api/workspace/sean-named-this-108 | jq .Workspace.Status | tr -d '"')
  if [[ ${STATUS} == "ready" ]]
  then
    echo "success"
    break
  fi
  echo 'sleeping'
  sleep 10
done
```

## Advanced Usage

Helium allows you to specify your own values.yaml file. Your values will take precedence, so it's possible to clobber a value helium needs to provision a working environment.

It's less supported, but calling the create api or workspace form with an already existing workspace name will allow you to update that workspace. This could be useful for updating the console image for instance. However, this code pathway is less exercised. If straying from the happy path of mutating image tags, things might not work as expected.  

GPUs and autoprovisioning - It works the same way as it did on Hub. If you correctly specify your pipelines, you can let the workers use GPUs.

Pachd or the other components of the helm chart can have their resource requests and limits set accordingly, and the cluster will autoprovision node pools if possible that meet the requirements of the requests. Limits do not cause autoprovisioning, but are important to specify for reproducible experimentation.

### Changing the Backend

Helium is capable of running many different pulumi programs, which generally live https://github.com/pachyderm/poc-pulumi. Each directory there that contains a pulumi.yaml file is a separate pulumi program, which helium refers to as a backend. Some of those backends are meant to be used inconjunction with one another.  Helium generally spins up standalone clusters with the gcp_cluster_only backend, which can then be pointed to with the -clusterStack parameter to deploy the regular gcp_namespace_only backends on top of it. Same principle also applies to some of the aws backends as well.


### Loadtesting

Additional Nodepools are defined here: https://github.com/pachyderm/infrastructure/blob/master/ci/main.go

One nodepool is adhoc-loadtesting, which will enabled you to scale from 0-8 n2d-standard-32 machines, allowing up to 256 CPU cores and 1 TB of Memory.

```
To enable it for your pipelines, add the following to your pipeline spec:
  "scheduling_spec": {
    "node_selector": {"adhoc-loadtesting": "enabled"},
},
```

To enable it for pachd or other helm chart components, add the following to your values.yaml file:
pachd:
```
  nodeSelector:
    adhoc-loadtesting: "enabled"
 ```
 Don't forget to set the correct memory and cpu requests as well for all of the various components!


## Troubleshooting Workspace Creation

Occasionally a workspace will fail to provision for a wide variety of reasons. The first thing to look at, assuming the workspace got far enough, is clicking through the Pulumi URL to see what the state of the stack was.  Credentials for Pulumi are found in 1password.

Generally the easiest thing to try is just creating a workspace again.

If that fails, try creating a workspace with no parameters to ensure that it's not an issue specific to your parameters.

Another resource would be the slack channel `#helium-sentry` as that'd show you any errors from Helium Code.  Logs from Helium itself are also recorded in stackdriver.  

## Infrastructure

***REMOVED***

***REMOVED***

***REMOVED***

Helium creates and manages the standalone clusters that are then used to create workspaces on top of.
For GCP, these are in the pulumi-ci project. The gcp_cluster_only and aws_cluster_only backends should provide the necessary details.

We're using the pulumi SaaS platform to manage state and concurrency control on any given stack, for the default pulumi_backends/gcp_namespace backend.

Sentry is setup to collect errors and message to the #helium-slack channel.

## Running Instructions:

When running locally, helium uses port `2323`, like so: http://localhost:2323/v1/api/workspaces

You will need the `HELIUM_CLIENT_SECRET` and `HELIUM_CLIENT_ID` environment variables set, both of which can be found in the Prod Auth0 tenant for Hub, with the name `Auth0 for Helium`: https://manage.auth0.com/dashboard/us/***REMOVED***/applications/cwqe6eu76gLVLvmcKsnJRE3tkJrwDIsG/settings

In order to run the API, in a terminal tab run:
```shell
HELIUM_MODE=API HELIUM_CLIENT_SECRET="XXXXXXXXXXXX" HELIUM_CLIENT_ID="XXXXXXXXXX"   go run main.go
```

In another tab in order to run the DeletionController:
```shell
HELIUM_MODE=CONTROLPLANE HELIUM_CLIENT_SECRET="XXXXXXXXXX" HELIUM_CLIENT_ID="XXXXXX"   go run main.go
```
The deletionController automatically queries every environment to check it's expiry, and if it's expired, it automatically deletes it.

## Development Overview

To run the api locally:  `HELIUM_MODE=API HELIUM_CLIENT_SECRET="XXXXXXXXXX" HELIUM_CLIENT_ID="XXXXXXXXX" HELIUM_GITHUB_PERSONAL_TOKEN="XXXXXXXXXX" AWS_ACCESS_KEY_ID="XXXXXXXXXX" AWS_SECRET_ACCESS_KEY="XXXXXXXXXXXX" PULUMI_K8S_DELETE_UNREACHABLE="true" go run main.go` and then you are able to curl the API at http://localhost:2323

pulumi_backends - CRUD operations with the pulumi automation API.

UI is provided by the templates in the /templates directory. They were heavily inspired by the Enterprise Keygen templates, with the additional of Tailwind CSS. Normal go templating is used to process the templates, the relevant handlers are prefixed with UI.

We're leveraging conditional go templating and meta refresh tags to do a version of polling with no javascript, until the workspace transitions away from the creating state.

Backend interface defines the interface any future backends must implement. Notably, the deletion controller is implemented in backend.go.

Sentry and terrors packages provide support for using sentry.

Handlers implement the necessary api and UI http handlers.

We're using mux as the router, it's defined in main.go. A sentry and auth middleware are setup, they live in handlers. main_test.go contains the e2e tests. They are disabled when calling with `go test ./... -short`.  

Testing - it's recommend to call `go test ./... -short` for unit tests, and an e2e test suite can be used without `-short` to exercise the api and create actual workspaces on GCP.  

## Known Issues:

- The tests are broken, but shouldn't affect anything other than controllers


## Renewing workspace wildcard cert

As of 12/02/2022 - this is a manual process.


```
sudo certbot certonly --manual -v \
 --preferred-challenges=dns \
 --email buildbot@***REMOVED*** \
 --server https://acme-v02.api.letsencrypt.org/directory \
 --agree-tos \
 --manual-public-ip-logging-ok \
 -d \*.***REMOVED***
```

Go to GCP cloud console and update the record in ***REMOVED*** zone, it lives in the pulumi-ci project.  

Test that it's updated: https://toolbox.googleapps.com/apps/dig/#TXT/_acme-challenge.***REMOVED***.

Then hit enter on the certbot command.

```
sudo kubectl create secret tls workspace-wildcard --cert /etc/letsencrypt/live/***REMOVED***/fullchain.pem --key /etc/letsencrypt/live/***REMOVED***/privkey.pem --dry-run=client --output=yaml > workspace-wildcard.yaml
```

make sure to add the following annotation:

```
metadata:
  annotations:
    replicator.v1.mittwald.de/replicate-to-matching: needs-workspace-tls=true
```


## Updating an underlying GKE cluster (ie for console preview environments)

Console preview environments currently live in their own GKE cluster: console-preview-cluster, in the pulumi-ci GCP Project.  If needing to update or recreate it for any reason, it's important to set the backend field to gcp_cluster_only, and set the expiry.  The following curl command was used to generate the cluster previously:

```
curl -X POST -H "Authorization: Bearer ***REMOVED***" -F name=console-preview-cluster -F expiry=2023-02-22  -F backend=gcp_cluster_only https://helium.***REMOVED***/v1/api/workspace
```
