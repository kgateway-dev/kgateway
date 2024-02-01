package xds_test

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gateway2/xds"
)

type testType struct {
	idx int
}

func TestDropItems(t *testing.T) {
	g := NewWithT(t)
	q := xds.NewAsyncQueue[testType]()
	zero := testType{idx: 0}
	one := testType{idx: 1}
	q.Enqueue(zero)
	q.Enqueue(one)
	next, err := q.Dequeue(context.Background())
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(next).To(Equal(one))
}

func TestReturnNone(t *testing.T) {
	g := NewWithT(t)
	ctx, cancel := context.WithCancel(context.Background())
	q := xds.NewAsyncQueue[testType]()
	zero := testType{idx: 0}
	q.Enqueue(zero)
	next, err := q.Dequeue(ctx)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(next).To(Equal(zero))
	cancel()
	_, err = q.Dequeue(ctx)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err).To(Equal(context.Canceled))
}
