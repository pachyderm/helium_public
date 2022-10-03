package util

import (
	"regexp"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

func TestName(t *testing.T) {
	log.Println("TestUtilName running")
	actual := Name()
	expected := regexp.MustCompile(`^[a-z0-9]([-a-z0-9]{1,61}[a-z0-9]{1})$`)
	assert.Regexp(t, expected, actual)
}

var defaultValues = `deployTarget: "GOOGLE"
global:
  postgresql:
    postgresqlPassword:         "***REMOVED***"
    postgresqlPostgresPassword: "***REMOVED***"
console:
  enabled: true
  config:
    graphqlPort: -40000.12321313
proxy:
  enabled: true
  tls:
    enabled: true
  service:
    type: "LoadBalancer"
oidc:
  issuerURI: "https://pachd-peer:30653"
  upstreamIDPs:
  - id: 40000
    name: "auth0"
    type: true
undefined:
`

func TestToPulumi(t *testing.T) {
	out := map[string]any{}
	if err := yaml.Unmarshal([]byte(defaultValues), &out); err != nil {
		t.Errorf(err.Error())
	}
	got := ToPulumi(out)
	want := pulumi.Map{
		"deployTarget": pulumi.String("GOOGLE"),
		"global": pulumi.Map{
			"postgresql": pulumi.Map{
				"postgresqlPassword":         pulumi.String("***REMOVED***"),
				"postgresqlPostgresPassword": pulumi.String("***REMOVED***"),
			},
		},
		"console": pulumi.Map{
			"enabled": pulumi.Bool(true),
			"config": pulumi.Map{
				"graphqlPort": pulumi.Float64(-40000.12321313),
			},
		},
		"proxy": pulumi.Map{
			"enabled": pulumi.Bool(true),
			"tls": pulumi.Map{
				"enabled": pulumi.Bool(true),
			},
			"service": pulumi.Map{
				"type": pulumi.String("LoadBalancer"),
			},
		},
		"oidc": pulumi.Map{
			"issuerURI": pulumi.String("https://pachd-peer:30653"),
			"upstreamIDPs": pulumi.MapArray{
				pulumi.Map{
					"id":   pulumi.Int(40000),
					"name": pulumi.String("auth0"),
					"type": pulumi.Bool(true),
				},
			},
		},
		"undefined": nil,
	}
	if !cmp.Equal(got, want) {
		t.Errorf(cmp.Diff(got, want))
	}
}

func TestMergeMap(t *testing.T) {
	src := map[string]any{}
	if err := yaml.Unmarshal([]byte(defaultValues), &src); err != nil {
		t.Errorf(err.Error())
	}
	testValues := `console:
  image:
    tag: "best-image"
  enabled: false
proxy:
  tls:
    enabled: false
oidc:
  upstreamIDPs:
  - id: 40000
    name: foobar
`
	dest := map[string]any{}
	if err := yaml.Unmarshal([]byte(testValues), &dest); err != nil {
		t.Errorf(err.Error())
	}
	got := MergeMaps(src, dest)
	want := map[string]any{
		"deployTarget": "GOOGLE",
		"global": map[string]any{
			"postgresql": map[string]any{
				"postgresqlPassword":         "***REMOVED***",
				"postgresqlPostgresPassword": "***REMOVED***",
			},
		},
		"console": map[string]any{
			"enabled": false,
			"config": map[string]any{
				"graphqlPort": -40000.12321313,
			},
			"image": map[string]any{
				"tag": "best-image",
			},
		},
		"proxy": map[string]any{
			"enabled": true,
			"tls": map[string]any{
				"enabled": false,
			},
			"service": map[string]any{
				"type": "LoadBalancer",
			},
		},
		"oidc": map[string]any{
			"issuerURI": "https://pachd-peer:30653",
			"upstreamIDPs": []any{
				map[string]any{
					"id":   40000,
					"name": "foobar",
				},
			},
		},
		"undefined": nil,
	}
	if !cmp.Equal(got, want) {
		t.Errorf(cmp.Diff(got, want))
	}
}
