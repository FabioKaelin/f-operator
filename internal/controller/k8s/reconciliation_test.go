package k8s

import (
	"context"
	k8sv1 "github.com/fabiokaelin/f-operator/api/k8s/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	"testing"
)

func testScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	for _, add := range []func(*runtime.Scheme) error{clientgoscheme.AddToScheme, k8sv1.AddToScheme, gatewayv1.Install} {
		if err := add(s); err != nil {
			t.Fatal(err)
		}
	}
	return s
}
func dummyDeployment() *k8sv1.Fdeployment {
	return &k8sv1.Fdeployment{ObjectMeta: metav1.ObjectMeta{Name: "backend", Namespace: "dummy", UID: "dummy-uid", Generation: 1}, Spec: k8sv1.FdeploymentSpec{Host: "dummy.example", Path: "/api", Replicas: 1, Port: 8080, Tag: "1.0.0", Resources: k8sv1.FdeploymentResources{Requests: k8sv1.Resource{CPU: "10m", Memory: "16Mi"}, Limits: k8sv1.Resource{CPU: "100m", Memory: "64Mi"}}, HealthCheck: k8sv1.FdeploymentHealthCheck{ReadinessProbe: k8sv1.HealthProbe{Path: "/ready"}, LivenessProbe: k8sv1.HealthProbe{Path: "/live"}}}}
}
func newTestReconciler(t *testing.T, objects ...client.Object) *FdeploymentReconciler {
	s := testScheme(t)
	return &FdeploymentReconciler{Client: fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&k8sv1.Fdeployment{}, &k8sv1.Fdatabase{}, &appsv1.Deployment{}, &gatewayv1.HTTPRoute{}, &gatewayv1.Gateway{}).WithObjects(objects...).Build(), Scheme: s, GatewayName: "shared", GatewayNamespace: "gateway", GatewayListener: "http"}
}
func runReconcile(t *testing.T, r *FdeploymentReconciler, f *k8sv1.Fdeployment) {
	t.Helper()
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(f)}); err != nil {
		t.Fatal(err)
	}
}
func TestDummyReconciliation(t *testing.T) {
	ctx := context.Background()
	f := dummyDeployment()
	r := newTestReconciler(t, f)
	runReconcile(t, r, f)
	route := &gatewayv1.HTTPRoute{}
	dep := &appsv1.Deployment{}
	svc := &corev1.Service{}
	key := client.ObjectKeyFromObject(f)
	for _, obj := range []client.Object{route, dep, svc} {
		if err := r.Get(ctx, key, obj); err != nil {
			t.Fatal(err)
		}
	}
	if route.Spec.Hostnames[0] != "dummy.example" || *route.Spec.Rules[0].Matches[0].Path.Value != "/api" || *route.Spec.Rules[0].Matches[0].Path.Type != gatewayv1.PathMatchPathPrefix || *route.Spec.Rules[0].BackendRefs[0].Port != 80 || string(route.Spec.Rules[0].BackendRefs[0].Name) != f.Name {
		t.Fatal("routing changed")
	}
	if *dep.Spec.Template.Spec.Containers[0].SecurityContext.Privileged || *dep.Spec.Template.Spec.AutomountServiceAccountToken {
		t.Fatal("unsafe application defaults")
	}
	initialVersions := []string{route.ResourceVersion, dep.ResourceVersion, svc.ResourceVersion}
	runReconcile(t, r, f)
	for i, obj := range []client.Object{route, dep, svc} {
		if err := r.Get(ctx, key, obj); err != nil {
			t.Fatal(err)
		}
		if obj.GetResourceVersion() != initialVersions[i] {
			t.Fatal("unchanged resource rewritten")
		}
	}
	if err := r.Get(ctx, key, f); err != nil {
		t.Fatal(err)
	}
	f.Spec.Host = "new.example"
	f.Spec.Path = "/v2"
	f.Spec.Tag = "1.0.1"
	f.Spec.Port = 9090
	if err := r.Update(ctx, f); err != nil {
		t.Fatal(err)
	}
	// A manually emptied container list must be repaired without indexing panic.
	dep.Spec.Template.Spec.Containers = nil
	if err := r.Update(ctx, dep); err != nil {
		t.Fatal(err)
	}
	runReconcile(t, r, f)
	if err := r.Get(ctx, key, dep); err != nil {
		t.Fatal(err)
	}
	if dep.Spec.Template.Spec.Containers[0].Ports[0].ContainerPort != 9090 {
		t.Fatal("port not updated")
	}
	if err := r.Get(ctx, key, route); err != nil {
		t.Fatal(err)
	}
	if route.Spec.Hostnames[0] != "new.example" || *route.Spec.Rules[0].Matches[0].Path.Value != "/v2" {
		t.Fatal("route not updated")
	}
}
func TestOwnershipCollision(t *testing.T) {
	f := dummyDeployment()
	r := newTestReconciler(t, f, &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: f.Name, Namespace: f.Namespace}})
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(f)}); err == nil {
		t.Fatal("adopted unrelated object")
	}
}
func TestInvalidQuantityDoesNotPanic(t *testing.T) {
	f := dummyDeployment()
	f.Spec.Resources.Requests.Memory = "invalid"
	r := newTestReconciler(t, f)
	runReconcile(t, r, f)
	if err := r.Get(context.Background(), client.ObjectKeyFromObject(f), f); err != nil {
		t.Fatal(err)
	}
	if c := meta.FindStatusCondition(f.Status.Conditions, "Available"); c == nil || c.Reason != "InvalidSpec" {
		t.Fatal("invalid spec not reported")
	}
}
func acceptedConditions(generation int64, names ...string) []metav1.Condition {
	result := []metav1.Condition{}
	for _, name := range names {
		result = append(result, metav1.Condition{Type: name, Status: metav1.ConditionTrue, ObservedGeneration: generation, Reason: "Ready", Message: "dummy ready", LastTransitionTime: metav1.Now()})
	}
	return result
}
func TestLegacyCleanupRequiresExplicitGateAndCurrentReadiness(t *testing.T) {
	ctx := context.Background()
	f := dummyDeployment()
	r := newTestReconciler(t, f)
	runReconcile(t, r, f)
	legacy := &networking.Ingress{ObjectMeta: metav1.ObjectMeta{Name: f.Name, Namespace: f.Namespace}}
	if err := ctrl.SetControllerReference(f, legacy, r.Scheme); err != nil {
		t.Fatal(err)
	}
	if err := r.Create(ctx, legacy); err != nil {
		t.Fatal(err)
	}
	route := &gatewayv1.HTTPRoute{}
	key := client.ObjectKeyFromObject(f)
	if err := r.Get(ctx, key, route); err != nil {
		t.Fatal(err)
	}
	route.Status.Parents = []gatewayv1.RouteParentStatus{{ParentRef: route.Spec.ParentRefs[0], ControllerName: "gateway.envoyproxy.io/gatewayclass-controller", Conditions: acceptedConditions(route.Generation, "Accepted", "ResolvedRefs")}}
	if err := r.Status().Update(ctx, route); err != nil {
		t.Fatal(err)
	}
	g := &gatewayv1.Gateway{ObjectMeta: metav1.ObjectMeta{Name: "shared", Namespace: "gateway", Generation: 1}, Status: gatewayv1.GatewayStatus{Conditions: acceptedConditions(1, "Accepted", "Programmed"), Listeners: []gatewayv1.ListenerStatus{{Name: "http", Conditions: acceptedConditions(1, "Accepted", "Programmed", "ResolvedRefs")}}}}
	if err := r.Create(ctx, g); err != nil {
		t.Fatal(err)
	}
	runReconcile(t, r, f)
	if err := r.Get(ctx, key, legacy); err != nil {
		t.Fatal("cleanup without gate", err)
	}
	r.CleanupLegacyIngress = true
	route.Status.Parents[0].Conditions[0].Status = metav1.ConditionFalse
	if err := r.Status().Update(ctx, route); err != nil {
		t.Fatal(err)
	}
	runReconcile(t, r, f)
	if err := r.Get(ctx, key, legacy); err != nil {
		t.Fatal("cleanup of rejected route", err)
	}
	route.Status.Parents[0].Conditions = acceptedConditions(route.Generation, "Accepted", "ResolvedRefs")
	if err := r.Status().Update(ctx, route); err != nil {
		t.Fatal(err)
	}
	runReconcile(t, r, f)
	if err := r.Get(ctx, key, legacy); err == nil {
		t.Fatal("owned legacy ingress not cleaned")
	}
}
func TestRouteStatusMustMatchConfiguredParent(t *testing.T) {
	f := dummyDeployment()
	r := newTestReconciler(t, f)
	route, err := r.routeForFDeployment(f)
	if err != nil {
		t.Fatal(err)
	}
	route.Generation = 2
	route.Status.Parents = []gatewayv1.RouteParentStatus{{ParentRef: route.Spec.ParentRefs[0], ControllerName: "gateway.envoyproxy.io/gatewayclass-controller", Conditions: acceptedConditions(1, "Accepted", "ResolvedRefs")}}
	if r.routeReady(route) {
		t.Fatal("accepted stale generation")
	}
	route.Status.Parents[0].Conditions = acceptedConditions(2, "Accepted", "ResolvedRefs")
	route.Status.Parents[0].ParentRef.Name = "other"
	if r.routeReady(route) {
		t.Fatal("accepted unrelated parent")
	}
}
func dummyDatabase() *k8sv1.Fdatabase {
	return &k8sv1.Fdatabase{ObjectMeta: metav1.ObjectMeta{Name: "db", Namespace: "dummy", UID: types.UID("db-uid")}, Spec: k8sv1.FdatabaseSpec{Database: k8sv1.DynamicConfig{Value: "dummy"}, User: k8sv1.DynamicConfig{Value: "dummy"}, Password: k8sv1.DynamicConfig{Value: "dummy-password"}, RootPassword: k8sv1.DynamicConfig{Value: "dummy-root"}}}
}
func TestDatabaseRetainsPVCAndAvoidsSecondWriter(t *testing.T) {
	ctx := context.Background()
	f := dummyDatabase()
	r0 := newTestReconciler(t, f)
	r := &FdatabaseReconciler{Client: r0.Client, Scheme: r0.Scheme}
	key := client.ObjectKeyFromObject(f)
	pvc, err := r.pvcForFDatabase(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(pvc.OwnerReferences) != 0 {
		t.Fatal("new PVC would be garbage collected")
	}
	if err := ctrl.SetControllerReference(f, pvc, r.Scheme); err != nil {
		t.Fatal(err)
	}
	if err := r.Create(ctx, pvc); err != nil {
		t.Fatal(err)
	}
	sts := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: f.Name, Namespace: f.Namespace}}
	if err := r.Create(ctx, sts); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatal(err)
	}
	if err := r.Get(ctx, key, pvc); err != nil {
		t.Fatal(err)
	}
	if len(pvc.OwnerReferences) != 0 {
		t.Fatal("old PVC ownership not removed")
	}
	if err := r.Get(ctx, key, &appsv1.Deployment{}); err == nil {
		t.Fatal("created duplicate database writer")
	}
	if err := r.Delete(ctx, f); err != nil {
		t.Fatal(err)
	}
	if err := r.Get(ctx, key, pvc); err != nil {
		t.Fatal("PVC lost on delete")
	}
}

