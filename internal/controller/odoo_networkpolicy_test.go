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
	"testing"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	odoov1alpha1 "cloud.alterway.fr/operator/api/v1alpha1"
)

func networkPolicyNames(policies []*networkingv1.NetworkPolicy) map[string]bool {
	names := make(map[string]bool, len(policies))
	for _, np := range policies {
		names[np.Name] = true
	}
	return names
}

func TestNetworkPoliciesForOdoo_ManagedPostgresAndRedis(t *testing.T) {
	r := &OdooReconciler{Scheme: runtime.NewScheme()}
	odoo := &odoov1alpha1.Odoo{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: odoov1alpha1.OdooSpec{
			Size:  1,
			Redis: odoov1alpha1.RedisSpec{Enabled: true},
		},
	}

	policies := r.networkPoliciesForOdoo(odoo)
	names := networkPolicyNames(policies)

	if !names["test-odoo-netpol"] || !names["test-postgres-netpol"] || !names["test-redis-netpol"] {
		t.Fatalf("expected odoo+postgres+redis policies for managed DB and managed+enabled Redis, got %v", names)
	}
	if len(policies) != 3 {
		t.Fatalf("expected exactly 3 policies, got %d", len(policies))
	}
}

func TestNetworkPoliciesForOdoo_ExternalDatabaseSkipsPostgresPolicy(t *testing.T) {
	r := &OdooReconciler{Scheme: runtime.NewScheme()}
	odoo := &odoov1alpha1.Odoo{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: odoov1alpha1.OdooSpec{
			Size:               1,
			Database:           odoov1alpha1.DatabaseSpec{Host: "external-db.example.com"},
			DatabaseSecretName: "external-db-secret",
		},
	}

	policies := r.networkPoliciesForOdoo(odoo)
	names := networkPolicyNames(policies)

	if names["test-postgres-netpol"] {
		t.Fatalf("expected no Postgres policy for an external database, got %v", names)
	}
	if !names["test-odoo-netpol"] {
		t.Fatalf("expected the Odoo policy regardless of database mode, got %v", names)
	}
}

func TestNetworkPoliciesForOdoo_RedisDisabledOrUnmanagedSkipsRedisPolicy(t *testing.T) {
	r := &OdooReconciler{Scheme: runtime.NewScheme()}

	// Redis disabled entirely.
	odoo := &odoov1alpha1.Odoo{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec:       odoov1alpha1.OdooSpec{Size: 1},
	}
	if names := networkPolicyNames(r.networkPoliciesForOdoo(odoo)); names["test-redis-netpol"] {
		t.Fatalf("expected no Redis policy when Redis is disabled, got %v", names)
	}

	// Redis enabled but externally managed (Managed: false).
	unmanaged := false
	odoo.Spec.Redis = odoov1alpha1.RedisSpec{Enabled: true, Managed: &unmanaged, Host: "external-redis.example.com"}
	if names := networkPolicyNames(r.networkPoliciesForOdoo(odoo)); names["test-redis-netpol"] {
		t.Fatalf("expected no Redis policy for an externally managed Redis, got %v", names)
	}
}

func TestNetworkPoliciesForOdoo_IngressNamespaceSelectorAddsExtraPeer(t *testing.T) {
	r := &OdooReconciler{Scheme: runtime.NewScheme()}
	selector := &metav1.LabelSelector{MatchLabels: map[string]string{"kubernetes.io/metadata.name": "ingress-nginx"}}
	odoo := &odoov1alpha1.Odoo{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: odoov1alpha1.OdooSpec{
			Size:          1,
			NetworkPolicy: odoov1alpha1.NetworkPolicySpec{Enabled: true, IngressNamespaceSelector: selector},
		},
	}

	policies := r.networkPoliciesForOdoo(odoo)
	var odooPolicy *networkingv1.NetworkPolicy
	for _, np := range policies {
		if np.Name == "test-odoo-netpol" {
			odooPolicy = np
		}
	}
	if odooPolicy == nil {
		t.Fatal("expected an odoo-netpol policy")
	}
	peers := odooPolicy.Spec.Ingress[0].From
	if len(peers) != 2 {
		t.Fatalf("expected same-namespace peer + namespace-selector peer, got %d peers", len(peers))
	}
	if peers[1].NamespaceSelector == nil {
		t.Fatalf("expected the second peer to carry the configured namespace selector")
	}
}
