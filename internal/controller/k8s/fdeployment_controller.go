package k8s

import (
	"context"
	"fmt"
	k8sv1 "github.com/fabiokaelin/f-operator/api/k8s/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	"strings"
	"time"
)

type FdeploymentReconciler struct {
	client.Client
	Scheme               *runtime.Scheme
	Recorder             record.EventRecorder
	GatewayName          string
	GatewayNamespace     string
	GatewayListener      string
	CleanupLegacyIngress bool
}

const typeAvailableFDeployment = "Available"
const fdeploymentFinalizer = "k8s.fabkli.ch/finalizer"

//+kubebuilder:rbac:groups=k8s.fabkli.ch,resources=fdeployments,verbs=get;list;watch;update;patch
//+kubebuilder:rbac:groups=k8s.fabkli.ch,resources=fdeployments/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=k8s.fabkli.ch,resources=fdeployments/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch
//+kubebuilder:rbac:groups=core,resources=services;serviceaccounts,verbs=get;list;watch;create;update;patch
//+kubebuilder:rbac:groups=core,resources=secrets;configmaps,verbs=get;list;watch
//+kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;delete
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes,verbs=get;list;watch;create;update;patch
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways,verbs=get;list;watch

func (r *FdeploymentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	f := &k8sv1.Fdeployment{}
	if err := r.Get(ctx, req.NamespacedName, f); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !f.DeletionTimestamp.IsZero() {
		if controllerutil.RemoveFinalizer(f, fdeploymentFinalizer) {
			return ctrl.Result{}, r.Update(ctx, f)
		}
		return ctrl.Result{}, nil
	}
	if r.GatewayName == "" || r.GatewayNamespace == "" || r.GatewayListener == "" {
		return ctrl.Result{}, fmt.Errorf("gateway name, namespace and listener must be configured")
	}
	if err := validateFdeployment(f); err != nil {
		return ctrl.Result{}, r.setDeploymentStatus(ctx, f, false, "InvalidSpec", err.Error())
	}
	if err := r.dependenciesReady(ctx, f); err != nil {
		if statusErr := r.setDeploymentStatus(ctx, f, false, "WaitingDependencies", err.Error()); statusErr != nil {
			return ctrl.Result{}, statusErr
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}
	desired, err := r.deploymentForFDeployment(f)
	if err != nil {
		return ctrl.Result{}, err
	}
	sa, err := r.serviceAccountForFDeployment(f)
	if err != nil {
		return ctrl.Result{}, err
	}
	svc, err := r.serviceForFDeployment(f)
	if err != nil {
		return ctrl.Result{}, err
	}
	route, err := r.routeForFDeployment(f)
	if err != nil {
		return ctrl.Result{}, err
	}
	for _, obj := range []client.Object{sa, desired, svc, route} {
		if err := r.reconcileOwned(ctx, f, obj); err != nil {
			return ctrl.Result{}, err
		}
	}
	deployment := &appsv1.Deployment{}
	if err := r.Get(ctx, req.NamespacedName, deployment); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.Get(ctx, req.NamespacedName, route); err != nil {
		return ctrl.Result{}, err
	}
	gateway := &gatewayv1.Gateway{}
	err = r.Get(ctx, types.NamespacedName{Name: r.GatewayName, Namespace: r.GatewayNamespace}, gateway)
	if err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, err
	}
	ready := err == nil && gatewayReady(gateway, r.GatewayListener) && r.routeReady(route)
	if ready && r.CleanupLegacyIngress {
		legacy := &networking.Ingress{}
		err := r.Get(ctx, req.NamespacedName, legacy)
		if err == nil && metav1.IsControlledBy(legacy, f) {
			if err := r.Delete(ctx, legacy); err != nil {
				return ctrl.Result{}, err
			}
		} else if err != nil && !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
	}
	ready = ready && deployment.Status.ObservedGeneration >= deployment.Generation && deployment.Status.AvailableReplicas >= f.Spec.Replicas && deployment.Status.UpdatedReplicas >= f.Spec.Replicas
	reason, message := "Progressing", "Waiting for current Deployment, Gateway and HTTPRoute readiness"
	if ready {
		reason, message = "Ready", "Deployment and Gateway route are ready"
	}
	if err := r.setDeploymentStatus(ctx, f, ready, reason, message); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *FdeploymentReconciler) setDeploymentStatus(ctx context.Context, f *k8sv1.Fdeployment, ready bool, reason, message string) error {
	before := f.DeepCopy()
	status := metav1.ConditionFalse
	if ready {
		status = metav1.ConditionTrue
	}
	meta.SetStatusCondition(&f.Status.Conditions, metav1.Condition{Type: typeAvailableFDeployment, Status: status, ObservedGeneration: f.Generation, Reason: reason, Message: message})
	if equality.Semantic.DeepEqual(before.Status, f.Status) {
		return nil
	}
	return r.Status().Patch(ctx, f, client.MergeFrom(before))
}

