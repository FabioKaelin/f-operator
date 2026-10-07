package k8s

import (
	"context"
	"fmt"
	k8sv1 "github.com/fabiokaelin/f-operator/api/k8s/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"time"
)

type FdatabaseReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

const fdatabaseFinalizer = "k8s.fabkli.ch/finalizer"

//+kubebuilder:rbac:groups=k8s.fabkli.ch,resources=fdatabases,verbs=get;list;watch;update;patch
//+kubebuilder:rbac:groups=k8s.fabkli.ch,resources=fdatabases/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=k8s.fabkli.ch,resources=fdatabases/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch
//+kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch

func (r *FdatabaseReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	f := &k8sv1.Fdatabase{}
	if err := r.Get(ctx, req.NamespacedName, f); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	pvc := &corev1.PersistentVolumeClaim{}
	err := r.Get(ctx, req.NamespacedName, pvc)
	if err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, err
	}
	// Remove only this CR's old owner reference, preserving storage and other metadata.
	if err == nil && metav1.IsControlledBy(pvc, f) {
		before := pvc.DeepCopy()
		refs := []metav1.OwnerReference{}
		for _, ref := range pvc.OwnerReferences {
			if ref.UID != f.UID {
				refs = append(refs, ref)
			}
		}
		pvc.OwnerReferences = refs
		if err := r.Patch(ctx, pvc, client.MergeFrom(before)); err != nil {
			return ctrl.Result{}, err
		}
	}
	if !f.DeletionTimestamp.IsZero() {
		if controllerutil.RemoveFinalizer(f, fdatabaseFinalizer) {
			return ctrl.Result{}, r.Update(ctx, f)
		}
		return ctrl.Result{}, nil
	}
	// Never start a second writer beside an independently managed StatefulSet.
	sts := &appsv1.StatefulSet{}
	err = r.Get(ctx, req.NamespacedName, sts)
	if err == nil {
		return ctrl.Result{}, r.databaseStatus(ctx, f, false, "ExternalDatabase", "Existing StatefulSet is independently managed; no Deployment created")
	}
	if !apierrors.IsNotFound(err) {
		return ctrl.Result{}, err
	}
	desired, err := r.deploymentForFDatabase(f)
	if err != nil {
		return ctrl.Result{}, err
	}
	svc, err := r.serviceForFDatabase(f)
	if err != nil {
		return ctrl.Result{}, err
	}
	if pvc.Name == "" {
		pvc, err = r.pvcForFDatabase(f)
		if err != nil {
			return ctrl.Result{}, err
		}
		if err := r.Create(ctx, pvc); err != nil {
			return ctrl.Result{}, err
		}
	} else if !metav1.IsControlledBy(pvc, f) && pvc.Labels["app.kubernetes.io/part-of"] != "f-operator" {
		return ctrl.Result{}, fmt.Errorf("refusing to use unrelated PVC %s", pvc.Name)
	}
	for _, obj := range []client.Object{svc, desired} {
		current := obj.DeepCopyObject().(client.Object)
		err := r.Get(ctx, client.ObjectKeyFromObject(obj), current)
		if apierrors.IsNotFound(err) {
			if err := r.Create(ctx, obj); err != nil {
				return ctrl.Result{}, err
			}
			continue
		}
		if err != nil {
			return ctrl.Result{}, err
		}
		if !metav1.IsControlledBy(current, f) {
			return ctrl.Result{}, fmt.Errorf("refusing to modify unowned database resource %s", current.GetName())
		}
		if dep, ok := current.(*appsv1.Deployment); ok {
			before := dep.DeepCopy()
			found := false
			for i := range dep.Spec.Template.Spec.Containers {
				if dep.Spec.Template.Spec.Containers[i].Name == f.Name {
					dep.Spec.Template.Spec.Containers[i].Env = desired.Spec.Template.Spec.Containers[0].Env
					found = true
				}
			}
			if !found {
				return ctrl.Result{}, fmt.Errorf("database container missing")
			}
			dep.Spec.Strategy = desired.Spec.Strategy
			dep.Spec.Template.Spec.AutomountServiceAccountToken = desired.Spec.Template.Spec.AutomountServiceAccountToken
			if !equality.Semantic.DeepEqual(before, dep) {
				if err := r.Patch(ctx, dep, client.MergeFrom(before)); err != nil {
					return ctrl.Result{}, err
				}
			}
		}
	}
	dep := &appsv1.Deployment{}
	if err := r.Get(ctx, req.NamespacedName, dep); err != nil {
		return ctrl.Result{}, err
	}
	ready := dep.Status.ObservedGeneration >= dep.Generation && dep.Status.AvailableReplicas == 1
	reason := "Progressing"
	if ready {
		reason = "Ready"
	}
	return ctrl.Result{RequeueAfter: 30 * time.Second}, r.databaseStatus(ctx, f, ready, reason, "Database workload readiness; PVC retained on CR deletion")
}
func (r *FdatabaseReconciler) databaseStatus(ctx context.Context, f *k8sv1.Fdatabase, ready bool, reason, message string) error {
	before := f.DeepCopy()
	status := metav1.ConditionFalse
	if ready {
		status = metav1.ConditionTrue
	}
	meta.SetStatusCondition(&f.Status.Conditions, metav1.Condition{Type: "Available", Status: status, ObservedGeneration: f.Generation, Reason: reason, Message: message})
	if equality.Semantic.DeepEqual(before.Status, f.Status) {
		return nil
	}
	return r.Status().Patch(ctx, f, client.MergeFrom(before))
}
func (r *FdatabaseReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).For(&k8sv1.Fdatabase{}).Owns(&corev1.Service{}).Owns(&appsv1.Deployment{}).Complete(r)
}
func (r *FdatabaseReconciler) pvcForFDatabase(fdatabase *k8sv1.Fdatabase) (*corev1.PersistentVolumeClaim, error) {
	name := fdatabase.Name
	storaceClassName := "standard"
	filesystem := corev1.PersistentVolumeFilesystem
	storage := "2Gi"

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			//TODO: Fabio: add labels
			Labels:    labelsForFDatabase(name),
			Name:      name,
			Namespace: fdatabase.Namespace,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: &storaceClassName,
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
			VolumeMode:       &filesystem,
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(storage),
				},
			},
		},
	}
	// PVCs intentionally have no owner reference: CR deletion must retain data.
	return pvc, nil
}

