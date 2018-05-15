package reporter

import (
	"github.com/pkg/errors"
	"github.com/solo-io/gloo/pkg/storage"

	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/log"
)

type reporter struct {
	store storage.Interface
}

func NewReporter(store storage.Interface) *reporter {
	return &reporter{store: store}
}

func (r *reporter) WriteReports(reports []ConfigObjectReport) error {
	for _, report := range reports {
		if err := r.writeReport(report); err != nil {
			return errors.Wrapf(err, "failed to write report for config object %v", report.CfgObject)
		}
		log.Debugf("wrote report for %v", report.CfgObject.GetName())
	}
	return nil
}

func (r *reporter) writeReport(report ConfigObjectReport) error {
	status := &v1.Status{
		State: v1.Status_Accepted,
	}
	if report.Err != nil {
		status.State = v1.Status_Rejected
		status.Reason = report.Err.Error()
	}
	name := report.CfgObject.GetName()
	switch report.CfgObject.(type) {
	case *v1.Upstream:
		us, err := r.store.V1().Upstreams().Get(report.CfgObject.GetName())
		if err != nil {
			return errors.Wrapf(err, "failed to find upstream %v", name)
		}
		// only update if status doesn't match
		if us.Status.Equal(status) {
			return nil
		}
		us.Status = status
		if _, err := r.store.V1().Upstreams().Update(us); err != nil {
			return errors.Wrapf(err, "failed to update upstream store with status report")
		}
	case *v1.VirtualService:
		virtualService, err := r.store.V1().VirtualServices().Get(name)
		if err != nil {
			return errors.Wrapf(err, "failed to find virtualservice %v", name)
		}
		// only update if status doesn't match
		if virtualService.Status.Equal(status) {
			return nil
		}
		virtualService.Status = status
		if _, err := r.store.V1().VirtualServices().Update(virtualService); err != nil {
			return errors.Wrapf(err, "failed to update virtualservice store with status report")
		}
	case *v1.Role:
		role, err := r.store.V1().Roles().Get(name)
		if err != nil {
			// try to create the role
			role, err = r.store.V1().Roles().Create(report.CfgObject.(*v1.Role))
			if err != nil {
				return errors.Wrapf(err, "failed to find or create role %v", name)
			}
		}
		// only update if status doesn't match
		if role.Status.Equal(status) {
			return nil
		}
		role.Status = status
		if _, err := r.store.V1().Roles().Update(role); err != nil {
			return errors.Wrapf(err, "failed to update role store with status report")
		}
	}
	return nil
}
