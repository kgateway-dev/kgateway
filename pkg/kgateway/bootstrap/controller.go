package bootstrap

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"math"
	"time"

	"golang.org/x/time/rate"
	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/kclient"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/kgateway-dev/kgateway/v2/pkg/apiclient"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
)

var (
	logger = logging.New("controller/bootstrap")

	_ manager.LeaderElectionRunnable = (*controller)(nil)
)

type controller struct {
	secretClient kclient.Client[*corev1.Secret]

	queue controllers.Queue
}

// NewController creates a new bootstrap controller that manages bootstrap configuration.
// Currently, it ensures that the OAuth2 HMAC secret key used by the OAuth2 policy is created
// at startup if it doesn't already exist or has been deleted.
func NewController(
	client apiclient.Client,
) *controller {
	c := &controller{
		secretClient: kclient.NewFiltered[*corev1.Secret](client, kclient.Filter{
			ObjectFilter:  client.ObjectFilter(),
			FieldSelector: "metadata.name=" + wellknown.OAuth2HMACSecret.Name,
			Namespace:     wellknown.OAuth2HMACSecret.Namespace,
		}),
	}

	// rateLimiter uses token bucket for overall rate limiting and exponential backoff for per-item rate limiting
	rateLimiter := workqueue.NewTypedMaxOfRateLimiter(
		workqueue.NewTypedItemExponentialFailureRateLimiter[any](500*time.Millisecond, 10*time.Second),
		// 10 qps, 100 bucket size.  This is only for retry speed and its only the overall factor (not per item)
		&workqueue.TypedBucketRateLimiter[any]{Limiter: rate.NewLimiter(rate.Limit(10), 100)},
	)
	c.queue = controllers.NewQueue("bootstrap", controllers.WithReconciler(c.reconcile), controllers.WithMaxAttempts(math.MaxInt), controllers.WithRateLimiter(rateLimiter))

	c.secretClient.AddEventHandler(
		controllers.FromEventHandler(func(o controllers.Event) {
			switch o.Event {
			case controllers.EventDelete:
				logger.Debug("reconciling bootstrap Secret on deletion", "ref", kubeutils.NamespacedNameFrom(o.Old))
				c.queue.AddObject(o.Old)

			default:
				// no-op for Update/Add
			}
		}))

	return c
}

// NeedLeaderElection returns true to ensure that the controller runs only on the leader
func (r *controller) NeedLeaderElection() bool {
	return true
}

// Start starts the controller and blocks until the Context is cancelled
func (c *controller) Start(ctx context.Context) error {
	// Seed the queue with an initial event to ensure OAuth2 HMAC secret creation on startup
	c.queue.Add(wellknown.OAuth2HMACSecret)
	kube.WaitForCacheSync("bootstrap", ctx.Done(), c.secretClient.HasSynced)
	c.queue.Run(ctx.Done())

	// Shutdown all the clients
	controllers.ShutdownAll(c.secretClient)
	return nil
}

func (r *controller) reconcile(req types.NamespacedName) error {
	oauthHMACSecret := r.secretClient.Get(req.Name, req.Namespace)

	// only reconcile if the Secret doesn't exist
	if oauthHMACSecret == nil || oauthHMACSecret.GetDeletionTimestamp() != nil {
		logger.Info("creating OAuth2 HMAC secret", "ref", req.String())
		if err := r.createOAuth2HMACSecret(); err != nil {
			return err
		}
	}
	return nil
}

func (r *controller) createOAuth2HMACSecret() error {
	// For full-entropy HMAC-SHA256, a 32-byte key is recommended.
	// Envoy uses HMAC-SHA256 for OAuth HMAC cookie: https://github.com/envoyproxy/envoy/blob/v1.36.2/source/extensions/filters/http/oauth2/filter.cc#L192
	keyLength := sha256.Size
	secretKey := make([]byte, keyLength)

	// Read cryptographically secure random bytes into the slice
	_, err := rand.Read(secretKey)
	if err != nil {
		fmt.Printf("error generating OAuth2 HMAC secret key: %v\n", err)
		return err
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      wellknown.OAuth2HMACSecret.Name,
			Namespace: wellknown.OAuth2HMACSecret.Namespace,
		},
		Data: map[string][]byte{
			wellknown.OAuth2HMACSecretKey: secretKey,
		},
	}
	_, err = r.secretClient.Create(secret)
	if err != nil {
		logger.Error("error creating OAuth2 HMAC secret", "ref", kubeutils.NamespacedNameFrom(secret).String(), "error", err)
		return err
	}

	return nil
}
