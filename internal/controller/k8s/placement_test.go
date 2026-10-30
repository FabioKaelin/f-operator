package k8s

import (
	"context"
	k8sv1 "github.com/fabiokaelin/f-operator/api/k8s/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"
)

func TestPlacementValidation(t *testing.T) {
	cases := []struct {
		name  string
		p     *k8sv1.FdeploymentPlacement
		valid bool
	}{
		{"legacy", nil, true},
		{"single", &k8sv1.FdeploymentPlacement{Mode: "SingleNode", Node: "raspy"}, true},
		{"missing node", &k8sv1.FdeploymentPlacement{Mode: "SingleNode"}, false},
		{"conflict", &k8sv1.FdeploymentPlacement{Mode: "SingleNode", Node: "raspy", NodeSelector: map[string]string{corev1.LabelHostname: "raspy4"}}, false},
		{"wrong mode", &k8sv1.FdeploymentPlacement{Mode: "spread"}, false},
		{"distributed", &k8sv1.FdeploymentPlacement{Mode: "Distributed"}, true},
		{"required", &k8sv1.FdeploymentPlacement{Mode: "Distributed", Spread: "Required"}, true},
		{"invalid spread", &k8sv1.FdeploymentPlacement{Mode: "Distributed", Spread: "random"}, false},
		{"distributed pinned", &k8sv1.FdeploymentPlacement{Mode: "Distributed", Node: "raspy"}, false},
		{"single spread", &k8sv1.FdeploymentPlacement{Mode: "SingleNode", Node: "raspy", Spread: "Required"}, false},
		{"invalid labels", &k8sv1.FdeploymentPlacement{Mode: "Distributed", NodeSelector: map[string]string{"bad key": "x"}}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if (validatePlacement(c.p) == nil) != c.valid {
				t.Fatal("unexpected validation result")
			}
		})
	}
}

func TestPlacementReconciliationTransitions(t *testing.T) {
	ctx := context.Background()
	f := dummyDeployment()
	f.Spec.Replicas = 2
	r := newTestReconciler(t, f)
	runReconcile(t, r, f)
	dep := &appsv1.Deployment{}
	key := client.ObjectKeyFromObject(f)
	if err := r.Get(ctx, key, dep); err != nil {
		t.Fatal(err)
	}
	// Reproduce an old release selector containing a version, plus an injected sidecar.
	dep.Spec.Selector.MatchLabels["app.kubernetes.io/version"] = "old"
	dep.Spec.Template.Labels["app.kubernetes.io/version"] = "old"
	dep.Spec.Template.Spec.Containers = append(dep.Spec.Template.Spec.Containers, corev1.Container{Name: "sidecar", Image: "example:1"})
	if err := r.Update(ctx, dep); err != nil {
		t.Fatal(err)
	}
	for _, spread := range []string{"Required", "Preferred"} {
		if err := r.Get(ctx, key, f); err != nil {
			t.Fatal(err)
		}
		f.Spec.Placement = &k8sv1.FdeploymentPlacement{Mode: "Distributed", Spread: spread, NodeSelector: map[string]string{"fabkli.ch/production": "true"}}
		if err := r.Update(ctx, f); err != nil {
			t.Fatal(err)
		}
		runReconcile(t, r, f)
		if err := r.Get(ctx, key, dep); err != nil {
			t.Fatal(err)
		}
		anti := dep.Spec.Template.Spec.Affinity.PodAntiAffinity
		var term corev1.PodAffinityTerm
		if spread == "Required" {
			if len(anti.RequiredDuringSchedulingIgnoredDuringExecution) != 1 {
				t.Fatal("required spread missing")
			}
			term = anti.RequiredDuringSchedulingIgnoredDuringExecution[0]
			if dep.Spec.Strategy.RollingUpdate.MaxSurge.IntVal != 0 || dep.Spec.Strategy.RollingUpdate.MaxUnavailable.IntVal != 1 {
				t.Fatal("strict rollout can deadlock")
			}
		} else {
			if len(anti.RequiredDuringSchedulingIgnoredDuringExecution) != 0 || len(anti.PreferredDuringSchedulingIgnoredDuringExecution) != 1 {
				t.Fatal("required spread retained")
			}
			term = anti.PreferredDuringSchedulingIgnoredDuringExecution[0].PodAffinityTerm
			if dep.Spec.Strategy.RollingUpdate == nil || dep.Spec.Strategy.RollingUpdate.MaxSurge.StrVal != "25%" {
				t.Fatal("strict rollout retained")
			}
		}
		if _, ok := term.LabelSelector.MatchLabels["app.kubernetes.io/version"]; ok {
			t.Fatal("spread must match old and new ReplicaSets")
		}
		if term.TopologyKey != corev1.LabelHostname || dep.Spec.Template.Spec.NodeSelector["fabkli.ch/production"] != "true" {
			t.Fatal("placement lost")
		}
		rv := dep.ResourceVersion
		runReconcile(t, r, f)
		_ = r.Get(ctx, key, dep)
		if rv != dep.ResourceVersion {
			t.Fatal("placement reconciliation not idempotent")
		}
	}
	_ = r.Get(ctx, key, f)
	f.Spec.Placement = &k8sv1.FdeploymentPlacement{Mode: "SingleNode", Node: "raspy"}
	_ = r.Update(ctx, f)
	runReconcile(t, r, f)
	_ = r.Get(ctx, key, dep)
	if dep.Spec.Template.Spec.NodeSelector[corev1.LabelHostname] != "raspy" || dep.Spec.Template.Spec.Affinity != nil {
		t.Fatal("single mode retained spreading")
	}
	_ = r.Get(ctx, key, f)
	f.Spec.Placement = nil
	_ = r.Update(ctx, f)
	runReconcile(t, r, f)
	_ = r.Get(ctx, key, dep)
	if len(dep.Spec.Template.Spec.NodeSelector) != 0 || dep.Spec.Template.Spec.Affinity != nil || dep.Annotations[placementManagedAnnotation] != "" {
		t.Fatal("removed placement still active")
	}
	if dep.Spec.Selector.MatchLabels["app.kubernetes.io/version"] != "old" || len(dep.Spec.Template.Spec.Containers) != 2 {
		t.Fatal("legacy selector or sidecar changed")
	}
}

func TestLegacyPlacementPreservesExternalScheduling(t *testing.T) {
	ctx := context.Background()
	f := dummyDeployment()
	r := newTestReconciler(t, f)
	runReconcile(t, r, f)
	dep := &appsv1.Deployment{}
	key := client.ObjectKeyFromObject(f)
	_ = r.Get(ctx, key, dep)
	dep.Spec.Template.Spec.NodeSelector = map[string]string{"existing": "label"}
	dep.Spec.Strategy = appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType}
	_ = r.Update(ctx, dep)
	runReconcile(t, r, f)
	_ = r.Get(ctx, key, dep)
	if dep.Spec.Template.Spec.NodeSelector["existing"] != "label" || dep.Spec.Strategy.Type != appsv1.RecreateDeploymentStrategyType {
		t.Fatal("legacy scheduling changed without opt-in")
	}
}