// Preserve API-assigned fields, immutable selectors, sidecars and annotations.
// Refuse to adopt resources belonging to another owner.
func (r *FdeploymentReconciler) reconcileOwned(ctx context.Context, f *k8sv1.Fdeployment, desired client.Object) error {
	current := desired.DeepCopyObject().(client.Object)
	err := r.Get(ctx, client.ObjectKeyFromObject(desired), current)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}
	if !metav1.IsControlledBy(current, f) {
		return fmt.Errorf("refusing to modify unowned %T %s", current, client.ObjectKeyFromObject(current))
	}
	before := current.DeepCopyObject().(client.Object)
	switch c := current.(type) {
	case *appsv1.Deployment:
		d := desired.(*appsv1.Deployment)
		c.Spec.Replicas = d.Spec.Replicas
		// Existing releases included the version in their immutable selector.
		// Keep its labels while updating the rest of the pod template.
		labels := c.Spec.Selector.MatchLabels
		d.Spec.Template.Labels = labels
		containers := c.Spec.Template.Spec.Containers
		found := false
		for i := range containers {
			if containers[i].Name == f.Name {
				wanted := d.Spec.Template.Spec.Containers[0]
				containers[i].Image = wanted.Image
				containers[i].ImagePullPolicy = wanted.ImagePullPolicy
				containers[i].Env = wanted.Env
				containers[i].Ports = wanted.Ports
				containers[i].Resources = wanted.Resources
				containers[i].ReadinessProbe = wanted.ReadinessProbe
				containers[i].LivenessProbe = wanted.LivenessProbe
				containers[i].SecurityContext = wanted.SecurityContext
				found = true
			}
		}
		if !found {
			containers = append(containers, d.Spec.Template.Spec.Containers[0])
		}
		c.Spec.Template.Spec.Containers = containers
		c.Spec.Template.Labels = labels
		c.Spec.Template.Spec.ServiceAccountName = d.Spec.Template.Spec.ServiceAccountName
		c.Spec.Template.Spec.AutomountServiceAccountToken = d.Spec.Template.Spec.AutomountServiceAccountToken
		c.Spec.Template.Spec.SecurityContext = d.Spec.Template.Spec.SecurityContext
	case *corev1.Service:
		d := desired.(*corev1.Service)
		c.Spec.Selector = d.Spec.Selector
		ports := d.Spec.Ports
		for i := range ports {
			for _, old := range c.Spec.Ports {
				if old.Port == ports[i].Port {
					ports[i].NodePort = old.NodePort
				}
			}
		}
		c.Spec.Ports = ports
	case *corev1.ServiceAccount:
		c.AutomountServiceAccountToken = desired.(*corev1.ServiceAccount).AutomountServiceAccountToken
	case *gatewayv1.HTTPRoute:
		c.Spec = desired.(*gatewayv1.HTTPRoute).Spec
	}
	if equality.Semantic.DeepEqual(current, before) {
		return nil
	}
	return r.Patch(ctx, current, client.MergeFrom(before))
}

