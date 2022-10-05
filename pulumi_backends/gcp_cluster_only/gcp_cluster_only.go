package gcp_cluster_only

import (
	"fmt"
	"os"

	"github.com/pulumi/pulumi-gcp/sdk/v6/go/gcp/container"
	"github.com/pulumi/pulumi-gcp/sdk/v6/go/gcp/projects"
	"github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes/core/v1"
	helm "github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes/helm/v3"
	"github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes/yaml"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	log "github.com/sirupsen/logrus"

	"github.com/pachyderm/helium/api"
)

const (
	BackendName         = "gcp-cluster-only"
	timeFormat          = "2006-01-02"
	DefaultJupyterImage = "v0.6.3"
	// This is an internal GCP ID, not sure if it's exposed at all through pulumi.  I got it by doing a GET call directly against their API here:
	// https://cloud.google.com/dns/docs/reference/v1/managedZones/get?apix_params=%7B%22project%22%3A%22***REMOVED***%22%2C%22managedZone%22%3A%22test-ci%22%7D
	WorkspaceManagedZoneGcpId = "***REMOVED***"
)

var (
	project           = "helium"
	clientSecret      = os.Getenv("HELIUM_CLIENT_SECRET")
	clientID          = os.Getenv("HELIUM_CLIENT_ID")
	expirationNumDays = os.Getenv("HELIUM_DEFAULT_EXPIRATION_DAYS")
	auth0Domain       = "https://***REMOVED***.auth0.com/"
)

