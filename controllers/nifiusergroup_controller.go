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
	"emperror.dev/errors"
	"fmt"
	"github.com/rayyno/k8soperator/pkg/clientwrappers/usergroup"
	"github.com/rayyno/k8soperator/pkg/k8sutil"
	"github.com/rayyno/k8soperator/pkg/util"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/record"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/rayyno/k8soperator/api/v1alpha1"
)

var userGroupFinalizer = "nifiusergroups.nifi.orange.com/finalizer"

// NifiUserGroupReconciler reconciles a NifiUserGroup object
type NifiUserGroupReconciler struct {
	client.Client
	Log      logr.Logger
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=nifi.orange.com,resources=nifiusergroups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=nifi.orange.com,resources=nifiusergroups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=nifi.orange.com,resources=nifiusergroups/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the NifiUserGroup object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.7.0/pkg/reconcile
func (r *NifiUserGroupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = r.Log.WithValues("nifiusergroup", req.NamespacedName)

	var err error

	// Fetch the NifiUserGroup instance
	instance := &v1alpha1.NifiUserGroup{}
	if err = r.Client.Get(ctx, req.NamespacedName, instance); err != nil {
		if apierrors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			return Reconciled()
		}
		// Error reading the object - requeue the request.
		return RequeueWithError(r.Log, err.Error(), err)
	}

	var users []*v1alpha1.NifiUser

	for _, userRef := range instance.Spec.UsersRef {
		var user *v1alpha1.NifiUser
		userNamespace := GetUserRefNamespace(instance.Namespace, userRef)

		if user, err = k8sutil.LookupNifiUser(r.Client, userRef.Name, userNamespace); err != nil {

			// This shouldn't trigger anymore, but leaving it here as a safetybelt
			if k8sutil.IsMarkedForDeletion(instance.ObjectMeta) {
				r.Log.Info("User is already gone, there is nothing we can do")
				if err = r.removeFinalizer(ctx, instance); err != nil {
					return RequeueWithError(r.Log, "failed to remove finalizer", err)
				}
				return Reconciled()
			}

			r.Recorder.Event(instance, corev1.EventTypeWarning, "ReferenceUserError",
				fmt.Sprintf("Failed to lookup reference user : %s in %s",
					userRef.Name, userNamespace))

			// the cluster does not exist - should have been caught pre-flight
			return RequeueWithError(r.Log, "failed to lookup referenced user", err)
		}

		// Check if cluster references are the same
		clusterNamespace := GetClusterRefNamespace(instance.Namespace, instance.Spec.ClusterRef)
		if user != nil && (userNamespace != clusterNamespace || user.Spec.ClusterRef.Name != instance.Spec.ClusterRef.Name) {
			r.Recorder.Event(instance, corev1.EventTypeWarning, "ReferenceClusterError",
				fmt.Sprintf("Failed to ensure consistency in cluster referece : %s in %s, with user : %s in %s",
					instance.Spec.ClusterRef.Name, clusterNamespace, userRef.Name, userRef.Namespace))
			return RequeueWithError(
				r.Log,
				"failed to lookup referenced cluster, due to inconsistency",
				errors.New("inconsistent cluster references"))
		}

		users = append(users, user)
	}

	// Get the referenced NifiCluster
	clusterNamespace := GetClusterRefNamespace(instance.Namespace, instance.Spec.ClusterRef)
	var cluster *v1alpha1.NifiCluster
	if cluster, err = k8sutil.LookupNifiCluster(r.Client, instance.Spec.ClusterRef.Name, clusterNamespace); err != nil {
		// This shouldn't trigger anymore, but leaving it here as a safetybelt
		if k8sutil.IsMarkedForDeletion(instance.ObjectMeta) {
			r.Log.Info("Cluster is already gone, there is nothing we can do")
			if err = r.removeFinalizer(ctx, instance); err != nil {
				return RequeueWithError(r.Log, "failed to remove finalizer", err)
			}
			return Reconciled()
		}

		r.Recorder.Event(instance, corev1.EventTypeWarning, "ReferenceClusterError",
			fmt.Sprintf("Failed to lookup reference cluster : %s in %s",
				instance.Spec.ClusterRef.Name, clusterNamespace))

		// the cluster does not exist - should have been caught pre-flight
		return RequeueWithError(r.Log, "failed to lookup referenced cluster", err)
	}

	// Check if marked for deletion and if so run finalizers
	if k8sutil.IsMarkedForDeletion(instance.ObjectMeta) {
		return r.checkFinalizers(ctx, instance, users, cluster)
	}

	r.Recorder.Event(instance, corev1.EventTypeNormal, "Reconciling",
		fmt.Sprintf("Reconciling user group %s", instance.Name))

	// Check if the NiFi user group already exist
	exist, err := usergroup.ExistUserGroup(r.Client, instance, cluster)
	if err != nil {
		return RequeueWithError(r.Log, "failure checking for existing user group", err)
	}

	if !exist {
		r.Recorder.Event(instance, corev1.EventTypeNormal, "Creating",
			fmt.Sprintf("Creating registry client %s", instance.Name))

		// Create NiFi user group
		status, err := usergroup.CreateUserGroup(r.Client, instance, users, cluster)
		if err != nil {
			return RequeueWithError(r.Log, "failure creating user group", err)
		}

		instance.Status = *status
		if err := r.Client.Status().Update(ctx, instance); err != nil {
			return RequeueWithError(r.Log, "failed to update NifiUserGroup status", err)
		}

		r.Recorder.Event(instance, corev1.EventTypeNormal, "Created",
			fmt.Sprintf("Created user group %s", instance.Name))
	}

	// Sync UserGroup resource with NiFi side component
	r.Recorder.Event(instance, corev1.EventTypeNormal, "Synchronizing",
		fmt.Sprintf("Synchronizing user group %s", instance.Name))
	status, err := usergroup.SyncUserGroup(r.Client, instance, users, cluster)
	if err != nil {
		r.Recorder.Event(instance, corev1.EventTypeNormal, "SynchronizingFailed",
			fmt.Sprintf("Synchronizing user group %s failed", instance.Name))
		return RequeueWithError(r.Log, "failed to sync NifiUserGroup", err)
	}

	instance.Status = *status
	if err := r.Client.Status().Update(ctx, instance); err != nil {
		return RequeueWithError(r.Log, "failed to update NifiUserGroup status", err)
	}

	r.Recorder.Event(instance, corev1.EventTypeNormal, "Synchronized",
		fmt.Sprintf("Synchronized user group %s", instance.Name))

	// Ensure NifiCluster label
	if instance, err = r.ensureClusterLabel(ctx, cluster, instance); err != nil {
		return RequeueWithError(r.Log, "failed to ensure NifiCluster label on user group", err)
	}

	// Ensure finalizer for cleanup on deletion
	if !util.StringSliceContains(instance.GetFinalizers(), userGroupFinalizer) {
		r.Log.Info("Adding Finalizer for NifiUserGroup")
		instance.SetFinalizers(append(instance.GetFinalizers(), userGroupFinalizer))
	}

	// Push any changes
	if instance, err = r.updateAndFetchLatest(ctx, instance); err != nil {
		return RequeueWithError(r.Log, "failed to update NifiUserGroup", err)
	}

	r.Recorder.Event(instance, corev1.EventTypeNormal, "Reconciled",
		fmt.Sprintf("Reconciling user group %s", instance.Name))

	r.Log.Info("Ensured User Group")

	return RequeueAfter(time.Duration(15) * time.Second)
}

