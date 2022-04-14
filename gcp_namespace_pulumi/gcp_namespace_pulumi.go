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
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optup"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/pachyderm/helium/api"
	"github.com/pachyderm/helium/runner"
	"github.com/pachyderm/helium/util"
)

// This implementation is mostly a thin wrapper around https://github.com/pachyderm/pulumihttp/
func testing() {
	fmt.Println("hello")
}

func init() {
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

const StackNamePrefix = "sean-testing"

var project = "pulumi_over_http"

type Runner struct {
}

func (r *Runner) Get(i api.ID) (string, error) {
	return "", nil
}

func (r *Runner) List() ([]api.ID, error) {
	return nil, nil
}

func (r *Runner) IsPrewarmInfra(i api.ID) (bool, error) {
	return false, nil
}
func (r *Runner) IsPrewarmWorkspace(i api.ID) (bool, error) {
	return false, nil
}

func (r *Runner) IsExpired(i api.ID) (bool, error) {
	return false, nil
}

func (r *Runner) Destroy(i api.ID) error {
	return nil
}

func (r *Runner) ProvisionInfra() (api.ID, error) {
	return "", nil
}

// Does this need an ID as a param?
func (r *Runner) ProvisionWorkspace() (api.ID, error) {
	return "", nil
}

func (r *Runner) Create(req api.CreateRequest) (api.CreateResponse, error) {

	ctx := context.Background()

	stackName := fmt.Sprintf("%s-%s", StackNamePrefix, util.RandomString(6))
	program := createPulumiProgram(stackName)

	s, err := auto.NewStackInlineSource(ctx, stackName, project, program)
	if err != nil {
		// TODO: throw a 409 error if stack already exists
		// if stack already exists, 409
		//if auto.IsCreateStack409Error(err) {
		//	w.WriteHeader(409)
		//	fmt.Fprintf(w, fmt.Sprintf("stack %q already exists", stackName))
		//	return
		//}

		//w.WriteHeader(500)
		//fmt.Fprintf(w, err.Error())
		return api.CreateResponse{}, err
	}
	s.SetConfig(ctx, "gcp:project", auto.ConfigValue{Value: "***REMOVED***"})
	s.SetConfig(ctx, "gcp:zone", auto.ConfigValue{Value: "us-east1-b"})

	// deploy the stack
	// we'll write all of the update logs to st	out so we can watch requests get processed
	_, err = s.Up(ctx, optup.ProgressStreams(os.Stdout))
	if err != nil {
		return api.CreateResponse{}, err
	}

	return api.CreateResponse{api.ID(stackName)}, nil
}

func (r *Runner) RestoreSeedData(bucket string) error {
	return nil
}

func (r *Runner) Register() *api.Backend {
	return &api.Backend{"gcp", "namespace", "pulumi"}
}

func (r *Runner) Setup() {
	ensurePlugins()
}

func (r *Runner) Controller() []runner.ControlLoops {
	return []runner.ControlLoops{
		runner.PrewarmWorkspaceLoop,
		runner.PrewarmInfraLoop,
		runner.DeletionControllerLoop,
	}
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
			//Name:     pulumi.String(id),
			Location: pulumi.String("US"),
			/*
				Labels: pulumi.StringMap{
					"workspace": pulumi.String("myfinecluster"),
				},
			*/
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
					"host":    pulumi.String(id + ".***REMOVED***"),
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
					"hosts": pulumi.StringArray{pulumi.String("jh-" + id + ".***REMOVED***")},
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