func (r *FdatabaseReconciler) deploymentForFDatabase(fdatabase *k8sv1.Fdatabase) (*appsv1.Deployment, error) {
	name := fdatabase.Name
	// int to int32
	replicas := int32(1)
	ls := labelsForFDatabase(fdatabase.Name)
	environment, err := getEnv(fdatabase)
	if err != nil {
		return nil, err
	}
	mountPropagation := corev1.MountPropagationNone

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Labels:    ls,
			Name:      name,
			Namespace: fdatabase.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Strategy: appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType},
			Selector: &metav1.LabelSelector{
				MatchLabels: ls,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: ls,
				},
				Spec: corev1.PodSpec{
					AutomountServiceAccountToken: &[]bool{false}[0],
					Containers: []corev1.Container{{
						Name:            name,
						Image:           "mariadb:11",
						ImagePullPolicy: corev1.PullIfNotPresent,
						Ports: []corev1.ContainerPort{{
							ContainerPort: 3306,
							Protocol:      corev1.ProtocolTCP,
						}},
						Env: environment,
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								"cpu":    resource.MustParse("50m"),
								"memory": resource.MustParse("128Mi"),
							},
							Limits: corev1.ResourceList{
								"cpu":    resource.MustParse("500m"),
								"memory": resource.MustParse("1Gi"),
							},
						},
						SecurityContext: &corev1.SecurityContext{
							RunAsNonRoot: &[]bool{true}[0],
							// Privileged:               &[]bool{true}[0],
							RunAsUser:                &[]int64{1001}[0],
							RunAsGroup:               &[]int64{1001}[0],
							AllowPrivilegeEscalation: &[]bool{false}[0],
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{
									"ALL",
								},
							},
						},
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:             name + "-pv",
								MountPath:        "/var/lib/mysql",
								ReadOnly:         false,
								MountPropagation: &mountPropagation,
							}},
					}},
					Volumes: []corev1.Volume{
						{
							Name: name + "-pv",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: name,
								},
							},
						},
					},

					Tolerations: []corev1.Toleration{
						{
							Effect:   corev1.TaintEffectNoSchedule,
							Key:      "kubernetes.azure.com/scalesetpriority",
							Operator: corev1.TolerationOpEqual,
							Value:    "spot",
						},
					},
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: &[]bool{true}[0],
						FSGroup:      &[]int64{2000}[0],
						RunAsUser:    &[]int64{1001}[0],
						RunAsGroup:   &[]int64{1001}[0],
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
				},
			},
		},
	}

	if err := ctrl.SetControllerReference(fdatabase, deployment, r.Scheme); err != nil {
		return nil, err
	}
	return deployment, nil
}

