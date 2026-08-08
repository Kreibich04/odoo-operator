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
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	odoov1alpha1 "cloud.alterway.fr/operator/api/v1alpha1"
)

func newTestReconciler(t *testing.T) *OdooReconciler {
	t.Helper()
	s := runtime.NewScheme()
	if err := odoov1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("failed to build scheme: %v", err)
	}
	return &OdooReconciler{Scheme: s}
}

func TestConfigMapForOdooBlocksDatabaseManagerByDefault(t *testing.T) {
	r := newTestReconciler(t)
	odoo := &odoov1alpha1.Odoo{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
	}

	cm := r.configMapForOdoo(odoo, "db-host", "", 0, "", "masterkey")

	if !strings.Contains(cm.Data["odoo.conf"], "list_db = false") {
		t.Fatalf("expected list_db = false by default, got:\n%s", cm.Data["odoo.conf"])
	}
}

func TestConfigMapForOdooAllowsDatabaseManagerWhenOverridden(t *testing.T) {
	r := newTestReconciler(t)
	odoo := &odoov1alpha1.Odoo{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec:       odoov1alpha1.OdooSpec{AllowDatabaseManager: true},
	}

	cm := r.configMapForOdoo(odoo, "db-host", "", 0, "", "masterkey")

	if !strings.Contains(cm.Data["odoo.conf"], "list_db = true") {
		t.Fatalf("expected list_db = true when AllowDatabaseManager is set, got:\n%s", cm.Data["odoo.conf"])
	}
}
