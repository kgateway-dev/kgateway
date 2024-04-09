package utils_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("sorting resources by creation timestamp", func() {
	When("the input is a slice of objects that implement client.Object", func() {
		It("sorts the objects by creation timestamp in ascending order", func() {
			now := metav1.Now()
			anHourAgo := metav1.NewTime(time.Now().Add(-1 * time.Hour))
			aDayAgo := metav1.NewTime(time.Now().Add(-1 * 24 * time.Hour))

			makeSvc := func(name string, creationTime *metav1.Time) *corev1.Service {
				svc := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: "default",
					},
				}
				if creationTime != nil {
					svc.CreationTimestamp = *creationTime.DeepCopy()
				}
				return svc
			}

			hourAgoSvc := makeSvc("hour-ago", &anHourAgo)
			nowSvc := makeSvc("now", &now)
			dayAgoSvc := makeSvc("day-ago", &aDayAgo)
			nowSvc2 := makeSvc("now-2", &now)     // check lexical sorting for equal timestamps
			noTimeSvc := makeSvc("nil-time", nil) // check nil case

			services := []*corev1.Service{hourAgoSvc, nowSvc, dayAgoSvc, nowSvc2, noTimeSvc}

			utils.SortByCreationTime(services)

			Expect(services).To(Equal([]*corev1.Service{noTimeSvc, dayAgoSvc, hourAgoSvc, nowSvc, nowSvc2}))
		})
	})

	It("does not panic on empty input slice", func() {
		Expect(func() { utils.SortByCreationTime([]interface{}{}) }).NotTo(Panic())
	})

	It("does not panic on typed nil input slice", func() {
		Expect(func() { utils.SortByCreationTime([]interface{}(nil)) }).NotTo(Panic())
	})

	It("panics on nil input", func() {
		Expect(func() { utils.SortByCreationTime(nil) }).To(Panic())
	})

	It("panics when the input is not a slice", func() {
		Expect(func() { utils.SortByCreationTime(123) }).To(Panic())
	})

	It("panics when the input is a slice of an unexpected object type", func() {
		Expect(func() { utils.SortByCreationTime([]string{"foo"}) }).To(Panic())
	})
})
