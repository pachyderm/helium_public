package pulumi_backends

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optdestroy"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optup"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/pulumi/pulumi/sdk/v3/go/common/workspace"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/pachyderm/helium/api"
	"github.com/pachyderm/helium/pulumi_backends/aws_cluster"
	"github.com/pachyderm/helium/pulumi_backends/gcp_cluster_only"
	"github.com/pachyderm/helium/util"

	log "github.com/sirupsen/logrus"
)

// This implementation is mostly a thin wrapper around https://github.com/pachyderm/pulumihttp/
func init() {
	ensurePlugins()
}

const (
	//BackendName = "gcp-namespace-pulumi"
	timeFormat = "2006-01-02"
)

var (
	project           = "helium"
	clientSecret      = os.Getenv("HELIUM_CLIENT_SECRET")
	clientID          = os.Getenv("HELIUM_CLIENT_ID")
	expirationNumDays = os.Getenv("HELIUM_DEFAULT_EXPIRATION_DAYS")
	auth0Domain       = "https://***REMOVED***.auth0.com/"
)

type Runner struct {
	//Name backend.Name
}

func (r *Runner) GetConnectionInfo(i api.ID) (*api.GetConnectionInfoResponse, error) {
	log.SetReportCaller(true)
	log.SetLevel(log.DebugLevel)
	log.WithField("backend", "pulumi").Debugf("Get Info")

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
	log.WithFields(log.Fields{
		"backend":          "pulumi",
		"name":             info.Name,
		"current":          info.Current,
		"lastupdate":       info.LastUpdate,
		"updateinprogress": info.UpdateInProgress,
		"url":              info.URL,
		"resourcecount":    info.ResourceCount,
	}).Infof("get stack info")

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
					// Updates aren't supported, so first update is always accurate
					// TODO: ^That isn't true anymore
					PulumiURL:   info.URL + "/updates/1",
					LastUpdated: info.LastUpdate,
				},
			}, nil
		}

		var pachdUrl string
		var pachdAddress string
		var pachdConnString string
		if pachdConnString, ok = outs["pachd-connection-string"].Value.(string); !ok {
			if pachdUrl, ok = outs["pachd-lb-url"].Value.(string); !ok {
				pachdUrl = outs["pachdip"].Value.(map[string]interface{})["ip"].(string)
			}
			// TODO: Deprecated. Remove in a future update once no more stacks are using it.
			pachdAddress = fmt.Sprintf("echo '{\"pachd_address\": \"%v://%v:%v\"}' pachctl config set context %v --overwrite && pachctl config set active-context %v", "grpc", pachdUrl, "30651", outs["k8sNamespace"].Value.(string), outs["k8sNamespace"].Value.(string))
		} else {
			pachdAddress = pachdConnString
		}

		var createdBy string
		if createdBy, ok = outs["createdBy"].Value.(string); !ok {
			createdBy = ""
		}

		var k8sInfo string
		if k8sInfo, ok = outs["k8sConnection"].Value.(string); !ok {
			k8sInfo = ""
		}

		var backendOutput string
		if backendOutput, ok = outs["backend"].Value.(string); !ok {
			backendOutput = ""
		}

		// initial aws support won't have
		// TODO: fix the blank https:// value with better string handling
		var juypterUrlInfo string
		if juypterUrlInfo, ok = outs["juypterUrl"].Value.(string); !ok {
			juypterUrlInfo = ""
		}

		return &api.GetConnectionInfoResponse{Workspace: api.ConnectionInfo{
			Status:       status,
			ID:           i,
			K8s:          k8sInfo,
			PulumiURL:    info.URL + "/updates/1",
			LastUpdated:  info.LastUpdate,
			K8sNamespace: outs["k8sNamespace"].Value.(string),
			ConsoleURL:   "https://" + outs["consoleUrl"].Value.(string),
			NotebooksURL: "https://" + juypterUrlInfo,
			GCSBucket:    outs["bucket"].Value.(string),
			Expiry:       outs["helium-expiry"].Value.(string),
			PachdIp:      "grpc://" + pachdUrl + ":30651",
			Pachctl:      pachdAddress,
			CreatedBy:    createdBy,
			Backend:      backendOutput,
		}}, nil
	}
	return &api.GetConnectionInfoResponse{
		Workspace: api.ConnectionInfo{
			Status:      "creating",
			ID:          i,
			PulumiURL:   info.URL + "/updates/1",
			LastUpdated: info.LastUpdate,
		},
	}, nil
}

