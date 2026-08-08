/*
Copyright 2025.

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

package controller

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	odoov1alpha1 "cloud.alterway.fr/operator/api/v1alpha1"
)

// OdooRestoreReconciler reconciles a OdooRestore object
type OdooRestoreReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// Reconcile only reads the OdooRestore CR and writes its status -- it creates no Job/PVC/Secret
// (see the NotImplemented rewrite above), so those markers were removed.
// +kubebuilder:rbac:groups=cloud.alterway.fr,resources=odoorestores,verbs=get;list;watch
// +kubebuilder:rbac:groups=cloud.alterway.fr,resources=odoorestores/status,verbs=get;update
// +kubebuilder:rbac:groups=cloud.alterway.fr,resources=odoorestores/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// OdooRestore is not implemented yet: restoring a database/PVC in place (StopAndRestore)
// or spinning up a new PVC and repointing the StatefulSet at it (NewPVC) both need
// careful coordination with the Odoo StatefulSet that this controller doesn't have.
// Previously this reconciled a dummy busybox Job that reported Completed without
// restoring anything, which is worse than doing nothing: it looked like it worked.
// Report the CR as explicitly unsupported instead.
func (r *OdooRestoreReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	restore := &odoov1alpha1.OdooRestore{}
	err := r.Get(ctx, req.NamespacedName, restore)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			log.Error(err, "unable to fetch OdooRestore")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	changed := upsertCondition(&restore.Status.Conditions, metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionFalse,
		Reason:  "NotImplemented",
		Message: "OdooRestore is not implemented yet: neither the StopAndRestore nor the NewPVC restoreMethod restores any data. No Job is created.",
	})
	if changed {
		if err := r.Status().Update(ctx, restore); err != nil {
			log.Error(err, "Failed to update OdooRestore status")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *OdooRestoreReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&odoov1alpha1.OdooRestore{}).
		Complete(r)
}
