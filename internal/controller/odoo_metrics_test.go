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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	odoov1alpha1 "cloud.alterway.fr/operator/api/v1alpha1"
)

func TestStatefulSetForPostgres_ExporterSidecar(t *testing.T) {
	r := &OdooReconciler{Scheme: runtime.NewScheme()}
	odoo := &odoov1alpha1.Odoo{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec:       odoov1alpha1.OdooSpec{Size: 1},
	}

	withoutExporter := r.statefulSetForPostgres(odoo, "test-postgres-secret")
	if len(withoutExporter.Spec.Template.Spec.Containers) != 1 {
		t.Fatalf("expected only the postgres container when Metrics.Postgres is false, got %d containers", len(withoutExporter.Spec.Template.Spec.Containers))
	}

	odoo.Spec.Metrics.Postgres = true
	withExporter := r.statefulSetForPostgres(odoo, "test-postgres-secret")
	containers := withExporter.Spec.Template.Spec.Containers
	if len(containers) != 2 {
		t.Fatalf("expected postgres + postgres-exporter containers when Metrics.Postgres is true, got %d", len(containers))
	}
	exporter := containers[1]
	if exporter.Name != "postgres-exporter" {
		t.Fatalf("expected second container to be postgres-exporter, got %q", exporter.Name)
	}
	if len(exporter.Ports) != 1 || exporter.Ports[0].Name != "metrics" || exporter.Ports[0].ContainerPort != 9187 {
		t.Fatalf("expected a single 'metrics' port 9187, got %+v", exporter.Ports)
	}
}

func TestStatefulSetForRedis_ExporterSidecar(t *testing.T) {
	r := &OdooReconciler{Scheme: runtime.NewScheme()}
	odoo := &odoov1alpha1.Odoo{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec:       odoov1alpha1.OdooSpec{Size: 1, Redis: odoov1alpha1.RedisSpec{Enabled: true}},
	}

	withoutExporter := r.statefulSetForRedis(odoo, "test-redis-secret")
	if len(withoutExporter.Spec.Template.Spec.Containers) != 1 {
		t.Fatalf("expected only the redis container when Metrics.Redis is false, got %d containers", len(withoutExporter.Spec.Template.Spec.Containers))
	}

	odoo.Spec.Metrics.Redis = true
	withExporter := r.statefulSetForRedis(odoo, "test-redis-secret")
	containers := withExporter.Spec.Template.Spec.Containers
	if len(containers) != 2 {
		t.Fatalf("expected redis + redis-exporter containers when Metrics.Redis is true, got %d", len(containers))
	}
	exporter := containers[1]
	if exporter.Name != "redis-exporter" {
		t.Fatalf("expected second container to be redis-exporter, got %q", exporter.Name)
	}
	if len(exporter.Ports) != 1 || exporter.Ports[0].Name != "metrics" || exporter.Ports[0].ContainerPort != 9121 {
		t.Fatalf("expected a single 'metrics' port 9121, got %+v", exporter.Ports)
	}
}
