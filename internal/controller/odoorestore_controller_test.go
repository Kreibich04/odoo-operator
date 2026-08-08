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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	odoov1alpha1 "cloud.alterway.fr/operator/api/v1alpha1"
)

var _ = Describe("OdooRestore Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-restore"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		restore := &odoov1alpha1.OdooRestore{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind OdooRestore")
			err := k8sClient.Get(ctx, typeNamespacedName, restore)
			// Create if not exists
			if err != nil {
				resource := &odoov1alpha1.OdooRestore{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: odoov1alpha1.OdooRestoreSpec{
						OdooRef: corev1.ObjectReference{
							Name: "test-odoo",
						},
						BackupSource: odoov1alpha1.BackupSourceSpec{
							ExternalURL: "s3://bucket/backup.tar.gz",
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &odoov1alpha1.OdooRestore{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				_ = k8sClient.Delete(ctx, resource)
			}
			// Cleanup job
			job := &batchv1.Job{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: resourceName + "-job", Namespace: "default"}, job)
			if err == nil {
				_ = k8sClient.Delete(ctx, job)
			}
		})

		It("should report NotImplemented and create no Job", func() {
			By("Reconciling the created resource")
			controllerReconciler := &OdooRestoreReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify status reflects NotImplemented, not a fake success
			updatedRestore := &odoov1alpha1.OdooRestore{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, updatedRestore)
				if err != nil {
					return false
				}
				if len(updatedRestore.Status.Conditions) == 0 {
					return false
				}
				last := updatedRestore.Status.Conditions[len(updatedRestore.Status.Conditions)-1]
				return last.Type == "Ready" && last.Status == metav1.ConditionFalse && last.Reason == "NotImplemented"
			}, "10s", "1s").Should(BeTrue(), "should report a NotImplemented condition")

			// No Job should ever be created
			job := &batchv1.Job{}
			jobKey := types.NamespacedName{Name: resourceName + "-job", Namespace: "default"}
			Consistently(func() error {
				return k8sClient.Get(ctx, jobKey, job)
			}, "2s", "500ms").ShouldNot(Succeed(), "no restore Job should be created")
		})
	})
})
