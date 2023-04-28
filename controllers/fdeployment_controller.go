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

package controllers

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"

	"time"

	appsv1 "k8s.io/api/apps/v1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/davecgh/go-spew/spew"
	k8sv1 "github.com/fabiokaelin/f-operator/api/v1"
	"github.com/go-logr/logr"

	"sigs.k8s.io/controller-runtime/pkg/log"
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

//+kubebuilder:rbac:groups=k8s.fabkli.ch,resources=fdeployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=k8s.fabkli.ch,resources=fdeployments/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=k8s.fabkli.ch,resources=fdeployments/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Fdeployment object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *FdeploymentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	r.Recorder = record.NewFakeRecorder(100)
	fdeployment := &k8sv1.Fdeployment{}
	err := r.Get(ctx, req.NamespacedName, fdeployment)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// If the custom resource is not found then, it usually means that it was deleted or not created
			// In this way, we will stop the reconciliation
			log.Info("fdeployment resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get fdeployment")
		return ctrl.Result{}, err
	}

	// fmt.Println("ooooooooooooooooooooooooooooooo")

	// fmt.Println("name:", req.Name)
	// fmt.Println("namespace:", req.Namespace)
	// fmt.Println("fdeployment")
	// spew.Dump(fdeployment)
	// fmt.Println("------------")
	// fmt.Println("req.NamespacedName")
	// spew.Dump(req.NamespacedName)
	// fmt.Println("ooooooooooooooooooooooooooooooo")

	// !Let's just set the status as Unknown when no status are available
	if fdeployment.Status.Conditions == nil || len(fdeployment.Status.Conditions) == 0 {
		err := r.setStatusToUnknown(ctx, fdeployment, req, log)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	// avoid this error:
	// Operation cannot be fulfilled on fdeployments.k8s.fabkli.ch \"fdeployment-sample\": the object has been modified; please apply your changes to the latest version and try again

	if !controllerutil.ContainsFinalizer(fdeployment, fdeploymentFinalizer) {
		log.Info("Adding Finalizer for fdeployment")
		if ok := controllerutil.AddFinalizer(fdeployment, fdeploymentFinalizer); !ok {
			log.Error(err, "Failed to add finalizer into the custom resource")
			return ctrl.Result{Requeue: true}, nil
		}

		// if err := r.Get(ctx, req.NamespacedName, fdeployment); err != nil {
		// 	log.Error(err, "Failed to re-fetch fdeployment")
		// 	return ctrl.Result{}, err
		// }

		if err = r.Update(ctx, fdeployment); err != nil {
			log.Error(err, "Failed to update custom resource to add finalizer")
			return ctrl.Result{}, err
		}
	} // Check if the fdeployment instance is marked to be deleted, which is
	// indicated by the deletion timestamp being set.
	isFDeploymentMarkedToBeDeleted := fdeployment.GetDeletionTimestamp() != nil
	if isFDeploymentMarkedToBeDeleted {
		if controllerutil.ContainsFinalizer(fdeployment, fdeploymentFinalizer) {
			log.Info("Performing Finalizer Operations for fdeployment before delete CR")

			// Let's add here an status "Downgrade" to define that this resource begin its process to be terminated.
			meta.SetStatusCondition(&fdeployment.Status.Conditions, metav1.Condition{Type: typeDegradedFDeployment,
				Status: metav1.ConditionUnknown, Reason: "Finalizing",
				Message: fmt.Sprintf("Performing finalizer operations for the custom resource: %s ", fdeployment.Name)})

			if err := r.Status().Update(ctx, fdeployment); err != nil {
				log.Error(err, "Failed to update fdeployment status")
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
			if err := r.Get(ctx, req.NamespacedName, fdeployment); err != nil {
				log.Error(err, "Failed to re-fetch fdeployment")
				return ctrl.Result{}, err
			}

			meta.SetStatusCondition(&fdeployment.Status.Conditions, metav1.Condition{Type: typeDegradedFDeployment,
				Status: metav1.ConditionTrue, Reason: "Finalizing",
				Message: fmt.Sprintf("Finalizer operations for custom resource %s name were successfully accomplished", fdeployment.Name)})

			if err := r.Status().Update(ctx, fdeployment); err != nil {
				log.Error(err, "Failed to update fdeployment status")
				return ctrl.Result{}, err
			}

			log.Info("Removing Finalizer for fdeployment after successfully perform the operations")
			if ok := controllerutil.RemoveFinalizer(fdeployment, fdeploymentFinalizer); !ok {
				log.Error(err, "Failed to remove finalizer for fdeployment")
				return ctrl.Result{Requeue: true}, nil
			}

			if err := r.Update(ctx, fdeployment); err != nil {
				log.Error(err, "Failed to remove finalizer for fdeployment")
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Check if the deployment already exists, if not create a new one
	found := &appsv1.Deployment{}
	err = r.Get(ctx, types.NamespacedName{Name: fdeployment.Name, Namespace: fdeployment.Namespace}, found)
	fmt.Println("-------------------", err)
	if err != nil && apierrors.IsNotFound(err) {
		// Define a new deployment
		dep, err := r.deploymentForFDeployment(fdeployment)
		if err != nil {
			log.Error(err, "Failed to define new Deployment resource for fdeployment")

			// The following implementation will update the status
			meta.SetStatusCondition(&fdeployment.Status.Conditions, metav1.Condition{Type: typeAvailableFDeployment,
				Status: metav1.ConditionFalse, Reason: "Reconciling",
				Message: fmt.Sprintf("Failed to create Deployment for the custom resource (%s): (%s)", fdeployment.Name, err)})

			if err := r.Status().Update(ctx, fdeployment); err != nil {
				log.Error(err, "Failed to update fdeployment status")
				return ctrl.Result{}, err
			}

			return ctrl.Result{}, err
		}

		log.Info("Creating a new Deployment",
			"Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
		// if err = r.Create(ctx, dep); err != nil {
		// 	log.Error(err, "Failed to create new Deployment",
		// 		"Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
		// 	return ctrl.Result{}, err
		// }

		// Deployment created successfully
		// We will requeue the reconciliation so that we can ensure the state
		// and move forward for the next operations
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	} else if err != nil {
		log.Error(err, "Failed to get Deployment")
		// Let's return the error for the reconciliation be re-trigged again
		return ctrl.Result{}, err
	}

	// The CRD API is defining that the fdeployment type, have a fdeploymentSpec.Size field
	// to set the quantity of Deployment instances is the desired state on the cluster.
	// Therefore, the following code will ensure the Deployment size is the same as defined
	// via the Size spec of the Custom Resource which we are reconciling.
	size := fdeployment.Spec.Replicas
	if *found.Spec.Replicas != size {
		found.Spec.Replicas = &size
		if err = r.Update(ctx, found); err != nil {
			log.Error(err, "Failed to update Deployment",
				"Deployment.Namespace", found.Namespace, "Deployment.Name", found.Name)

			// Re-fetch the fdeployment Custom Resource before update the status
			// so that we have the latest state of the resource on the cluster and we will avoid
			// raise the issue "the object has been modified, please apply
			// your changes to the latest version and try again" which would re-trigger the reconciliation
			if err := r.Get(ctx, req.NamespacedName, fdeployment); err != nil {
				log.Error(err, "Failed to re-fetch fdeployment")
				return ctrl.Result{}, err
			}

			// The following implementation will update the status
			meta.SetStatusCondition(&fdeployment.Status.Conditions, metav1.Condition{Type: typeAvailableFDeployment,
				Status: metav1.ConditionFalse, Reason: "Resizing",
				Message: fmt.Sprintf("Failed to update the size for the custom resource (%s): (%s)", fdeployment.Name, err)})

			if err := r.Status().Update(ctx, fdeployment); err != nil {
				log.Error(err, "Failed to update FDeployment status")
				return ctrl.Result{}, err
			}

			return ctrl.Result{}, err
		}

		// Now, that we update the size we want to requeue the reconciliation
		// so that we can ensure that we have the latest state of the resource before
		// update. Also, it will help ensure the desired state on the cluster
		return ctrl.Result{Requeue: true}, nil
	}
	if err := r.Get(ctx, req.NamespacedName, fdeployment); err != nil {
		log.Error(err, "Failed to re-fetch fdeployment")
		return ctrl.Result{}, err
	}

	// The following implementation will update the status
	meta.SetStatusCondition(&fdeployment.Status.Conditions, metav1.Condition{Type: typeAvailableFDeployment,
		Status: metav1.ConditionTrue, Reason: "Reconciling",
		Message: fmt.Sprintf("Deployment for custom resource (%s) with %d replicas created successfully", fdeployment.Name, size)})

	if err := r.Status().Update(ctx, fdeployment); err != nil {
		log.Error(err, "Failed to update FDeployment status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *FdeploymentReconciler) setStatusToUnknown(ctx context.Context, fdeployment *k8sv1.Fdeployment, req reconcile.Request, log logr.Logger) error {
	if err := r.Get(ctx, req.NamespacedName, fdeployment); err != nil {
		log.Error(err, "Failed to re-fetch fdeployment")
		return err
	}
	spew.Dump(fdeployment)
	meta.SetStatusCondition(&fdeployment.Status.Conditions, metav1.Condition{Type: typeAvailableFDeployment, Status: metav1.ConditionUnknown, Reason: "Reconciling", Message: "Starting reconciliation"})
	if err := r.Status().Update(ctx, fdeployment); err != nil {
		log.Error(err, "Failed to update fdeployment status")
		return err
	}

	// Let's re-fetch the fdeployment Custom Resource after update the status
	// so that we have the latest state of the resource on the cluster and we will avoid
	// raise the issue "the object has been modified, please apply
	// your changes to the latest version and try again" which would re-trigger the reconciliation
	// if we try to update it again in the following operations
	if err := r.Get(ctx, req.NamespacedName, fdeployment); err != nil {
		log.Error(err, "Failed to re-fetch fdeployment")
		return err
	}
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
	fmt.Println("-----Deleting the Custom Resource")
	fmt.Println(cr.Name)
	fmt.Println(cr.Namespace)
	fmt.Println(r.Recorder)
	r.Recorder.Event(cr, "Warning", "Deleting",
		fmt.Sprintf("Custom Resource %s is being deleted from the namespace %s",
			cr.Name,
			cr.Namespace))
	fmt.Println("-----Deleting the Custom Resource2")
}

// deploymentForFDeployment returns a FDeployment Deployment object
func (r *FdeploymentReconciler) deploymentForFDeployment(
	fdeployment *k8sv1.Fdeployment) (*appsv1.Deployment, error) {
	component := fdeployment.Spec.Component
	app := fdeployment.Spec.App
	// path := fdeployment.Spec.Path
	replicas := fdeployment.Spec.Replicas
	port := fdeployment.Spec.Port
	name := fmt.Sprintf("%s-%s", app, component)
	image := fmt.Sprintf("ghcr.io/fabiokaelin/%s-%s", app, component)
	ls := labelsForFDeployment(fdeployment.Name, image)

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
					// TODO(user): Uncomment the following code to configure the nodeAffinity expression
					// according to the platforms which are supported by your solution. It is considered
					// best practice to support multiple architectures. build your manager image using the
					// makefile target docker-buildx. Also, you can use docker manifest inspect <image>
					// to check what are the platforms supported.
					// More info: https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/#node-affinity
					//Affinity: &corev1.Affinity{
					//	NodeAffinity: &corev1.NodeAffinity{
					//		RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
					//			NodeSelectorTerms: []corev1.NodeSelectorTerm{
					//				{
					//					MatchExpressions: []corev1.NodeSelectorRequirement{
					//						{
					//							Key:      "kubernetes.io/arch",
					//							Operator: "In",
					//							Values:   []string{"amd64", "arm64", "ppc64le", "s390x"},
					//						},
					//						{
					//							Key:      "kubernetes.io/os",
					//							Operator: "In",
					//							Values:   []string{"linux"},
					//						},
					//					},
					//				},
					//			},
					//		},
					//	},
					//},
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: &[]bool{true}[0],
						// IMPORTANT: seccomProfile was introduced with Kubernetes 1.19
						// If you are looking for to produce solutions to be supported
						// on lower versions you must remove this option.
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
					Containers: []corev1.Container{{
						Image:           image,
						Name:            "fdeployment",
						ImagePullPolicy: corev1.PullIfNotPresent,
						// Ensure restrictive context for the container
						// More info: https://kubernetes.io/docs/concepts/security/pod-security-standards/#restricted
						SecurityContext: &corev1.SecurityContext{
							// WARNING: Ensure that the image used defines an UserID in the Dockerfile
							// otherwise the Pod will not run and will fail with "container has runAsNonRoot and image has non-numeric user"".
							// If you want your workloads admitted in namespaces enforced with the restricted mode in OpenShift/OKD vendors
							// then, you MUST ensure that the Dockerfile defines a User ID OR you MUST leave the "RunAsNonRoot" and
							// "RunAsUser" fields empty.
							RunAsNonRoot: &[]bool{true}[0],
							// The memcached image does not use a non-zero numeric user as the default user.
							// Due to RunAsNonRoot field being set to true, we need to force the user in the
							// container to a non-zero numeric user. We do this using the RunAsUser field.
							// However, if you are looking to provide solution for K8s vendors like OpenShift
							// be aware that you cannot run under its restricted-v2 SCC if you set this value.
							RunAsUser:                &[]int64{1001}[0],
							AllowPrivilegeEscalation: &[]bool{false}[0],
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{
									"ALL",
								},
							},
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

// labelsForFDeployment returns the labels for selecting the resources
// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/common-labels/
func labelsForFDeployment(name string, image string) map[string]string {
	var imageTag string
	return map[string]string{
		// "app.kubernetes.io/name":     "Memcached",
		"app.kubernetes.io/instance": name,
		"app.kubernetes.io/version":  imageTag,
		// "app.kubernetes.io/part-of":    "f-operator",
		// "app.kubernetes.io/created-by": "controller-manager",
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *FdeploymentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&k8sv1.Fdeployment{}).
		Complete(r)
}
