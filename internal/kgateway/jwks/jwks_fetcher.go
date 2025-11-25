package jwks

import (
	"container/heap"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/go-jose/go-jose/v4"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// JwksFetcher is used for fetching and periodic updates of jwks.
// Fetched jwks are stored in jwksCache. All access to jwksCache is synchronized via mu mutex.
// When a jwks is updated, registered subscribers are sent the update.
type JwksFetcher struct {
	mu            sync.Mutex
	cache         *jwksCache
	jwksClient    JwksHttpClient
	keysetSources map[string]*JwksSource
	schedule      FetchingSchedule
	subscribers   []chan map[string]string
}

type FetchingSchedule []fetchAt

//go:generate go tool mockgen -destination mocks/mock_jwks_http_client.go -package mocks -source ./jwks_fetcher.go
type JwksHttpClient interface {
	FetchJwks(ctx context.Context, jwksURL string) (jose.JSONWebKeySet, error)
}

type JwksSource struct {
	JwksURL string
	Ttl     time.Duration
	Deleted bool
}

type JwksSources []JwksSource

func (js JwksSources) ResourceName() string {
	return "jwkssources"
}

type fetchAt struct {
	at           time.Time
	keysetSource *JwksSource
	retryAttempt int
}

type jwksHttpClientImpl struct {
	Client *http.Client
}

func NewJwksFetcher(cache *jwksCache) *JwksFetcher {
	toret := &JwksFetcher{
		cache: cache,
		jwksClient: &jwksHttpClientImpl{
			Client: &http.Client{Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true, //nolint:gosec
				},
			}}},
		keysetSources: make(map[string]*JwksSource),
		schedule:      make([]fetchAt, 0),
		subscribers:   make([]chan map[string]string, 0),
	}
	heap.Init(&toret.schedule)

	return toret
}

// heap implementation
func (s FetchingSchedule) Len() int           { return len(s) }
func (s FetchingSchedule) Less(i, j int) bool { return s[i].at.Before(s[j].at) }
func (s FetchingSchedule) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s *FetchingSchedule) Push(x any) {
	*s = append(*s, x.(fetchAt))
}
func (s *FetchingSchedule) Pop() any {
	old := *s
	n := len(old)
	x := old[n-1]
	*s = old[0 : n-1]
	return x
}
func (s FetchingSchedule) Peek() *fetchAt {
	if len(s) == 0 {
		return nil
	}
	return &s[0]
}

func (f *JwksFetcher) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			f.maybeFetchJwks(ctx)
		}
	}
}

func (f *JwksFetcher) maybeFetchJwks(ctx context.Context) {
	log := log.FromContext(ctx)
	updates := make(map[string]string)

	f.mu.Lock()
	defer f.mu.Unlock()

	now := time.Now()
	for {
		maybeFetch := f.schedule.Peek()
		if maybeFetch == nil || maybeFetch.at.After(now) {
			break
		}

		fetch := heap.Pop(&f.schedule).(fetchAt)
		if fetch.keysetSource.Deleted {
			continue
		}
		jwks, err := f.jwksClient.FetchJwks(ctx, fetch.keysetSource.JwksURL)
		if err != nil {
			log.Error(err, "error fetching jwks from ", fetch.keysetSource.JwksURL)
			if fetch.retryAttempt < 5 { // backoff by 5s * retry attempt number
				heap.Push(&f.schedule, fetchAt{at: now.Add(time.Duration(5*(fetch.retryAttempt+1)) * time.Second), keysetSource: fetch.keysetSource, retryAttempt: fetch.retryAttempt + 1})
			} else {
				// give up retrying and schedule an update at a later time
				heap.Push(&f.schedule, fetchAt{at: now.Add(fetch.keysetSource.Ttl), keysetSource: fetch.keysetSource})
			}
			continue
		}

		maybeUpdatedJwks, err := f.cache.compareAndAddJwks(fetch.keysetSource.JwksURL, jwks)
		// error serializing jwks, shouldn't happen, retry
		if err != nil {
			log.Error(err, "error adding jwks", "uri", fetch.keysetSource.JwksURL)
			heap.Push(&f.schedule, fetchAt{at: now.Add(time.Duration(5*(fetch.retryAttempt+1)) * time.Second), keysetSource: fetch.keysetSource, retryAttempt: fetch.retryAttempt + 1})
			continue
		}

		heap.Push(&f.schedule, fetchAt{at: now.Add(fetch.keysetSource.Ttl), keysetSource: fetch.keysetSource})
		if maybeUpdatedJwks != "" {
			updates[fetch.keysetSource.JwksURL] = maybeUpdatedJwks
		}
	}

	if len(updates) > 0 {
		for _, s := range f.subscribers {
			s <- updates
		}
	}
}