func validatedResources(v k8sv1.FdeploymentResources) (corev1.ResourceRequirements, error) {
	result := corev1.ResourceRequirements{Requests: corev1.ResourceList{}, Limits: corev1.ResourceList{}}
	for _, entry := range []struct {
		name           corev1.ResourceName
		request, limit string
	}{{corev1.ResourceCPU, v.Requests.CPU, v.Limits.CPU}, {corev1.ResourceMemory, v.Requests.Memory, v.Limits.Memory}} {
		request, err := resource.ParseQuantity(entry.request)
		if err != nil || request.Sign() <= 0 {
			return result, fmt.Errorf("invalid positive %s request", entry.name)
		}
		limit, err := resource.ParseQuantity(entry.limit)
		if err != nil || limit.Sign() <= 0 || request.Cmp(limit) > 0 {
			return result, fmt.Errorf("invalid %s limit or request exceeds limit", entry.name)
		}
		result.Requests[entry.name], result.Limits[entry.name] = request, limit
	}
	return result, nil
}
func validateFdeployment(f *k8sv1.Fdeployment) error {
	if f.Spec.Security.NginxCompatibility && f.Spec.Security.RunAsNonRoot {
		return fmt.Errorf("nginxCompatibility requires the standard root master process")
	}
	if _, err := validatedResources(f.Spec.Resources); err != nil {
		return err
	}
	if f.Spec.Port < 1 || f.Spec.Port > 65535 || f.Spec.Replicas < 1 || f.Spec.Replicas > 5 {
		return fmt.Errorf("invalid port or replicas")
	}
	if f.Spec.Host == "" || !strings.HasPrefix(f.Spec.Path, "/") || !strings.HasPrefix(f.Spec.HealthCheck.ReadinessProbe.Path, "/") || !strings.HasPrefix(f.Spec.HealthCheck.LivenessProbe.Path, "/") {
		return fmt.Errorf("hostname and absolute route/probe paths required")
	}
	if f.Spec.Tag == "" {
		return fmt.Errorf("an image tag is required")
	}
	return nil
}
func (r *FdeploymentReconciler) routeForFDeployment(f *k8sv1.Fdeployment) (*gatewayv1.HTTPRoute, error) {
	ns := gatewayv1.Namespace(r.GatewayNamespace)
	section := gatewayv1.SectionName(r.GatewayListener)
	pathType := gatewayv1.PathMatchPathPrefix
	port := gatewayv1.PortNumber(80)
	group := gatewayv1.Group(gatewayv1.GroupName)
	kind := gatewayv1.Kind("Gateway")
	backendGroup := gatewayv1.Group("")
	backendKind := gatewayv1.Kind("Service")
	weight := int32(1)
	route := &gatewayv1.HTTPRoute{ObjectMeta: metav1.ObjectMeta{Name: f.Name, Namespace: f.Namespace}, Spec: gatewayv1.HTTPRouteSpec{
		CommonRouteSpec: gatewayv1.CommonRouteSpec{ParentRefs: []gatewayv1.ParentReference{{Group: &group, Kind: &kind, Name: gatewayv1.ObjectName(r.GatewayName), Namespace: &ns, SectionName: &section}}},
		Hostnames:       []gatewayv1.Hostname{gatewayv1.Hostname(f.Spec.Host)},
		Rules:           []gatewayv1.HTTPRouteRule{{Matches: []gatewayv1.HTTPRouteMatch{{Path: &gatewayv1.HTTPPathMatch{Type: &pathType, Value: &f.Spec.Path}}}, BackendRefs: []gatewayv1.HTTPBackendRef{{BackendRef: gatewayv1.BackendRef{Weight: &weight, BackendObjectReference: gatewayv1.BackendObjectReference{Group: &backendGroup, Kind: &backendKind, Name: gatewayv1.ObjectName(f.Name), Port: &port}}}}}},
	}}
	return route, ctrl.SetControllerReference(f, route, r.Scheme)
}
func (r *FdeploymentReconciler) routeReady(route *gatewayv1.HTTPRoute) bool {
	for _, parent := range route.Status.Parents {
		ref := parent.ParentRef
		namespace := route.Namespace
		if ref.Namespace != nil {
			namespace = string(*ref.Namespace)
		}
		if string(ref.Name) != r.GatewayName || namespace != r.GatewayNamespace || ref.SectionName == nil || string(*ref.SectionName) != r.GatewayListener || parent.ControllerName != "gateway.envoyproxy.io/gatewayclass-controller" {
			continue
		}
		if ref.Group != nil && *ref.Group != gatewayv1.GroupName {
			continue
		}
		if ref.Kind != nil && *ref.Kind != "Gateway" {
			continue
		}
		if conditionReady(parent.Conditions, "Accepted", route.Generation) && conditionReady(parent.Conditions, "ResolvedRefs", route.Generation) {
			return true
		}
	}
	return false
}
func conditionReady(conditions []metav1.Condition, name string, generation int64) bool {
	c := meta.FindStatusCondition(conditions, name)
	return c != nil && c.Status == metav1.ConditionTrue && c.ObservedGeneration >= generation
}
func gatewayReady(g *gatewayv1.Gateway, listener string) bool {
	if !conditionReady(g.Status.Conditions, "Accepted", g.Generation) || !conditionReady(g.Status.Conditions, "Programmed", g.Generation) {
		return false
	}
	for _, l := range g.Status.Listeners {
		if string(l.Name) == listener {
			return conditionReady(l.Conditions, "Accepted", g.Generation) && conditionReady(l.Conditions, "Programmed", g.Generation) && conditionReady(l.Conditions, "ResolvedRefs", g.Generation)
		}
	}
	return false
}
func (r *FdeploymentReconciler) dependenciesReady(ctx context.Context, f *k8sv1.Fdeployment) error {
	for _, env := range f.Spec.Environments {
		if env.FromSecret.Name != "" && env.Value == "" && env.FromConfig.Name == "" {
			obj := &corev1.Secret{}
			if err := r.Get(ctx, types.NamespacedName{Name: env.FromSecret.Name, Namespace: f.Namespace}, obj); err != nil {
				return fmt.Errorf("Secret dependency %s unavailable", env.FromSecret.Name)
			}
			if _, ok := obj.Data[env.FromSecret.Key]; !ok {
				return fmt.Errorf("Secret dependency key unavailable for %s", env.Name)
			}
		} else if env.FromConfig.Name != "" && env.Value == "" {
			obj := &corev1.ConfigMap{}
			if err := r.Get(ctx, types.NamespacedName{Name: env.FromConfig.Name, Namespace: f.Namespace}, obj); err != nil {
				return fmt.Errorf("ConfigMap dependency %s unavailable", env.FromConfig.Name)
			}
			if _, ok := obj.Data[env.FromConfig.Key]; !ok {
				return fmt.Errorf("ConfigMap dependency key unavailable for %s", env.Name)
			}
		}
	}
	return nil
}
func (r *FdeploymentReconciler) enqueueDependency(ctx context.Context, obj client.Object) []reconcile.Request {
	list := &k8sv1.FdeploymentList{}
	if err := r.List(ctx, list); err != nil {
		return nil
	}
	result := []reconcile.Request{}
	for _, f := range list.Items {
		match := false
		switch obj.(type) {
		case *gatewayv1.Gateway:
			match = obj.GetName() == r.GatewayName && obj.GetNamespace() == r.GatewayNamespace
		case *corev1.Secret:
			for _, env := range f.Spec.Environments {
				if obj.GetNamespace() == f.Namespace && env.FromSecret.Name == obj.GetName() {
					match = true
				}
			}
		case *corev1.ConfigMap:
			for _, env := range f.Spec.Environments {
				if obj.GetNamespace() == f.Namespace && env.FromConfig.Name == obj.GetName() {
					match = true
				}
			}
		}
		if match {
			result = append(result, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&f)})
		}
	}
	return result
}
func (r *FdeploymentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	mapping := handler.EnqueueRequestsFromMapFunc(r.enqueueDependency)
	return ctrl.NewControllerManagedBy(mgr).For(&k8sv1.Fdeployment{}).Owns(&corev1.ServiceAccount{}).Owns(&corev1.Service{}).Owns(&appsv1.Deployment{}).Owns(&gatewayv1.HTTPRoute{}).Watches(&gatewayv1.Gateway{}, mapping).Watches(&corev1.ConfigMap{}, mapping).Watches(&corev1.Secret{}, mapping).Complete(r)
}

