package gcp_namespace_only

import (
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/pulumi/pulumi-gcp/sdk/v6/go/gcp/compute"
	"github.com/pulumi/pulumi-gcp/sdk/v6/go/gcp/container"
	"github.com/pulumi/pulumi-gcp/sdk/v6/go/gcp/dns"
	"github.com/pulumi/pulumi-gcp/sdk/v6/go/gcp/storage"
	"github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes"
	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes/core/v1"
	helm "github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes/helm/v3"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
	log "github.com/sirupsen/logrus"

	"gopkg.in/yaml.v3"

	"github.com/pachyderm/helium/util"
)

const (
	BackendName         = "gcp-namespace-only"
	timeFormat          = "2006-01-02"
	DefaultJupyterImage = "v0.6.3"
	// This is an internal GCP ID, not sure if it's exposed at all through pulumi.  I got it by doing a GET call directly against their API here:
	// https://cloud.google.com/dns/docs/reference/v1/managedZones/get?apix_params=%7B%22project%22%3A%22***REMOVED***%22%2C%22managedZone%22%3A%22test-ci%22%7D
	WorkspaceManagedZoneGcpId = "***REMOVED***"
)

var (
	project = "helium"
)

func CreatePulumiProgram(id, expiry, helmChartVersion, consoleVersion, pachdVersion, notebooksVersion, valuesYaml, createdBy, clusterStack string, cleanup2, disableNotebooks bool, infraJson *api.InfraJson, valuesYamlContent []byte) pulumi.RunFunc {
	return func(ctx *pulumi.Context) error {
		conf := config.New(ctx, project)

		id := conf.Require("id")
		expiry := conf.Require("expiry")
		helmChartVersion := conf.Require("helm-chart-version")
		consoleVersion := conf.Require("console-version")
		pachdVersion := conf.Require("pachd-version")
		notebooksVersion := conf.Require("notebooks-version")
		valuesYamlContent := conf.Require("pachd-values-content")
		createdBy := conf.Require("created-by")
		clusterStack := conf.Require("cluster-stack")
		cleanup := conf.Require("cleanup-on-failure")
		workspaceManagedZoneGcpId := conf.Require("workspace-managed-zone-gcp-id")
		clientSecret := conf.Require("client-secret")
		clientID := conf.Require("client-id")
		authDomain := conf.Require("auth-domain")
		authSubDomain := conf.Require("auth-subdomain")
		postgresPassword := conf.Require("postgres-password")
		postgresPgPassword := conf.Require("postgres-pg-password")
		consoleOauthClientSecret := conf.Require("console-oauthClientSecret")
		pachdOauthClientSecret := conf.Require("pachd-oauthClientSecret")
		pachdRootToken := conf.Require("pachd-root-token")
		pachdEnterpriseSecret := conf.Require("pachd-enterprise-secret")
		pachdEnterpriseLicense := conf.Require("pachd-enterprise-license")

		cleanup2, err := strconv.ParseBool(cleanup)
		if err != nil {
			return err
		}

		slug := "pachyderm/helium/default-cluster"
		if clusterStack != "" {
			slug = clusterStack
		}
		stackRef, _ := pulumi.NewStackReference(ctx, slug, nil)

		kubeConfig := stackRef.GetOutput(pulumi.String("kubeconfig"))
		kubeName := stackRef.GetOutput(pulumi.String("kube-cluster-name"))

		k8sProvider, err := kubernetes.NewProvider(ctx, "k8sprovider", &kubernetes.ProviderArgs{
			Kubeconfig: pulumi.StringOutput(kubeConfig),
		}) //, pulumi.DependsOn([]pulumi.Resource{cluster})
		if err != nil {
			return err
		}

		urlSuffix := "***REMOVED***"
		url := pulumi.String(id + "." + urlSuffix)
		jupyterURL := pulumi.String("jh-" + id + "." + urlSuffix)

		workspaceManagedZone, err := dns.GetManagedZone(ctx, "workspace", pulumi.ID(pulumi.String(workspaceManagedZoneGcpId)), nil)
		if err != nil {
			return err
		}

		haproxyExternalOutput := stackRef.GetOutput(pulumi.String("ha-proxy-name"))

		namespace, err := corev1.NewNamespace(ctx, "pach-ns", &corev1.NamespaceArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Name: pulumi.String(id),
				Labels: pulumi.StringMap{
					"needs-workspace-tls": pulumi.String("true"), //Uses kubernetes replicator to replicate TLS secret to new NS
					"helium-expiry":       pulumi.String(expiry),
				},
			},
		}, pulumi.Provider(k8sProvider))

		if err != nil {
			return err
		}

		if !disableNotebooks {
			_, err = dns.NewRecordSet(ctx, "jh-record-set", &dns.RecordSetArgs{
				Name:        jupyterURL + ".",
				Type:        pulumi.String("A"),
				Ttl:         pulumi.Int(300),
				ManagedZone: workspaceManagedZone.Name,
				Rrdatas:     pulumi.StringArray{pulumi.Sprintf("%s", haproxyExternalOutput)},
			})
			if err != nil {
				return err
			}
		}

		bucket, err := storage.NewBucket(ctx, "pach-bucket", &storage.BucketArgs{
			Name:         pulumi.String(id),
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
		consoleRedirectURI := fmt.Sprintf("https://%v/dex/callback", url)
		oicdRedirectURI := fmt.Sprintf("https://%v/dex", url)

		defaultValues := fmt.Sprintf(`deployTarget: "GOOGLE"
global:
  postgresql:
    postgresqlPassword: %s
    postgresqlPostgresPassword: %s
console:
  enabled: true
  config:
    oauthClientID: "console"
    oauthClientSecret: %s
    graphqlPort: 4000
    pachdAddress: "pachd-peer:30653"
    disableTelemetry: false
pachd:
  oidcRedirectURI: %s
  oauthClientSecret: %s
  rootToken: %s
  enterpriseSecret: %s
  enterpriseLicenseKey: %s
  annotations:
    "cluster-autoscaler.kubernetes.io/safe-to-evict": "true"
proxy:
  enabled: true
  tls:
    enabled: true
  service:
    type: "LoadBalancer"
oidc:
  mockIDP: false
`, postgresPassword, postgresPgPassword, consoleOauthClientSecret, consoleRedirectURI, pachdOauthClientSecret, pachdRootToken, pachdEnterpriseSecret, pachdEnterpriseLicense)

		heliumValues := fmt.Sprintf(`
proxy:
  host: %s
  tls:
    secretName: %s
oidc:
  issuerURI: %s
  upstreamIDPs:
  - id: "auth0"
    name: "auth0"
    type: "oidc"
    config:
      issuer:  %s
      clientID: %s
      clientSecret: %s
      redirectURI:  %s
pachd:
  storage:
    google:
      bucket: %s
`, url, "workspace-wildcard", oicdRedirectURI, authDomain, clientID, clientSecret, consoleRedirectURI, id)

		if pachdVersion != "" {
			pachdVersionYaml := fmt.Sprintf(`  image:
    tag: %s
`, pachdVersion)
			heliumValues = heliumValues + "\n" + pachdVersionYaml

		}

		if consoleVersion != "" {
			consoleVersionYaml := fmt.Sprintf(`console:
  image:
    tag: %s
`, consoleVersion)
			heliumValues = heliumValues + "\n" + consoleVersionYaml
		}

		out := map[string]any{}
		if err := yaml.Unmarshal([]byte(defaultValues), &out); err != nil {
			return err
		}

		in := map[string]any{}
		if err := yaml.Unmarshal([]byte(heliumValues), &in); err != nil {
			return err
		}

		intermediate := util.MergeMaps(out, in)

		user := map[string]any{}
		if err := yaml.Unmarshal([]byte(valuesYamlContent), &user); err != nil {
			return err
		}

		final := util.MergeMaps(intermediate, user)

		values := util.ToPulumi(final)

		// DEBUG
		// import "github.com/davecgh/go-spew/spew"
		// spew.Dump(values)

		corePach, err := helm.NewChart(ctx, id, helm.ChartArgs{
			Namespace: pulumi.String(id),
			FetchArgs: helm.FetchArgs{
				Repo: pulumi.String("https://helm.***REMOVED***"),
			},
			Version: pulumi.String(helmChartVersion),
			Chart:   pulumi.String("pachyderm"),
			Values:  values.(pulumi.MapInput),
		}, pulumi.Provider(k8sProvider))
		if err != nil {
			return err
		}

		gcpL4LoadBalancerIP := corePach.GetResource("v1/Service", "pachyderm-proxy", id).ApplyT(func(r interface{}) (pulumi.StringOutput, error) {
			svc := r.(*corev1.Service)
			return svc.Status.LoadBalancer().Ingress().Index(pulumi.Int(0)).Ip().Elem(), nil
		}).(pulumi.StringOutput)

		_, err = dns.NewRecordSet(ctx, "frontendRecordSet", &dns.RecordSetArgs{
			Name: url + ".",
			// TODO: This will be a CNAME for AWS?
			Type:        pulumi.String("A"),
			Ttl:         pulumi.Int(300),
			ManagedZone: workspaceManagedZone.Name,
			Rrdatas:     pulumi.StringArray{gcpL4LoadBalancerIP},
		})
		if err != nil {
			return err
		}

		if !disableNotebooks {
			nbUserImage := "pachyderm/notebooks-user" + ":" + DefaultJupyterImage
			//	jupyterImage := DefaultJupyterImage
			if notebooksVersion != "" {
				nbUserImage = "pachyderm/notebooks-user:" + notebooksVersion
			}

			pachdImage := corePach.GetResource("apps/v1/Deployment", "pachd", id).ApplyT(func(r interface{}) pulumi.StringOutput {
				return r.(*appsv1.Deployment).Spec.Elem().Template().Spec().Elem().Containers().Index(pulumi.Int(0)).Image().Elem()
			}).(pulumi.StringOutput)
			mountServerImage := pachdImage.ApplyT(func(image string) string {
				if notebooksVersion != "" {
					return "pachyderm/mount-server:" + notebooksVersion
				}
				return "pachyderm/mount-server:" + strings.Split(image, ":")[1]
			}).(pulumi.StringOutput)

			// file, err := ioutil.ReadFile("./root.py")
			volumeFile, err := ioutil.ReadFile("./volume.py")
			if err != nil {
				return err
			}
			volumeStr := string(volumeFile)
			// fileStr := string(file)

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
						"cloudMetadata": pulumi.Map{
							"blockWithIptables": pulumi.Bool(false),
						},
						"profileList": pulumi.MapArray{
							pulumi.Map{
								"display_name": pulumi.String("combined"),
								"description":  pulumi.String("Run mount server in Jupyter container"),
								"slug":         pulumi.String("combined"),
								"default":      pulumi.Bool(true),
								"kubespawner_override": pulumi.Map{
									"image":  pulumi.String(nbUserImage),
									"cmd":    pulumi.String("start-singleuser.sh"),
									"uid":    pulumi.Int(0),
									"fs_gid": pulumi.Int(0),
									"environment": pulumi.Map{
										"GRANT_SUDO":         pulumi.String("yes"),
										"NOTEBOOK_ARGS":      pulumi.String("--allow-root"),
										"JUPYTER_ENABLE_LAB": pulumi.String("yes"),
										"CHOWN_HOME":         pulumi.String("yes"),
										"CHOWN_HOME_OPTS":    pulumi.String("-R"),
									},
									"container_security_context": pulumi.Map{
										"allowPrivilegeEscalation": pulumi.Bool(true),
										"runAsUser":                pulumi.Int(0),
										"privileged":               pulumi.Bool(true),
										"capabilities": pulumi.Map{
											"add": pulumi.StringArray{pulumi.String("SYS_ADMIN")},
										},
									},
								},
							},
							pulumi.Map{

								"display_name": pulumi.String("sidecar"),
								"slug":         pulumi.String("sidecar"),
								"description":  pulumi.String("Run mount server as a sidecar"),
								"kubespawner_override": pulumi.Map{
									"image": pulumi.String(nbUserImage),
									"environment": pulumi.Map{
										"SIDECAR_MODE": pulumi.String("true"),
									},
									"extra_containers": pulumi.MapArray{
										pulumi.Map{
											"name":    pulumi.String("mount-server-manager"),
											"image":   mountServerImage,
											"command": pulumi.StringArray{pulumi.String("/bin/bash"), pulumi.String("-c"), pulumi.String("mount-server")},
											"securityContext": pulumi.Map{
												"privileged": pulumi.Bool(true),
												"runAsUser":  pulumi.Int(0),
											},
											"volumeMounts": pulumi.MapArray{
												pulumi.Map{
													"name":             pulumi.String("shared-pfs"),
													"mountPath":        pulumi.String("/pfs"),
													"mountPropagation": pulumi.String("Bidirectional"),
												},
											},
										},
									},
								},
							},
						},
					},
					//cull
					"ingress": pulumi.Map{
						"enabled": pulumi.Bool(true),
						"annotations": pulumi.Map{
							"kubernetes.io/ingress.class": pulumi.String("haproxy"),
							//"traefik.ingress.kubernetes.io/router.tls": pulumi.String("true"),
						},
						"hosts": pulumi.StringArray{jupyterURL},
						"tls": pulumi.MapArray{
							pulumi.Map{
								"hosts":      pulumi.StringArray{jupyterURL},
								"secretName": pulumi.String("workspace-wildcard"),
							},
						},
					},
					"prePuller": pulumi.Map{
						"hook": pulumi.Map{
							"enabled": pulumi.Bool(false),
						},
					},
					"hub": pulumi.Map{
						"config": pulumi.Map{
							"Auth0OAuthenticator": pulumi.Map{
								"client_id":          pulumi.String(clientID),
								"client_secret":      pulumi.String(clientSecret),
								"oauth_callback_url": pulumi.String("https://" + jupyterURL + "/hub/oauth_callback"),
								"scope":              pulumi.StringArray{pulumi.String("openid"), pulumi.String("email")},
								"auth0_subdomain":    pulumi.String(auth0SubDomain),
							},
							"Authenticator": pulumi.Map{
								"auto_login": pulumi.Bool(true),
							},
							"JupyterHub": pulumi.Map{
								"authenticator_class": pulumi.String("auth0"),
							},
						},
						"extraConfig": pulumi.Map{
							//"podRoot": pulumi.String(fileStr),
							"volume": pulumi.String(volumeStr),
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
			}, pulumi.Provider(k8sProvider))

			if err != nil {
				return err
			}
		}

		pachdAddress := fmt.Sprintf("%v://%v:%v", "grpcs", url, "443")
		pachdConnectionString := fmt.Sprintf("echo '{\"pachd_address\": \"%v\"}' | pachctl config set context %v --overwrite && pachctl config set active-context %v", pachdAddress, id, id)

		ctx.Export("createdBy", pulumi.String(createdBy))
		ctx.Export("status", pulumi.String("ready"))
		ctx.Export("pachdip", gcpL4LoadBalancerIP)
		ctx.Export("juypterUrl", jupyterURL)
		ctx.Export("consoleUrl", url)
		ctx.Export("k8sNamespace", namespace.Metadata.Elem().Name())
		ctx.Export("bucket", bucket.Name)
		ctx.Export("helium-expiry", pulumi.String(expiry))
		ctx.Export("k8sConnection", pulumi.Sprintf("gcloud container clusters get-credentials %s --zone us-east1-b --project ***REMOVED***", kubeName))
		ctx.Export("backend", pulumi.String(BackendName))
		// TODO: look into and cleanup pachd-lb and pachdip
		ctx.Export("pachd-lb-ip", gcpL4LoadBalancerIP)
		ctx.Export("pachd-lb-url", url)
		ctx.Export("pachd-address", pulumi.String(pachdAddress))
		ctx.Export("pachd-connection-string", pulumi.String(pachdConnectionString))

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