func (r *Runner) List() (*api.ListResponse, error) {
	log.SetReportCaller(true)
	log.SetLevel(log.DebugLevel)
	log.WithField("backend", "pulumi").Debugf("list")

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
	log.WithField("backend", "pulumi").Debugf("list ids: %v", ids)
	return &api.ListResponse{IDs: ids}, nil
}

func (r *Runner) IsExpired(i api.ID) (bool, error) {
	log.SetReportCaller(true)
	log.SetLevel(log.DebugLevel)
	log.WithField("backend", "pulumi").Debugf("isexpired")
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
		return false, fmt.Errorf("expected stack output 'helium-expiry' not found for stack: %v", stackName)
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
	log.WithField("backend", "pulumi").Debugf("create")

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

	var disableNotebooks bool
	if req.DisableNotebooks == "True" {
		disableNotebooks = true
	}

	backend := strings.ToLower(req.Backend)
	var program pulumi.RunFunc
	gcpProjectID := "***REMOVED***"

	//	// GitAuth is the authentication details that can be specified for a private Git repo.
	//	// There are 3 different authentication paths:
	//	// * PersonalAccessToken
	//	// * SSHPrivateKeyPath (and it's potential password)
	//	// * Username and Password
	//	// Only 1 authentication path is valid. If more than 1 is specified it will result in an error
	//	type GitAuth struct {
	//		// The absolute path to a private key for access to the git repo
	//		// When using `SSHPrivateKeyPath`, the URL of the repository must be in the format
	//		// git@github.com:org/repository.git - if the url is not in this format, then an error
	//		// `unable to clone repo: invalid auth method` will be returned
	//		SSHPrivateKeyPath string
	//		// The (contents) private key for access to the git repo.
	//		// When using `SSHPrivateKey`, the URL of the repository must be in the format
	//		// git@github.com:org/repository.git - if the url is not in this format, then an error
	//		// `unable to clone repo: invalid auth method` will be returned
	//		SSHPrivateKey string
	//		// The password that pairs with a username or as part of an SSH Private Key
	//		Password string
	//		// PersonalAccessToken is a Git personal access token in replacement of your password
	//		PersonalAccessToken string
	//		// Username is the username to use when authenticating to a git repository
	//		Username string
	//	}
	//
	//	// GitRepo contains info to acquire and setup a Pulumi program from a git repository.
	//	type GitRepo struct {
	//		// URL to clone git repo
	//		URL string
	//		// Optional path relative to the repo root specifying location of the pulumi program.
	//		// Specifying this option will update the Workspace's WorkDir accordingly.
	//		ProjectPath string
	//		// Optional branch to checkout.
	//		Branch string
	//		// Optional commit to checkout.
	//		CommitHash string
	//		// Optional function to execute after enlisting in the specified repo.
	//		Setup string
	//		// GitAuth is the different Authentication options for the Git repository
	//		Auth *GitAuth
	//	}
	var stackSet bool
	var s auto.Stack
	switch backend {
	case "gcp_cluster_only":
		//project = "helium-gke-clusters"
		program = gcp_cluster_only.CreatePulumiProgram(stackName, expiryStr, helmchartVersion, req.ConsoleVersion, req.PachdVersion, req.NotebooksVersion, req.ValuesYAML, req.CreatedBy, req.ClusterStack, cleanup, req.InfraJSONContent, req.ValuesYAMLContent)

	case "aws_cluster":
		program = aws_cluster.CreatePulumiProgram(stackName, expiryStr, helmchartVersion, req.ConsoleVersion, req.PachdVersion, req.NotebooksVersion, req.ValuesYAML, req.CreatedBy, cleanup, req.InfraJSONContent, req.ValuesYAMLContent)
		//
	default:
		repo := auto.GitRepo{
			URL:         "https://github.com/pachyderm/poc-pulumi.git",
			ProjectPath: "gcp_namespace_only/cli",
			Branch:      "refs/heads/main",
			Auth: &auto.GitAuth{
				PersonalAccessToken: os.Getenv("HELIUM_GITHUB_PERSONAL_TOKEN"),
			},
		}

		s, err = auto.UpsertStackRemoteSource(ctx, stackName, repo)
		if err != nil {
			fmt.Printf("Failed to create or select stack: %v\n", err)
			os.Exit(1)
		}
		stackSet = true

		config := map[string]string{
			"id":                   stackName,
			"expiry":               expiryStr,
			"helm-chart-version":   helmchartVersion,
			"console-version":      req.ConsoleVersion,
			"pachd-version":        req.PachdVersion,
			"notebooks-version":    req.NotebooksVersion,
			"pachd-values-file":    req.ValuesYAML,
			"created-by":           req.CreatedBy,
			"cluster-stack":        req.ClusterStack,
			"cleanup-on-failure":   strconv.FormatBool(cleanup),
			"pachd-values-content": string(req.ValuesYAMLContent),
			// TODO: Wire up infrajson through config
			//"infra-json-content":        req.InfraJSONContent,

			// This is an internal GCP ID, not sure if it's exposed at all through pulumi.  I got it by doing a GET call directly against their API here:
			// https://cloud.google.com/dns/docs/reference/v1/managedZones/get?apix_params=%7B%22project%22%3A%22***REMOVED***%22%2C%22managedZone%22%3A%22test-ci%22%7D
			"workspace-managed-zone-gcp-id": "***REMOVED***",
			"client-secret":                 os.Getenv("HELIUM_CLIENT_SECRET"),
			"client-id":                     os.Getenv("HELIUM_CLIENT_ID"),
			"auth-domain":                   "https://***REMOVED***.auth0.com/",
			"auth-subdomain":                "***REMOVED***",
			"postgres-password":             "***REMOVED***",
			"postgres-pg-password":          "***REMOVED***",
			"console-oauthClientSecret":     "***REMOVED***",
			"pachd-oauthClientSecret":       "***REMOVED***",
			"pachd-root-token":              "***REMOVED***",
			"pachd-enterprise-secret":       "***REMOVED***",
			"pachd-enterprise-license":      "***REMOVED***",
		}

		for k, v := range config {
			s.SetConfig(ctx, fmt.Sprintf("helium:%s", k), auto.ConfigValue{Value: v})
		}
		//program = gcp_namespace_only.CreatePulumiProgram()
	}
	if !stackSet {
		s, err = auto.SelectStackInlineSource(ctx, stackName, project, program)
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
	}

	// TODO: should be able to switch gcp project to
	s.SetConfig(ctx, "gcp:project", auto.ConfigValue{Value: gcpProjectID})
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
	log.WithField("backend", "pulumi").Debugf("destroy")

	ctx := context.Background()
	stackName := string(i)
	// program doesn't matter for destroying a stack
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
	//s.SetConfig(ctx, "gcp:project", auto.ConfigValue{Value: "***REMOVED***"})
	//s.SetConfig(ctx, "gcp:zone", auto.ConfigValue{Value: "us-east1-b"})

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

// TODO: Document need to add plugins for other providers
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
	//fmt.Printf("aws access key id: %v", os.Getenv("AWS_ACCESS_KEY_ID"))
	//fmt.Printf("aws secret access key: %v", os.Getenv("AWS_SECRET_ACCESS_KEY"))
	err = w.InstallPlugin(ctx, "aws", "v5.7.0")
	if err != nil {
		fmt.Printf("Failed to install program plugins: %v\n", err)
		os.Exit(1)
	}
	err = w.InstallPlugin(ctx, "eks", "v0.40.0")
	if err != nil {
		fmt.Printf("Failed to install program plugins: %v\n", err)
		os.Exit(1)
	}
	err = w.InstallPlugin(ctx, "postgresql", "v3.4.0")
	if err != nil {
		fmt.Printf(("Failed to install program plugins: %v\n"), err)
		os.Exit(1)
	}
}
