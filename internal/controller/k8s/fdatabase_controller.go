/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package k8s

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
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
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	k8sv1 "github.com/fabiokaelin/f-operator/api/k8s/v1"
	"github.com/fabiokaelin/f-operator/internal/utils"
	"github.com/go-logr/logr"
)

// FdatabaseReconciler reconciles a Fdatabase object
type FdatabaseReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

const (
	typeAvailableFDatabase = "Available"
	typeDegradedFDatabase  = "Degraded"
)

const fdatabaseFinalizer = "k8s.fabkli.ch/finalizer"

// +kubebuilder:rbac:groups=k8s.fabkli.ch,resources=fdatabases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=k8s.fabkli.ch,resources=fdatabases/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=k8s.fabkli.ch,resources=fdatabases/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services;persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
func (r *FdatabaseReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	fmt.Println("|||||||||||||||||||||||||||||||||||||||||||||||||")
	log := log.FromContext(ctx)
	flog := utils.Init()
	r.Recorder = record.NewFakeRecorder(100)

	// i need: PVC, Service, Deployment

	fdatabase := &k8sv1.Fdatabase{}
	err := r.Get(ctx, req.NamespacedName, fdatabase)
	if err != nil {
		if apierrors.IsNotFound(err) {
			flog.Info("fdatabase resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		flog.Info(err, "Failed to get fdatabase")
		return ctrl.Result{}, err
	}
	{
		// !Let's just set the status as Unknown when no status are available
		if fdatabase.Status.Conditions == nil || len(fdatabase.Status.Conditions) == 0 {
			err := r.setStatusToUnknown(ctx, fdatabase, req, log, flog)
			if err != nil {
				return ctrl.Result{}, err
			}
		}
		if !controllerutil.ContainsFinalizer(fdatabase, fdatabaseFinalizer) {
			flog.Info("Adding Finalizer for fdatabase")
			if ok := controllerutil.AddFinalizer(fdatabase, fdatabaseFinalizer); !ok {
				flog.Info(err, "Failed to add finalizer into the custom resource")
				return ctrl.Result{Requeue: true}, nil
			}
			flog.Info("Update 1 before")
			err = r.Update(ctx, fdatabase)
			flog.Info("Update 1 after")

			if err != nil {
				flog.Info(err, "Failed to update custom resource to add finalizer")
				return ctrl.Result{}, err
			}
		}
		isFDatabaseMarkedToBeDeleted := fdatabase.GetDeletionTimestamp() != nil
		if isFDatabaseMarkedToBeDeleted {
			if controllerutil.ContainsFinalizer(fdatabase, fdatabaseFinalizer) {
				flog.Info("Performing Finalizer Operations for fdatabase before delete CR")
				meta.SetStatusCondition(&fdatabase.Status.Conditions, metav1.Condition{Type: typeDegradedFDatabase,
					Status: metav1.ConditionUnknown, Reason: "Finalizing",
					Message: fmt.Sprintf("Performing finalizer operations for the custom resource: %s ", fdatabase.Name)})
				flog.Info("Update 2 before")
				err := r.Status().Update(ctx, fdatabase)
				flog.Info("Update 2 after")
				if err != nil {
					flog.Info(err, "Failed to update fdatabase status 1")
					return ctrl.Result{}, err
				}
				r.doFinalizerOperationsForFDatabase(fdatabase)

				meta.SetStatusCondition(&fdatabase.Status.Conditions, metav1.Condition{Type: typeDegradedFDatabase,
					Status: metav1.ConditionTrue, Reason: "Finalizing",
					Message: fmt.Sprintf("Finalizer operations for custom resource %s name were successfully accomplished", fdatabase.Name)})

				flog.Info("Update 3 before")
				err = r.Status().Update(ctx, fdatabase)
				flog.Info("Update 3 after")

				if err != nil {
					flog.Info(err, "Failed to update fdatabase status 2")
					return ctrl.Result{}, err
				}

				flog.Info("Removing Finalizer for fdatabase after successfully perform the operations")
				if ok := controllerutil.RemoveFinalizer(fdatabase, fdatabaseFinalizer); !ok {
					flog.Info(err, "Failed to remove finalizer for fdatabase")
					return ctrl.Result{Requeue: true}, nil
				}

				flog.Info("Update 4 before")
				err = r.Update(ctx, fdatabase)
				flog.Info("Update 4 after")

				if err != nil {
					flog.Info(err, "Failed to remove finalizer for fdatabase")
					return ctrl.Result{}, err
				}
			}
			return ctrl.Result{}, nil
		}
	}

	foundPVC := &corev1.PersistentVolumeClaim{}
	err = r.Get(ctx, types.NamespacedName{Name: fdatabase.Name, Namespace: fdatabase.Namespace}, foundPVC)
	if err != nil && apierrors.IsNotFound(err) {
		pvc, err := r.pvcForFDatabase(fdatabase)
		if err != nil {
			flog.Info(err, "Failed to define new pvc resource for fdatabase")

			// The following implementation will update the status
			meta.SetStatusCondition(&fdatabase.Status.Conditions, metav1.Condition{Type: typeAvailableFDatabase,
				Status: metav1.ConditionFalse, Reason: "Reconciling",
				Message: fmt.Sprintf("Failed to create pvc for the custom resource (%s): (%s)", fdatabase.Name, err)})

			flog.Info("Update 5 before")
			err := r.Status().Update(ctx, fdatabase)
			flog.Info("Update 5 after")

			if err != nil {
				flog.Info(err, "Failed to update fdatabase status 3")
				return ctrl.Result{}, err
			}

			return ctrl.Result{}, err
		}
		flog.Info("Creating a new pvc")

		if err = r.Create(ctx, pvc); err != nil {
			flog.Info(err, "Failed to create new pvc")
			return ctrl.Result{}, err
		}

		// pvc created successfully
		// We will requeue the reconciliation so that we can ensure the state
		// and move forward for the next operations
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	} else if err != nil {
		flog.Info(err, "Failed to get pvc")
		return ctrl.Result{}, err
	}

	foundDeployment := &appsv1.Deployment{}
	err = r.Get(ctx, types.NamespacedName{Name: fdatabase.Name, Namespace: fdatabase.Namespace}, foundDeployment)
	if err != nil && apierrors.IsNotFound(err) {
		deployment, err := r.deploymentForFDatabase(fdatabase)
		if err != nil {
			flog.Info(err, "Failed to define new deployment resource for fdatabase")

			// The following implementation will update the status
			meta.SetStatusCondition(&fdatabase.Status.Conditions, metav1.Condition{Type: typeAvailableFDatabase,
				Status: metav1.ConditionFalse, Reason: "Reconciling",
				Message: fmt.Sprintf("Failed to create for the custom resource (%s): (%s)", fdatabase.Name, err)})

			flog.Info("Update 5 before")
			err := r.Status().Update(ctx, fdatabase)
			flog.Info("Update 5 after")

			if err != nil {
				flog.Info(err, "Failed to update fdatabase status 3")
				return ctrl.Result{}, err
			}

			return ctrl.Result{}, err
		}
		flog.Info("Creating a new deployment")

		if err = r.Create(ctx, deployment); err != nil {
			flog.Info(err, "Failed to create new deployment")
			return ctrl.Result{}, err
		}

		// deployment created successfully
		// We will requeue the reconciliation so that we can ensure the state
		// and move forward for the next operations
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	} else if err != nil {
		flog.Info(err, "Failed to get deployment")
		return ctrl.Result{}, err
	}

	foundService := &corev1.Service{}
	err = r.Get(ctx, types.NamespacedName{Name: fdatabase.Name, Namespace: fdatabase.Namespace}, foundService)
	if err != nil && apierrors.IsNotFound(err) {
		svc, err := r.serviceForFDatabase(fdatabase)
		if err != nil {
			flog.Info(err, "Failed to define new Service resource for fdatabase")

			// The following implementation will update the status
			meta.SetStatusCondition(&fdatabase.Status.Conditions, metav1.Condition{Type: typeAvailableFDatabase,
				Status: metav1.ConditionFalse, Reason: "Reconciling",
				Message: fmt.Sprintf("Failed to create Service for the custom resource (%s): (%s)", fdatabase.Name, err)})

			flog.Info("Update 5 before")
			err := r.Status().Update(ctx, fdatabase)
			flog.Info("Update 5 after")

			if err != nil {
				flog.Info(err, "Failed to update fdatabase status 3")
				return ctrl.Result{}, err
			}

			return ctrl.Result{}, err
		}
		flog.Info("Creating a new Service")

		if err = r.Create(ctx, svc); err != nil {
			flog.Info(err, "Failed to create new Service")
			return ctrl.Result{}, err
		}

		// service created successfully
		// We will requeue the reconciliation so that we can ensure the state
		// and move forward for the next operations
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	} else if err != nil {
		flog.Info(err, "Failed to get service")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *FdatabaseReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&k8sv1.Fdatabase{}).
		Owns(&corev1.Service{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Complete(r)
}

func (r *FdatabaseReconciler) setStatusToUnknown(ctx context.Context, fdatabase *k8sv1.Fdatabase, req reconcile.Request, log logr.Logger, flog utils.Log) error {

	meta.SetStatusCondition(&fdatabase.Status.Conditions, metav1.Condition{Type: typeAvailableFDatabase, Status: metav1.ConditionUnknown, Reason: "Reconciling", Message: "Starting reconciliation"})

	flog.Info("Update 8 before")
	err := r.Status().Update(ctx, fdatabase)
	flog.Info("Update 8 after")
	if err != nil {
		flog.Info(err, "Failed to update fdatabase status 6")
		return err
	}
	return nil
}

func (r *FdatabaseReconciler) doFinalizerOperationsForFDatabase(cr *k8sv1.Fdatabase) {
	r.Recorder.Event(cr, "Warning", "Deleting",
		fmt.Sprintf("Custom Resource %s is being deleted from the namespace %s",
			cr.Name,
			cr.Namespace))
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
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(storage),
				},
			},
		},
	}
	if err := ctrl.SetControllerReference(fdatabase, pvc, r.Scheme); err != nil {
		return nil, err
	}
	return pvc, nil
}

