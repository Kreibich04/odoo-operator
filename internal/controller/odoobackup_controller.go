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
	"fmt"
	"path"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	odoov1alpha1 "cloud.alterway.fr/operator/api/v1alpha1"
)

// OdooBackupReconciler reconciles a OdooBackup object
type OdooBackupReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// The controller never creates/deletes OdooBackup CRs themselves (only humans do, via the
// odoo_admin_role.yaml/odoo_editor_role.yaml personas), never Updates the base resource (only
// its status), and never touches PersistentVolumeClaims directly (it only references a PVC name
// in a Job's volume spec).
// +kubebuilder:rbac:groups=cloud.alterway.fr,resources=odoobackups,verbs=get;list;watch
// +kubebuilder:rbac:groups=cloud.alterway.fr,resources=odoobackups/status,verbs=get;update
// +kubebuilder:rbac:groups=cloud.alterway.fr,resources=odoobackups/finalizers,verbs=update
// +kubebuilder:rbac:groups=cloud.alterway.fr,resources=odoos,verbs=get;list;watch
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=create;delete;get;list;watch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch

// setBackupCondition upserts a condition by Type (see upsertCondition), avoiding growing the
// condition list on every reconcile while the backup sits in the same state.
func setBackupCondition(backup *odoov1alpha1.OdooBackup, condition metav1.Condition) bool {
	return upsertCondition(&backup.Status.Conditions, condition)
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *OdooBackupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the OdooBackup instance
	backup := &odoov1alpha1.OdooBackup{}
	err := r.Get(ctx, req.NamespacedName, backup)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			log.Error(err, "unable to fetch OdooBackup")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	if backup.Spec.StorageLocation.PVC == nil {
		if setBackupCondition(backup, metav1.Condition{
			Type:    "Ready",
			Status:  metav1.ConditionFalse,
			Reason:  "NotImplemented",
			Message: "Only storageLocation.pvc is currently supported; storageLocation.s3 is not implemented yet.",
		}) {
			if err := r.Status().Update(ctx, backup); err != nil {
				log.Error(err, "Failed to update OdooBackup status")
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Resolve the target Odoo instance. Cross-namespace odooRef isn't supported: the backup
	// Job (and its SecretKeyRef into the Odoo DB secret) is always created in backup.Namespace,
	// and Secret references can't cross namespaces, so a different odooRef.namespace would only
	// fail confusingly at Job runtime. Refuse explicitly instead.
	odooNamespace := backup.Spec.OdooRef.Namespace
	if odooNamespace == "" {
		odooNamespace = backup.Namespace
	} else if odooNamespace != backup.Namespace {
		if setBackupCondition(backup, metav1.Condition{
			Type:    "Ready",
			Status:  metav1.ConditionFalse,
			Reason:  "NotImplemented",
			Message: fmt.Sprintf("odooRef.namespace (%s) must match the OdooBackup's own namespace (%s); cross-namespace backups are not supported.", odooNamespace, backup.Namespace),
		}) {
			if err := r.Status().Update(ctx, backup); err != nil {
				log.Error(err, "Failed to update OdooBackup status")
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}
	odoo := &odoov1alpha1.Odoo{}
	if err := r.Get(ctx, types.NamespacedName{Name: backup.Spec.OdooRef.Name, Namespace: odooNamespace}, odoo); err != nil {
		log.Error(err, "Failed to get referenced Odoo resource", "Odoo.Name", backup.Spec.OdooRef.Name, "Odoo.Namespace", odooNamespace)
		return ctrl.Result{}, err
	}

	dbHost, secretName, err := resolveDatabaseConnection(odoo)
	if err != nil {
		log.Error(err, "Failed to resolve database connection for referenced Odoo")
		return ctrl.Result{}, err
	}

	// Define the Backup Job
	jobName := backup.Name + "-job"
	job := &batchv1.Job{}
	err = r.Get(ctx, types.NamespacedName{Name: jobName, Namespace: backup.Namespace}, job)

	if err != nil && errors.IsNotFound(err) {
		// Job does not exist, create it
		job = r.jobForBackup(backup, dbHost, secretName, odoo.Spec.Database.PostgresVersion)
		log.Info("Creating a new Backup Job", "Job.Namespace", job.Namespace, "Job.Name", job.Name)
		if err := r.Create(ctx, job); err != nil {
			log.Error(err, "Failed to create Backup Job")
			return ctrl.Result{}, err
		}
		setBackupCondition(backup, metav1.Condition{
			Type:    "Running",
			Status:  metav1.ConditionTrue,
			Reason:  "JobCreated",
			Message: "Backup Job has been created",
		})
		if err := r.Status().Update(ctx, backup); err != nil {
			log.Error(err, "Failed to update OdooBackup status")
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	} else if err != nil {
		log.Error(err, "Failed to get Backup Job")
		return ctrl.Result{}, err
	}

	// Job exists, check status
	if job.Status.Succeeded > 0 {
		if setBackupCondition(backup, metav1.Condition{
			Type:    "Completed",
			Status:  metav1.ConditionTrue,
			Reason:  "JobSucceeded",
			Message: "Backup Job completed successfully",
		}) {
			now := metav1.Now()
			backup.Status.LastBackupTime = &now
			backup.Status.LastBackupName = backup.Name
			if err := r.Status().Update(ctx, backup); err != nil {
				log.Error(err, "Failed to update OdooBackup status")
				return ctrl.Result{}, err
			}
		}
	} else if job.Status.Failed > 0 {
		if setBackupCondition(backup, metav1.Condition{
			Type:    "Completed",
			Status:  metav1.ConditionFalse,
			Reason:  "JobFailed",
			Message: "Backup Job failed; see the Job's pod logs for details. The Job is left in place for inspection.",
		}) {
			if err := r.Status().Update(ctx, backup); err != nil {
				log.Error(err, "Failed to update OdooBackup status")
				return ctrl.Result{}, err
			}
		}
	}

	return ctrl.Result{}, nil
}

// jobForBackup returns a Job object that runs pg_dump against the resolved database
// and writes the dump to the backup PVC.
func (r *OdooBackupReconciler) jobForBackup(backup *odoov1alpha1.OdooBackup, dbHost, secretName, postgresVersion string) *batchv1.Job {
	ls := map[string]string{"app": "odoo-backup", "backup_cr": backup.Name}

	if postgresVersion == "" {
		postgresVersion = "16"
	}

	// path.Clean rooted at "/" can't escape above the root (Go collapses ".." segments there
	// instead of climbing past it), so a spec.storageLocation.pvc.path like "../../etc" can't
	// break out of the backup-storage mount.
	backupDir := "/backup"
	if raw := backup.Spec.StorageLocation.PVC.Path; raw != "" {
		if cleaned := path.Clean("/" + raw); cleaned != "/" {
			backupDir = "/backup" + cleaned
		}
	}
	script := fmt.Sprintf(`set -e
FILE="%s/%s-$(date +%%Y%%m%%d%%H%%M%%S).dump"
mkdir -p "$(dirname "$FILE")"
PGPASSWORD="$POSTGRES_PASSWORD" pg_dump -h "$DB_HOST" -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc -f "$FILE"
`, backupDir, backup.Name)

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      backup.Name + "-job",
			Namespace: backup.Namespace,
			Labels:    ls,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: ls,
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyOnFailure,
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot:   func() *bool { b := true; return &b }(),
						RunAsUser:      func() *int64 { i := int64(999); return &i }(),
						FSGroup:        func() *int64 { i := int64(999); return &i }(),
						SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
					},
					Containers: []corev1.Container{
						{
							Name:    "backup",
							Image:   fmt.Sprintf("postgres:%s", postgresVersion),
							Command: []string{"sh", "-c", script},
							Env: []corev1.EnvVar{
								{Name: "DB_HOST", Value: dbHost},
								{Name: "POSTGRES_USER", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: secretName}, Key: "user"}}},
								{Name: "POSTGRES_PASSWORD", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: secretName}, Key: "password"}}},
								{Name: "POSTGRES_DB", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: secretName}, Key: "dbname"}}},
							},
							SecurityContext: &corev1.SecurityContext{
								AllowPrivilegeEscalation: func() *bool { b := false; return &b }(),
								Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
							},
							VolumeMounts: []corev1.VolumeMount{
								{Name: "backup-storage", MountPath: "/backup"},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "backup-storage",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: backup.Spec.StorageLocation.PVC.ClaimName,
								},
							},
						},
					},
				},
			},
		},
	}
	_ = ctrl.SetControllerReference(backup, job, r.Scheme)
	return job
}

// SetupWithManager sets up the controller with the Manager.
func (r *OdooBackupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&odoov1alpha1.OdooBackup{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}