func (f *JwksFetcher) SubscribeToUpdates() chan map[string]string {
	f.mu.Lock()
	defer f.mu.Unlock()

	subscriber := make(chan map[string]string)
	f.subscribers = append(f.subscribers, subscriber)

	return subscriber
}

func (f *JwksFetcher) UpdateJwksSources(ctx context.Context, updates JwksSources) error {
	var errs []error
	maybeUpdates := make(map[string]JwksSource)
	for _, s := range updates {
		maybeUpdates[s.JwksURL] = s
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	todelete := make([]string, 0)
	for s := range f.keysetSources {
		if _, ok := maybeUpdates[s]; !ok {
			todelete = append(todelete, s)
		}
	}

	for _, s := range updates {
		if _, ok := f.keysetSources[s.JwksURL]; !ok {
			if err := f.addKeyset(s.JwksURL, s.Ttl); err != nil {
				errs = append(errs, err)
			}
			continue
		}
		if *f.keysetSources[s.JwksURL] != s {
			if err := f.updateKeyset(s.JwksURL, s.Ttl); err != nil {
				errs = append(errs, err)
			}
		}
	}

	removals := make(map[string]string)
	for _, jwksUri := range todelete {
		if f.removeKeyset(jwksUri) {
			removals[jwksUri] = ""
		}
	}

	if len(removals) > 0 {
		for _, s := range f.subscribers {
			s <- removals
		}
	}

	return errors.Join(errs...)
}

func (f *JwksFetcher) addKeyset(jwksUrl string, ttl time.Duration) error {
	if _, err := url.Parse(jwksUrl); err != nil {
		return fmt.Errorf("error parsing jwks url %w", err)
	}

	keysetSource := &JwksSource{JwksURL: jwksUrl, Ttl: ttl, Deleted: false}
	f.keysetSources[jwksUrl] = keysetSource
	heap.Push(&f.schedule, fetchAt{at: time.Now(), keysetSource: keysetSource}) // schedule an immediate fetch

	return nil
}

func (f *JwksFetcher) removeKeyset(jwksUrl string) bool {
	if keysetSource, ok := f.keysetSources[jwksUrl]; ok {
		delete(f.keysetSources, jwksUrl)
		f.cache.deleteJwks(jwksUrl)
		keysetSource.Deleted = true
		return true
	}
	return false
}

func (f *JwksFetcher) updateKeyset(jwksUrl string, ttl time.Duration) error {
	if keysetSource, ok := f.keysetSources[jwksUrl]; ok {
		delete(f.keysetSources, jwksUrl)
		keysetSource.Deleted = true
	}
	return f.addKeyset(jwksUrl, ttl)
}

func (c *jwksHttpClientImpl) FetchJwks(ctx context.Context, jwksURL string) (jose.JSONWebKeySet, error) {
	log := log.FromContext(ctx)
	log.Info("fetching jwks", "url", jwksURL)

	request, err := http.NewRequest(http.MethodGet, jwksURL, nil)
	if err != nil {
		return jose.JSONWebKeySet{}, fmt.Errorf("could not build request to get JWKS: %w", err)
	}

	// TODO (dmitri-d) control the size here maybe?
	response, err := c.Client.Do(request)
	if err != nil {
		return jose.JSONWebKeySet{}, err
	}
	defer response.Body.Close() //nolint:errcheck

	if response.StatusCode != http.StatusOK {
		return jose.JSONWebKeySet{}, fmt.Errorf("unexpected status code from jwks endpoint at %s: %d", jwksURL, response.StatusCode)
	}

	var jwks jose.JSONWebKeySet
	if err := json.NewDecoder(response.Body).Decode(&jwks); err != nil {
		return jose.JSONWebKeySet{}, fmt.Errorf("could not decode jwks: %w", err)
	}

	return jwks, nil
}