// SetupWithManager sets up the controller with the Manager.
func (r *NifiUserGroupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.NifiUserGroup{}).
		Complete(r)
}

func (r *NifiUserGroupReconciler) ensureClusterLabel(ctx context.Context, cluster *v1alpha1.NifiCluster,
	userGroup *v1alpha1.NifiUserGroup) (*v1alpha1.NifiUserGroup, error) {

	labels := ApplyClusterRefLabel(cluster, userGroup.GetLabels())
	if !reflect.DeepEqual(labels, userGroup.GetLabels()) {
		userGroup.SetLabels(labels)
		return r.updateAndFetchLatest(ctx, userGroup)
	}
	return userGroup, nil
}

func (r *NifiUserGroupReconciler) updateAndFetchLatest(ctx context.Context,
	userGroup *v1alpha1.NifiUserGroup) (*v1alpha1.NifiUserGroup, error) {

	typeMeta := userGroup.TypeMeta
	err := r.Client.Update(ctx, userGroup)
	if err != nil {
		return nil, err
	}
	userGroup.TypeMeta = typeMeta
	return userGroup, nil
}

func (r *NifiUserGroupReconciler) checkFinalizers(ctx context.Context, userGroup *v1alpha1.NifiUserGroup,
	users []*v1alpha1.NifiUser, cluster *v1alpha1.NifiCluster) (reconcile.Result, error) {

	r.Log.Info("NiFi user group is marked for deletion")
	var err error
	if util.StringSliceContains(userGroup.GetFinalizers(), userGroupFinalizer) {
		if err = r.finalizeNifiNifiUserGroup(userGroup, users, cluster); err != nil {
			return RequeueWithError(r.Log, "failed to finalize nifiusergroup", err)
		}
		if err = r.removeFinalizer(ctx, userGroup); err != nil {
			return RequeueWithError(r.Log, "failed to remove finalizer from kafkatopic", err)
		}
	}
	return Reconciled()
}

func (r *NifiUserGroupReconciler) removeFinalizer(ctx context.Context, userGroup *v1alpha1.NifiUserGroup) error {
	userGroup.SetFinalizers(util.StringSliceRemove(userGroup.GetFinalizers(), userGroupFinalizer))
	_, err := r.updateAndFetchLatest(ctx, userGroup)
	return err
}

func (r *NifiUserGroupReconciler) finalizeNifiNifiUserGroup(
	userGroup *v1alpha1.NifiUserGroup,
	users []*v1alpha1.NifiUser,
	cluster *v1alpha1.NifiCluster) error {

	if err := usergroup.RemoveUserGroup(r.Client, userGroup, users, cluster); err != nil {
		return err
	}

	r.Log.Info("Delete Registry client")

	return nil
}