func getEnvironment(fdeployment *k8sv1.Fdeployment) ([]corev1.EnvVar, error) {
	envVars := []corev1.EnvVar{}
	for _, env := range fdeployment.Spec.Environments {
		var currentEnv corev1.EnvVar
		if env.Value != "" {
			currentEnv = corev1.EnvVar{
				Name:  env.Name,
				Value: env.Value,
			}
		} else if env.FromConfig.Name != "" && env.FromConfig.Key != "" {
			currentEnv = corev1.EnvVar{
				Name: env.Name,
				ValueFrom: &corev1.EnvVarSource{
					ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: env.FromConfig.Name,
						},
						Key: env.FromConfig.Key,
					},
				},
			}
		} else if env.FromSecret.Name != "" && env.FromSecret.Key != "" {
			currentEnv = corev1.EnvVar{
				Name: env.Name,
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: env.FromSecret.Name,
						},
						Key: env.FromSecret.Key,
					},
				},
			}
		} else {
			return nil, fmt.Errorf("invalid environment variable %s", env.Name)
		}
		envVars = append(envVars, currentEnv)
	}
	versionEnv := corev1.EnvVar{
		Name:  "F_VERSION",
		Value: fdeployment.Spec.Tag,
	}
	versionEnvVite := corev1.EnvVar{
		Name:  "VITE_F_VERSION",
		Value: fdeployment.Spec.Tag,
	}
	envVars = append(envVars, versionEnv)
	envVars = append(envVars, versionEnvVite)
	return envVars, nil
}

