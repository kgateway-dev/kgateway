package krtcollections

import (
	"fmt"

	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

type From struct {
	schema.GroupKind
	Namespace string
}

type SecretIndex struct {
	secrets   map[schema.GroupKind]krt.Collection[ir.Secret]
	refgrants *RefGrantIndex
}

func NewSecretIndex(secrets map[schema.GroupKind]krt.Collection[ir.Secret], refgrants *RefGrantIndex) *SecretIndex {
	return &SecretIndex{secrets: secrets, refgrants: refgrants}
}

func (s *SecretIndex) HasSynced() bool {
	if !s.refgrants.HasSynced() {
		return false
	}
	for _, col := range s.secrets {
		if !col.HasSynced() {
			return false
		}
	}
	return true
}

// GetSecret retrieves a secret from the index, validating reference grants to ensure
// the source object is allowed to reference the target secret. Returns an error if
// the secret kind is unknown, reference grants are missing, or the secret is not found.
func (s *SecretIndex) GetSecret(kctx krt.HandlerContext, from From, secretRef gwv1.SecretObjectReference) (*ir.Secret, error) {
	secretKind := "Secret"
	secretGroup := ""
	toNs := strOr(secretRef.Namespace, from.Namespace)
	if secretRef.Group != nil {
		secretGroup = string(*secretRef.Group)
	}
	if secretRef.Kind != nil {
		secretKind = string(*secretRef.Kind)
	}

	to := ir.ObjectSource{
		Group:     secretGroup,
		Kind:      secretKind,
		Namespace: toNs,
		Name:      string(secretRef.Name),
	}
	col := s.secrets[schema.GroupKind{Group: secretGroup, Kind: secretKind}]
	if col == nil {
		// should never happen
		return nil, fmt.Errorf("internal error looking up secret %s", to.NamespacedName())
	}

	if !s.refgrants.ReferenceAllowed(kctx, from.GroupKind, from.Namespace, to) {
		return nil, fmt.Errorf("cannot reference secret %s : %w", to.NamespacedName(), ErrMissingReferenceGrant)
	}
	secret := krt.FetchOne(kctx, col, krt.FilterKey(to.ResourceName()))
	if secret == nil {
		return nil, &NotFoundError{NotFoundObj: to}
	}
	return secret, nil
}

// GetSecretsBySelector retrieves secrets matching the label selector,
// validating reference grants to ensure the source (from) object is allowed to reference each secret.
// Processes all matching secrets, skipping those without required ReferenceGrants.
// Returns all accessible secrets. Only returns an error if no accessible secrets were found and some matching secrets
// were skipped due to missing ReferenceGrants (indicating a possible configuration issue).
func (s *SecretIndex) GetSecretsBySelector(
	kctx krt.HandlerContext,
	from From,
	secretGK schema.GroupKind,
	matchLabels map[string]string,
) ([]ir.Secret, error) {
	col := s.secrets[secretGK]
	if col == nil {
		return nil, ErrUnknownBackendKind
	}

	// First, fetch all secrets matching the label selector
	labelMatchedSecrets := krt.Fetch(kctx, col,
		krt.FilterGeneric(func(obj any) bool {
			secret := obj.(ir.Secret)

			// Check labels from the underlying Kubernetes Secret object
			if secret.Obj == nil {
				return false
			}
			objLabels := secret.Obj.GetLabels()
			if objLabels == nil {
				return false
			}
			// Check if all matchLabels are present and match
			for key, value := range matchLabels {
				if objLabels[key] != value {
					return false
				}
			}
			return true
		}),
	)

	// Validate ReferenceGrant for cross-namespace secrets and collect allowed ones
	var allowedSecrets []ir.Secret
	var hasMissingGrants bool
	for _, secret := range labelMatchedSecrets {
		// Only check ReferenceGrant if this is a cross-namespace reference
		if from.Namespace != secret.Namespace {
			to := ir.ObjectSource{
				Group:     secret.Group,
				Kind:      secret.Kind,
				Namespace: secret.Namespace,
				Name:      secret.Name,
			}
			if !s.refgrants.ReferenceAllowed(kctx, from.GroupKind, from.Namespace, to) {
				hasMissingGrants = true
				continue
			}
		}
		allowedSecrets = append(allowedSecrets, secret)
	}

	// Only return an error if no allowed secrets were found and there were missing grants.
	// We don't want to list all the secrets that were skipped. We only want to hint
	// the user that it might be a configuration issue.
	if len(allowedSecrets) == 0 && hasMissingGrants {
		return allowedSecrets, ErrMissingReferenceGrant
	}

	return allowedSecrets, nil
}
