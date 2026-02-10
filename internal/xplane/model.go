package xplane

import (
	"fmt"
	"strings"
	"time"

	"github.com/brunoluiz/xpdig/internal/xplane/xpkg"
	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"
	pkgv1 "github.com/crossplane/crossplane/apis/pkg/v1"
	gcrname "github.com/google/go-containerregistry/pkg/name"
	corev1 "k8s.io/api/core/v1"
	errv1 "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Resource resource trace model, extracted from crossplane CLI codebase.
type Resource struct {
	Unstructured unstructured.Unstructured `json:"object"`
	Error        *errv1.StatusError        `json:"error,omitempty"`
	Children     []*Resource               `json:"children,omitempty"`
}

// GetCondition of this resource.
func (r *Resource) GetCondition(ct xpv1.ConditionType) xpv1.Condition {
	conditioned := xpv1.ConditionedStatus{}
	// The path is directly `status` because conditions are inline.
	if err := fieldpath.Pave(r.Unstructured.Object).GetValueInto("status", &conditioned); err != nil {
		return xpv1.Condition{}
	}
	// We didn't use xpv1.CondidionedStatus.GetCondition because that's defaulting the
	// status to unknown if the condition is not found at all.
	for _, c := range conditioned.Conditions {
		if c.Type == ct {
			return c
		}
	}
	return xpv1.Condition{}
}

type ResourceStatus struct {
	Name                 string
	ResourceName         string
	Ready                string
	ReadyLastTransition  time.Time
	Synced               string
	SyncedLastTransition time.Time
	Status               string
	Ok                   bool
	HasSyncedCondition   bool
}

func IsPkg(gk schema.GroupKind) bool {
	return xpkg.IsPackageType(gk) || xpkg.IsPackageRevisionType(gk)
}

// getResourceStatus returns a string that represents an entire row of status
// information for the resource.
func GetResourceStatus(r *Resource, name string) ResourceStatus {
	readyCond := r.GetCondition(xpv1.TypeReady)
	syncedCond := r.GetCondition(xpv1.TypeSynced)
	ok, hasSyncedCondition := okForReadySynced(readyCond, syncedCond)

	var status, m string
	switch {
	case r.Unstructured.GetDeletionTimestamp() != nil:
		// Report the status as deleted if the resource is being deleted
		status = "Deleting"
	case r.Error != nil:
		// if there is an error we want to show it
		status = "Error"
		m = r.Error.Error()
	default:
		// Ready is the primary reason when healthy, but we favor synced failures.
		status, m = resourceStatusFromConditions(readyCond, syncedCond)
	}

	// Append the message to the status if it's not empty
	if m != "" {
		status = fmt.Sprintf("%s: %s", status, m)
	}

	return ResourceStatus{
		Name:                 name,
		ResourceName:         r.Unstructured.GetAnnotations()["crossplane.io/composition-resource-name"],
		Ready:                mapEmptyStatusToDash(readyCond.Status),
		ReadyLastTransition:  readyCond.LastTransitionTime.Time,
		Synced:               mapEmptyStatusToDash(syncedCond.Status),
		SyncedLastTransition: syncedCond.LastTransitionTime.Time,
		Status:               status,
		Ok:                   ok,
		HasSyncedCondition:   hasSyncedCondition,
	}
}

type PkgResourceStatus struct {
	Name                    string
	PackageImg              string
	Version                 string
	Installed               string
	InstalledLastTransition time.Time
	Healthy                 string
	HealthyLastTransition   time.Time
	State                   string
	Status                  string
	Ok                      bool
}

func GetPkgResourceStatus(r *Resource, name string) PkgResourceStatus {
	var err error
	var packageImg, state, status, m string

	healthyCond := r.GetCondition(pkgv1.TypeHealthy)
	installedCond := r.GetCondition(pkgv1.TypeInstalled)
	hasInstalledCondition := conditionPresent(installedCond)

	gk := r.Unstructured.GroupVersionKind().GroupKind()
	switch {
	case r.Error != nil:
		// If there is an error we want to show it, regardless of what type this
		// resource is and what conditions it has.
		status = "Error"
		m = r.Error.Error()
	case xpkg.IsPackageType(gk):
		status, m = pkgStatusFromConditions(healthyCond, installedCond, hasInstalledCondition)

		if packageImg, err = fieldpath.Pave(r.Unstructured.Object).GetString("spec.package"); err != nil {
			state = err.Error()
		}
	case xpkg.IsPackageRevisionType(gk):
		// package revisions only have the healthy condition, so use that
		status, m = pkgStatusFromConditions(healthyCond, installedCond, false)

		// Get the state (active vs. inactive) of this package revision.
		var err error
		state, err = fieldpath.Pave(r.Unstructured.Object).GetString("spec.desiredState")
		if err != nil {
			state = err.Error()
		}
		// Get the image used.
		if packageImg, err = fieldpath.Pave(r.Unstructured.Object).GetString("spec.image"); err != nil {
			state = err.Error()
		}
	case xpkg.IsPackageRuntimeConfigType(gk):
		// nothing to do here
	default:
		status = "Unknown package type"
	}

	// Append the message to the status if it's not empty
	if m != "" {
		status = fmt.Sprintf("%s: %s", status, m)
	}

	// Parse the image reference extracting the tag, we'll leave it empty if we
	// couldn't parse it and leave the whole thing as package instead. We pass
	// an empty default registry here so the displayed package image will be
	// unmodified from what we found in the spec, similar to how kubectl output
	// behaves.
	var packageImgTag string
	if tag, err := gcrname.NewTag(packageImg, gcrname.WithDefaultRegistry("")); err == nil {
		packageImgTag = tag.TagStr()
		packageImg = tag.RepositoryStr()
		if tag.RegistryStr() != "" {
			packageImg = fmt.Sprintf("%s/%s", tag.RegistryStr(), packageImg)
		}
	}

	return PkgResourceStatus{
		Name:                    name,
		PackageImg:              packageImg,
		Version:                 packageImgTag,
		Installed:               mapEmptyStatusToDash(installedCond.Status),
		InstalledLastTransition: installedCond.LastTransitionTime.Time,
		Healthy:                 mapEmptyStatusToDash(healthyCond.Status),
		HealthyLastTransition:   healthyCond.LastTransitionTime.Time,
		State:                   mapEmptyStatusToDash(corev1.ConditionStatus(state)),
		Status:                  status,
		Ok: okForHealthyInstalled(healthyCond, installedCond, hasInstalledCondition) ||
			strings.HasPrefix(status, "Active") ||
			strings.HasPrefix(status, "Healthy"),
	}
}

// UsageAssociation represents the relationship between a Usage resource,
// its consumer ("by") and its target ("of").
type UsageAssociation struct {
	UsageID   string // ID of the Usage resource itself
	UsageName string // Display name of the Usage (Kind/Name)
	ByID      string // ID of the "by" resource (consumer)
	ByLabel   string // Display label (Kind/Name) of the "by" resource
	OfID      string // ID of the "of" resource (target/protected)
	OfLabel   string // Display label (Kind/Name) of the "of" resource
}

// IsUsage returns true if the resource is a Crossplane Usage kind.
func IsUsage(r *Resource) bool {
	return r.Unstructured.GetKind() == "Usage"
}

// GetUsageAssociation extracts the by/of association from a Usage resource.
// Returns nil if not a Usage or if the association fields are missing.
func GetUsageAssociation(r *Resource) *UsageAssociation {
	if !IsUsage(r) {
		return nil
	}

	paved := fieldpath.Pave(r.Unstructured.Object)

	byID, byLabel := usageRefToIDAndLabel(paved, "spec.by")
	ofID, ofLabel := usageRefToIDAndLabel(paved, "spec.of")

	if byID == "" && ofID == "" {
		return nil
	}

	group := r.Unstructured.GroupVersionKind().Group
	usageID := fmt.Sprintf("%s.%s/%s", r.Unstructured.GetKind(), group, r.Unstructured.GetName())
	usageName := fmt.Sprintf("%s/%s", r.Unstructured.GetKind(), r.Unstructured.GetName())

	return &UsageAssociation{
		UsageID:   usageID,
		UsageName: usageName,
		ByID:      byID,
		ByLabel:   byLabel,
		OfID:      ofID,
		OfLabel:   ofLabel,
	}
}

// usageRefToIDAndLabel extracts a resource reference from the given path prefix
// and returns both the row ID (Kind.Group/Name) and display label (Kind/Name).
func usageRefToIDAndLabel(paved *fieldpath.Paved, prefix string) (string, string) {
	kind, err := paved.GetString(prefix + ".kind")
	if err != nil || kind == "" {
		return "", ""
	}
	name, err := paved.GetString(prefix + ".resourceRef.name")
	if err != nil || name == "" {
		return "", ""
	}
	apiVersion, _ := paved.GetString(prefix + ".apiVersion")
	group := apiVersionToGroup(apiVersion)

	id := fmt.Sprintf("%s.%s/%s", kind, group, name)
	label := fmt.Sprintf("%s/%s", kind, name)
	return id, label
}

// apiVersionToGroup extracts the group from an apiVersion string.
// e.g. "network.azure.upbound.io/v1beta2" -> "network.azure.upbound.io"
// e.g. "v1" -> ""
func apiVersionToGroup(apiVersion string) string {
	parts := strings.SplitN(apiVersion, "/", 2)
	if len(parts) == 2 {
		return parts[0]
	}
	return ""
}

// UsageData holds the collected Usage relationship data for a resource tree.
type UsageData struct {
	// Associations is the list of all Usage associations found in the tree.
	Associations []UsageAssociation
	// TargetUsageLabels maps a target row ID to the Usage display name that references it.
	// Used to add markers like "◆ (Usage/name)" on target rows.
	TargetUsageLabels map[string]string
}

// CollectUsageData walks the resource tree and collects all Usage
// associations plus target-to-usage label mappings.
func CollectUsageData(root *Resource) UsageData {
	data := UsageData{
		TargetUsageLabels: make(map[string]string),
	}
	collectUsageDataRecursive(root, &data)
	return data
}

func collectUsageDataRecursive(r *Resource, data *UsageData) {
	if assoc := GetUsageAssociation(r); assoc != nil {
		data.Associations = append(data.Associations, *assoc)
		if assoc.ByID != "" {
			data.TargetUsageLabels[assoc.ByID] = assoc.UsageName
		}
		if assoc.OfID != "" {
			data.TargetUsageLabels[assoc.OfID] = assoc.UsageName
		}
	}
	for _, child := range r.Children {
		collectUsageDataRecursive(child, data)
	}
}

func mapEmptyStatusToDash(s corev1.ConditionStatus) string {
	if s == "" {
		return "-"
	}
	return string(s)
}
