package gcp_namespace_pulumi

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"time"

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

const (
	BackendName = "gcp-namespace-pulumi"
	timeFormat  = "2006-01-02"
)

var (
	project           = "helium"
	clientSecret      = os.Getenv("HELIUM_CLIENT_SECRET")
	clientID          = os.Getenv("HELIUM_CLIENT_ID")
	expirationNumDays = os.Getenv("HELIUM_DEFAULT_EXPIRATION_DAYS")
	auth0Domain       = "https://***REMOVED***.auth0.com/"
)

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

	info, err := s.Info(ctx)
	if err != nil {
		return nil, err
	}
	log.Debugf("info name: %v", info.Name)
	log.Debugf("info current: %v", info.Current)
	log.Debugf("info lastupdate: %v", info.LastUpdate)
	log.Debugf("info UpdateInProgress: %v", info.UpdateInProgress)
	//	log.Debugf("info ResourceCount: %v", info.ResourceCount)
	log.Debugf("info URL: %v", info.URL)

	if !info.UpdateInProgress {
		// fetch the outputs from the stack
		outs, err := s.Outputs(ctx)
		if err != nil {
			return nil, err
		}
		// Output is only set on success. If update is not in progess, and no outputs, we know it's in a failed state
		status, ok := outs["status"].Value.(string)
		if !ok {
			return &api.GetConnectionInfoResponse{
				Workspace: api.ConnectionInfo{
					Status: "failed",
					ID:     i,
					K8s:    "gcloud container clusters get-credentials ***REMOVED*** --zone us-east1-b --project ***REMOVED***",
					// Updates aren't supported, so first update is always accurate
					// TODO: ^That isn't true anymore
					PulumiURL:   info.URL + "/updates/1",
					LastUpdated: info.LastUpdate,
				},
			}, nil
		}
		pachdip := outs["pachdip"].Value.(map[string]interface{})["ip"].(string)
		pachdAddress := fmt.Sprintf("echo '{\"pachd_address\": \"%v://%v:%v\", \"source\": 2}' | tr -d \\ | pachctl config set context %v --overwrite && pachctl config set active-context %v", "grpc", pachdip, "30651", outs["k8sNamespace"].Value.(string), outs["k8sNamespace"].Value.(string))

		return &api.GetConnectionInfoResponse{Workspace: api.ConnectionInfo{
			Status:       status,
			ID:           i,
			K8s:          "gcloud container clusters get-credentials ***REMOVED*** --zone us-east1-b --project ***REMOVED***",
			PulumiURL:    info.URL + "/updates/1",
			LastUpdated:  info.LastUpdate,
			K8sNamespace: outs["k8sNamespace"].Value.(string),
			ConsoleURL:   "https://" + outs["consoleUrl"].Value.(string),
			NotebooksURL: "https://" + outs["juypterUrl"].Value.(string),
			GCSBucket:    outs["bucket"].Value.(string),
			Expiry:       outs["helium-expiry"].Value.(string),
			PachdIp:      "grpc://" + pachdip + ":30651",
			Pachctl:      pachdAddress,
		}}, nil
	}
	return &api.GetConnectionInfoResponse{
		Workspace: api.ConnectionInfo{
			Status:      "creating",
			ID:          i,
			K8s:         "gcloud container clusters get-credentials ***REMOVED*** --zone us-east1-b --project ***REMOVED***",
			PulumiURL:   info.URL + "/updates/1",
			LastUpdated: info.LastUpdate,
		},
	}, nil
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
	log.SetReportCaller(true)
	log.SetLevel(log.DebugLevel)
	log.Infof("isExpired request")
	//
	stackName := string(i)
	// we don't need a program since we're just getting stack outputs
	var program pulumi.RunFunc = nil
	ctx := context.Background()
	s, err := auto.SelectStackInlineSource(ctx, stackName, project, program)
	if err != nil {
		// if the stack doesn't already exist, 404
		if auto.IsSelectStack404Error(err) {
			return false, fmt.Errorf("stack %q not found: %w", stackName, err)
		}
		return false, err
	}
	info, err := s.Info(ctx)
	if err != nil {
		return false, err
	}
	// an update is currently ongoing, it can't be expired while actively updating
	if info.UpdateInProgress {
		return false, nil
	}
	// fetch the outputs from the stack
	outs, err := s.Outputs(ctx)
	if err != nil {
		return false, err
	}
	if outs["helium-expiry"].Value == nil {
		log.Warnf("expected stack output 'helium-expiry' not found for stack: %v", stackName)
		return false, nil
	}
	log.Debugf("Expiry: %v", outs["helium-expiry"].Value.(string))
	expiry, err := time.Parse(timeFormat, outs["helium-expiry"].Value.(string))
	if err != nil {
		return false, err
	}
	if time.Now().After(expiry) {
		return true, nil
	}
	return false, nil
}

