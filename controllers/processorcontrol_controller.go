/*
Copyright 2020.

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

    "github.com/Orange-OpenSource/nifikop/pkg/k8sutil"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/record"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/Orange-OpenSource/nifikop/api/v1alpha1"
)

// var processorcontrolFinalizer = "processorcontrols.nifi.orange.com/finalizer"

// ProcessorControlReconciler reconciles a ProcessorControl object
type ProcessorControlReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
	Recorder record.EventRecorder
}

//+kubebuilder:rbac:groups=nifi.orange.com,resources=processorcontrols,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=nifi.orange.com,resources=processorcontrols/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=nifi.orange.com,resources=processorcontrols/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the ProcessorControl object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.7.2/pkg/reconcile
func (r *ProcessorControlReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = r.Log.WithValues("processorcontrol", req.NamespacedName)

	// your logic here //////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

	var err error

	// Fetch the Nifi instance
	instance := &v1alpha1.ProcessorControl{}
	if err = r.Client.Get(ctx, req.NamespacedName, instance); err != nil {
		if apierrors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			return Reconciled()
		}
		// Error reading the object - requeue the request.
		return RequeueWithError(r.Log, err.Error(), err)
	}

// 	// Ensure finalizer for cleanup on deletion
// 	if !util.StringSliceContains(instance.GetFinalizers(), processorcontrolFinalizer) {
// 		r.Log.Info("Adding Finalizer for ProcessorControl")
// 		instance.SetFinalizers(append(instance.GetFinalizers(), processorcontrolFinalizer))
// 	}

	// Push any changes
	if instance, err = r.updateAndFetchLatest(ctx, instance); err != nil {
		return RequeueWithError(r.Log, "failed to update ProcessorControl", err)
	}

	var cluster *v1alpha1.NifiCluster
	if cluster, err = k8sutil.LookupNifiCluster(r.Client, instance.Spec.ClusterRef.Name, clusterNamespace); err != nil {
		// This shouldn't trigger anymore, but leaving it here as a safetybelt
// 		if k8sutil.IsMarkedForDeletion(instance.ObjectMeta) {
// 			r.Log.Info("Cluster is already gone, there is nothing we can do")
// 			if err = r.removeFinalizer(ctx, instance); err != nil {
// 				return RequeueWithError(r.Log, "failed to remove finalizer", err)
// 			}
// 			return Reconciled()
// 		}

		// the cluster does not exist - should have been caught pre-flight
		return RequeueWithError(r.Log, "failed to lookup referenced cluster", err)
	}

	// Check if marked for deletion and if so run finalizers
// 	if k8sutil.IsMarkedForDeletion(instance.ObjectMeta) {
// 		return r.checkFinalizers(ctx, instance, cluster)
// 	}

	// if the processor is running then reconciled
	if instance.Status.State == v1alpha1.ProcessorControlStateRan {
		return Reconciled()
	}

	r.Recorder.Event(instance, corev1.EventTypeWarning, "Reconciling", fmt.Sprintf("Reconciling LOG TEST THIS IS A TEST!!!!"))

	// Check if the processors already exist
	existing, err := processorcontrol.ProcessorExist(r.Client, instance, cluster)
	if err != nil {
		return RequeueWithError(r.Log, "failure checking for existing processors", err)
	}

	// Schedule the processor
	if instance.Status.State == v1alpha1.ProcessorControlStateStopped {
		instance.Status.State = v1alpha1.ProcessorControlStateStarting
		if err := r.Client.Status().Update(ctx, instance); err != nil {
			return RequeueWithError(r.Log, "failed to update ProcessorControl status", err)
		}

		r.Recorder.Event(instance, corev1.EventTypeNormal, "Starting",
			fmt.Sprintf("Starting processor with uuid %s ", instance.Spec.ProcessorControlID))

		if err := processorcontrol.ScheduleProcessor(r.Client, instance, cluster); err != nil {
		r.Recorder.Event(instance, corev1.EventTypeWarning, "StartingFailed",
		fmt.Sprintf("Starting processor with uuid %s failed.", instance.Spec.ProcessorControlID))
		return RequeueWithError(r.Log, "failed to run Processor", err)
		}

		instance.Status.State = v1alpha1.ProcessorControlStateRan
		if err := r.Client.Status().Update(ctx, instance); err != nil {
			return RequeueWithError(r.Log, "failed to update ProcessorControl status", err)
		}

		r.Recorder.Event(instance, corev1.EventTypeNormal, "Ran",
			fmt.Sprintf("Ran processor with uuid %s", instance.Spec.ProcessorControlID))
	}


	/////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

	//return ctrl.Result{}, nil
	return RequeueAfter(time.Duration(5) * time.Second)
}

// SetupWithManager sets up the controller with the Manager.
func (r *ProcessorControlReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.ProcessorControl{}).
		Complete(r)
}
