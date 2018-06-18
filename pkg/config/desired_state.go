package config

import (
	"fmt"

	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/log"
	"github.com/solo-io/gloo/pkg/storage"
)

const OwnerAnnotationKey = "generated_by"

type UpstreamSyncer struct {
	GlooStorage      storage.Interface
	DesiredUpstreams func() ([]*v1.Upstream, error)
	Owner            string
}

func (c *UpstreamSyncer) SyncDesiredState() error {

	desiredUpstreams, err := c.DesiredUpstreams()
	if err != nil {
		return fmt.Errorf("failed to generate desired upstreams: %v", err)
	}

	c.setOwnerAnnotation(desiredUpstreams)

	actualUpstreams, err := c.getActualUpstreams()
	if err != nil {
		return fmt.Errorf("failed to list actual upstreams: %v", err)
	}
	if err := c.syncUpstreams(desiredUpstreams, actualUpstreams); err != nil {
		return fmt.Errorf("failed to sync actual with desired upstreams: %v", err)
	}
	return nil
}

func (c *UpstreamSyncer) setOwnerAnnotation(uss []*v1.Upstream) {
	for _, us := range uss {
		if us.Metadata == nil {
			us.Metadata = &v1.Metadata{}
		}
		if us.Metadata.Annotations == nil {
			us.Metadata.Annotations = make(map[string]string)
		}
		us.Metadata.Annotations[OwnerAnnotationKey] = c.Owner
	}
}

func (c *UpstreamSyncer) getActualUpstreams() ([]*v1.Upstream, error) {
	upstreams, err := c.GlooStorage.V1().Upstreams().List()
	if err != nil {
		return nil, fmt.Errorf("failed to get upstream crd list: %v", err)
	}
	var ourUpstreams []*v1.Upstream
	for _, us := range upstreams {
		if us.Metadata != nil && us.Metadata.Annotations[OwnerAnnotationKey] == c.Owner {
			// our upstream, we supervise it
			ourUpstreams = append(ourUpstreams, us)
		}
	}
	return ourUpstreams, nil
}

func (c *UpstreamSyncer) syncUpstreams(desiredUpstreams, actualUpstreams []*v1.Upstream) error {
	var (
		upstreamsToCreate []*v1.Upstream
		upstreamsToUpdate []*v1.Upstream
	)
	for _, desiredUpstream := range desiredUpstreams {
		var update bool
		for i, actualUpstream := range actualUpstreams {
			if desiredUpstream.Name == actualUpstream.Name {
				// modify existing upstream
				// set metadata if it's nil
				if desiredUpstream.Metadata == nil {
					desiredUpstream.Metadata = &v1.Metadata{}
				}
				// update resource version
				desiredUpstream.Metadata.ResourceVersion = actualUpstream.Metadata.ResourceVersion
				update = true
				if !desiredUpstream.Equal(actualUpstream) {
					// only actually update if the spec has changed
					upstreamsToUpdate = append(upstreamsToUpdate, desiredUpstream)
				}
				// remove it from the list we match against
				actualUpstreams = append(actualUpstreams[:i], actualUpstreams[i+1:]...)
				break
			}
		}
		if !update {
			// desired was not found, mark for creation
			upstreamsToCreate = append(upstreamsToCreate, desiredUpstream)
		}
	}
	for _, us := range upstreamsToCreate {

		// TODO: think about caring about already exists errors
		// This workaround is necessary because the ingress controller may be running and creating upstreams
		if _, err := c.GlooStorage.V1().Upstreams().Create(us); err != nil && !storage.IsAlreadyExists(err) {
			log.Debugf("creating upstream %v", us.Name)
			return fmt.Errorf("failed to create upstream crd %s: %v", us.Name, err)
		}
	}
	for _, us := range upstreamsToUpdate {
		log.Debugf("updating upstream %v", us.Name)
		// preserve functions that may have already been discovered
		currentUpstream, err := c.GlooStorage.V1().Upstreams().Get(us.Name)
		if err != nil {
			return fmt.Errorf("failed to get existing upstream %s: %v", us.Name, err)
		}
		// all we want to do is update the spec and merge the annotations
		currentUpstream.Spec = us.Spec
		if currentUpstream.Metadata == nil {
			currentUpstream.Metadata = &v1.Metadata{}
		}
		currentUpstream.Metadata.Annotations = mergeAnnotations(currentUpstream.Metadata.Annotations, us.Metadata.Annotations)
		if _, err := c.GlooStorage.V1().Upstreams().Update(currentUpstream); err != nil {
			return fmt.Errorf("failed to update upstream %s: %v", us.Name, err)
		}
	}
	// only remaining are no longer desired, delete em!
	for _, us := range actualUpstreams {
		log.Debugf("deleting upstream %v", us.Name)
		if err := c.GlooStorage.V1().Upstreams().Delete(us.Name); err != nil {
			return fmt.Errorf("failed to update upstream crd %s: %v", us.Name, err)
		}
	}
	return nil
}

// get the unique set of funcs between two lists
// if conflict, new wins
func mergeAnnotations(oldAnnotations, newAnnotations map[string]string) map[string]string {
	merged := make(map[string]string)
	for k, v := range oldAnnotations {
		merged[k] = v
	}
	for k, v := range newAnnotations {
		merged[k] = v
	}
	return merged
}
