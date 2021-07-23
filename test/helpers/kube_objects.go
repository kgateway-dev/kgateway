package helpers

import (
	"github.com/onsi/gomega"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ObjectGetter func() (client.Object, error)

func EventuallyObjectDeleted(getter ObjectGetter, intervals ...interface{}) {
	EventuallyObjectDeletedWithOffset(1, getter, intervals...)
}

func EventuallyObjectDeletedWithOffset(offset int, getter ObjectGetter, intervals ...interface{}) {
	gomega.EventuallyWithOffset(offset+1, func() (bool, error) {
		_, err := getter()
		if err != nil && k8serrors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	}, intervals...).Should(gomega.BeTrue())
}
