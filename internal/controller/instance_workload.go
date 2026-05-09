package controller

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	pocketidv1alpha1 "github.com/tobiash/pocketid-operator/api/v1alpha1"
)

func desiredInstanceConfigMap(instance *pocketidv1alpha1.PocketIDInstance, schemeSetter func(clientObject any) error) (*corev1.ConfigMap, error) {
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-config", instance.Name),
			Namespace: instance.Namespace,
		},
		Data: map[string]string{
			"APP_URL":          instance.Spec.AppURL,
			"TRUST_PROXY":      fmt.Sprintf("%t", instance.Spec.TrustProxy),
			"SESSION_DURATION": fmt.Sprintf("%d", instance.Spec.SessionDuration),
			"DB_PROVIDER":      instance.Spec.Database.Provider,
		},
	}

	if instance.Spec.SMTP != nil {
		configMap.Data["SMTP_HOST"] = instance.Spec.SMTP.Host
		configMap.Data["SMTP_PORT"] = fmt.Sprintf("%d", instance.Spec.SMTP.Port)
		configMap.Data["SMTP_FROM"] = instance.Spec.SMTP.From
		configMap.Data["SMTP_TLS"] = fmt.Sprintf("%t", instance.Spec.SMTP.TLS)
	}

	if err := schemeSetter(configMap); err != nil {
		return nil, err
	}
	return configMap, nil
}

func desiredInstanceService(instance *pocketidv1alpha1.PocketIDInstance, schemeSetter func(clientObject any) error) (*corev1.Service, error) {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-svc", instance.Name),
			Namespace: instance.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: instanceLabels(instance),
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       80,
					TargetPort: intstr.FromInt(1411),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}

	if err := schemeSetter(service); err != nil {
		return nil, err
	}
	return service, nil
}

func desiredInstanceStatefulSet(instance *pocketidv1alpha1.PocketIDInstance, schemeSetter func(clientObject any) error) (*appsv1.StatefulSet, error) {
	configMapName := fmt.Sprintf("%s-config", instance.Name)
	encryptionSecretName := fmt.Sprintf("%s-encryption-key", instance.Name)
	apiKeySecretName := fmt.Sprintf("%s-api-key", instance.Name)

	if instance.Spec.EncryptionKeySecretRef != nil {
		encryptionSecretName = instance.Spec.EncryptionKeySecretRef.Name
	}
	if instance.Spec.StaticAPIKeySecretRef != nil {
		apiKeySecretName = instance.Spec.StaticAPIKeySecretRef.Name
	}

	replicas := int32(1)
	if instance.Spec.Replicas != nil {
		replicas = *instance.Spec.Replicas
	}

	image := "ghcr.io/pocket-id/pocket-id:latest"
	if instance.Spec.Image != "" {
		image = instance.Spec.Image
	}

	labels := instanceLabels(instance)
	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name,
			Namespace: instance.Namespace,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:    &replicas,
			ServiceName: fmt.Sprintf("%s-svc", instance.Name),
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "pocketid",
							Image: image,
							Ports: []corev1.ContainerPort{
								{Name: "http", ContainerPort: 1411, Protocol: corev1.ProtocolTCP},
							},
							EnvFrom: []corev1.EnvFromSource{
								{ConfigMapRef: &corev1.ConfigMapEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: configMapName}}},
								{SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: encryptionSecretName}}},
								{SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: apiKeySecretName}}},
							},
							Resources: instance.Spec.Resources,
							ReadinessProbe: &corev1.Probe{
								ProbeHandler:        corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/healthz", Port: intstr.FromInt(1411)}},
								InitialDelaySeconds: 30,
								PeriodSeconds:       10,
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler:        corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/healthz", Port: intstr.FromInt(1411)}},
								InitialDelaySeconds: 45,
								PeriodSeconds:       20,
							},
							VolumeMounts: []corev1.VolumeMount{{Name: "data", MountPath: "/app/data"}},
						},
					},
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "data"},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
						Resources: corev1.VolumeResourceRequirements{
							Requests: corev1.ResourceList{corev1.ResourceStorage: parseStorageQuantity(instance.Spec.Storage.PVC)},
						},
					},
				},
			},
		},
	}
	if instance.Spec.Storage.PVC != nil && instance.Spec.Storage.PVC.StorageClassName != nil {
		statefulSet.Spec.VolumeClaimTemplates[0].Spec.StorageClassName = instance.Spec.Storage.PVC.StorageClassName
	}

	if err := schemeSetter(statefulSet); err != nil {
		return nil, err
	}
	return statefulSet, nil
}

func instanceLabels(instance *pocketidv1alpha1.PocketIDInstance) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       "pocketid",
		"app.kubernetes.io/instance":   instance.Name,
		"app.kubernetes.io/managed-by": "pocketid-operator",
	}
}

func parseStorageQuantity(pvc *pocketidv1alpha1.PVCConfig) resource.Quantity {
	size := "1Gi"
	if pvc != nil && pvc.Size != "" {
		size = pvc.Size
	}
	q, err := resource.ParseQuantity(size)
	if err != nil {
		q, _ = resource.ParseQuantity("1Gi")
	}
	return q
}

func ownerSetter(owner *pocketidv1alpha1.PocketIDInstance, scheme *runtime.Scheme) func(any) error {
	return func(object any) error {
		clientObject, ok := object.(client.Object)
		if !ok {
			return fmt.Errorf("object does not implement client.Object")
		}
		return controllerutil.SetControllerReference(owner, clientObject, scheme)
	}
}
