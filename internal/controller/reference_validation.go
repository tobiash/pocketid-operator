package controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	pocketidv1alpha1 "github.com/tobiash/pocketid-operator/api/v1alpha1"
)

// ValidateCrossNamespaceReference checks if a namespace can reference an instance
// Returns (allowed bool, reason string, error)
func ValidateCrossNamespaceReference(
	ctx context.Context,
	c client.Client,
	instance *pocketidv1alpha1.PocketIDInstance,
	referrerNamespace string,
) (bool, string, error) {
	// Same namespace is always allowed
	if instance.Namespace == referrerNamespace {
		return true, "", nil
	}

	// Check if cross-namespace references are configured
	if instance.Spec.AllowedReferences == nil || instance.Spec.AllowedReferences.Namespaces == nil {
		return false, fmt.Sprintf("instance %s/%s does not allow cross-namespace references", instance.Namespace, instance.Name), nil
	}

	namespaces := instance.Spec.AllowedReferences.Namespaces
	from := pocketidv1alpha1.NamespacesFromSame
	if namespaces.From != nil {
		from = *namespaces.From
	}

	switch from {
	case pocketidv1alpha1.NamespacesFromAll:
		return true, "", nil

	case pocketidv1alpha1.NamespacesFromSame:
		return false, fmt.Sprintf("instance %s/%s only allows same-namespace references", instance.Namespace, instance.Name), nil

	case pocketidv1alpha1.NamespacesFromSelector:
		if namespaces.Selector == nil {
			return false, fmt.Sprintf("instance %s/%s has Selector mode but no selector defined", instance.Namespace, instance.Name), nil
		}

		// Get the referrer namespace object
		ns := &corev1.Namespace{}
		if err := c.Get(ctx, types.NamespacedName{Name: referrerNamespace}, ns); err != nil {
			return false, "", fmt.Errorf("failed to get namespace %s: %w", referrerNamespace, err)
		}

		// Check if namespace matches selector
		selector, err := labels.Parse(labels.SelectorFromSet(namespaces.Selector.MatchLabels).String())
		if err != nil {
			return false, "", fmt.Errorf("failed to parse selector: %w", err)
		}

		if selector.Matches(labels.Set(ns.Labels)) {
			return true, "", nil
		}

		return false, fmt.Sprintf("namespace %s does not match selector for instance %s/%s", referrerNamespace, instance.Namespace, instance.Name), nil

	default:
		return false, fmt.Sprintf("unknown namespace selection mode: %s", from), nil
	}
}

// ResolveInstanceReference resolves a cross-namespace instance reference
func ResolveInstanceReference(
	ctx context.Context,
	c client.Client,
	ref pocketidv1alpha1.CrossNamespaceObjectReference,
	defaultNamespace string,
) (*pocketidv1alpha1.PocketIDInstance, error) {
	instanceNamespace := defaultNamespace
	if ref.Namespace != nil {
		instanceNamespace = *ref.Namespace
	}

	instance := &pocketidv1alpha1.PocketIDInstance{}
	err := c.Get(ctx, types.NamespacedName{
		Name:      ref.Name,
		Namespace: instanceNamespace,
	}, instance)

	return instance, err
}
