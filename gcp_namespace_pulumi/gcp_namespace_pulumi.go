package gcp_namespace_pulumi

import (
	"context"
	"fmt"
	"os"

	"github.com/pulumi/pulumi-gcp/sdk/v6/go/gcp/compute"
	"github.com/pulumi/pulumi-gcp/sdk/v6/go/gcp/storage"
	"github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes/core/v1"
	helm "github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes/helm/v3"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optdestroy"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optup"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/pulumi/pulumi/sdk/v3/go/common/workspace"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/pachyderm/helium/api"
	"github.com/pachyderm/helium/backend"
	"github.com/pachyderm/helium/util"

	log "github.com/sirupsen/logrus"
)

// This implementation is mostly a thin wrapper around https://github.com/pachyderm/pulumihttp/
func init() {
	ensurePlugins()
}

//
const (
	BackendName     = "gcp-namespace-pulumi"
	StackNamePrefix = "sean-testing"
)

var project = "pulumi_over_http"

type Runner struct {
	Name backend.Name
}

func (r *Runner) GetConnectionInfo(i api.ID) (*api.GetConnectionInfoResponse, error) {
	log.SetReportCaller(true)
	log.SetLevel(log.DebugLevel)
	log.Infof("get request")

	stackName := string(i)
	// we don't need a program since we're just getting stack outputs
	var program pulumi.RunFunc = nil
	ctx := context.Background()
	s, err := auto.SelectStackInlineSource(ctx, stackName, project, program)
	if err != nil {
		// if the stack doesn't already exist, 404
		if auto.IsSelectStack404Error(err) {
			return nil, fmt.Errorf("stack %q not found: %w", stackName, err)
		}
		return nil, err
	}
	//
	// fetch the outputs from the stack
	outs, err := s.Outputs(ctx)
	if err != nil {
		return nil, err
	}

	return &api.GetConnectionInfoResponse{ConnectionInfo: api.ConnectionInfo{
		K8s:          "gcloud container clusters get-credentials ci-cluster-b9c3629 --zone us-east1-b --project ***REMOVED***",
		K8sNamespace: outs["k8sNamespace"].Value.(string),
		ConsoleURL:   "https://" + outs["consoleUrl"].Value.(string),
		NotebooksURL: "https://" + outs["juypterUrl"].Value.(string),
		Pachctl:      outs["pachdAddress"].Value.(string),
	}}, nil
}

func (r *Runner) List() (*api.ListResponse, error) {
	log.SetReportCaller(true)
	log.SetLevel(log.DebugLevel)
	log.Infof("list request")

	ctx := context.Background()
	// set up a workspace with only enough information for the list stack operations
	ws, err := auto.NewLocalWorkspace(ctx, auto.Project(workspace.Project{
		Name:    tokens.PackageName(project),
		Runtime: workspace.NewProjectRuntimeInfo("go", nil),
	}))
	if err != nil {
		return nil, err
	}
	stacks, err := ws.ListStacks(ctx)
	if err != nil {
		return nil, err
	}
	var ids []api.ID
	for _, stack := range stacks {
		ids = append(ids, api.ID(stack.Name))
	}
	log.Debugf("list ids: %v", ids)
	return &api.ListResponse{IDs: ids}, nil
}

func (r *Runner) IsExpired(i api.ID) (bool, error) {
	return false, nil
}

func (r *Runner) Create(req *api.CreateRequest) (*api.CreateResponse, error) {

	ctx := context.Background()

	stackName := util.Name()
	program := createPulumiProgram(stackName)

	s, err := auto.NewStackInlineSource(ctx, stackName, project, program)
	if err != nil {
		return nil, err
	}
	s.SetConfig(ctx, "gcp:project", auto.ConfigValue{Value: "***REMOVED***"})
	s.SetConfig(ctx, "gcp:zone", auto.ConfigValue{Value: "us-east1-b"})

	// deploy the stack
	// we'll write all of the update logs to st	out so we can watch requests get processed
	_, err = s.Up(ctx, optup.ProgressStreams(os.Stdout))
	if err != nil {
		return nil, err
	}

	return &api.CreateResponse{api.ID(stackName)}, nil
}

