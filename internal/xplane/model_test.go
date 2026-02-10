package xplane

import (
	"testing"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestGetResourceStatusNoSyncedCondition(t *testing.T) {
	resource := &Resource{
		Unstructured: unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "usage.crossplane.io/v1alpha1",
				"kind":       "Usage",
				"status": map[string]interface{}{
					"conditions": []interface{}{
						map[string]interface{}{
							"type":   string(xpv1.TypeReady),
							"status": string(corev1.ConditionTrue),
						},
					},
				},
			},
		},
	}

	status := GetResourceStatus(resource, "Usage/example")
	if status.HasSyncedCondition {
		t.Fatalf("expected HasSyncedCondition to be false")
	}
	if !status.Ok {
		t.Fatalf("expected Ok to be true when Ready is true and Synced condition is absent")
	}
}

func TestOkForReadySynced(t *testing.T) {
	readyTrue := xpv1.Condition{Type: xpv1.TypeReady, Status: corev1.ConditionTrue}
	syncedTrue := xpv1.Condition{Type: xpv1.TypeSynced, Status: corev1.ConditionTrue}
	syncedFalse := xpv1.Condition{Type: xpv1.TypeSynced, Status: corev1.ConditionFalse}

	ok, hasSynced := okForReadySynced(readyTrue, syncedTrue)
	if !hasSynced || !ok {
		t.Fatalf("expected ok with both conditions true")
	}

	ok, hasSynced = okForReadySynced(readyTrue, syncedFalse)
	if !hasSynced || ok {
		t.Fatalf("expected not ok when synced is false")
	}

	ok, hasSynced = okForReadySynced(readyTrue, xpv1.Condition{})
	if hasSynced || !ok {
		t.Fatalf("expected ok when synced is absent and ready is true")
	}
}

func TestGetUsageAssociation(t *testing.T) {
	// Usage resource with both "by" and "of" references
	usage := &Resource{
		Unstructured: unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "apiextensions.crossplane.io/v1alpha1",
				"kind":       "Usage",
				"metadata": map[string]interface{}{
					"name": "my-usage",
				},
				"spec": map[string]interface{}{
					"by": map[string]interface{}{
						"apiVersion": "http.crossplane.io/v1alpha2",
						"kind":       "DisposableRequest",
						"resourceRef": map[string]interface{}{
							"name": "my-request",
						},
					},
					"of": map[string]interface{}{
						"apiVersion": "network.azure.upbound.io/v1beta2",
						"kind":       "PrivateEndpoint",
						"resourceRef": map[string]interface{}{
							"name": "my-endpoint",
						},
					},
				},
			},
		},
	}

	assoc := GetUsageAssociation(usage)
	if assoc == nil {
		t.Fatal("expected association, got nil")
	}
	if assoc.UsageID != "Usage.apiextensions.crossplane.io/my-usage" {
		t.Fatalf("unexpected UsageID: %s", assoc.UsageID)
	}
	if assoc.UsageName != "Usage/my-usage" {
		t.Fatalf("unexpected UsageName: %s", assoc.UsageName)
	}
	if assoc.ByID != "DisposableRequest.http.crossplane.io/my-request" {
		t.Fatalf("unexpected ByID: %s", assoc.ByID)
	}
	if assoc.ByLabel != "DisposableRequest/my-request" {
		t.Fatalf("unexpected ByLabel: %s", assoc.ByLabel)
	}
	if assoc.OfID != "PrivateEndpoint.network.azure.upbound.io/my-endpoint" {
		t.Fatalf("unexpected OfID: %s", assoc.OfID)
	}
	if assoc.OfLabel != "PrivateEndpoint/my-endpoint" {
		t.Fatalf("unexpected OfLabel: %s", assoc.OfLabel)
	}
}

func TestGetUsageAssociationNonUsage(t *testing.T) {
	// Non-Usage resource should return nil
	resource := &Resource{
		Unstructured: unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "network.azure.upbound.io/v1beta2",
				"kind":       "PrivateEndpoint",
				"metadata": map[string]interface{}{
					"name": "my-endpoint",
				},
			},
		},
	}

	assoc := GetUsageAssociation(resource)
	if assoc != nil {
		t.Fatal("expected nil for non-Usage resource")
	}
}

func TestCollectUsageAssociations(t *testing.T) {
	root := &Resource{
		Unstructured: unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "myapi.io/v1",
				"kind":       "Claim",
				"metadata":   map[string]interface{}{"name": "root"},
			},
		},
		Children: []*Resource{
			{
				Unstructured: unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "apiextensions.crossplane.io/v1alpha1",
						"kind":       "Usage",
						"metadata":   map[string]interface{}{"name": "my-usage"},
						"spec": map[string]interface{}{
							"by": map[string]interface{}{
								"apiVersion":  "http.crossplane.io/v1alpha2",
								"kind":        "DisposableRequest",
								"resourceRef": map[string]interface{}{"name": "my-request"},
							},
							"of": map[string]interface{}{
								"apiVersion":  "network.azure.upbound.io/v1beta2",
								"kind":        "PrivateEndpoint",
								"resourceRef": map[string]interface{}{"name": "my-endpoint"},
							},
						},
					},
				},
			},
			{
				Unstructured: unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "network.azure.upbound.io/v1beta2",
						"kind":       "PrivateEndpoint",
						"metadata":   map[string]interface{}{"name": "my-endpoint"},
					},
				},
			},
		},
	}

	data := CollectUsageData(root)

	// Should contain target labels for by and of IDs
	expectedTargets := map[string]string{
		"DisposableRequest.http.crossplane.io/my-request":      "Usage/my-usage",
		"PrivateEndpoint.network.azure.upbound.io/my-endpoint": "Usage/my-usage",
	}
	for id, expectedLabel := range expectedTargets {
		label, ok := data.TargetUsageLabels[id]
		if !ok {
			t.Fatalf("expected %s in target usage labels", id)
		}
		if label != expectedLabel {
			t.Fatalf("expected label %q for %s, got %q", expectedLabel, id, label)
		}
	}

	// Should have one association
	if len(data.Associations) != 1 {
		t.Fatalf("expected 1 association, got %d", len(data.Associations))
	}

	// Root should NOT be in the target labels
	if _, ok := data.TargetUsageLabels["Claim.myapi.io/root"]; ok {
		t.Fatal("root should not be in target usage labels")
	}
}

func TestApiVersionToGroup(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"network.azure.upbound.io/v1beta2", "network.azure.upbound.io"},
		{"http.crossplane.io/v1alpha2", "http.crossplane.io"},
		{"v1", ""},
		{"", ""},
	}

	for _, tt := range tests {
		got := apiVersionToGroup(tt.input)
		if got != tt.expected {
			t.Fatalf("apiVersionToGroup(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}
