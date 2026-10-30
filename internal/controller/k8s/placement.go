package k8s

import (
	"fmt"
	k8sv1 "github.com/fabiokaelin/f-operator/api/k8s/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation"
)

const placementManagedAnnotation = "k8s.fabkli.ch/placement-managed"

func validatePlacement(p *k8sv1.FdeploymentPlacement) error {
	if p == nil {
		return nil
	}
	if p.Mode != "SingleNode" && p.Mode != "Distributed" {
		return fmt.Errorf("placement mode must be SingleNode or Distributed")
	}
	for key, value := range p.NodeSelector {
		if len(validation.IsQualifiedName(key)) != 0 || len(validation.IsValidLabelValue(value)) != 0 {
			return fmt.Errorf("invalid placement nodeSelector")
		}
	}
	if p.Mode == "SingleNode" {
		if p.Node == "" || len(validation.IsValidLabelValue(p.Node)) != 0 {
			return fmt.Errorf("SingleNode placement requires a valid hostname label value")
		}
		if value, ok := p.NodeSelector[corev1.LabelHostname]; ok && value != p.Node {
			return fmt.Errorf("placement node conflicts with nodeSelector hostname")
		}
		if p.Spread != "" {
			return fmt.Errorf("spread applies only to Distributed placement")
		}
	} else {
		if p.Node != "" {
			return fmt.Errorf("Distributed placement must not specify node")
		}
		if p.Spread != "" && p.Spread != "Preferred" && p.Spread != "Required" {
			return fmt.Errorf("spread must be Preferred or Required")
		}
	}
	return nil
}

func applyPlacement(dep *appsv1.Deployment, p *k8sv1.FdeploymentPlacement) {
	if p == nil {
		return
	}
	if dep.Annotations == nil {
		dep.Annotations = map[string]string{}
	}
	dep.Annotations[placementManagedAnnotation] = "true"
	// Match API defaults explicitly, avoiding an endless patch/default reconciliation loop.
	quarter := intstr.FromString("25%")
	dep.Spec.Strategy = appsv1.DeploymentStrategy{Type: appsv1.RollingUpdateDeploymentStrategyType, RollingUpdate: &appsv1.RollingUpdateDeployment{MaxSurge: &quarter, MaxUnavailable: &quarter}}

	if len(p.NodeSelector) > 0 || p.Mode == "SingleNode" {
		dep.Spec.Template.Spec.NodeSelector = make(map[string]string, len(p.NodeSelector)+1)
	}
	for k, v := range p.NodeSelector {
		dep.Spec.Template.Spec.NodeSelector[k] = v
	}
	if p.Mode == "SingleNode" {
		dep.Spec.Template.Spec.NodeSelector[corev1.LabelHostname] = p.Node
		return
	}
	// Stable labels span old/new ReplicaSets, including deployments with legacy version selectors.
	term := corev1.PodAffinityTerm{TopologyKey: corev1.LabelHostname, LabelSelector: &metav1.LabelSelector{MatchLabels: map[string]string{
		"app.kubernetes.io/name": dep.Name, "app.kubernetes.io/instance": dep.Name, "app.kubernetes.io/part-of": "f-operator",
	}}}
	anti := &corev1.PodAntiAffinity{}
	if p.Spread == "Required" {
		anti.RequiredDuringSchedulingIgnoredDuringExecution = []corev1.PodAffinityTerm{term}
		// A surge pod cannot schedule when every eligible node already has a replica.
		zero, one := intstr.FromInt32(0), intstr.FromInt32(1)
		dep.Spec.Strategy = appsv1.DeploymentStrategy{Type: appsv1.RollingUpdateDeploymentStrategyType, RollingUpdate: &appsv1.RollingUpdateDeployment{MaxSurge: &zero, MaxUnavailable: &one}}
	} else {
		anti.PreferredDuringSchedulingIgnoredDuringExecution = []corev1.WeightedPodAffinityTerm{{Weight: 100, PodAffinityTerm: term}}
	}
	dep.Spec.Template.Spec.Affinity = &corev1.Affinity{PodAntiAffinity: anti}
}
