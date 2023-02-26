package validation_test

import (
	"context"
	"errors"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	. "github.com/solo-io/gloo/projects/gateway/pkg/validation"
)

var _ = Describe("NotificationChannel", func() {
	It("refreshes the stream when a receive fails", func() {
		client := &mockValidationClient{
			response: &validation.NotifyOnResyncResponse{},
		}
		notifications, err := MakeNotificationChannel(context.TODO(), client)
		Expect(err).NotTo(HaveOccurred())

		Expect(client.streamStartedTimes).To(Equal(1))

		// we should get notifications
		Eventually(func() int {
			return len(notifications)
		}).Should(BeNumerically(">", 0))

		// start returning errors, channel should attempt restart
		client.set(nil)

		Eventually(func() int {
			client.l.RLock()
			times := client.streamStartedTimes
			client.l.RUnlock()
			return times
		}).Should(BeNumerically(">", 1))

	})
})

type mockValidationClient struct {
	response           *validation.NotifyOnResyncResponse
	l                  sync.RWMutex
	streamStartedTimes int
}

func (m *mockValidationClient) set(response *validation.NotifyOnResyncResponse) {
	m.l.Lock()
	m.response = response
	m.l.Unlock()
}

func (m *mockValidationClient) NotifyOnResync(ctx context.Context, in *validation.NotifyOnResyncRequest, opts ...grpc.CallOption) (validation.GlooValidationService_NotifyOnResyncClient, error) {
	m.l.Lock()
	m.streamStartedTimes++
	m.l.Unlock()
	return m, nil
}

func (m *mockValidationClient) Recv() (*validation.NotifyOnResyncResponse, error) {
	m.l.RLock()
	resp := m.response
	m.l.RUnlock()
	if resp == nil {
		return nil, errors.New("empty")
	}
	m.l.RLock()
	defer m.l.RUnlock()
	return m.response, nil
}

func (m *mockValidationClient) Header() (metadata.MD, error) {
	panic("implement me")
}

func (m *mockValidationClient) Trailer() metadata.MD {
	panic("implement me")
}

func (m *mockValidationClient) CloseSend() error {
	panic("implement me")
}

func (m *mockValidationClient) Context() context.Context {
	panic("implement me")
}

func (m *mockValidationClient) SendMsg(v interface{}) error {
	panic("implement me")
}

func (m *mockValidationClient) RecvMsg(v interface{}) error {
	panic("implement me")
}

func (m *mockValidationClient) Validate(ctx context.Context, in *validation.GlooValidationServiceRequest, opts ...grpc.CallOption) (*validation.GlooValidationServiceResponse, error) {
	panic("implement me")
}
