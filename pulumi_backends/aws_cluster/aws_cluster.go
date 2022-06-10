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

	"github.com/pachyderm/helium/api"
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
	// This is an internal GCP ID, not sure if it's exposed at all through pulumi.  I got it by doing a GET call directly against their API here:
	// https://cloud.google.com/dns/docs/reference/v1/managedZones/get?apix_params=%7B%22project%22%3A%22***REMOVED***%22%2C%22managedZone%22%3A%22test-ci%22%7D
	TestCiManagedZoneGcpId = "***REMOVED***"
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
	infraJson *api.InfraJson,
) pulumi.RunFunc {
	return func(ctx *pulumi.Context) error {
		// TODO: remove me later...
		cleanup2 = false

		urlSuffix := "***REMOVED***"
		url := pulumi.String(id + "." + urlSuffix)

		testCiManagedZone, err := dns.GetManagedZone(ctx, "test-ci", pulumi.ID(pulumi.String(TestCiManagedZoneGcpId)), nil)
		if err != nil {
			return err
		}

		cluster, err := eks.NewCluster(ctx, id, &eks.ClusterArgs{
			InstanceType:    pulumi.String(infraJson.K8S.Nodepools[0].NodeType),
			DesiredCapacity: pulumi.Int(infraJson.K8S.Nodepools[0].NodeNumInstances),
			MinSize:         pulumi.Int(0),
			MaxSize:         pulumi.Int(infraJson.K8S.Nodepools[0].NodeNumInstances),
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

		namespace, err := corev1.NewNamespace(ctx, "test-ns", &corev1.NamespaceArgs{},
			pulumi.Provider(k8sProvider))
		if err != nil {
			return err
		}

		traefikRelease, err := helm.NewRelease(ctx, "traefik", &helm.ReleaseArgs{
			RepositoryOpts: helm.RepositoryOptsArgs{
				Repo: pulumi.String("https://helm.traefik.io/traefik"),
			},
			Chart: pulumi.String("traefik"),
			//Namespace: namespace.Metadata.Elem().Name(),
		}, pulumi.Provider(k8sProvider))

		if err != nil {
			return err
		}

		traefikExternalOutput := pulumi.All(traefikRelease.Status.Name()).ApplyT(func(args []interface{}) (interface{}, error) {
			//arr := r.([]interface{})
			//	namespace := args[0].(*string)
			svcName := args[0].(*string)
			svc, err := corev1.GetService(ctx, "traefik-svc", pulumi.ID(fmt.Sprintf("%s/%s", "default", *svcName)), nil, pulumi.Timeouts(&pulumi.CustomTimeouts{Create: "15m"}), pulumi.Provider(k8sProvider))
			if err != nil {
				log.Errorf("error getting loadbalancer IP: %v", err)
				return nil, err
			}
			// Hostname is used instead of IP for aws loadbalancers
			return svc.Status.LoadBalancer().Ingress().Index(pulumi.Int(0)).Hostname().Elem(), nil
		})

		// Add trailing . to rrdatas
		_, err = dns.NewRecordSet(ctx, "traefik-test-ci-record-set", &dns.RecordSetArgs{
			Name: url + ".",
			// TODO: This will be a CNAME for AWS?
			Type:        pulumi.String("CNAME"),
			Ttl:         pulumi.Int(300),
			ManagedZone: testCiManagedZone.Name,
			Rrdatas:     pulumi.StringArray{pulumi.Sprintf("%s.", traefikExternalOutput)},
		})
		if err != nil {
			return err
		}

		r, err := rds.NewInstance(ctx, "helium-postgresql", &rds.InstanceArgs{
			AllocatedStorage:   pulumi.Int(infraJson.RDS.DiskSize),
			Engine:             pulumi.String("postgres"),
			InstanceClass:      pulumi.String(infraJson.RDS.NodeType),
			DbName:             pulumi.String("pachyderm"),
			Password:           pulumi.String("correcthorsebatterystaple"),
			SkipFinalSnapshot:  pulumi.Bool(true),
			StorageType:        pulumi.String(infraJson.RDS.DiskType),
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

		//enterpriseKey := os.Getenv("PACH_ENTERPRISE_TOKEN")

		awsSAkey := os.Getenv("AWS_ACCESS_KEY_ID")
		awsSAsecret := os.Getenv("AWS_SECRET_ACCESS_KEY")

		// if enterpriseKey == "" {
		// 	return errors.New("Need to supply env var PACH_ENTERPRISE_TOKEN")
		// }

		//urlSuffix := "fancy-elephant.com"
		//managedZone := urlSuffix + "."
		//url := pulumi.String(id + "." + urlSuffix)

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

		loadBalancerIP := pulumi.All(corePach.Status.Namespace()).ApplyT(func(args []interface{}) (pulumi.StringOutput, error) {
			namespace := args[0].(*string)
			svc, err := corev1.GetService(ctx, "pachd-lb-svc", pulumi.ID(fmt.Sprintf("%s/pachd-lb", *namespace)), nil, pulumi.Timeouts(&pulumi.CustomTimeouts{Create: "10m"}), pulumi.Provider(k8sProvider))
			if err != nil {
				log.Errorf("error getting loadbalancer IP: %v", err)
			}
			// Hostname is used instead of IP for aws loadbalancers
			return svc.Status.LoadBalancer().Ingress().Index(pulumi.Int(0)).Hostname().Elem(), nil
		}).(pulumi.StringOutput)

		_, err = dns.NewRecordSet(ctx, "pachdlb-test-ci-record-set", &dns.RecordSetArgs{
			Name: url + ".",
			// TODO: This will be a CNAME for AWS?
			Type:        pulumi.String("CNAME"),
			Ttl:         pulumi.Int(300),
			ManagedZone: testCiManagedZone.Name,
			Rrdatas:     pulumi.StringArray{pulumi.Sprintf("%s.", loadBalancerIP)},
		})
		if err != nil {
			return err
		}

		////
		////
		////
		////
		///

		//traefikExternalSvc := pulumi.All(corePach.Status.Namespace()).ApplyT(func(r interface{}) (interface{}, error) {
		//	arr := r.([]interface{})
		//	namespace := arr[0].(*string)
		//
		//	return svc.Status.LoadBalancer().Ingress().Index(pulumi.Int(0)).Ip().Elem(), nil
		//})

		// svc.Status.LoadBalancer().Ingress().Index(pulumi.Int(0)).Ip().Elem()
		//serviceName := pulumi.Sprintf("%s/%s", namespace.Metadata.Elem().Name(), traefikRelease.Name)
		//svc, err := corev1.GetService(ctx, "svc", pulumi.ID(serviceName), nil,
		//	pulumi.Timeouts(&pulumi.CustomTimeouts{Create: "10m"}),
		//	pulumi.Provider(k8sProvider))
		//if err != nil {
		//	log.Errorf("error getting loadbalancer IP: %v", err)
		//	return err
		//}

		//traefikExternalOutput := svcOut.Status.ApplyT(func(status *corev1.ServiceStatus) (string, error) {
		//	ingress := status.LoadBalancer.Ingress[0]
		//	if ingress.Ip == nil {
		//		return "", fmt.Errorf("empty ingress ip")
		//	}
		//	return *ingress.Ip, nil
		//}).(pulumi.StringOutput)

		//	ctx.Export("traefikip", traefikExternalOutput)
		//
		//	_, err = dns.NewRecordSet(ctx, "frontendRecordSet", &dns.RecordSetArgs{
		//		Name:        url,
		//		Type:        pulumi.String("CNAME"),
		//		Ttl:         pulumi.Int(300),
		//		ManagedZone: pulumi.String(managedZone),
		//		Rrdatas:     pulumi.StringArray{traefikExternalOutput},
		//	})
		//	if err != nil {
		//		return err
		//	}

		ctx.Export("createdBy", pulumi.String(createdBy))
		ctx.Export("status", pulumi.String("ready"))
		ctx.Export("pachdip", loadBalancerIP)
		ctx.Export("juypterUrl", pulumi.String("comming soon.."))
		ctx.Export("consoleUrl", pulumi.String(url))
		ctx.Export("k8sNamespace", namespace.Metadata.Elem().Name())
		ctx.Export("bucket", bucket.Bucket)
		ctx.Export("helium-expiry", pulumi.String(expiry))
		//cluster.EksCluster.Name()
		ctx.Export("k8sConnection", pulumi.String(fmt.Sprintf("aws eks --region us-west-2 update-kubeconfig --name %s", "hi")))
		ctx.Export("backend", pulumi.String(BackendName))
		ctx.Export("pachd-lb-ip", loadBalancerIP)
		ctx.Export("pachd-lb-url", url)

		return nil
	}

}
