package crd

import (
	"time"

	"github.com/pkg/errors"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/storage"
	crdclientset "github.com/solo-io/gloo/pkg/storage/crd/client/clientset/versioned"
	crdv1 "github.com/solo-io/gloo/pkg/storage/crd/solo.io/v1"
	apiexts "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"

	"github.com/solo-io/gloo/pkg/storage/crud"
	kuberrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/cache"
	"github.com/solo-io/gloo/pkg/log"
)

type reportsClient struct {
	crds    crdclientset.Interface
	apiexts apiexts.Interface
	// write and read objects to this namespace if not specified on the GlooObjects
	namespace     string
	syncFrequency time.Duration
}

func (c *reportsClient) Create(item *v1.Report) (*v1.Report, error) {
	return c.createOrUpdateReportCrd(item, crud.OperationCreate)
}

func (c *reportsClient) Update(item *v1.Report) (*v1.Report, error) {
	return c.createOrUpdateReportCrd(item, crud.OperationUpdate)
}

func (c *reportsClient) Delete(name string) error {
	return c.crds.GlooV1().Reports(c.namespace).Delete(name, nil)
}

func (c *reportsClient) Get(name string) (*v1.Report, error) {
	crdReport, err := c.crds.GlooV1().Reports(c.namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "failed performing get api request")
	}
	var returnedReport v1.Report
	if err := ConfigObjectFromCrd(
		crdReport.ObjectMeta,
		crdReport.Spec,
		&returnedReport); err != nil {
		return nil, errors.Wrap(err, "converting returned crd to report")
	}
	return &returnedReport, nil
}

func (c *reportsClient) List() ([]*v1.Report, error) {
	crdList, err := c.crds.GlooV1().Reports(c.namespace).List(metav1.ListOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "failed performing list api request")
	}
	var returnedReports []*v1.Report
	for _, crdReport := range crdList.Items {
		var returnedReport v1.Report
		if err := ConfigObjectFromCrd(
			crdReport.ObjectMeta,
			crdReport.Spec,
			&returnedReport); err != nil {
			return nil, errors.Wrap(err, "converting returned crd to report")
		}
		returnedReports = append(returnedReports, &returnedReport)
	}
	return returnedReports, nil
}

func (u *reportsClient) Watch(handlers ...storage.ReportEventHandler) (*storage.Watcher, error) {
	lw := cache.NewListWatchFromClient(u.crds.GlooV1().RESTClient(), crdv1.ReportCRD.Plural, u.namespace, fields.Everything())
	sw := cache.NewSharedInformer(lw, new(crdv1.Report), u.syncFrequency)
	for _, h := range handlers {
		sw.AddEventHandler(&reportEventHandler{handler: h, store: sw.GetStore()})
	}
	return storage.NewWatcher(func(stop <-chan struct{}, _ chan error) {
		sw.Run(stop)
	}), nil
}

func (c *reportsClient) createOrUpdateReportCrd(report *v1.Report, op crud.Operation) (*v1.Report, error) {
	reportCrd, err := ConfigObjectToCrd(c.namespace, report)
	if err != nil {
		return nil, errors.Wrap(err, "converting gloo object to crd")
	}
	reports := c.crds.GlooV1().Reports(reportCrd.GetNamespace())
	var returnedCrd *crdv1.Report
	switch op {
	case crud.OperationCreate:
		returnedCrd, err = reports.Create(reportCrd.(*crdv1.Report))
		if err != nil {
			if kuberrs.IsAlreadyExists(err) {
				return nil, storage.NewAlreadyExistsErr(err)
			}
			return nil, errors.Wrap(err, "kubernetes create api request")
		}
	case crud.OperationUpdate:
		// need to make sure we preserve labels
		currentCrd, err := reports.Get(reportCrd.GetName(), metav1.GetOptions{ResourceVersion: reportCrd.GetResourceVersion()})
		if err != nil {
			return nil, errors.Wrap(err, "kubernetes get api request")
		}
		// copy labels
		reportCrd.SetLabels(currentCrd.Labels)
		returnedCrd, err = reports.Update(reportCrd.(*crdv1.Report))
		if err != nil {
			return nil, errors.Wrap(err, "kubernetes update api request")
		}
	}
	var returnedReport v1.Report
	if err := ConfigObjectFromCrd(
		returnedCrd.ObjectMeta,
		returnedCrd.Spec,
		&returnedReport); err != nil {
		return nil, errors.Wrap(err, "converting returned crd to report")
	}
	return &returnedReport, nil
}

// implements the kubernetes ResourceEventHandler interface
type reportEventHandler struct {
	handler storage.ReportEventHandler
	store   cache.Store
}

func (eh *reportEventHandler) getUpdatedList() []*v1.Report {
	updatedList := eh.store.List()
	var updatedReportList []*v1.Report
	for _, updated := range updatedList {
		reportCrd, ok := updated.(*crdv1.Report)
		if !ok {
			continue
		}
		var returnedReport v1.Report
		if err := ConfigObjectFromCrd(
			reportCrd.ObjectMeta,
			reportCrd.Spec,
			&returnedReport); err != nil {
			log.Warnf("watch event: %v", errors.Wrap(err, "converting returned crd to report"))
		}
		updatedReportList = append(updatedReportList, &returnedReport)
	}
	return updatedReportList
}

func convertReport(obj interface{}) (*v1.Report, bool) {
	reportCrd, ok := obj.(*crdv1.Report)
	if !ok {
		return nil, ok
	}
	var returnedReport v1.Report
	if err := ConfigObjectFromCrd(
		reportCrd.ObjectMeta,
		reportCrd.Spec,
		&returnedReport); err != nil {
		log.Warnf("watch event: %v", errors.Wrap(err, "converting returned crd to report"))
		return nil, false
	}
	return &returnedReport, true
}

func (eh *reportEventHandler) OnAdd(obj interface{}) {
	report, ok := convertReport(obj)
	if !ok {
		return
	}
	eh.handler.OnAdd(eh.getUpdatedList(), report)
}
func (eh *reportEventHandler) OnUpdate(_, newObj interface{}) {
	newReport, ok := convertReport(newObj)
	if !ok {
		return
	}
	eh.handler.OnUpdate(eh.getUpdatedList(), newReport)
}

func (eh *reportEventHandler) OnDelete(obj interface{}) {
	report, ok := convertReport(obj)
	if !ok {
		return
	}
	eh.handler.OnDelete(eh.getUpdatedList(), report)
}