func CreatePulumiProgram(id, expiry, helmChartVersion, consoleVersion, pachdVersion, notebooksVersion, valuesYaml, createdBy, clusterStack string, cleanup2 bool, infraJson *api.InfraJson, valuesYamlContent []byte) pulumi.RunFunc {
	return func(ctx *pulumi.Context) error {
		containerService, err := projects.NewService(ctx, "project", &projects.ServiceArgs{
			Service: pulumi.String("container.googleapis.com"),
		})
		if err != nil {
			return err
		}

		cluster, err := container.NewCluster(ctx, id, &container.ClusterArgs{
			InitialNodeCount:      pulumi.Int(1),
			RemoveDefaultNodePool: pulumi.Bool(true),
		}, pulumi.DependsOn([]pulumi.Resource{containerService}), pulumi.DeleteBeforeReplace(true))

		if err != nil {
			return err
		}

		np1, err := container.NewNodePool(ctx, id, &container.NodePoolArgs{
			Cluster:   cluster.ID(),
			NodeCount: pulumi.Int(1),
			Autoscaling: &container.NodePoolAutoscalingArgs{
				MaxNodeCount: pulumi.Int(8),
				MinNodeCount: pulumi.Int(0),
			},
			NodeConfig: &container.NodePoolNodeConfigArgs{
				MachineType: pulumi.String("n1-standard-8"),
				OauthScopes: pulumi.StringArray{
					pulumi.String("https://www.googleapis.com/auth/compute"),
					pulumi.String("https://www.googleapis.com/auth/devstorage.read_write"), //TODO Change back to read-only
					pulumi.String("https://www.googleapis.com/auth/logging.write"),
					pulumi.String("https://www.googleapis.com/auth/monitoring"),
				},
				//SandboxConfig: &container.NodePoolNodeConfigSandboxConfigArgs{
				//	SandboxType: pulumi.String("gvisor"),
				//},
			},
		}, pulumi.DependsOn([]pulumi.Resource{cluster}))
		if err != nil {
			return err
		}

		_, err = container.NewNodePool(ctx, "gpu", &container.NodePoolArgs{
			Cluster:   cluster.ID(),
			NodeCount: pulumi.Int(0),
			Autoscaling: &container.NodePoolAutoscalingArgs{
				MaxNodeCount: pulumi.Int(8),
				MinNodeCount: pulumi.Int(0),
			},
			NodeConfig: &container.NodePoolNodeConfigArgs{
				MachineType: pulumi.String("n1-standard-8"),
				GuestAccelerators: container.NodePoolNodeConfigGuestAcceleratorArray{container.NodePoolNodeConfigGuestAcceleratorArgs{
					Count: pulumi.Int(1),
					Type:  pulumi.String("nvidia-tesla-p100"),
				}},
				OauthScopes: pulumi.StringArray{
					pulumi.String("https://www.googleapis.com/auth/compute"),
					pulumi.String("https://www.googleapis.com/auth/devstorage.read_write"), //TODO Change back to read-only
					pulumi.String("https://www.googleapis.com/auth/logging.write"),
					pulumi.String("https://www.googleapis.com/auth/monitoring"),
				},
				//SandboxConfig: &container.NodePoolNodeConfigSandboxConfigArgs{
				//	SandboxType: pulumi.String("gvisor"),
				//},
			},
		}, pulumi.DependsOn([]pulumi.Resource{cluster}))
		if err != nil {
			return err
		}

		// adhoc loadtesting giant nodepool - this allows us to run
		// 256 CPUs and 1TB of memory split across 4 nodes.
		_, err = container.NewNodePool(ctx, "adhoc-load-test", &container.NodePoolArgs{
			Cluster:   cluster.ID(),
			NodeCount: pulumi.Int(0),
			Autoscaling: &container.NodePoolAutoscalingArgs{
				MaxNodeCount: pulumi.Int(8),
				MinNodeCount: pulumi.Int(0),
			},
			NodeConfig: &container.NodePoolNodeConfigArgs{
				MachineType: pulumi.String("n2d-standard-32"),
				Labels:      pulumi.StringMap{"adhoc-loadtesting": pulumi.String("enabled"), "loadtest": pulumi.String("enabled")},
				OauthScopes: pulumi.StringArray{
					pulumi.String("https://www.googleapis.com/auth/compute"),
					pulumi.String("https://www.googleapis.com/auth/devstorage.read_write"), //TODO Change back to read-only
					pulumi.String("https://www.googleapis.com/auth/logging.write"),
					pulumi.String("https://www.googleapis.com/auth/monitoring"),
				},
			},
		})
		if err != nil {
			return err
		}

		ctx.Export("kubeconfig", generateKubeconfig(cluster.Endpoint, cluster.Name, cluster.MasterAuth))
		ctx.Export("kube-cluster-name", cluster.Name)

		k8sProvider, err := kubernetes.NewProvider(ctx, "k8sprovider", &kubernetes.ProviderArgs{
			Kubeconfig: generateKubeconfig(cluster.Endpoint, cluster.Name, cluster.MasterAuth),
		}, pulumi.DependsOn([]pulumi.Resource{cluster, np1}))
		if err != nil {
			return err
		}

		namespace, err := corev1.GetNamespace(ctx, "default-ns", pulumi.ID(pulumi.String("default")), nil, pulumi.Provider(k8sProvider))
		if err != nil {
			return err
		}
		haProxyRelease, err := helm.NewRelease(ctx, "ha-proxy", &helm.ReleaseArgs{
			RepositoryOpts: helm.RepositoryOptsArgs{
				Repo: pulumi.String("https://haproxytech.github.io/helm-charts"),
			},
			Chart: pulumi.String("kubernetes-ingress"),
			Values: pulumi.Map{
				"controller": pulumi.Map{
					"ingressClassResource": pulumi.Map{
						"name": pulumi.String("haproxy"),
					},
					"service": pulumi.Map{
						"type": pulumi.String("LoadBalancer"),
					},
				},
				//				"fullnameOverride": pulumi.String("haproxy-s"),
			},
			Namespace: namespace.Metadata.Elem().Name(),
			Timeout:   pulumi.Int(300),
		}, pulumi.Provider(k8sProvider))

		if err != nil {
			return err
		}

		haproxyExternalOutput := pulumi.All(namespace.Metadata.Elem().Name(), haProxyRelease.Status.Name()).ApplyT(func(args []interface{}) (interface{}, error) {
			//arr := r.([]interface{})
			namespace := args[0].(*string)
			svcName := args[1].(*string)
			svc, err := corev1.GetService(ctx, "ha-proxy-svc", pulumi.ID(fmt.Sprintf("%s/%s-kubernetes-ingress", *namespace, *svcName)), nil, pulumi.Timeouts(&pulumi.CustomTimeouts{Create: "15m"}), pulumi.Provider(k8sProvider))
			if err != nil {
				log.Errorf("error getting loadbalancer IP: %v", err)
				return nil, err
			}
			return svc.Status.LoadBalancer().Ingress().Index(pulumi.Int(0)).Ip().Elem(), nil
		})

		// TODO: use this IP in stack
		ctx.Export("ha-proxy-name", haproxyExternalOutput)

		// TODO: make the workspace wildcard cert work with namespaces
		_, err = helm.NewRelease(ctx, "kube-replicator", &helm.ReleaseArgs{
			RepositoryOpts: helm.RepositoryOptsArgs{
				Repo: pulumi.String("https://helm.mittwald.de"),
			},
			Chart: pulumi.String("kubernetes-replicator"),
		}, pulumi.Provider(k8sProvider))

		if err != nil {
			return err
		}

		_, err = yaml.NewConfigFile(ctx, "nvidia-driver-installer", &yaml.ConfigFileArgs{
			File: "gpu-daemonset.yaml",
		}, pulumi.Provider(k8sProvider))
		if err != nil {
			return err
		}

		_, err = yaml.NewConfigFile(ctx, "workspace-wildcard-tls", &yaml.ConfigFileArgs{
			File: "workspace-wildcard.yaml",
		}, pulumi.Provider(k8sProvider))
		if err != nil {
			return err
		}

		ctx.Export("createdBy", pulumi.String(createdBy))
		ctx.Export("status", pulumi.String("ready"))
		ctx.Export("pachdip", pulumi.String(""))
		ctx.Export("juypterUrl", pulumi.String(""))
		ctx.Export("consoleUrl", pulumi.String(""))
		ctx.Export("k8sNamespace", namespace.Metadata.Elem().Name())
		ctx.Export("bucket", pulumi.String(""))
		ctx.Export("helium-expiry", pulumi.String(expiry))
		ctx.Export("k8sConnection", pulumi.Sprintf("gcloud container clusters get-credentials %s --zone us-east1-b --project ***REMOVED***", cluster.Name))
		ctx.Export("backend", pulumi.String(BackendName))
		ctx.Export("pachd-lb-ip", pulumi.String(""))
		ctx.Export("pachd-lb-url", pulumi.String(""))

		return nil
	}
}

func generateKubeconfig(clusterEndpoint pulumi.StringOutput, clusterName pulumi.StringOutput,
	clusterMasterAuth container.ClusterMasterAuthOutput) pulumi.StringOutput {
	context := pulumi.Sprintf("demo_%s", clusterName)

	return pulumi.Sprintf(`apiVersion: v1
clusters:
- cluster:
    certificate-authority-data: %s
    server: https://%s
  name: %s
contexts:
- context:
    cluster: %s
    user: %s
  name: %s
current-context: %s
kind: Config
preferences: {}
users:
- name: %s
  user:
    auth-provider:
      config:
        cmd-args: config config-helper --format=json
        cmd-path: gcloud
        expiry-key: '{.credential.token_expiry}'
        token-key: '{.credential.access_token}'
      name: gcp`,
		clusterMasterAuth.ClusterCaCertificate().Elem(),
		clusterEndpoint, context, context, context, context, context, context)
}
