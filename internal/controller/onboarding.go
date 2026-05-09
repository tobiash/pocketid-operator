package controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	pocketidv1alpha1 "github.com/tobiash/pocketid-operator/api/v1alpha1"
	"github.com/tobiash/pocketid-operator/internal/pocketid"
)

type onboardingResult struct {
	LinkCreated bool
	EmailSent   bool
	EmailSentAt *metav1.Time
}

func (r *PocketIDUserReconciler) reconcileOnboarding(ctx context.Context, apiClient *pocketid.Client, user *pocketidv1alpha1.PocketIDUser, pocketIDUser *pocketid.User, instance *pocketidv1alpha1.PocketIDInstance) onboardingResult {
	result := onboardingResult{
		LinkCreated: user.Status.OnboardingLinkCreated,
		EmailSent:   user.Status.OnboardingEmailSent,
		EmailSentAt: user.Status.OnboardingEmailSentAt,
	}

	secretName := resolveOnboardingSecretName(user)
	if secretName == "" || result.LinkCreated {
		return result
	}

	logger := log.FromContext(ctx)
	logger.Info("Creating one-time access token", "userId", pocketIDUser.ID, "secret", secretName)
	resp, err := apiClient.CreateOneTimeAccessToken(ctx, pocketIDUser.ID)
	if err != nil {
		logger.Error(err, "Failed to create one-time access token")
		return result
	}

	if resp != nil {
		link := fmt.Sprintf("%s/login/%s", instance.Spec.AppURL, resp.Token)
		if err := r.storeOneTimeAccessLink(ctx, user, secretName, link); err != nil {
			logger.Error(err, "Failed to store one-time access link")
			return result
		}
	}

	result.LinkCreated = true

	if user.Spec.SendOnboardingEmail && !result.EmailSent {
		now := metav1.Now()
		result.EmailSent = true
		result.EmailSentAt = &now
	}

	return result
}

func resolveOnboardingSecretName(user *pocketidv1alpha1.PocketIDUser) string {
	if user.Spec.OneTimeAccessSecretRef != nil {
		return user.Spec.OneTimeAccessSecretRef.Name
	}
	return user.Name + "-onboarding"
}

func (r *PocketIDUserReconciler) storeOneTimeAccessLink(ctx context.Context, user *pocketidv1alpha1.PocketIDUser, secretName, link string) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: user.Namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"ONE_TIME_ACCESS_LINK": []byte(link),
		},
	}

	if err := controllerutil.SetControllerReference(user, secret, r.Scheme); err != nil {
		return err
	}

	existing := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: user.Namespace}, existing)
	if err != nil {
		return r.Create(ctx, secret)
	}
	existing.Data = secret.Data
	return r.Update(ctx, existing)
}