func (r *Runner) Destroy(i api.ID) error {
	log.SetReportCaller(true)
	log.SetLevel(log.DebugLevel)
	log.Infof("destroy request")

	ctx := context.Background()
	stackName := string(i)
	// program doesn't matter for destroying a stack
	program := createPulumiProgram("")

	s, err := auto.SelectStackInlineSource(ctx, stackName, project, program)
	if err != nil {
		// if stack doesn't already exist, 404
		if auto.IsSelectStack404Error(err) {
			log.Errorf("stack %q not found", stackName)
			return err
		}
		return err
	}
	s.SetConfig(ctx, "gcp:project", auto.ConfigValue{Value: "***REMOVED***"})
	s.SetConfig(ctx, "gcp:zone", auto.ConfigValue{Value: "us-east1-b"})
	// destroy the stack
	// we'll write all of the logs to stdout so we can watch requests get processed
	_, err = s.Destroy(ctx, optdestroy.ProgressStreams(os.Stdout))
	if err != nil {
		return err
	}

	// delete the stack and all associated history and config
	err = s.Workspace().RemoveStack(ctx, stackName)
	if err != nil {
		return err
	}
	log.Infof("deleted all associated stack information with: %s", stackName)
	return nil
}

func (r *Runner) Register() *api.CreateRequest {
	return &api.CreateRequest{ //ApiDefaultRequest: api.ApiDefaultRequest{
		Backend: BackendName, //}
	}
}

// func New() []backend.Controller { //[]Somethings
// 	r.Name = BackendName
// 	return []backend.Controller{
// 		r.DeletionController,
// 	}
// }

func (r *Runner) Controller(ctx context.Context) []backend.Controller {
	return []backend.Controller{
		r.DeletionController,
	}
}

func (r *Runner) DeletionController(ctx context.Context) error {
	return backend.RunDeletionController(ctx, r)
}