// deploymentForFDeployment returns a FDeployment Deployment object
func (r *FdeploymentReconciler) deploymentForFDeployment(
	fdeployment *k8sv1.Fdeployment) (*appsv1.Deployment, error) {
	resources, err := validatedResources(fdeployment.Spec.Resources)
	if err != nil {
		return nil, err
	}
	replicas := fdeployment.Spec.Replicas
	port := fdeployment.Spec.Port
	name := fdeployment.Name
	image := ""
	if fdeployment.Spec.Image != "" {
		image = fmt.Sprintf("ghcr.io/fabiokaelin/%s:%s", fdeployment.Spec.Image, fdeployment.Spec.Tag)
	} else {
		image = fmt.Sprintf("ghcr.io/fabiokaelin/%s:%s", fdeployment.Name, fdeployment.Spec.Tag)
	}
	ls := labelsForFDeployment(fdeployment.Name, image)
	delete(ls, "app.kubernetes.io/version")

	// create env vars
	envVars, err := getEnvironment(fdeployment)
	if err != nil {
		return nil, err
	}
	// valueFrom:
	//   configMapKeyRef:
	// 	name: game-demo           # The ConfigMap this value comes from.
	// 	key: player_initial_lives # The key to fetch.

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			// Namespace: app,
			Namespace: fdeployment.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: ls,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: ls,
				},
				Spec: corev1.PodSpec{
					// automountServiceAccountToken: false

					AutomountServiceAccountToken: &[]bool{false}[0],
					ServiceAccountName:           name,
					SecurityContext: &corev1.PodSecurityContext{
						SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
						RunAsNonRoot:   &fdeployment.Spec.Security.RunAsNonRoot,
						// 	SeccompProfile: &corev1.SeccompProfile{
						// 		Type: corev1.SeccompProfileTypeRuntimeDefault,
						// 	},
					},
					Tolerations: []corev1.Toleration{{
						Key:      "kubernetes.azure.com/scalesetpriority",
						Operator: corev1.TolerationOpEqual,
						Value:    "spot",
						Effect:   corev1.TaintEffectNoSchedule,
					}},
					ImagePullSecrets: []corev1.LocalObjectReference{{
						Name: "regcred",
					}},

					Containers: []corev1.Container{{
						Image: image,
						Name:  name,
						Env:   envVars,
						ReadinessProbe: &corev1.Probe{
							FailureThreshold: 3,
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{
									Path:   fdeployment.Spec.HealthCheck.ReadinessProbe.Path,
									Port:   intstr.FromInt(int(port)),
									Scheme: corev1.URISchemeHTTP,
								},
							},
							PeriodSeconds:       20,
							SuccessThreshold:    1,
							TimeoutSeconds:      3,
							InitialDelaySeconds: 5,
						},
						LivenessProbe: &corev1.Probe{
							FailureThreshold: 3,
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{
									Path:   fdeployment.Spec.HealthCheck.LivenessProbe.Path,
									Port:   intstr.FromInt(int(port)),
									Scheme: corev1.URISchemeHTTP,
								},
							},
							PeriodSeconds:       30,
							SuccessThreshold:    1,
							TimeoutSeconds:      3,
							InitialDelaySeconds: 10,
						},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								"cpu":    resources.Requests[corev1.ResourceCPU],
								"memory": resources.Requests[corev1.ResourceMemory],
							},
							Limits: corev1.ResourceList{
								"cpu":    resources.Limits[corev1.ResourceCPU],
								"memory": resources.Limits[corev1.ResourceMemory],
							},
						},

						ImagePullPolicy: corev1.PullAlways,
						SecurityContext: &corev1.SecurityContext{
							RunAsNonRoot:             &fdeployment.Spec.Security.RunAsNonRoot,
							Privileged:               &fdeployment.Spec.Security.Privileged,
							AllowPrivilegeEscalation: &fdeployment.Spec.Security.Privileged,
							Capabilities:             applicationCapabilities(fdeployment.Spec.Security),
							// 	RunAsUser:                &[]int64{1001}[0],
							// 	AllowPrivilegeEscalation: &[]bool{false}[0],
							// 	Capabilities: &corev1.Capabilities{
							// 		Drop: []corev1.Capability{
							// 			"ALL",
							// 		},
							// 	},
						},
						Ports: []corev1.ContainerPort{{
							ContainerPort: port,
							Protocol:      corev1.ProtocolTCP,
							Name:          "containerport",
						}},
						// Command: []string{"memcached", "-m=64", "-o", "modern", "-v"},
					}},
				},
			},
		},
	}

	// Set the ownerRef for the Deployment
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/owners-dependents/
	if err := ctrl.SetControllerReference(fdeployment, dep, r.Scheme); err != nil {
		return nil, err
	}
	return dep, nil
}

