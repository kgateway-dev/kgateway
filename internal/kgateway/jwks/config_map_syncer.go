package jwks

import (
	"context"
	"crypto/md5" //nolint:gosec
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kgateway-dev/kgateway/v2/pkg/apiclient"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

const jwksStorePrefix = "jwks-store"
const jwksStoreComponent = "app.kubernetes.io/component"
const JwksStoreLabelString = jwksStoreComponent + "=" + jwksStorePrefix

var JwksStoreLabelMap = map[string]string{jwksStoreComponent: jwksStorePrefix}

// configMapSyncer is used for writing/reading jwks' to/from ConfigMaps.
type configMapSyncer struct {
	deploymentNamespace string
	client              apiclient.Client
}

func NewConfigMapSyncer(client apiclient.Client, deploymentNamespace string, krtOptions krtutil.KrtOptions) *configMapSyncer {
	toret := configMapSyncer{
		client:              client,
		deploymentNamespace: deploymentNamespace,
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

// Generates ConfigMap name based on jwks uri. Resulting name is a concatenation of "jwks-store-" prefix and an MD5 hash of the jwks uri.
// The length of the name is a constant 32 chars (hash) + legth of the prefix.
func JwksConfigMapName(jwksUri string) string {
	hash := md5.Sum([]byte(jwksUri)) //nolint:gosec
	return fmt.Sprintf("%s-%s", jwksStorePrefix, hex.EncodeToString(hash[:]))
}

func SetJwksInConfigMap(cm *corev1.ConfigMap, uri, jwks string) error {
	b, err := json.Marshal(map[string]string{uri: jwks})
	if err != nil {
		return err
	}
	cm.Data[jwksStorePrefix] = string(b)
	return nil
}

// Loads all jwks persisted in ConfigMaps. The result is a map of jwks-uri to serialized jwks.
func (cs *configMapSyncer) LoadJwksFromConfigMaps(ctx context.Context) (map[string]string, error) {
	log := log.FromContext(ctx)

	allPersistedJwks, err := cs.client.Kube().CoreV1().ConfigMaps(cs.deploymentNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: JwksStoreLabelString,
	})
	if err != nil {
		return nil, err
	}

	if len(allPersistedJwks.Items) == 0 {
		return nil, nil
	}

	errs := make([]error, 0)
	toret := make(map[string]string)
	for _, cm := range allPersistedJwks.Items {
		jwks, err := JwksFromConfigMap(&cm)
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
			Labels:    JwksStoreLabelMap,
		},
		Data: make(map[string]string),
	}
}
