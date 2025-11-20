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

var JwksStoreLabel = map[string]string{JwksStoreComponent: jwksStorePrefix}

// configMapSyncer is used for writing/reading jwks' to/from ConfigMaps.
type configMapSyncer struct {
	client              apiclient.Client
	deploymentNamespace string
	cmCollection        krt.Collection[*corev1.ConfigMap]
}

func NewConfigMapSyncer(client apiclient.Client, deploymentNamespace string, krtOptions krtutil.KrtOptions) *configMapSyncer {
	toret := configMapSyncer{
		client:              client,
		deploymentNamespace: deploymentNamespace,
		cmCollection: krt.NewInformerFiltered[*corev1.ConfigMap](client,
			kclient.Filter{
				ObjectFilter:  client.ObjectFilter(),
				LabelSelector: JwksStoreComponent + "=" + jwksStorePrefix},
			krtOptions.ToOptions("config_map_syncer/ConfigMaps")...),
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
	return cs.client.Core().WaitForCacheSync("config_map_syncer/ConfigMaps", ctx.Done(), cs.cmCollection.HasSynced)
}

// Generates ConfigMap name based on jwks uri. Resulting name is a concatenation of "jwks-store-" prefix and an MD5 hash of the jwks uri.
// The length of the name is a constant 32 chars (hash) + legth of the prefix.
func JwksConfigMapName(jwksUri string) string {
	hash := md5.Sum([]byte(jwksUri)) //nolint:gosec
	return fmt.Sprintf("%s-%s", jwksStorePrefix, hex.EncodeToString(hash[:]))
}

// Write out jwks' in updates to ConfigMaps, one jwks uri per ConfigMap. updates contains a map of jwks-uri to serialized jwks.
// Each ConfigMap is labelled with "app.kubernetes.io/component":"jwks-store" to support bulk loading of jwks' handled by LoadJwksFromConfigMaps().
func (cs *configMapSyncer) WriteJwksToConfigMaps(ctx context.Context, updates map[string]string) error {
	log := log.FromContext(ctx)
	errs := make([]error, 0)

	for uri, jwks := range updates {
		switch jwks {
		case "": // empty jwks == remove the underlying ConfigMap
			err := cs.client.Kube().CoreV1().ConfigMaps(cs.deploymentNamespace).Delete(ctx, JwksConfigMapName(uri), metav1.DeleteOptions{})
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

			existing := cs.fetchPersistedJwks(uri)
			if existing == nil {
				cm := cs.newJwksStoreConfigMap(JwksConfigMapName(uri))

				cm.Data[jwksStorePrefix] = string(cmData)
				cm, err := cs.client.Kube().CoreV1().ConfigMaps(cs.deploymentNamespace).Create(ctx, cm, metav1.CreateOptions{})
				if err != nil {
					log.Error(err, "error persisting jwks to ConfigMap")
					errs = append(errs, err)
					continue
				}
			} else {
				existing.Data[jwksStorePrefix] = string(cmData)
				_, err = cs.client.Kube().CoreV1().ConfigMaps(cs.deploymentNamespace).Update(ctx, existing, metav1.UpdateOptions{})
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

	allPersistedJwks := cs.cmCollection.List()

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
			Labels:    JwksStoreLabel,
		},
		Data: make(map[string]string),
	}
}

func (cs *configMapSyncer) fetchPersistedJwks(jwksUri string) *corev1.ConfigMap {
	cmPtr := cs.cmCollection.GetKey(types.NamespacedName{Namespace: cs.deploymentNamespace, Name: JwksConfigMapName(jwksUri)}.String())
	if cmPtr == nil {
		return nil
	}
	return ptr.Flatten(cmPtr)
}
