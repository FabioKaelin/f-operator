package k8s

import (
	"context"
	k8sv1 "github.com/fabiokaelin/f-operator/api/k8s/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"os"
	"path/filepath"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	"testing"
	"time"
)

// envtest starts its own loopback API server and etcd. It never reads kubeconfig
// or connects to Minikube, and does not run Deployments on any node.
func TestIsolatedManager(t *testing.T) {
	assets := os.Getenv("KUBEBUILDER_ASSETS")
	if assets == "" {
		t.Fatal("run make test to download isolated envtest assets")
	}
	s := testScheme(t)
	gatewayCRDs := filepath.Join(os.Getenv("GOPATH"), "pkg", "mod", "sigs.k8s.io", "gateway-api@v1.6.1", "config", "crd", "standard")
	if os.Getenv("GOPATH") == "" {
		gatewayCRDs = filepath.Join(os.Getenv("HOME"), "go", "pkg", "mod", "sigs.k8s.io", "gateway-api@v1.6.1", "config", "crd", "standard")
	}
	env := &envtest.Environment{BinaryAssetsDirectory: assets, CRDDirectoryPaths: []string{filepath.Join("..", "..", "..", "config", "crd", "bases"), gatewayCRDs}, ErrorIfCRDPathMissing: true}
	cfg, err := env.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := env.Stop(); err != nil {
			t.Error(err)
		}
	})
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{Scheme: s, Metrics: server.Options{BindAddress: "0"}, HealthProbeBindAddress: "0"})
	if err != nil {
		t.Fatal(err)
	}
	r := &FdeploymentReconciler{Client: mgr.GetClient(), Scheme: s, GatewayName: "shared", GatewayNamespace: "gateway", GatewayListener: "http"}
	if err := r.SetupWithManager(mgr); err != nil {
		t.Fatal(err)
	}
	db := &FdatabaseReconciler{Client: mgr.GetClient(), Scheme: s}
	if err := db.SetupWithManager(mgr); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- mgr.Start(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(10 * time.Second):
			t.Error("manager did not stop")
		}
	})
	c, err := client.New(cfg, client.Options{Scheme: s})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"dummy", "gateway"} {
		if err := c.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}); err != nil {
			t.Fatal(err)
		}
	}
	f := dummyDeployment()
	f.UID = ""
	f.Generation = 0
	if err := c.Create(ctx, f); err != nil {
		t.Fatal(err)
	}
	key := client.ObjectKeyFromObject(f)
	route := &gatewayv1.HTTPRoute{}
	dep := &appsv1.Deployment{}
	eventually(t, func() bool { return c.Get(ctx, key, route) == nil && c.Get(ctx, key, dep) == nil })
	if *route.Spec.Rules[0].Matches[0].Path.Value != "/api" {
		t.Fatal("wrong route")
	}
	// Real API validation rejects malformed port and path.
	bad := dummyDeployment()
	bad.Name = "invalid"
	bad.UID = ""
	bad.Spec.Port = 0
	if err := c.Create(ctx, bad); err == nil {
		t.Fatal("CRD admitted invalid port")
	}
	if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := c.Get(ctx, key, f); err != nil {
			return err
		}
		f.Spec.Path = "/v2"
		f.Spec.Tag = "1.0.1"
		return c.Update(ctx, f)
	}); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool {
		return c.Get(ctx, key, route) == nil && *route.Spec.Rules[0].Matches[0].Path.Value == "/v2" && c.Get(ctx, key, dep) == nil && dep.Spec.Template.Spec.Containers[0].Image == "ghcr.io/fabiokaelin/backend:1.0.1"
	})
	if err := c.Get(ctx, key, f); err != nil {
		t.Fatal(err)
	}
	condition := meta.FindStatusCondition(f.Status.Conditions, "Available")
	if condition == nil || condition.Status != metav1.ConditionFalse {
		t.Fatal("reported ready without gateway or running workload")
	}
	// A missing Gateway becomes Ready after current gateway/route/workload status.
	g := &gatewayv1.Gateway{ObjectMeta: metav1.ObjectMeta{Name: "shared", Namespace: "gateway"}, Spec: gatewayv1.GatewaySpec{GatewayClassName: "envoy", Listeners: []gatewayv1.Listener{{Name: "http", Port: 80, Protocol: gatewayv1.HTTPProtocolType}}}}
	if err := c.Create(ctx, g); err != nil {
		t.Fatal(err)
	}
	g.Status.Conditions = acceptedConditions(g.Generation, "Accepted", "Programmed")
	g.Status.Listeners = []gatewayv1.ListenerStatus{{Name: "http", SupportedKinds: []gatewayv1.RouteGroupKind{{Kind: "HTTPRoute"}}, Conditions: acceptedConditions(g.Generation, "Accepted", "Programmed", "ResolvedRefs")}}
	if err := c.Status().Update(ctx, g); err != nil {
		t.Fatal(err)
	}
	route.Status.Parents = []gatewayv1.RouteParentStatus{{ParentRef: route.Spec.ParentRefs[0], ControllerName: "gateway.envoyproxy.io/gatewayclass-controller", Conditions: acceptedConditions(route.Generation, "Accepted", "ResolvedRefs")}}
	if err := c.Status().Update(ctx, route); err != nil {
		t.Fatal(err)
	}
	dep.Status.ObservedGeneration = dep.Generation
	dep.Status.Replicas = 1
	dep.Status.ReadyReplicas = 1
	dep.Status.AvailableReplicas = 1
	dep.Status.UpdatedReplicas = 1
	if err := c.Status().Update(ctx, dep); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool {
		if c.Get(ctx, key, f) != nil {
			return false
		}
		return meta.IsStatusConditionTrue(f.Status.Conditions, "Available")
	})
	// No steady-state resource writes, including Kubernetes defaulted fields.
	if err := c.Get(ctx, key, dep); err != nil {
		t.Fatal(err)
	}
	version := dep.ResourceVersion
	if err := c.Get(ctx, key, route); err != nil {
		t.Fatal(err)
	}
	routeVersion := route.ResourceVersion
	time.Sleep(time.Second)
	if _, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatal(err)
	}
	if err := c.Get(ctx, key, dep); err != nil {
		t.Fatal(err)
	}
	if dep.ResourceVersion != version {
		t.Fatal("steady-state deployment rewritten")
	}
	if err := c.Get(ctx, key, route); err != nil {
		t.Fatal(err)
	}
	if route.ResourceVersion != routeVersion {
		t.Fatal("steady-state route rewritten")
	}
	if err := c.Delete(ctx, f); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool { return c.Get(ctx, key, &k8sv1.Fdeployment{}) != nil })
}
func eventually(t *testing.T, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("timed out waiting for isolated reconciliation")
}
