package aws_cluster

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/rds"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/s3"
	"github.com/pulumi/pulumi-eks/sdk/go/eks"
	"github.com/pulumi/pulumi-gcp/sdk/v6/go/gcp/dns"
	"github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes/core/v1"
	"github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes/helm/v3"
	postgresql "github.com/pulumi/pulumi-postgresql/sdk/v3/go/postgresql"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	log "github.com/sirupsen/logrus"
)

//import (
//  "github.com/pachyderm/helium/pulumi_backends/aws_cluster"
//)
//
//func main() {
//  pulumi.Run(aws_cluster.CreatePulumiProgram(id, expiry, helmChartVersion, consoleVersion, pachdVersion, notebooksVersion, valuesYaml, createdBy string, cleanup2 bool))
//}

const (
	BackendName = "aws-cluster"
	timeFormat  = "2006-01-02"
)

var (
	project           = "helium"
	clientSecret      = os.Getenv("HELIUM_CLIENT_SECRET")
	clientID          = os.Getenv("HELIUM_CLIENT_ID")
	expirationNumDays = os.Getenv("HELIUM_DEFAULT_EXPIRATION_DAYS")
	auth0Domain       = "https://***REMOVED***.auth0.com/"
)

func CreatePulumiProgram(id,
	expiry,
	helmChartVersion,
	consoleVersion,
	pachdVersion,
	notebooksVersion,
	valuesYaml,
	createdBy string,
	cleanup2 bool,
) pulumi.RunFunc {
	return func(ctx *pulumi.Context) error {

		r, err := rds.NewInstance(ctx, "helium-postgresql", &rds.InstanceArgs{
			AllocatedStorage:   pulumi.Int(1000),
			Engine:             pulumi.String("postgres"),
			InstanceClass:      pulumi.String("db.m6g.2xlarge"),
			DbName:             pulumi.String("pachyderm"),
			Password:           pulumi.String("correcthorsebatterystaple"),
			SkipFinalSnapshot:  pulumi.Bool(true),
			StorageType:        pulumi.String("gp2"),
			Username:           pulumi.String("postgres"),
			PubliclyAccessible: pulumi.Bool(true),
		})

		if err != nil {
			return err
		}

		postgresProvider, err := postgresql.NewProvider(ctx, "helium-postgresql", &postgresql.ProviderArgs{
			Host:     r.Address,
			Port:     r.Port,
			Username: r.Username,
			Password: r.Password,
		})

		if err != nil {
			return err
		}

		dexDb, err := postgresql.NewDatabase(ctx, "dex", &postgresql.DatabaseArgs{}, pulumi.Provider(postgresProvider))

		if err != nil {
			return err
		}

		bucket, err := s3.NewBucket(ctx, "helium-bucket", &s3.BucketArgs{
			Acl: pulumi.String("public-read-write"),
		})

		if err != nil {
			return err
		}

		cluster, err := eks.NewCluster(ctx, id, &eks.ClusterArgs{
			InstanceType:    pulumi.String("t2.medium"),
			DesiredCapacity: pulumi.Int(2),
			MinSize:         pulumi.Int(1),
			MaxSize:         pulumi.Int(2),
		})
		if err != nil {
			return err
		}

		kubeConf := cluster.Kubeconfig.ApplyT(func(kconf interface{}) string {
			kConfMap := kconf.(map[string]interface{})
			output, err := json.Marshal(kConfMap)
			if err != nil {
				return ""
			}
			return string(output)
		}).(pulumi.StringOutput)

		k8sProvider, err := kubernetes.NewProvider(ctx, "k8sprovider", &kubernetes.ProviderArgs{
			Kubeconfig: kubeConf,
		})

		if err != nil {
			return err
		}

		traefikRelease, err := helm.NewRelease(ctx, "traefik", &helm.ReleaseArgs{
			RepositoryOpts: helm.RepositoryOptsArgs{
				Repo: pulumi.String("https://helm.traefik.io/traefik"),
			},
			Chart: pulumi.String("traefik"),
		}, pulumi.Provider(k8sProvider))

		if err != nil {
			return err
		}

		namespace, err := corev1.NewNamespace(ctx, "test-ns", &corev1.NamespaceArgs{},
			pulumi.Provider(k8sProvider))

		if err != nil {
			return err
		}

		//enterpriseKey := os.Getenv("PACH_ENTERPRISE_TOKEN")

		awsSAkey := os.Getenv("AWS_ACCESS_KEY_ID")
		awsSAsecret := os.Getenv("AWS_SECRET_ACCESS_KEY")

		// if enterpriseKey == "" {
		// 	return errors.New("Need to supply env var PACH_ENTERPRISE_TOKEN")
		// }

		urlSuffix := "fancy-elephant.com"
		managedZone := urlSuffix + "."
		url := pulumi.String(id + urlSuffix)

		array := []pulumi.AssetOrArchiveInput{}
		array = append(array, pulumi.AssetOrArchiveInput(pulumi.NewFileAsset(valuesYaml)))
		corePach, err := helm.NewRelease(ctx, "pach-release", &helm.ReleaseArgs{
			Atomic:        pulumi.Bool(cleanup2),
			CleanupOnFail: pulumi.Bool(cleanup2),
			Namespace:     namespace.Metadata.Elem().Name(),
			Version:       pulumi.String(helmChartVersion),
			RepositoryOpts: helm.RepositoryOptsArgs{
				Repo: pulumi.String("https://helm.***REMOVED***"),
			},
			Chart:          pulumi.String("pachyderm"),
			ValueYamlFiles: pulumi.AssetOrArchiveArray(array),
			Values: pulumi.Map{
				"ingress": pulumi.Map{
					"enabled": pulumi.Bool(true),
					"host":    pulumi.String(url),
					"annotations": pulumi.Map{
						"kubernetes.io/ingress.class": pulumi.String("traefik"),
						//"traefik.ingress.kubernetes.io/router.tls": "true",
					},
				},
				"console": pulumi.Map{
					"enabled": pulumi.Bool(true),
					"config": pulumi.Map{
						"oauthClientSecret": pulumi.String("***REMOVED***"),
					},
				},
				"pachd": pulumi.Map{
					"storage": pulumi.Map{
						"amazon": pulumi.Map{
							"bucket": bucket.Bucket,
							"region": pulumi.String("us-west-2"),
							"id":     pulumi.String(awsSAkey),
							"secret": pulumi.String(awsSAsecret),
						},
					},
					"externalService": pulumi.Map{
						"enabled": pulumi.Bool(true),
					},
					"enterpriseLicenseKey": pulumi.String("***REMOVED***"), //Set in .circleci/config.yml
					"oauthClientSecret":    pulumi.String("***REMOVED***"),
					"rootToken":            pulumi.String("***REMOVED***"),
					"enterpriseSecret":     pulumi.String("***REMOVED***"),
				},
				"deployTarget": pulumi.String("AMAZON"),
				"global": pulumi.Map{
					"postgresql": pulumi.Map{
						"postgresqlHost":                   r.Address,
						"postgresqlUsername":               pulumi.String("postgres"),
						"postgresqlPassword":               pulumi.String("correcthorsebatterystaple"),
						"postgresqlPostgresPassword":       pulumi.String("correcthorsebatterystaple"),
						"identityDatabaseFullNameOverride": dexDb.Name,
					},
				},
				"postgresql": pulumi.Map{
					"enabled": pulumi.Bool(false),
				},
			},
		}, pulumi.Provider(k8sProvider))

		if err != nil {
			return err
		}

		result := pulumi.All(corePach.Status.Namespace()).ApplyT(func(r interface{}) ([]interface{}, error) {
			arr := r.([]interface{})
			namespace := arr[0].(*string)
			svc, err := corev1.GetService(ctx, "svc", pulumi.ID(fmt.Sprintf("%s/pachd-lb", *namespace)), nil, pulumi.Timeouts(&pulumi.CustomTimeouts{Create: "10m"}), pulumi.Provider(k8sProvider))
			if err != nil {
				log.Errorf("error getting loadbalancer IP: %v", err)
				return nil, err
			}
			return []interface{}{svc.Status.LoadBalancer().Ingress().Index(pulumi.Int(0)), svc.Metadata.Name().Elem()}, nil
		})

		//traefikExternalSvc := pulumi.All(corePach.Status.Namespace()).ApplyT(func(r interface{}) (interface{}, error) {
		//	arr := r.([]interface{})
		//	namespace := arr[0].(*string)
		//
		//	return svc.Status.LoadBalancer().Ingress().Index(pulumi.Int(0)).Ip().Elem(), nil
		//})

		// svc.Status.LoadBalancer().Ingress().Index(pulumi.Int(0)).Ip().Elem()
		svc, err := corev1.GetService(ctx, "svc", pulumi.ID(
			fmt.Sprintf("%v/%v", corePach.Status.Namespace().Elem(),
				traefikRelease.Name)), nil,
			pulumi.Timeouts(&pulumi.CustomTimeouts{Create: "10m"}),
			pulumi.Provider(k8sProvider))
		if err != nil {
			log.Errorf("error getting loadbalancer IP: %v", err)
			return err
		}

		traefikExternalOutput := svc.Status.ApplyT(func(status *corev1.ServiceStatus) (string, error) {
			ingress := status.LoadBalancer.Ingress[0]
			if ingress.Ip == nil {
				return "", fmt.Errorf("empty ingress ip")
			}
			return *ingress.Ip, nil
		}).(pulumi.StringOutput)

		ctx.Export("traefikip", traefikExternalOutput)

		//	arr2 := traefikExternalSvc.(pulumi.ArrayOutput)
		// if things start panicking, this might be the culprit
		//	traefikExternal := arr2.Index(pulumi.Int(0)).(pulumi.StringOutput)

		_, err = dns.NewRecordSet(ctx, "frontendRecordSet", &dns.RecordSetArgs{
			Name:        url,
			Type:        pulumi.String("CNAME"),
			Ttl:         pulumi.Int(300),
			ManagedZone: pulumi.String(managedZone),
			Rrdatas:     pulumi.StringArray{traefikExternalOutput},
		})

		if err != nil {
			return err
		}

		arr := result.(pulumi.ArrayOutput)
		ctx.Export("createdBy", pulumi.String(createdBy))
		ctx.Export("status", pulumi.String("ready"))
		ctx.Export("pachdip", arr.Index(pulumi.Int(0)))
		ctx.Export("juypterUrl", pulumi.String("comming soon.."))
		ctx.Export("consoleUrl", pulumi.String(url))
		ctx.Export("k8sNamespace", namespace.Metadata.Elem().Name())
		ctx.Export("bucket", bucket.Bucket)
		ctx.Export("helium-expiry", pulumi.String(expiry))
		//cluster.EksCluster.Name()
		ctx.Export("k8sConnection", pulumi.String(fmt.Sprintf("aws eks --region us-west-2 update-kubeconfig --name %s", "hi")))
		ctx.Export("backend", pulumi.String(BackendName))

		return nil
	}

}