// deploymentForFDeployment returns a FDeployment service object
func (r *FdeploymentReconciler) serviceForFDeployment(
	fdeployment *k8sv1.Fdeployment) (*corev1.Service, error) {
	// path := fdeployment.Spec.Path
	// replicas := fdeployment.Spec.Replicas
	port := fdeployment.Spec.Port
	name := fdeployment.Name
	// image := fmt.Sprintf("ghcr.io/fabiokaelin/%s:%s", fdeployment.Name, fdeployment.Spec.Tag)
	// ls := labelsForFDeployment(fdeployment.Name, image)

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: fdeployment.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app.kubernetes.io/name": name,
			},
			Ports: []corev1.ServicePort{{
				Port:       80,
				TargetPort: intstr.FromInt(int(port)),
				Protocol:   corev1.ProtocolTCP,
			}},
		},
	}

	// Set the ownerRef for the Deployment
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/owners-dependents/
	if err := ctrl.SetControllerReference(fdeployment, svc, r.Scheme); err != nil {
		return nil, err
	}
	return svc, nil
}

// deploymentForFDeployment returns a FDeployment Deployment object
func (r *FdeploymentReconciler) serviceAccountForFDeployment(
	fdeployment *k8sv1.Fdeployment) (*corev1.ServiceAccount, error) {
	name := fdeployment.Name

	//create service account
	svcAcc := &corev1.ServiceAccount{
		AutomountServiceAccountToken: &[]bool{false}[0],
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			// Namespace: app,
			Namespace: fdeployment.Namespace,
		},
	}

	// Set the ownerRef for the ServiceAccount
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/owners-dependents/
	if err := ctrl.SetControllerReference(fdeployment, svcAcc, r.Scheme); err != nil {
		return nil, err
	}
	return svcAcc, nil
}

// labelsForFDeployment returns the labels for selecting the resources
// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/common-labels/
func labelsForFDeployment(name string, image string) map[string]string {
	imageTag := image[strings.LastIndex(image, ":")+1:]
	return map[string]string{
		"app.kubernetes.io/name":     name,
		"app.kubernetes.io/instance": name,
		"app.kubernetes.io/version":  imageTag,
		"app.kubernetes.io/part-of":  "f-operator",
	}
}

// Standard NGINX starts a root master, initializes cache ownership, then drops
// worker UID/GID. Keep all other capabilities dropped and privilege escalation off.
func applicationCapabilities(security k8sv1.FdeploymentSecurity) *corev1.Capabilities {
	result := &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}}
	if security.NginxCompatibility {
		result.Add = []corev1.Capability{"CHOWN", "SETUID", "SETGID", "NET_BIND_SERVICE"}
	}
	return result
}