func TestNginxCompatibilityReconciliation(t *testing.T) {
	f := dummyDeployment()
	r := newTestReconciler(t, f)
	runReconcile(t, r, f)
	dep := &appsv1.Deployment{}
	key := client.ObjectKeyFromObject(f)
	if err := r.Get(context.Background(), key, dep); err != nil {
		t.Fatal(err)
	}
	if len(dep.Spec.Template.Spec.Containers[0].SecurityContext.Capabilities.Add) != 0 {
		t.Fatal("default grants capabilities")
	}
	if err := r.Get(context.Background(), key, f); err != nil {
		t.Fatal(err)
	}
	f.Spec.Security.NginxCompatibility = true
	if err := r.Update(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	runReconcile(t, r, f)
	if err := r.Get(context.Background(), key, dep); err != nil {
		t.Fatal(err)
	}
	security := dep.Spec.Template.Spec.Containers[0].SecurityContext
	if len(security.Capabilities.Add) != 4 || *security.Privileged || *security.AllowPrivilegeEscalation || *security.RunAsNonRoot || *dep.Spec.Template.Spec.AutomountServiceAccountToken {
		t.Fatal("NGINX profile is missing or grants excess privilege")
	}
	f.Spec.Security.RunAsNonRoot = true
	if validateFdeployment(f) == nil {
		t.Fatal("accepted conflicting security options")
	}
	f.Spec.Security.RunAsNonRoot = false
	f.Spec.Security.NginxCompatibility = false
	if err := r.Update(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	runReconcile(t, r, f)
	if err := r.Get(context.Background(), key, dep); err != nil {
		t.Fatal(err)
	}
	if len(dep.Spec.Template.Spec.Containers[0].SecurityContext.Capabilities.Add) != 0 {
		t.Fatal("capability exception was not removed")
	}
}

func TestExistingLatestFrontendRemainsCompatible(t *testing.T) {
	f := dummyDeployment()
	f.Spec.Tag = "latest"
	f.Spec.Security.NginxCompatibility = true
	if err := validateFdeployment(f); err != nil {
		t.Fatal(err)
	}
}
