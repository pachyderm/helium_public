package gcp_cluster

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/pulumi/pulumi-gcp/sdk/v6/go/gcp/compute"
	"github.com/pulumi/pulumi-gcp/sdk/v6/go/gcp/container"
	"github.com/pulumi/pulumi-gcp/sdk/v6/go/gcp/dns"
	"github.com/pulumi/pulumi-gcp/sdk/v6/go/gcp/projects"
	"github.com/pulumi/pulumi-gcp/sdk/v6/go/gcp/storage"
	"github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes/core/v1"
	helm "github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes/helm/v3"
	"github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes/yaml"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	log "github.com/sirupsen/logrus"

	"github.com/pachyderm/helium/api"
)

const (
	BackendName         = "gcp-cluster"
	timeFormat          = "2006-01-02"
	DefaultJupyterImage = "v0.5.1"
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

func CreatePulumiProgram(id, expiry, helmChartVersion, consoleVersion, pachdVersion, notebooksVersion, valuesYaml, createdBy string, cleanup2 bool, infraJson *api.InfraJson) pulumi.RunFunc {
	return func(ctx *pulumi.Context) error {
		urlSuffix := "***REMOVED***"
		pachdUrl := pulumi.String(id + "-pachd" + "." + urlSuffix)
		url := pulumi.String(id + "." + urlSuffix)

		workspaceManagedZone, err := dns.GetManagedZone(ctx, "workspace", pulumi.ID(pulumi.String(WorkspaceManagedZoneGcpId)), nil)
		if err != nil {
			return err
		}

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
				MaxNodeCount: pulumi.Int(1),
				MinNodeCount: pulumi.Int(1),
			},
			NodeConfig: &container.NodePoolNodeConfigArgs{
				MachineType: pulumi.String("n1-standard-8"),
				//TODO Custom ServiceAccount: nodeSA.Email,
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

		ctx.Export("kubeconfig", generateKubeconfig(cluster.Endpoint, cluster.Name, cluster.MasterAuth))

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

		ctx.Export("ha-proxy-name", haproxyExternalOutput)

		_, err = dns.NewRecordSet(ctx, "ha-proxy-workspace-record-set", &dns.RecordSetArgs{
			Name:        url + ".",
			Type:        pulumi.String("A"),
			Ttl:         pulumi.Int(300),
			ManagedZone: workspaceManagedZone.Name,
			Rrdatas:     pulumi.StringArray{pulumi.Sprintf("%s", haproxyExternalOutput)},
		})
		if err != nil {
			return err
		}

		bucket, err := storage.NewBucket(ctx, "pach-bucket", &storage.BucketArgs{
			Location:     pulumi.String("US"),
			ForceDestroy: pulumi.Bool(true),
		})
		if err != nil {
			return err
		}

		//TODO Create Service account for each pach install and assign to bucket
		defaultSA := compute.GetDefaultServiceAccountOutput(ctx, compute.GetDefaultServiceAccountOutputArgs{}, nil)
		if err != nil {
			return err
		}

		log.Debugf("Service Account: %v", defaultSA.Email())

		_, err = storage.NewBucketIAMMember(ctx, "bucket-role", &storage.BucketIAMMemberArgs{
			Bucket: bucket.Name,
			Role:   pulumi.String("roles/storage.admin"),
			Member: defaultSA.Email().ApplyT(func(s string) string { return "serviceAccount:" + s }).(pulumi.StringOutput),
		})
		if err != nil {
			return err
		}

		_, err = yaml.NewConfigFile(ctx, "workspace-wildcard-tls", &yaml.ConfigFileArgs{
			File: "workspace-wildcard.yaml",
		}, pulumi.Provider(k8sProvider))
		if err != nil {
			return err
		}

		consoleUrl := pulumi.String(id + ".***REMOVED***")

		type JSONoidc struct {
			Issuer       string `json:issuer`
			ClientID     string `json:clientID`
			ClientSecret string `json:clientSecret`
			RedirectURI  string `json:redirectURI`
		}
		oidcInfo := &JSONoidc{
			Issuer:       auth0Domain,
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURI:  fmt.Sprintf("https://%v/dex/callback", consoleUrl),
		}

		jsonOidcBlob, err := json.Marshal(oidcInfo)
		if err != nil {
			return err
		}
		consoleValues := pulumi.Map{
			"enabled": pulumi.Bool(true),
			"config": pulumi.Map{
				"oauthClientID":     pulumi.String("console"),
				"oauthClientSecret": pulumi.String("***REMOVED***"), //# Autogenerated on install if blank
				"graphqlPort":       pulumi.Int(4000),
				"pachdAddress":      pulumi.String("pachd-peer:30653"),
				"disableTelemetry":  pulumi.Bool(false), // # Disables analytics and error data collection
			},
		}
		if consoleVersion != "" {
			consoleValues = pulumi.Map{
				"enabled": pulumi.Bool(true),
				"image": pulumi.Map{
					"tag": pulumi.String(consoleVersion),
				},
				"config": pulumi.Map{
					"oauthClientID":     pulumi.String("console"),
					"oauthClientSecret": pulumi.String("***REMOVED***"), //# Autogenerated on install if blank
					"graphqlPort":       pulumi.Int(4000),
					"pachdAddress":      pulumi.String("pachd-peer:30653"),
					"disableTelemetry":  pulumi.Bool(false), // # Disables analytics and error data collection
				},
			}
		}

		pachdValues := pulumi.Map{
			//"pachAuthClusterRoleBindings": pulumi.Map{
			//	"pachAuthClusterRoleBindings": pulumi.Map{
			//		"allClusterUsers": pulumi.StringArray{pulumi.String("clusterAdmin")},
			//	},
			//},
			"externalService": pulumi.Map{
				"enabled": pulumi.Bool(true),
				//						"loadBalancerIP": ipAddress,
				"apiGRPCPort":   pulumi.Int(30651), //Dynamic Value
				"s3GatewayPort": pulumi.Int(30601), //Dynamic Value
			},
			"oauthClientSecret":    pulumi.String("***REMOVED***"),
			"rootToken":            pulumi.String("***REMOVED***"),
			"enterpriseSecret":     pulumi.String("***REMOVED***"),
			"enterpriseLicenseKey": pulumi.String("***REMOVED***"), //Set in .circleci/config.yml
			"storage": pulumi.Map{
				"google": pulumi.Map{
					"bucket": bucket.Name,
				},
				"tls": pulumi.Map{
					"enabled":    pulumi.Bool(true),
					"secretName": pulumi.String("workspace-wildcard"),
				},
			},
			"annotations": pulumi.Map{
				"cluster-autoscaler.kubernetes.io/safe-to-evict": pulumi.String("true"),
			},
		}
		if pachdVersion != "" {
			pachdValues = pulumi.Map{
				"image": pulumi.Map{
					"tag": pulumi.String(pachdVersion),
				},
				//"pachAuthClusterRoleBindings": pulumi.Map{
				//	"pachAuthClusterRoleBindings": pulumi.Map{
				//		"allClusterUsers": pulumi.StringArray{pulumi.String("clusterAdmin")},
				//	},
				//},
				"externalService": pulumi.Map{
					"enabled": pulumi.Bool(true),
					//						"loadBalancerIP": ipAddress,
					"apiGRPCPort":   pulumi.Int(30651), //Dynamic Value
					"s3GatewayPort": pulumi.Int(30601), //Dynamic Value
				},
				"oauthClientSecret":    pulumi.String("***REMOVED***"),
				"rootToken":            pulumi.String("***REMOVED***"),
				"enterpriseSecret":     pulumi.String("***REMOVED***"),
				"enterpriseLicenseKey": pulumi.String("***REMOVED***"), //Set in .circleci/config.yml
				"storage": pulumi.Map{
					"google": pulumi.Map{
						"bucket": bucket.Name,
					},
					"tls": pulumi.Map{
						"enabled":    pulumi.Bool(true),
						"secretName": pulumi.String("workspace-wildcard"),
					},
				},
				"annotations": pulumi.Map{
					"cluster-autoscaler.kubernetes.io/safe-to-evict": pulumi.String("true"),
				},
			}
		}
		array := []pulumi.AssetOrArchiveInput{}
		array = append(array, pulumi.AssetOrArchiveInput(pulumi.NewFileAsset(valuesYaml)))
		corePach, err := helm.NewRelease(ctx, "pach-release", &helm.ReleaseArgs{
			Atomic:        pulumi.Bool(cleanup2),
			CleanupOnFail: pulumi.Bool(cleanup2),
			Timeout:       pulumi.Int(900),
			Namespace:     namespace.Metadata.Elem().Name(),
			RepositoryOpts: helm.RepositoryOptsArgs{
				Repo: pulumi.String("https://helm.***REMOVED***"), //TODO Use Chart files in Repo
			},
			Version:        pulumi.String(helmChartVersion),
			Chart:          pulumi.String("pachyderm"),
			ValueYamlFiles: pulumi.AssetOrArchiveArray(array), // pulumi.NewFileAsset("./metrics.yml"),
			//
			Values: pulumi.Map{
				"deployTarget": pulumi.String("GOOGLE"),
				"global": pulumi.Map{
					"postgresql": pulumi.Map{
						"postgresqlPassword":         pulumi.String("***REMOVED***"),
						"postgresqlPostgresPassword": pulumi.String("***REMOVED***"),
					},
				},
				"console": consoleValues,
				"ingress": pulumi.Map{
					"annotations": pulumi.Map{
						"kubernetes.io/ingress.class": pulumi.String("haproxy"),
						//	"traefik.ingress.kubernetes.io/router.tls": pulumi.String("true"),
					},
					"enabled": pulumi.Bool(true),
					"host":    consoleUrl,
					"tls": pulumi.Map{
						"enabled":    pulumi.Bool(true),
						"secretName": pulumi.String("workspace-wildcard"), // Dynamic Value
					},
				},
				"pachd": pachdValues,
				"oidc": pulumi.Map{
					"mockIDP": pulumi.Bool(false),
					"upstreamIDPs": pulumi.Array{
						pulumi.Map{
							"id":         pulumi.String("auth0"),
							"name":       pulumi.String("auth0"),
							"type":       pulumi.String("oidc"),
							"jsonConfig": pulumi.String(jsonOidcBlob),
						},
					},
				},
			},
		}, pulumi.DependsOn([]pulumi.Resource{haProxyRelease}), pulumi.Provider(k8sProvider))
		if err != nil {
			return err
		}

		gcpL4LoadBalancerIP := pulumi.All(corePach.Status.Namespace()).ApplyT(func(args []interface{}) (pulumi.StringOutput, error) {
			namespace := args[0].(*string)
			svc, err := corev1.GetService(ctx, "svc", pulumi.ID(fmt.Sprintf("%s/pachd-lb", *namespace)), nil, pulumi.Timeouts(&pulumi.CustomTimeouts{Create: "10m"}), pulumi.Provider(k8sProvider))
			if err != nil {
				log.Errorf("error getting loadbalancer IP: %v", err)
			}
			return svc.Status.LoadBalancer().Ingress().Index(pulumi.Int(0)).Ip().Elem(), nil
		}).(pulumi.StringOutput)

		_, err = dns.NewRecordSet(ctx, "frontendRecordSet", &dns.RecordSetArgs{
			Name: pachdUrl + ".",
			// TODO: This will be a CNAME for AWS?
			Type:        pulumi.String("A"),
			Ttl:         pulumi.Int(300),
			ManagedZone: workspaceManagedZone.Name,
			Rrdatas:     pulumi.StringArray{gcpL4LoadBalancerIP},
		})
		if err != nil {
			return err
		}

		jupyterImage := DefaultJupyterImage
		if notebooksVersion != "" {
			jupyterImage = notebooksVersion
		}
		file, err := ioutil.ReadFile("./root.py")
		if err != nil {
			return err
		}
		fileStr := string(file)

		juypterURL := pulumi.String("jh-" + id + ".***REMOVED***")

		_, err = helm.NewRelease(ctx, "jh-release", &helm.ReleaseArgs{
			Namespace: namespace.Metadata.Elem().Name(),
			RepositoryOpts: helm.RepositoryOptsArgs{
				Repo: pulumi.String("https://jupyterhub.github.io/helm-chart/"),
			},
			Atomic:        pulumi.Bool(cleanup2),
			CleanupOnFail: pulumi.Bool(cleanup2),
			Timeout:       pulumi.Int(600),
			Chart:         pulumi.String("jupyterhub"),
			Values: pulumi.Map{
				"singleuser": pulumi.Map{
					"defaultUrl": pulumi.String("/lab"),
					"image": pulumi.Map{
						"name": pulumi.String("pachyderm/notebooks-user"),
						"tag":  pulumi.String(jupyterImage),
					},
					"cloudMetadata": pulumi.Map{
						"blockWithIptables": pulumi.Bool(false),
					},
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
						"kubernetes.io/ingress.class": pulumi.String("haproxy"),
						//"traefik.ingress.kubernetes.io/router.tls": pulumi.String("true"),
					},
					"hosts": pulumi.StringArray{juypterURL},
					"tls": pulumi.MapArray{
						pulumi.Map{
							"hosts":      pulumi.StringArray{pulumi.String("jh-" + id + ".***REMOVED***")},
							"secretName": pulumi.String("workspace-wildcard"),
						},
					},
				},
				// "hub": pulumi.Map{},//Auth stuff
				"prePuller": pulumi.Map{
					"hook": pulumi.Map{
						"enabled": pulumi.Bool(false),
					},
				},
				"hub": pulumi.Map{
					"extraConfig": pulumi.Map{
						"podRoot": pulumi.String(fileStr),
					},
				},
				"proxy": pulumi.Map{
					"service": pulumi.Map{
						"type": pulumi.String("ClusterIP"),
					},
				},
				"scheduling": pulumi.Map{
					"userScheduler": pulumi.Map{
						"enabled": pulumi.Bool(false),
					},
				},
			},
		}, pulumi.DependsOn([]pulumi.Resource{haProxyRelease}), pulumi.Provider(k8sProvider))

		if err != nil {
			return err
		}

		ctx.Export("createdBy", pulumi.String(createdBy))
		ctx.Export("status", pulumi.String("ready"))
		ctx.Export("pachdip", gcpL4LoadBalancerIP)
		ctx.Export("juypterUrl", juypterURL)
		ctx.Export("consoleUrl", consoleUrl)
		ctx.Export("k8sNamespace", namespace.Metadata.Elem().Name())
		ctx.Export("bucket", bucket.Name)
		ctx.Export("helium-expiry", pulumi.String(expiry))
		ctx.Export("k8sConnection", pulumi.Sprintf("gcloud container clusters get-credentials %s --zone us-east1-b --project ***REMOVED***", cluster.Name))
		ctx.Export("backend", pulumi.String(BackendName))
		ctx.Export("pachd-lb-ip", gcpL4LoadBalancerIP)
		ctx.Export("pachd-lb-url", pachdUrl)

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