func (r *Runner) Create(req *api.Spec) (*api.CreateResponse, error) {

	ctx := context.Background()

	//type Spec struct {
	//	Name             string
	//	Expiry           string
	//	PachdVersion     string
	//	ConsoleVersion   string
	//	NotebooksVersion string
	//	ValuesYAML       string
	//  CleanupOnFail    string
	//}

	log.Debugf("Name: %v", req.Name)
	log.Debugf("Expiry: %v", req.Expiry)
	log.Debugf("PachdVersion: %v", req.PachdVersion)
	log.Debugf("ConsoleVersion: %v", req.ConsoleVersion)
	log.Debugf("NotebooksVersion: %v", req.NotebooksVersion)
	log.Debugf("HelmVersion: %v", req.HelmVersion)
	log.Debugf("CleanupOnFail: %v", req.CleanupOnFail)
	log.Debugf("ValuesYAML: %v", req.ValuesYAML)

	helmchartVersion := req.HelmVersion
	stackName := req.Name

	var expiry time.Time
	var err error
	if req.Expiry != "" {
		expiry, err = time.Parse(timeFormat, req.Expiry)
		if err != nil {
			return nil, err
		}
	}
	//var expiryDefault int
	//if expirationNumDays == "" {
	//	expiryDefault = 1
	//} else {
	//	expiryDefault, err = strconv.Atoi(expirationNumDays)
	//	if err != nil {
	//		return nil, err
	//	}
	//}
	//if expiryDefault == 0 {
	//	expiryDefault = 1
	//}

	if expiry.IsZero() {
		// default to 1 day for expiry
		expiry = time.Now().AddDate(0, 0, 1)
		//expiry = time.Now().AddDate(0, 0, 1*expiryDefaul)
		log.Debugf("Expiry: %v", expiry)
	} else if expiry.After(time.Now().AddDate(0, 0, 1*90)) {
		// Max expiration date is 90 days from now
		expiry = time.Now().AddDate(0, 0, 1*90)
		log.Debugf("Expiry: %v", expiry)
	}
	expiryStr := expiry.Format(timeFormat)

	cleanup := true
	if req.CleanupOnFail == "False" {
		cleanup = false
	}

	program := createPulumiProgram(stackName, expiryStr, helmchartVersion, req.ConsoleVersion, req.PachdVersion, req.NotebooksVersion, req.ValuesYAML, cleanup)

	s, err := auto.SelectStackInlineSource(ctx, stackName, project, program)
	if err != nil {
		if auto.IsSelectStack404Error(err) {
			s, err = auto.NewStackInlineSource(ctx, stackName, project, program)
			if err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}
	s.SetConfig(ctx, "gcp:project", auto.ConfigValue{Value: "***REMOVED***"})
	s.SetConfig(ctx, "gcp:zone", auto.ConfigValue{Value: "us-east1-b"})

	// deploy the stack
	// we'll write all of the update logs to st	out so we can watch requests get processed
	_, err = s.Up(ctx, optup.ProgressStreams(util.NewLogWriter(log.WithFields(log.Fields{"pulumi_op": "create", "stream": "stdout"}))))
	if err != nil {
		s.SetConfig(ctx, "status", auto.ConfigValue{Value: "failed"})
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
	//	program := createPulumiProgram("", "")
	program := createEmptyPulumiProgram()

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
	//	_, err = s.Destroy(ctx, optdestroy.ProgressStreams(os.Stdout))
	_, err = s.Destroy(ctx, optdestroy.ProgressStreams(util.NewLogWriter(log.WithFields(log.Fields{"pulumi_op": "create", "stream": "stdout"}))))
	if err != nil {
		return err
	}

	// delete the stack and all associated history and config
	// Apparently unimplemented: optremov.ProgressStreams()
	err = s.Workspace().RemoveStack(ctx, stackName)
	if err != nil {
		return err
	}
	log.Infof("deleted all associated stack information with: %s", stackName)
	return nil
}

func createEmptyPulumiProgram() pulumi.RunFunc {
	return func(ctx *pulumi.Context) error {
		return nil
	}
}

func createPulumiProgram(id, expiry, helmChartVersion, consoleVersion, pachdVersion, notebooksVersion, valuesYaml string, cleanup2 bool) pulumi.RunFunc {
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
					"needs-ci-tls":  pulumi.String("true"), //Uses kubernetes replicator to replicate TLS secret to new NS
					"helium-expiry": pulumi.String(expiry),
				},
			},
		}, pulumi.Provider(k8sProvider))

		if err != nil {
			return err
		}

		// TODO: Hardcoded static IP address
		// ipAddress := "34.148.221.170"
		// ipAddress := static.Address
		// log.Debugf("static IP: %v", ipAddress)

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
					"secretName": pulumi.String("wildcard-tls"),
				},
			},
		}
		if pachdVersion != "" {
			pachdValues = pulumi.Map{
				"image": pulumi.Map{
					"tag": pulumi.String(pachdVersion),
				},
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
						"secretName": pulumi.String("wildcard-tls"),
					},
				},
			}
		}

		array := []pulumi.AssetOrArchiveInput{}
		array = append(array, pulumi.AssetOrArchiveInput(pulumi.NewFileAsset(valuesYaml)))
		corePach, err := helm.NewRelease(ctx, "pach-release", &helm.ReleaseArgs{
			Atomic:        pulumi.Bool(cleanup2),
			CleanupOnFail: pulumi.Bool(cleanup2),
			Timeout:       pulumi.Int(600),
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
		}, pulumi.Provider(k8sProvider))
		if err != nil {
			return err
		}
		result := pulumi.All(corePach.Status.Namespace()).ApplyT(func(r interface{}) ([]interface{}, error) {
			arr := r.([]interface{})
			namespace := arr[0].(*string)
			svc, err := corev1.GetService(ctx, "svc", pulumi.ID(fmt.Sprintf("%s/pachd-lb", *namespace)), nil, pulumi.Timeouts(&pulumi.CustomTimeouts{Create: "10m"}))
			if err != nil {
				log.Errorf("error getting loadbalancer IP: %v", err)
				return nil, err
			}
			return []interface{}{svc.Status.LoadBalancer().Ingress().Index(pulumi.Int(0)), svc.Metadata.Name().Elem()}, nil
		})
		jupyterImage := "904d4965029e35a434c7c049a0470d9e4800c990"
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
			},
		}, pulumi.Provider(k8sProvider))

		if err != nil {
			return err
		}

		arr := result.(pulumi.ArrayOutput)
		ctx.Export("status", pulumi.String("ready"))
		ctx.Export("pachdip", arr.Index(pulumi.Int(0)))
		ctx.Export("juypterUrl", juypterURL)
		ctx.Export("consoleUrl", consoleUrl)
		ctx.Export("k8sNamespace", namespace.Metadata.Elem().Name())
		ctx.Export("bucket", bucket.Name)
		ctx.Export("helium-expiry", pulumi.String(expiry))

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