func getEnv(fdatabase *k8sv1.Fdatabase) ([]corev1.EnvVar, error) {
	password, err := generateDynamicConfig(&fdatabase.Spec.Password, "MARIADB_PASSWORD")
	if err != nil {
		return nil, err
	}
	user, err := generateDynamicConfig(&fdatabase.Spec.User, "MARIADB_USER")
	if err != nil {
		return nil, err
	}
	database, err := generateDynamicConfig(&fdatabase.Spec.Database, "MARIADB_DATABASE")
	if err != nil {
		return nil, err
	}
	rootPassword, err := generateDynamicConfig(&fdatabase.Spec.RootPassword, "MARIADB_ROOT_PASSWORD")
	if err != nil {
		return nil, err
	}

	environment := []corev1.EnvVar{
		database,
		rootPassword,
		password,
		user,
	}
	if fdatabase.Spec.RootHost != "" {
		environment = append(environment, corev1.EnvVar{
			Name:  "MARIADB_ROOT_HOST",
			Value: fdatabase.Spec.RootHost,
		})
	}
	return environment, nil
}

func generateDynamicConfig(config *k8sv1.DynamicConfig, name string) (corev1.EnvVar, error) {
	var currentConfig corev1.EnvVar
	if config.Value != "" {
		currentConfig = corev1.EnvVar{
			Name:  name,
			Value: config.Value,
		}
	} else if config.FromConfig.Name != "" && config.FromConfig.Key != "" {
		currentConfig = corev1.EnvVar{
			Name: name,
			ValueFrom: &corev1.EnvVarSource{
				ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: config.FromConfig.Name,
					},
					Key: config.FromConfig.Key,
				},
			},
		}
	} else if config.FromSecret.Name != "" && config.FromSecret.Key != "" {
		currentConfig = corev1.EnvVar{
			Name: name,
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: config.FromSecret.Name,
					},
					Key: config.FromSecret.Key,
				},
			},
		}
	} else {
		return corev1.EnvVar{}, fmt.Errorf("invalid environment variable %s", name)
	}
	return currentConfig, nil
}

func (r *FdatabaseReconciler) serviceForFDatabase(fdatabase *k8sv1.Fdatabase) (*corev1.Service, error) {
	name := fdatabase.Name
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: fdatabase.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app.kubernetes.io/name": name + "-db",
			},
			Ports: []corev1.ServicePort{{
				Port:       3306,
				TargetPort: intstr.FromInt(int(3306)),
				Protocol:   corev1.ProtocolTCP,
			}},
		},
	}
	if err := ctrl.SetControllerReference(fdatabase, svc, r.Scheme); err != nil {
		return nil, err
	}
	return svc, nil
}

// labelsForFDatabase returns the labels for selecting the resources
// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/common-labels/
func labelsForFDatabase(name string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":     name + "-db",
		"app.kubernetes.io/instance": name + "-db",
		"app.kubernetes.io/part-of":  "f-operator",
	}
}
