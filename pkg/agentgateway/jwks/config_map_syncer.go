package jwks

import (
	"context"
	"crypto/md5" //nolint:gosec
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"

	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/ptr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kgateway-dev/kgateway/v2/pkg/apiclient"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

const jwksStorePrefix = "jwks-store"
const JwksStoreComponent = "app.kubernetes.io/component"

// configMapSyncer is used for writing/reading jwks' to/from ConfigMaps.
type configMapSyncer struct {
	deploymentNamespace string
	storePrefix         string
	cmAccessor          cmAccessor
}

// this is an abstraction over ConfigMap access to facilitate testing
type cmAccessor interface {
	Create(context.Context, *corev1.ConfigMap) error
	Update(context.Context, *corev1.ConfigMap) error
	Delete(context.Context, string) error
	List() []*corev1.ConfigMap
	Get(string) *corev1.ConfigMap
	WaitForCacheSync(ctx context.Context) bool
}

func NewConfigMapSyncer(client apiclient.Client, storePrefix, deploymentNamespace string, krtOptions krtutil.KrtOptions) *configMapSyncer {
	cmCollection := krt.NewFilteredInformer[*corev1.ConfigMap](client,
		kclient.Filter{
			ObjectFilter:  client.ObjectFilter(),
			LabelSelector: JwksStoreComponent + "=" + storePrefix},
		krtOptions.ToOptions("config_map_syncer/ConfigMaps")...)

	toret := configMapSyncer{
		deploymentNamespace: deploymentNamespace,
		storePrefix:         storePrefix,
		cmAccessor: &defaultCmAccessor{
			client:              client,
			deploymentNamespace: deploymentNamespace,
			cmCollection:        cmCollection,
		},
	}

	return &toret
}

// Load jwks from a ConfigMap.
// Returns a map of jwks-uri -> jwks (currently one jwks-uri per ConfigMap).
func JwksFromConfigMap(cm *corev1.ConfigMap) (map[string]string, error) {
	jwksStore := cm.Data[jwksStorePrefix]
	jwks := make(map[string]string)
	err := json.Unmarshal(([]byte)(jwksStore), &jwks)
	if err != nil {
		return nil, err
	}
	return jwks, nil
}

func (cs *configMapSyncer) WaitForCacheSync(ctx context.Context) bool {
	return cs.cmAccessor.WaitForCacheSync(ctx)
}

// Generates ConfigMap name based on jwks uri. Resulting name is a concatenation of "jwks-store-" prefix and an MD5 hash of the jwks uri.
// The length of the name is a constant 32 chars (hash) + legth of the prefix.
func JwksConfigMapName(storePrefix, jwksUri string) string {
	hash := md5.Sum([]byte(jwksUri)) //nolint:gosec
	return fmt.Sprintf("%s-%s", storePrefix, hex.EncodeToString(hash[:]))
}

// Write out jwks' in updates to ConfigMaps, one jwks uri per ConfigMap. updates contains a map of jwks-uri to serialized jwks.
// Each ConfigMap is labelled with "app.kubernetes.io/component":"jwks-store" to support bulk loading of jwks' handled by LoadJwksFromConfigMaps().
func (cs *configMapSyncer) WriteJwksToConfigMaps(ctx context.Context, updates map[string]string) error {
	log := log.FromContext(ctx)
	errs := make([]error, 0)

	for uri, jwks := range updates {
		switch jwks {
		case "": // empty jwks == remove the underlying ConfigMap
			err := cs.cmAccessor.Delete(ctx, JwksConfigMapName(cs.storePrefix, uri))
			if client.IgnoreNotFound(err) != nil {
				log.Error(err, "error deleting jwks ConfigMap")
				errs = append(errs, err)
			}
		default:
			cmData, err := json.Marshal(map[string]string{uri: jwks})
			if err != nil {
				log.Error(err, "error serialiazing jwks")
				errs = append(errs, err)
				continue
			}

			existing := cs.cmAccessor.Get(JwksConfigMapName(cs.storePrefix, uri))
			if existing == nil {
				cm := cs.newJwksStoreConfigMap(JwksConfigMapName(cs.storePrefix, uri))
				cm.Data[jwksStorePrefix] = string(cmData)

				err := cs.cmAccessor.Create(ctx, cm)
				if err != nil {
					log.Error(err, "error persisting jwks to ConfigMap")
					errs = append(errs, err)
					continue
				}
			} else {
				existing.Data[jwksStorePrefix] = string(cmData)
				err = cs.cmAccessor.Update(ctx, existing)
				if err != nil {
					log.Error(err, "error updating jwks ConfigMap")
					errs = append(errs, err)
					continue
				}
			}
		}
	}

	return errors.Join(errs...)
}

// Loads all jwks persisted in ConfigMaps. The result is a map of jwks-uri to serialized jwks.
func (cs *configMapSyncer) LoadJwksFromConfigMaps(ctx context.Context) (map[string]string, error) {
	log := log.FromContext(ctx)

	allPersistedJwks := cs.cmAccessor.List()

	if len(allPersistedJwks) == 0 {
		return nil, nil
	}

	errs := make([]error, 0)
	toret := make(map[string]string)
	for _, cm := range allPersistedJwks {
		jwks, err := JwksFromConfigMap(cm)
		if err != nil {
			log.Error(err, "error deserializing jwks ConfigMap", "ConfigMap", cm.Name)
			errs = append(errs, err)
			continue
		}

		maps.Copy(toret, jwks)
	}

	return toret, errors.Join(errs...)
}

func (cs *configMapSyncer) newJwksStoreConfigMap(name string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: cs.deploymentNamespace,
			Labels:    map[string]string{JwksStoreComponent: cs.storePrefix},
		},
		Data: make(map[string]string),
	}
}

type defaultCmAccessor struct {
	cmCollection        krt.Collection[*corev1.ConfigMap]
	client              apiclient.Client
	deploymentNamespace string
}

var _ cmAccessor = &defaultCmAccessor{}

func (cma *defaultCmAccessor) Create(ctx context.Context, newCm *corev1.ConfigMap) error {
	_, err := cma.client.Kube().CoreV1().ConfigMaps(cma.deploymentNamespace).Create(ctx, newCm, metav1.CreateOptions{})
	return err
}

func (cma *defaultCmAccessor) Update(ctx context.Context, existingCm *corev1.ConfigMap) error {
	_, err := cma.client.Kube().CoreV1().ConfigMaps(cma.deploymentNamespace).Update(ctx, existingCm, metav1.UpdateOptions{})
	return err
}

func (cma *defaultCmAccessor) Delete(ctx context.Context, cmName string) error {
	return cma.client.Kube().CoreV1().ConfigMaps(cma.deploymentNamespace).Delete(ctx, cmName, metav1.DeleteOptions{})
}

func (cma *defaultCmAccessor) Get(cmName string) *corev1.ConfigMap {
	cmPtr := cma.cmCollection.GetKey(types.NamespacedName{Namespace: cma.deploymentNamespace, Name: cmName}.String())
	if cmPtr == nil {
		return nil
	}
	return ptr.Flatten(cmPtr)
}

func (cma *defaultCmAccessor) List() []*corev1.ConfigMap {
	return cma.cmCollection.List()
}

func (cma *defaultCmAccessor) WaitForCacheSync(ctx context.Context) bool {
	return cma.client.Core().WaitForCacheSync("config_map_syncer/ConfigMaps", ctx.Done(), cma.cmCollection.HasSynced)
}
