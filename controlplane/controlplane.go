package controlplane

import (
	"os"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/pachyderm/helium/api"
	"github.com/pachyderm/helium/pulumi_backends"
)

const (
	InfraPrewarmCount     = 2
	WorkspacePrewarmCount = 2
)

var deletionControllerMode string = os.Getenv("HELIUM_CONTROLPLANE_DELETE_ALL")

func RunDeletionController() error {
	//For each Pach, check Expiry. If true, call Delete

	id, err := pulumi_backends.List()
	if err != nil {
		return err
	}
	var nightlyPresent bool
	for _, v := range id.IDs {
		if v == "nightly-cluster" {
			nightlyPresent = true
		}
		b, err := pulumi_backends.IsExpired(v)
		if err != nil {
			if strings.Contains(err.Error(), "expected stack output 'helium-expiry' not found for stack") {
				log.Debugf("deletion controller destroying because expiry not found: %v", v)
				err := pulumi_backends.Destroy(v)
				if err != nil {
					log.Errorf("deletion controller error destroying: %v", err)
				}
			} else {
				log.Errorf("expiry error: %v", err)
				continue
			}
		}
		if b || deletionControllerMode == "True" {
			log.Debugf("deletion controller destroying: %v", v)
			err := pulumi_backends.Destroy(v)
			if err != nil {
				log.Errorf("deletion controller error destroying: %v", err)
			}
			time.Sleep(time.Second * 10)
			// TODO: This is a bit of a hack for feeddog. Will cause a circular import if anything in pulumi_backends needs this package
			if v == "nightly-cluster" {
				time.Sleep(time.Minute * 5)
				spec := &api.Spec{
					Name:    "nightly-cluster",
					Backend: "gcp_cluster_only",
				}
				_, err = pulumi_backends.Create(spec)
				if err != nil {
					log.Errorf("create handler: %v", err)
				}
			}
		}
	}
	if !nightlyPresent {
		spec := &api.Spec{
			Name:    "nightly-cluster",
			Backend: "gcp_cluster_only",
		}
		_, err = pulumi_backends.Create(spec)
		if err != nil {
			log.Errorf("create handler: %v", err)
		}
	}

	return nil
}
