package controller

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func setSyncedCondition(conditions *[]metav1.Condition, generation int64, message string) {
	setCondition(conditions, generation, "Ready", metav1.ConditionTrue, "Synced", message)
}

func setErrorCondition(conditions *[]metav1.Condition, generation int64, reason string, err error) {
	setCondition(conditions, generation, "Ready", metav1.ConditionFalse, reason, err.Error())
}

func setCondition(conditions *[]metav1.Condition, generation int64, conditionType string, status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(conditions, metav1.Condition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: generation,
	})
}