func (r *FdatabaseReconciler) deploymentForFDatabase(fdatabase *k8sv1.Fdatabase) (*appsv1.Deployment, error) {
	name := fdatabase.Name
	// int to int32
	replicas := int32(1)
	ls := labelsForFDatabase(fdatabase.Name)
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
	mountPropagation := corev1.MountPropagationNone

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Labels:    ls,
			Name:      name,
			Namespace: fdatabase.Namespace,
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
					Containers: []corev1.Container{{
						Name:            name,
						Image:           "mariadb:11",
						ImagePullPolicy: corev1.PullIfNotPresent,
						Ports: []corev1.ContainerPort{{
							ContainerPort: 3306,
							Protocol:      corev1.ProtocolTCP,
						}},
						Env: []corev1.EnvVar{
							database,
							rootPassword,
							password,
							user,
						},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								"cpu":    resource.MustParse("128Mi"),
								"memory": resource.MustParse("50m"),
							},
							Limits: corev1.ResourceList{
								"cpu":    resource.MustParse("1024Mi"),
								"memory": resource.MustParse("200m"),
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

	if fdatabase.Spec.RootHost != "" {
		deployment.Spec.Template.Spec.Containers[0].Env = append(deployment.Spec.Template.Spec.Containers[0].Env, corev1.EnvVar{
			Name:  "MARIADB_ROOT_HOST",
			Value: fdatabase.Spec.RootHost,
		})
	}

	if err := ctrl.SetControllerReference(fdatabase, deployment, r.Scheme); err != nil {
		return nil, err
	}
	return deployment, nil
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

// labelsForFDeployment returns the labels for selecting the resources
// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/common-labels/
func labelsForFDatabase(name string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":     name + "-db",
		"app.kubernetes.io/instance": name + "-db",
		"app.kubernetes.io/part-of":  "f-operator",
	}
}
