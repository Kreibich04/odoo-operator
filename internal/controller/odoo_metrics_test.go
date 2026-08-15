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
	if len(exporter.Ports) != 1 || exporter.Ports[0].Name != metricsPortName || exporter.Ports[0].ContainerPort != 9187 {
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
	if len(exporter.Ports) != 1 || exporter.Ports[0].Name != metricsPortName || exporter.Ports[0].ContainerPort != 9121 {
		t.Fatalf("expected a single 'metrics' port 9121, got %+v", exporter.Ports)
	}
}

func TestEffectiveModules_FoldsInExporterModuleOnlyWhenEnabled(t *testing.T) {
	odoo := &odoov1alpha1.Odoo{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: odoov1alpha1.OdooSpec{
			Size:    1,
			Modules: odoov1alpha1.ModulesSpec{Install: []string{"sale", "purchase"}},
		},
	}

	without := effectiveModules(odoo)
	if len(without.Install) != 2 {
		t.Fatalf("expected the original 2-module install list unchanged, got %v", without.Install)
	}

	odoo.Spec.Metrics.Odoo = true
	with := effectiveModules(odoo)
	if len(with.Install) != 3 || with.Install[2] != metricsExporterModuleName {
		t.Fatalf("expected %q appended to the install list, got %v", metricsExporterModuleName, with.Install)
	}
	// original CR spec must not be mutated by effectiveModules
	if len(odoo.Spec.Modules.Install) != 2 {
		t.Fatalf("effectiveModules must not mutate the stored CR spec, got %v", odoo.Spec.Modules.Install)
	}

	// idempotent: calling twice (e.g. once for hash, once for the install command) doesn't
	// duplicate the module if it's already present.
	again := effectiveModules(odoo)
	if len(again.Install) != 3 {
		t.Fatalf("expected effectiveModules to be idempotent, got %v", again.Install)
	}
}

func TestStatefulSetForOdoo_MetricsPort(t *testing.T) {
	r := &OdooReconciler{Scheme: runtime.NewScheme()}
	odoo := &odoov1alpha1.Odoo{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec:       odoov1alpha1.OdooSpec{Size: 1},
	}

	without := r.statefulSetForOdoo(odoo, "postgres-host", "test-postgres-secret", "confighash")
	for _, p := range without.Spec.Template.Spec.Containers[0].Ports {
		if p.Name == metricsPortName {
			t.Fatalf("did not expect a 'metrics' port when Metrics.Odoo is false, got %+v", without.Spec.Template.Spec.Containers[0].Ports)
		}
	}

	odoo.Spec.Metrics.Odoo = true
	with := r.statefulSetForOdoo(odoo, "postgres-host", "test-postgres-secret", "confighash")
	found := false
	for _, p := range with.Spec.Template.Spec.Containers[0].Ports {
		if p.Name == metricsPortName && p.ContainerPort == 8069 {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a 'metrics' port on 8069 when Metrics.Odoo is true, got %+v", with.Spec.Template.Spec.Containers[0].Ports)
	}
}

func TestConfigMapForOdoo_MetricsAddonsPath(t *testing.T) {
	r := &OdooReconciler{Scheme: runtime.NewScheme()}
	odoo := &odoov1alpha1.Odoo{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec:       odoov1alpha1.OdooSpec{Size: 1, Metrics: odoov1alpha1.MetricsSpec{Odoo: true}},
	}

	cm := r.configMapForOdoo(odoo, "db-host", "", 0, "", "masterkey")
	wantPath := "/mnt/extra-addons/" + metricsExporterRepoName
	if !strings.Contains(cm.Data["odoo.conf"], wantPath) {
		t.Fatalf("expected addons_path to contain %q, got:\n%s", wantPath, cm.Data["odoo.conf"])
	}
}

func TestJobForAddonsDownload_PythonDepsInstallStep(t *testing.T) {
	r := &OdooReconciler{Scheme: runtime.NewScheme()}
	odoo := &odoov1alpha1.Odoo{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec:       odoov1alpha1.OdooSpec{Size: 1, Version: "19.0"},
	}
	repos := []odoov1alpha1.GitRepositorySpec{{Name: metricsExporterRepoName, URL: metricsExporterRepoURL, Version: "19.0"}}

	job := r.jobForAddonsDownload(odoo, repos, nil)
	spec := job.Spec.Template.Spec

	if len(spec.InitContainers) != 1 || spec.InitContainers[0].Name != "git-clone" {
		t.Fatalf("expected git-clone to run as the (sole) init container, got %+v", spec.InitContainers)
	}
	if len(spec.Containers) != 1 || spec.Containers[0].Name != "install-python-deps" {
		t.Fatalf("expected install-python-deps as the main container, got %+v", spec.Containers)
	}
	pyDeps := spec.Containers[0]
	if pyDeps.Image != "odoo:19.0" {
		t.Fatalf("expected install-python-deps to use the same odoo:<version> image, got %q", pyDeps.Image)
	}
	if !strings.Contains(pyDeps.Command[len(pyDeps.Command)-1], pythonDepsTargetDir) {
		t.Fatalf("expected the install script to reference %q", pythonDepsTargetDir)
	}
}
