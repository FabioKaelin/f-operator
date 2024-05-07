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
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	networking "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	k8sv1 "github.com/fabiokaelin/f-operator/api/k8s/v1"
	"github.com/fabiokaelin/f-operator/internal/utils"
	"github.com/go-logr/logr"
)

// FdeploymentReconciler reconciles a Fdeployment object
type FdeploymentReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

const (
	// typeAvailableMemcached represents the status of the Deployment reconciliation
	typeAvailableFDeployment = "Available"
	// typeDegradedMemcached represents the status used when the custom resource is deleted and the finalizer operations are must to occur.
	typeDegradedFDeployment = "Degraded"
)

const fdeploymentFinalizer = "k8s.fabkli.ch/finalizer"

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Fdeployment object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.15.0/pkg/reconcile

//+kubebuilder:rbac:groups=k8s.fabkli.ch,resources=fdeployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=k8s.fabkli.ch,resources=fdeployments/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=k8s.fabkli.ch,resources=fdeployments/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services;serviceaccounts;pods;secrets;configmaps;persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete;

func (r *FdeploymentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	fmt.Println("------------------")
	log := log.FromContext(ctx)
	flog := utils.Init()
	r.Recorder = record.NewFakeRecorder(100)
	fdeployment := &k8sv1.Fdeployment{}
	flog.Info("req.NamespacedName", req.NamespacedName)
	err := r.Get(ctx, req.NamespacedName, fdeployment)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// If the custom resource is not found then, it usually means that it was deleted or not created
			// In this way, we will stop the reconciliation
			// flog.Info("fdeployment resource not found. Ignoring since object must be deleted")
			flog.Info("fdeployment resource not found. Ignoring since object must be deleted")
			// log.Info("fdeployment resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		flog.Info(err, "Failed to get fdeployment")
		// log.Error(err, "Failed to get fdeployment")
		return ctrl.Result{}, err
	}

	// flog.Info("ooooooooooooooooooooooooooooooo")

	// flog.Info("name:", req.Name)
	// flog.Info("namespace:", req.Namespace)
	// flog.Info("fdeployment")
	// spew.Dump(fdeployment)
	// flog.Info("------------")
	// flog.Info("req.NamespacedName")
	// spew.Dump(req.NamespacedName)
	// flog.Info("ooooooooooooooooooooooooooooooo")

	// !Let's just set the status as Unknown when no status are available
	if fdeployment.Status.Conditions == nil || len(fdeployment.Status.Conditions) == 0 {
		err := r.setStatusToUnknown(ctx, fdeployment, req, log, flog)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	// avoid this error:
	// Operation cannot be fulfilled on fdeployments.k8s.fabkli.ch \"fdeployment-sample\": the object has been modified; please apply your changes to the latest version and try again

	if !controllerutil.ContainsFinalizer(fdeployment, fdeploymentFinalizer) {
		flog.Info("Adding Finalizer for fdeployment")
		// log.Info("Adding Finalizer for fdeployment")
		if ok := controllerutil.AddFinalizer(fdeployment, fdeploymentFinalizer); !ok {
			flog.Info(err, "Failed to add finalizer into the custom resource")
			// log.Error(err, "Failed to add finalizer into the custom resource")
			return ctrl.Result{Requeue: true}, nil
		}

		// if err := r.Get(ctx, req.NamespacedName, fdeployment); err != nil {
		// 	log.Error(err, "Failed to re-fetch fdeployment")
		// 	return ctrl.Result{}, err
		// }
		flog.Info("Update 1 before")
		err = r.Update(ctx, fdeployment)
		flog.Info("Update 1 after")

		if err != nil {
			flog.Info(err, "Failed to update custom resource to add finalizer")
			// log.Error(err, "Failed to update custom resource to add finalizer")
			return ctrl.Result{}, err
		}
	} // Check if the fdeployment instance is marked to be deleted, which is
	// indicated by the deletion timestamp being set.
	isFDeploymentMarkedToBeDeleted := fdeployment.GetDeletionTimestamp() != nil
	if isFDeploymentMarkedToBeDeleted {
		if controllerutil.ContainsFinalizer(fdeployment, fdeploymentFinalizer) {
			// log.Info("Performing Finalizer Operations for fdeployment before delete CR")
			flog.Info("Performing Finalizer Operations for fdeployment before delete CR")

			// Let's add here an status "Downgrade" to define that this resource begin its process to be terminated.
			meta.SetStatusCondition(&fdeployment.Status.Conditions, metav1.Condition{Type: typeDegradedFDeployment,
				Status: metav1.ConditionUnknown, Reason: "Finalizing",
				Message: fmt.Sprintf("Performing finalizer operations for the custom resource: %s ", fdeployment.Name)})

			flog.Info("Update 2 before")
			err := r.Status().Update(ctx, fdeployment)
			flog.Info("Update 2 after")

			if err != nil {
				// log.Error(err, "Failed to update fdeployment status")
				flog.Info(err, "Failed to update fdeployment status 1")
				return ctrl.Result{}, err
			}

			// Perform all operations required before remove the finalizer and allow
			// the Kubernetes API to remove the custom resource.
			r.doFinalizerOperationsForFDeployment(fdeployment)

			// TODO(user): If you add operations to the doFinalizerOperationsForFDeployment method
			// then you need to ensure that all worked fine before deleting and updating the Downgrade status
			// otherwise, you should requeue here.

			// Re-fetch the fdeployment Custom Resource before update the status
			// so that we have the latest state of the resource on the cluster and we will avoid
			// raise the issue "the object has been modified, please apply
			// your changes to the latest version and try again" which would re-trigger the reconciliation
			// if err := r.Get(ctx, req.NamespacedName, fdeployment); err != nil {
			// 	flog.Info(err, "Failed to re-fetch fdeployment")
			// 	// log.Error(err, "Failed to re-fetch fdeployment")
			// 	return ctrl.Result{}, err
			// }

			meta.SetStatusCondition(&fdeployment.Status.Conditions, metav1.Condition{Type: typeDegradedFDeployment,
				Status: metav1.ConditionTrue, Reason: "Finalizing",
				Message: fmt.Sprintf("Finalizer operations for custom resource %s name were successfully accomplished", fdeployment.Name)})

			flog.Info("Update 3 before")
			err = r.Status().Update(ctx, fdeployment)
			flog.Info("Update 3 after")

			if err != nil {
				flog.Info(err, "Failed to update fdeployment status 2")
				// log.Error(err, "Failed to update fdeployment status")
				return ctrl.Result{}, err
			}

			flog.Info("Removing Finalizer for fdeployment after successfully perform the operations")
			// log.Info("Removing Finalizer for fdeployment after successfully perform the operations")
			if ok := controllerutil.RemoveFinalizer(fdeployment, fdeploymentFinalizer); !ok {
				flog.Info(err, "Failed to remove finalizer for fdeployment")
				// log.Error(err, "Failed to remove finalizer for fdeployment")
				return ctrl.Result{Requeue: true}, nil
			}

			flog.Info("Update 4 before")
			err = r.Update(ctx, fdeployment)
			flog.Info("Update 4 after")

			if err != nil {
				flog.Info(err, "Failed to remove finalizer for fdeployment")
				// log.Error(err, "Failed to remove finalizer for fdeployment")
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	foundServiceAccount := &corev1.ServiceAccount{}

	err = r.Get(ctx, types.NamespacedName{Name: fdeployment.Name, Namespace: fdeployment.Namespace}, foundServiceAccount)
	if err != nil && apierrors.IsNotFound(err) {
		// Define a new ServiceAccount object
		svcAcc, err := r.serviceAccountForFDeployment(fdeployment)
		if err != nil {
			flog.Info(err, "Failed to define new ServiceAccount resource for fdeployment")

			// The following implementation will update the status
			meta.SetStatusCondition(&fdeployment.Status.Conditions, metav1.Condition{Type: typeAvailableFDeployment,
				Status: metav1.ConditionFalse, Reason: "Reconciling",
				Message: fmt.Sprintf("Failed to create ServiceAccount for the custom resource (%s): (%s)", fdeployment.Name, err)})

			flog.Info("Update 5 before")
			err := r.Status().Update(ctx, fdeployment)
			flog.Info("Update 5 after")

			if err != nil {
				flog.Info(err, "Failed to update fdeployment status 3")
				return ctrl.Result{}, err
			}

			return ctrl.Result{}, err
		}
		flog.Info("Creating a new ServiceAccount")

		if err = r.Create(ctx, svcAcc); err != nil {
			flog.Info(err, "Failed to create new ServiceAccount")
			return ctrl.Result{}, err
		}

		// ServiceAccount created successfully
		// We will requeue the reconciliation so that we can ensure the state
		// and move forward for the next operations
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	} else if err != nil {
		flog.Info(err, "Failed to get ServiceAccount")
		return ctrl.Result{}, err
	}

	// Check if the deployment already exists, if not create a new one
	foundDeployment := &appsv1.Deployment{}
	err = r.Get(ctx, types.NamespacedName{Name: fdeployment.Name, Namespace: fdeployment.Namespace}, foundDeployment)
	if err != nil && apierrors.IsNotFound(err) {
		// Define a new deployment
		dep, err := r.deploymentForFDeployment(fdeployment)
		if err != nil {
			flog.Info(err, "Failed to define new Deployment resource for fdeployment")
			// log.Error(err, "Failed to define new Deployment resource for fdeployment")

			// The following implementation will update the status
			meta.SetStatusCondition(&fdeployment.Status.Conditions, metav1.Condition{Type: typeAvailableFDeployment,
				Status: metav1.ConditionFalse, Reason: "Reconciling",
				Message: fmt.Sprintf("Failed to create Deployment for the custom resource (%s): (%s)", fdeployment.Name, err)})

			flog.Info("Update 5 before")
			err := r.Status().Update(ctx, fdeployment)
			flog.Info("Update 5 after")

			if err != nil {
				flog.Info(err, "Failed to update fdeployment status 3")
				// log.Error(err, "Failed to update fdeployment status")
				return ctrl.Result{}, err
			}

			return ctrl.Result{}, err
		}
		flog.Info("Creating a new Deployment")
		// log.Info("Creating a new Deployment",
		// 	"Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)

		if err = r.Create(ctx, dep); err != nil {
			flog.Info(err, "Failed to create new Deployment")
			// log.Error(err, "Failed to create new Deployment",
			// 	"Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
			return ctrl.Result{}, err
		}

		// Deployment created successfully
		// We will requeue the reconciliation so that we can ensure the state
		// and move forward for the next operations
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	} else if err != nil {
		flog.Info(err, "Failed to get Deployment")
		return ctrl.Result{}, err
	}

	// Check if the deployment already exists, if not create a new one
	foundService := &corev1.Service{}
	err = r.Get(ctx, types.NamespacedName{Name: fdeployment.Name, Namespace: fdeployment.Namespace}, foundService)
	if err != nil && apierrors.IsNotFound(err) {
		// Define a new deployment
		svc, err := r.serviceForFDeployment(fdeployment)
		if err != nil {
			flog.Info(err, "Failed to define new Service resource for fdeployment")
			// log.Error(err, "Failed to define new Deployment resource for fdeployment")

			// The following implementation will update the status
			meta.SetStatusCondition(&fdeployment.Status.Conditions, metav1.Condition{Type: typeAvailableFDeployment,
				Status: metav1.ConditionFalse, Reason: "Reconciling",
				Message: fmt.Sprintf("Failed to create Service for the custom resource (%s): (%s)", fdeployment.Name, err)})

			flog.Info("Update 5 before")
			err := r.Status().Update(ctx, fdeployment)
			flog.Info("Update 5 after")

			if err != nil {
				flog.Info(err, "Failed to update fdeployment status 3")
				// log.Error(err, "Failed to update fdeployment status")
				return ctrl.Result{}, err
			}

			return ctrl.Result{}, err
		}
		flog.Info("Creating a new Service")
		// log.Info("Creating a new Deployment",
		// 	"Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)

		if err = r.Create(ctx, svc); err != nil {
			flog.Info(err, "Failed to create new Service")
			// log.Error(err, "Failed to create new Deployment",
			// 	"Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
			return ctrl.Result{}, err
		}

		// Deployment created successfully
		// We will requeue the reconciliation so that we can ensure the state
		// and move forward for the next operations
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	} else if err != nil {
		flog.Info(err, "Failed to get Deployment")
		return ctrl.Result{}, err
	}

	// Check if the deployment already exists, if not create a new one
	foundIngress := &networking.Ingress{}
	err = r.Get(ctx, types.NamespacedName{Name: fdeployment.Name, Namespace: fdeployment.Namespace}, foundIngress)
	if err != nil && apierrors.IsNotFound(err) {
		// Define a new deployment
		ing, err := r.ingressForFDeployment(fdeployment)
		if err != nil {
			flog.Info(err, "Failed to define new Ingress resource for fdeployment")
			// log.Error(err, "Failed to define new Deployment resource for fdeployment")

			// The following implementation will update the status
			meta.SetStatusCondition(&fdeployment.Status.Conditions, metav1.Condition{Type: typeAvailableFDeployment,
				Status: metav1.ConditionFalse, Reason: "Reconciling",
				Message: fmt.Sprintf("Failed to create Ingress for the custom resource (%s): (%s)", fdeployment.Name, err)})

			flog.Info("Update 5 before")
			err := r.Status().Update(ctx, fdeployment)
			flog.Info("Update 5 after")

			if err != nil {
				flog.Info(err, "Failed to update fdeployment status 3")
				// log.Error(err, "Failed to update fdeployment status")
				return ctrl.Result{}, err
			}

			return ctrl.Result{}, err
		}
		flog.Info("Creating a new Ingress")
		// log.Info("Creating a new Deployment",
		// 	"Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)

		if err = r.Create(ctx, ing); err != nil {
			flog.Info(err, "Failed to create new Ingress")
			// log.Error(err, "Failed to create new Deployment",
			// 	"Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
			return ctrl.Result{}, err
		}

		// Deployment created successfully
		// We will requeue the reconciliation so that we can ensure the state
		// and move forward for the next operations
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	} else if err != nil {
		flog.Info(err, "Failed to get Deployment")
		return ctrl.Result{}, err
	}

	foundDeployment.Spec.Replicas = &fdeployment.Spec.Replicas
	foundDeployment.Spec.Template.Spec.Containers[0].Ports[0].ContainerPort = fdeployment.Spec.Port

	imageName := ""
	if fdeployment.Spec.Image != "" {
		imageName = fmt.Sprintf("ghcr.io/fabiokaelin/%s:%s", fdeployment.Spec.Image, fdeployment.Spec.Tag)
	} else {
		imageName = fmt.Sprintf("ghcr.io/fabiokaelin/%s:%s", fdeployment.Name, fdeployment.Spec.Tag)
	}

	foundDeployment.Spec.Template.Spec.Containers[0].Image = imageName
	foundDeployment.Spec.Template.Spec.Containers[0].ReadinessProbe.HTTPGet.Port = intstr.FromInt(int(fdeployment.Spec.Port))
	foundDeployment.Spec.Template.Spec.Containers[0].ReadinessProbe.HTTPGet.Path = fdeployment.Spec.HealthCheck.ReadinessProbe.Path
	foundDeployment.Spec.Template.Spec.Containers[0].LivenessProbe.HTTPGet.Port = intstr.FromInt(int(fdeployment.Spec.Port))
	foundDeployment.Spec.Template.Spec.Containers[0].LivenessProbe.HTTPGet.Path = fdeployment.Spec.HealthCheck.LivenessProbe.Path
	foundService.Spec.Ports[0].TargetPort = intstr.FromInt(int(fdeployment.Spec.Port))
	foundIngress.Spec.Rules[0].Host = fdeployment.Spec.Host
	foundIngress.Spec.Rules[0].HTTP.Paths[0].Path = fdeployment.Spec.Path
	// resource limits and request
	limitCpu := resource.MustParse(fdeployment.Spec.Resources.Limits.CPU)
	foundDeployment.Spec.Template.Spec.Containers[0].Resources.Limits.Cpu().Set(limitCpu.Value())
	limitMemory := resource.MustParse(fdeployment.Spec.Resources.Limits.Memory)
	foundDeployment.Spec.Template.Spec.Containers[0].Resources.Limits.Memory().Set(limitMemory.Value())
	requestCpu := resource.MustParse(fdeployment.Spec.Resources.Requests.CPU)
	foundDeployment.Spec.Template.Spec.Containers[0].Resources.Requests.Cpu().Set(requestCpu.Value())
	requestMemory := resource.MustParse(fdeployment.Spec.Resources.Requests.Memory)
	foundDeployment.Spec.Template.Spec.Containers[0].Resources.Requests.Memory().Set(requestMemory.Value())

	// update env
	env, err := getEnvironment(fdeployment)
	if err != nil {
		flog.Info(err, "Failed to get environment")
		return ctrl.Result{}, err
	}
	foundDeployment.Spec.Template.Spec.Containers[0].Env = env

	err = r.Update(ctx, foundDeployment)
	if err != nil {
		// spew.Dump(found)
		flog.Info(err, "Failed to update Deployment (6)")
		// log.Error(err, "Failed to update Deployment",
		// 	"Deployment.Namespace", found.Namespace, "Deployment.Name", found.Name)

		// The following implementation will update the status
		meta.SetStatusCondition(&fdeployment.Status.Conditions, metav1.Condition{Type: typeAvailableFDeployment,
			Status: metav1.ConditionFalse, Reason: "Resizing",
			Message: fmt.Sprintf("Failed to update the size for the custom resource (%s): (%s)", fdeployment.Name, err)})

		flog.Info("Update 7 before")
		err := r.Status().Update(ctx, fdeployment)
		flog.Info("Update 7 after")

		if err != nil {
			flog.Info(err, "Failed to update FDeployment status 4")
			// log.Error(err, "Failed to update FDeployment status")
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, err
	}
	err = r.Update(ctx, foundService)
	if err != nil {
		// spew.Dump(found)
		flog.Info(err, "Failed to update Service (6)")
		// log.Error(err, "Failed to update Deployment",
		// 	"Deployment.Namespace", found.Namespace, "Deployment.Name", found.Name)

		// The following implementation will update the status
		meta.SetStatusCondition(&fdeployment.Status.Conditions, metav1.Condition{Type: typeAvailableFDeployment,
			Status: metav1.ConditionFalse, Reason: "Resizing",
			Message: fmt.Sprintf("Failed to update the size for the custom resource (%s): (%s)", fdeployment.Name, err)})

		flog.Info("Update 7 before")
		err := r.Status().Update(ctx, fdeployment)
		flog.Info("Update 7 after")

		if err != nil {
			flog.Info(err, "Failed to update FDeployment status 4")
			// log.Error(err, "Failed to update FDeployment status")
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, err
	}
	err = r.Update(ctx, foundIngress)
	if err != nil {
		// spew.Dump(found)
		flog.Info(err, "Failed to update Ingress (6)")
		// log.Error(err, "Failed to update Deployment",
		// 	"Deployment.Namespace", found.Namespace, "Deployment.Name", found.Name)

		// The following implementation will update the status
		meta.SetStatusCondition(&fdeployment.Status.Conditions, metav1.Condition{Type: typeAvailableFDeployment,
			Status: metav1.ConditionFalse, Reason: "Resizing",
			Message: fmt.Sprintf("Failed to update the size for the custom resource (%s): (%s)", fdeployment.Name, err)})

		flog.Info("Update 7 before")
		err := r.Status().Update(ctx, fdeployment)
		flog.Info("Update 7 after")

		if err != nil {
			flog.Info(err, "Failed to update FDeployment status 4")
			// log.Error(err, "Failed to update FDeployment status")
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, err
	}

	// The following implementation will update the status
	meta.SetStatusCondition(&fdeployment.Status.Conditions, metav1.Condition{Type: typeAvailableFDeployment,
		Status: metav1.ConditionTrue, Reason: "Reconciling",
		Message: fmt.Sprintf("Deployment for custom resource (%s) with %d replicas created successfully", fdeployment.Name, fdeployment.Spec.Replicas)})

	err = r.Status().Update(ctx, fdeployment)

	if err != nil {
		flog.Info(err, "Failed to update FDeployment status 5")
		// log.Error(err, "Failed to update FDeployment status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *FdeploymentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&k8sv1.Fdeployment{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&corev1.Service{}).
		Owns(&appsv1.Deployment{}).
		Owns(&networking.Ingress{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: 2}).
		Complete(r)
}

func (r *FdeploymentReconciler) setStatusToUnknown(ctx context.Context, fdeployment *k8sv1.Fdeployment, req reconcile.Request, log logr.Logger, flog utils.Log) error {
	// if err := r.Get(ctx, req.NamespacedName, fdeployment); err != nil {
	// 	flog.Info(err, "Failed to re-fetch fdeployment")
	// 	// log.Error(err, "Failed to re-fetch fdeployment")
	// 	return err
	// }
	// spew.Dump(fdeployment)
	meta.SetStatusCondition(&fdeployment.Status.Conditions, metav1.Condition{Type: typeAvailableFDeployment, Status: metav1.ConditionUnknown, Reason: "Reconciling", Message: "Starting reconciliation"})

	flog.Info("Update 8 before")
	err := r.Status().Update(ctx, fdeployment)
	flog.Info("Update 8 after")
	if err != nil {
		flog.Info(err, "Failed to update fdeployment status 6")
		// log.Error(err, "Failed to update fdeployment status")
		return err
	}

	// Let's re-fetch the fdeployment Custom Resource after update the status
	// so that we have the latest state of the resource on the cluster and we will avoid
	// raise the issue "the object has been modified, please apply
	// your changes to the latest version and try again" which would re-trigger the reconciliation
	// if we try to update it again in the following operations
	// if err := r.Get(ctx, req.NamespacedName, fdeployment); err != nil {
	// 	flog.Info(err, "Failed to re-fetch fdeployment")
	// 	// log.Error(err, "Failed to re-fetch fdeployment")
	// 	return err
	// }
	return nil
}

// finalizeMemcached will perform the required operations before delete the CR.
func (r *FdeploymentReconciler) doFinalizerOperationsForFDeployment(cr *k8sv1.Fdeployment) {
	// TODO(user): Add the cleanup steps that the operator
	// needs to do before the CR can be deleted. Examples
	// of finalizers include performing backups and deleting
	// resources that are not owned by this CR, like a PVC.

	// Note: It is not recommended to use finalizers with the purpose of delete resources which are
	// created and managed in the reconciliation. These ones, such as the Deployment created on this reconcile,
	// are defined as depended of the custom resource. See that we use the method ctrl.SetControllerReference.
	// to set the ownerRef which means that the Deployment will be deleted by the Kubernetes API.
	// More info: https://kubernetes.io/docs/tasks/administer-cluster/use-cascading-deletion/

	// The following implementation will raise an event
	// flog.Info("-----Deleting the Custom Resource")
	// flog.Info(cr.Name)
	// flog.Info(cr.Namespace)
	// flog.Info(r.Recorder)
	r.Recorder.Event(cr, "Warning", "Deleting",
		fmt.Sprintf("Custom Resource %s is being deleted from the namespace %s",
			cr.Name,
			cr.Namespace))
	// flog.Info("-----Deleting the Custom Resource2")
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
	envVars = append(envVars, versionEnv)
	return envVars, nil
}

// deploymentForFDeployment returns a FDeployment Deployment object
func (r *FdeploymentReconciler) deploymentForFDeployment(
	fdeployment *k8sv1.Fdeployment) (*appsv1.Deployment, error) {
	// path := fdeployment.Spec.Path
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
						RunAsNonRoot: &[]bool{false}[0],
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
							TimeoutSeconds:      1,
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
							TimeoutSeconds:      1,
							InitialDelaySeconds: 10,
						},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								"cpu":    resource.MustParse(fdeployment.Spec.Resources.Requests.CPU),
								"memory": resource.MustParse(fdeployment.Spec.Resources.Requests.Memory),
							},
							Limits: corev1.ResourceList{
								"cpu":    resource.MustParse(fdeployment.Spec.Resources.Limits.CPU),
								"memory": resource.MustParse(fdeployment.Spec.Resources.Limits.Memory),
							},
						},

						ImagePullPolicy: corev1.PullAlways,
						SecurityContext: &corev1.SecurityContext{
							RunAsNonRoot: &[]bool{false}[0],
							Privileged:   &[]bool{true}[0],
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

// deploymentForFDeployment returns a FDeployment ingress object
func (r *FdeploymentReconciler) ingressForFDeployment(
	fdeployment *k8sv1.Fdeployment) (*networking.Ingress, error) {
	path := fdeployment.Spec.Path
	// replicas := fdeployment.Spec.Replicas
	// port := fdeployment.Spec.Port
	name := fdeployment.Name
	host := fdeployment.Spec.Host
	// image := fmt.Sprintf("ghcr.io/fabiokaelin/%s:%s", fdeployment.Name, fdeployment.Spec.Tag)
	// ls := labelsForFDeployment(fdeployment.Name, image)

	// ingress networking.k8s.io/v1
	pathType := networking.PathTypePrefix
	ing := &networking.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: fdeployment.Namespace,
		},
		Spec: networking.IngressSpec{
			Rules: []networking.IngressRule{{
				Host: host,
				IngressRuleValue: networking.IngressRuleValue{
					HTTP: &networking.HTTPIngressRuleValue{
						Paths: []networking.HTTPIngressPath{{
							Path:     path,
							PathType: &pathType,
							Backend: networking.IngressBackend{
								Service: &networking.IngressServiceBackend{
									Name: name,
									Port: networking.ServiceBackendPort{
										Number: 80,
									},
								},
							},
						}},
					},
				},
			}},
		},
	}

	// Set the ownerRef for the Deployment
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/owners-dependents/
	if err := ctrl.SetControllerReference(fdeployment, ing, r.Scheme); err != nil {
		return nil, err
	}
	return ing, nil
}

// deploymentForFDeployment returns a FDeployment Deployment object
func (r *FdeploymentReconciler) serviceAccountForFDeployment(
	fdeployment *k8sv1.Fdeployment) (*corev1.ServiceAccount, error) {
	name := fdeployment.Name

	//create service account
	svcAcc := &corev1.ServiceAccount{
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
	imageTag := strings.Split(image, ":")[1]
	return map[string]string{
		"app.kubernetes.io/name":     name,
		"app.kubernetes.io/instance": name,
		"app.kubernetes.io/version":  imageTag,
		"app.kubernetes.io/part-of":  "f-operator",
	}
}
