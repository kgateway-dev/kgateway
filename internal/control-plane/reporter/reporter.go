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

func NewReporter(store storage.Interface) Interface {
	return &reporter{store: store}
}

func (r *reporter) WriteGlobalReports(cfgObjectErrs []ConfigObjectReport) error {
	for _, cfgObjectErr := range cfgObjectErrs {
		report := createReport(cfgObjectErr)
		if err := r.writeReport(report); err != nil {
			return errors.Wrapf(err, "failed to write report for upstream %v", cfgObjectErr.CfgObject)
		}
		log.Debugf("wrote report for %v", cfgObjectErr.CfgObject.GetName())
	}
	return nil
}

func (r *reporter) WriteRoleReports(role string, cfgObjectErrs []ConfigObjectReport) error {
	for _, cfgObjectErr := range cfgObjectErrs {
		report := createReportForRole(role, cfgObjectErr)
		if err := r.writeReport(report); err != nil {
			return errors.Wrapf(err, "failed to write report for config object %v", cfgObjectErr.CfgObject)
		}
		log.Debugf("wrote report for %v", cfgObjectErr.CfgObject.GetName())
	}
	return nil
}

func (r *reporter) writeReport(report *v1.Report) error {
	if existingReport, err := r.store.V1().Reports().Get(report.Name); err == nil {
		// check if existing report equals the one we have, ignoring resource version
		if existingReport.Metadata != nil {
			report.Metadata.ResourceVersion = existingReport.Metadata.ResourceVersion
			existingReport.Metadata.ResourceVersion = ""
		}
		if existingReport.Equal(report) {
			// nothing to do
			return nil
		}
		if _, err := r.store.V1().Reports().Update(report); err != nil {
			return errors.Wrapf(err, "failed to update report "+report.Name)
		}
		return nil
	}
	if _, err := r.store.V1().Reports().Create(report); err != nil {
		return errors.Wrapf(err, "failed to create report "+report.Name)
	}
	return nil
}

func createReport(cfgObjectErr ConfigObjectReport) *v1.Report {
	status := &v1.Status{
		State: v1.Status_Accepted,
	}
	if cfgObjectErr.Err != nil {
		status.State = v1.Status_Rejected
		status.Reason = cfgObjectErr.Err.Error()
	}
	return &v1.Report{
		Name:            reportName(cfgObjectErr.CfgObject),
		ObjectReference: objectReference(cfgObjectErr.CfgObject),
		Status:          status,
		Metadata: &v1.Metadata{
			Namespace: namespace(cfgObjectErr.CfgObject),
		},
	}
}

func reportName(item v1.ConfigObject) string {
	return item.GetName()
}

func createReportForRole(role string, cfgObjectErr ConfigObjectReport) *v1.Report {
	status := &v1.Status{
		State: v1.Status_Accepted,
	}
	if cfgObjectErr.Err != nil {
		status.State = v1.Status_Rejected
		status.Reason = cfgObjectErr.Err.Error()
	}
	return &v1.Report{
		Name:            reportNameForRole(role, cfgObjectErr.CfgObject),
		ObjectReference: objectReference(cfgObjectErr.CfgObject),
		Status:          status,
		Metadata: &v1.Metadata{
			Namespace: namespace(cfgObjectErr.CfgObject),
		},
	}
}

func reportNameForRole(role string, item v1.ConfigObject) string {
	return role+"-"+item.GetName()
}

func objectReference(item v1.ConfigObject) *v1.ObjectReference {
	var t v1.ObjectReference_ObjectType
	switch item.(type) {
	case *v1.Upstream:
		t = v1.ObjectReference_Upstream
	case *v1.VirtualService:
		t = v1.ObjectReference_Upstream
	default:
		panic("invalid config object, cannot create a reference")
	}
	return &v1.ObjectReference{
		ObjectType: t,
		Name:       item.GetName(),
		Namespace:  namespace(item),
	}
}

func namespace(item v1.ConfigObject) string {
	if meta := item.GetMetadata(); meta != nil {
		return meta.Namespace
	}
	return ""
}
