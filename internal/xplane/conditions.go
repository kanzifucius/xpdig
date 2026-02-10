package xplane

import (
	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	corev1 "k8s.io/api/core/v1"
)

func conditionPresent(c xpv1.Condition) bool {
	return c.Type != ""
}

func conditionHasDetails(c xpv1.Condition) bool {
	return c.Reason != "" || c.Message != ""
}

func resourceStatusFromConditions(readyCond, syncedCond xpv1.Condition) (string, string) {
	var status, message string

	switch {
	case syncedCond.Status == corev1.ConditionTrue && readyCond.Status == corev1.ConditionTrue:
		status = string(readyCond.Reason)
	case syncedCond.Status != corev1.ConditionTrue && conditionHasDetails(syncedCond):
		status = string(syncedCond.Reason)
		message = syncedCond.Message
	case readyCond.Status != corev1.ConditionTrue && conditionHasDetails(readyCond):
		status = string(readyCond.Reason)
		message = readyCond.Message
	default:
		status = string(readyCond.Reason)
		message = readyCond.Message
	}

	return status, message
}

func okForReadySynced(readyCond, syncedCond xpv1.Condition) (bool, bool) {
	hasSyncedCondition := conditionPresent(syncedCond)
	if hasSyncedCondition {
		return syncedCond.Status == corev1.ConditionTrue && readyCond.Status == corev1.ConditionTrue, hasSyncedCondition
	}
	return readyCond.Status == corev1.ConditionTrue, hasSyncedCondition
}

func pkgStatusFromConditions(healthyCond, installedCond xpv1.Condition, hasInstalledCondition bool) (string, string) {
	var status, message string

	if hasInstalledCondition {
		switch {
		case healthyCond.Status == corev1.ConditionTrue && installedCond.Status == corev1.ConditionTrue:
			status = string(healthyCond.Reason)
		case installedCond.Status != corev1.ConditionTrue && conditionHasDetails(installedCond):
			status = string(installedCond.Reason)
			message = installedCond.Message
		case healthyCond.Status != corev1.ConditionTrue && conditionHasDetails(healthyCond):
			status = string(healthyCond.Reason)
			message = healthyCond.Message
		default:
			status = string(installedCond.Reason)
			message = installedCond.Message
		}
		return status, message
	}

	status = string(healthyCond.Reason)
	message = healthyCond.Message
	return status, message
}

func okForHealthyInstalled(healthyCond, installedCond xpv1.Condition, hasInstalledCondition bool) bool {
	if hasInstalledCondition {
		return installedCond.Status == corev1.ConditionTrue && healthyCond.Status == corev1.ConditionTrue
	}
	return healthyCond.Status == corev1.ConditionTrue
}