func createPulumiProgram(id string) pulumi.RunFunc {
	return func(ctx *pulumi.Context) error {

		slug := "pachyderm/ci-cluster/dev"
		stackRef, _ := pulumi.NewStackReference(ctx, slug, nil)

		kubeConfig := stackRef.GetOutput(pulumi.String("kubeconfig"))

		k8sProvider, err := kubernetes.NewProvider(ctx, "k8sprovider", &kubernetes.ProviderArgs{
			Kubeconfig: pulumi.StringOutput(kubeConfig),
		}) //, pulumi.DependsOn([]pulumi.Resource{cluster})
		if err != nil {
			return err
		}

		bucket, err := storage.NewBucket(ctx, "pach-bucket", &storage.BucketArgs{
			Location: pulumi.String("US"),
		})
		if err != nil {
			return err
		}

		//TODO Create Service account for each pach install and assign to bucket
		defaultSA := compute.GetDefaultServiceAccountOutput(ctx, compute.GetDefaultServiceAccountOutputArgs{}, nil)
		if err != nil {
			return err
		}

		_, err = storage.NewBucketIAMMember(ctx, "bucket-role", &storage.BucketIAMMemberArgs{
			Bucket: bucket.Name,
			Role:   pulumi.String("roles/storage.admin"),
			Member: defaultSA.Email().ApplyT(func(s string) string { return "serviceAccount:" + s }).(pulumi.StringOutput),
		})
		if err != nil {
			return err
		}

		namespace, err := corev1.NewNamespace(ctx, "pach-ns", &corev1.NamespaceArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Name: pulumi.String(id),
				Labels: pulumi.StringMap{
					"needs-ci-tls": pulumi.String("true"), //Uses kubernetes replicator to replicate TLS secret to new NS
				},
			},
		}, pulumi.Provider(k8sProvider))

		if err != nil {
			return err
		}

		consoleUrl := pulumi.String(id + ".***REMOVED***")

		_, err = helm.NewRelease(ctx, "pach-release", &helm.ReleaseArgs{
			Namespace: namespace.Metadata.Elem().Name(),
			RepositoryOpts: helm.RepositoryOptsArgs{
				Repo: pulumi.String("https://helm.***REMOVED***"), //TODO Use Chart files in Repo
			},
			Chart: pulumi.String("pachyderm"),
			Values: pulumi.Map{
				"deployTarget": pulumi.String("GOOGLE"),
				"console": pulumi.Map{
					"enabled": pulumi.Bool(true),
				},
				"ingress": pulumi.Map{
					"annotations": pulumi.Map{
						"kubernetes.io/ingress.class":              pulumi.String("traefik"),
						"traefik.ingress.kubernetes.io/router.tls": pulumi.String("true"),
					},
					"enabled": pulumi.Bool(true),
					"host":    consoleUrl,
					"tls": pulumi.Map{
						"enabled":    pulumi.Bool(true),
						"secretName": pulumi.String("wildcard-tls"), // Dynamic Value
					},
				},
				"pachd": pulumi.Map{
					/*"externalService": pulumi.Map{
						"enabled":        pulumi.Bool(true),
						"loadBalancerIP": ipAddress,
						"apiGRPCPort":    pulumi.Int(30651), //Dynamic Value
						"s3GatewayPort":  pulumi.Int(30601), //Dynamic Value
					},*/
					"enterpriseLicenseKey": pulumi.String("***REMOVED***"), //Set in .circleci/config.yml
					"storage": pulumi.Map{
						"google": pulumi.Map{
							"bucket": bucket.Name,
						},
						"tls": pulumi.Map{
							"enabled":    pulumi.Bool(true),
							"secretName": pulumi.String("wildcard-tls"),
						},
					},
				},
			},
		}, pulumi.Provider(k8sProvider))

		if err != nil {
			return err
		}

		juypterURL := pulumi.String("jh-" + id + ".***REMOVED***")

		_, err = helm.NewRelease(ctx, "jh-release", &helm.ReleaseArgs{
			Namespace: namespace.Metadata.Elem().Name(),
			RepositoryOpts: helm.RepositoryOptsArgs{
				Repo: pulumi.String("https://jupyterhub.github.io/helm-chart/"),
			},
			Chart: pulumi.String("jupyterhub"),
			Values: pulumi.Map{
				"singleuser": pulumi.Map{
					"defaultUrl": pulumi.String("/lab"),
					"image": pulumi.Map{
						"name": pulumi.String("pachyderm/notebooks-user"),
						"tag":  pulumi.String("904d4965029e35a434c7c049a0470d9e4800c990"),
					},
					//cloudMetadata
					"cmd":   pulumi.String("start-singleuser.sh"),
					"uid":   pulumi.Int(0),
					"fsGid": pulumi.Int(0),
					"extraEnv": pulumi.Map{
						"GRANT_SUDO":         pulumi.String("yes"),
						"NOTEBOOK_ARGS":      pulumi.String("--allow-root"),
						"JUPYTER_ENABLE_LAB": pulumi.String("yes"),
						"CHOWN_HOME":         pulumi.String("yes"),
						"CHOWN_HOME_OPTS":    pulumi.String("-R"),
					},
					//profileList
				},
				//cull
				"ingress": pulumi.Map{
					"enabled": pulumi.Bool(true),
					"annotations": pulumi.Map{
						"kubernetes.io/ingress.class":              pulumi.String("traefik"),
						"traefik.ingress.kubernetes.io/router.tls": pulumi.String("true"),
					},
					"hosts": pulumi.StringArray{juypterURL},
					"tls": pulumi.MapArray{
						pulumi.Map{
							"hosts":      pulumi.StringArray{pulumi.String("jh-" + id + ".***REMOVED***")},
							"secretName": pulumi.String("wildcard-tls"),
						},
					},
				},
				// "hub": pulumi.Map{},//Auth stuff
				"proxy": pulumi.Map{
					"service": pulumi.Map{
						"type": pulumi.String("ClusterIP"),
					},
				},
			},
		}, pulumi.Provider(k8sProvider))

		if err != nil {
			return err
		}
		ctx.Export("juypterUrl", juypterURL)
		ctx.Export("consoleUrl", consoleUrl)
		ctx.Export("pachdAddress", consoleUrl)
		//ctx.Export("kubeConfig", kubeConfig)
		ctx.Export("k8sNamespace", namespace.Metadata.Elem().Name())
		ctx.Export("bucket", bucket.Name)

		return nil
	}
}

func ensurePlugins() {
	ctx := context.Background()
	w, err := auto.NewLocalWorkspace(ctx)
	if err != nil {
		fmt.Printf("Failed to setup and run http server: %v\n", err)
		os.Exit(1)
	}
	err = w.InstallPlugin(ctx, "gcp", "v6.5.0")
	if err != nil {
		fmt.Printf("Failed to install program plugins: %v\n", err)
		os.Exit(1)
	}
	err = w.InstallPlugin(ctx, "kubernetes", "v3.12.1")
	if err != nil {
		fmt.Printf("Failed to install program plugins: %v\n", err)
		os.Exit(1)
	}
}
