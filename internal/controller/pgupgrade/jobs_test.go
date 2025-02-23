// Copyright 2021 - 2023 Crunchy Data Solutions, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package pgupgrade

import (
	"context"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/yaml"

	"github.com/crunchydata/postgres-operator/pkg/apis/postgres-operator.crunchydata.com/v1beta1"
)

// marshalMatches converts actual to YAML and compares that to expected.
func marshalMatches(actual interface{}, expected string) cmp.Comparison {
	b, err := yaml.Marshal(actual)
	if err != nil {
		return func() cmp.Result { return cmp.ResultFromError(err) }
	}
	return cmp.DeepEqual(string(b), strings.Trim(expected, "\t\n")+"\n")
}

func TestGenerateUpgradeJob(t *testing.T) {
	ctx := context.Background()
	reconciler := &PGUpgradeReconciler{}

	upgrade := &v1beta1.PGUpgrade{}
	upgrade.Namespace = "ns1"
	upgrade.Name = "pgu2"
	upgrade.UID = "uid3"
	upgrade.Spec.Image = pointer.StringPtr("img4")
	upgrade.Spec.PostgresClusterName = "pg5"
	upgrade.Spec.FromPostgresVersion = 19
	upgrade.Spec.ToPostgresVersion = 25
	upgrade.Spec.Resources.Requests = corev1.ResourceList{
		corev1.ResourceCPU: resource.MustParse("3.14"),
	}

	startup := &appsv1.StatefulSet{}
	startup.Spec.Template.Spec = corev1.PodSpec{
		Containers: []corev1.Container{{
			Name: ContainerDatabase,

			SecurityContext: &corev1.SecurityContext{Privileged: new(bool)},
			VolumeMounts: []corev1.VolumeMount{
				{Name: "vm1", MountPath: "/mnt/some/such"},
			},
		}},
		Volumes: []corev1.Volume{
			{
				Name: "vol2",
				VolumeSource: corev1.VolumeSource{
					HostPath: new(corev1.HostPathVolumeSource),
				},
			},
		},
	}

	job := reconciler.generateUpgradeJob(ctx, upgrade, startup)
	assert.Assert(t, marshalMatches(job, `
apiVersion: batch/v1
kind: Job
metadata:
  creationTimestamp: null
  labels:
    postgres-operator.crunchydata.com/cluster: pg5
    postgres-operator.crunchydata.com/pgupgrade: pgu2
    postgres-operator.crunchydata.com/role: pgupgrade
    postgres-operator.crunchydata.com/version: "25"
  name: pgu2-pgdata
  namespace: ns1
  ownerReferences:
  - apiVersion: postgres-operator.crunchydata.com/v1beta1
    blockOwnerDeletion: true
    controller: true
    kind: PGUpgrade
    name: pgu2
    uid: uid3
spec:
  backoffLimit: 0
  template:
    metadata:
      creationTimestamp: null
      labels:
        postgres-operator.crunchydata.com/cluster: pg5
        postgres-operator.crunchydata.com/pgupgrade: pgu2
        postgres-operator.crunchydata.com/role: pgupgrade
        postgres-operator.crunchydata.com/version: "25"
    spec:
      containers:
      - command:
        - bash
        - -ceu
        - --
        - |-
          declare -r data_volume='/pgdata' old_version="$1" new_version="$2"
          printf 'Performing PostgreSQL upgrade from version "%s" to "%s" ...\n\n' "$@"
          gid=$(id -G); NSS_WRAPPER_GROUP=$(mktemp)
          (sed "/^postgres:x:/ d; /^[^:]*:x:${gid%% *}:/ d" /etc/group
          echo "postgres:x:${gid%% *}:") > "${NSS_WRAPPER_GROUP}"
          uid=$(id -u); NSS_WRAPPER_PASSWD=$(mktemp)
          (sed "/^postgres:x:/ d; /^[^:]*:x:${uid}:/ d" /etc/passwd
          echo "postgres:x:${uid}:${gid%% *}::${data_volume}:") > "${NSS_WRAPPER_PASSWD}"
          export LD_PRELOAD='libnss_wrapper.so' NSS_WRAPPER_GROUP NSS_WRAPPER_PASSWD
          cd /pgdata || exit
          echo -e "Step 1: Making new pgdata directory...\n"
          mkdir /pgdata/pg"${new_version}"
          echo -e "Step 2: Initializing new pgdata directory...\n"
          /usr/pgsql-"${new_version}"/bin/initdb -k -D /pgdata/pg"${new_version}"
          echo -e "\nStep 3: Setting the expected permissions on the old pgdata directory...\n"
          chmod 700 /pgdata/pg"${old_version}"
          echo -e "Step 4: Copying shared_preload_libraries setting to new postgresql.conf file...\n"
          echo "shared_preload_libraries = '$(/usr/pgsql-"""${old_version}"""/bin/postgres -D \
          /pgdata/pg"""${old_version}""" -C shared_preload_libraries)'" >> /pgdata/pg"${new_version}"/postgresql.conf
          echo -e "Step 5: Running pg_upgrade check...\n"
          time /usr/pgsql-"${new_version}"/bin/pg_upgrade --old-bindir /usr/pgsql-"${old_version}"/bin \
          --new-bindir /usr/pgsql-"${new_version}"/bin --old-datadir /pgdata/pg"${old_version}"\
           --new-datadir /pgdata/pg"${new_version}" --link --check
          echo -e "\nStep 6: Running pg_upgrade...\n"
          time /usr/pgsql-"${new_version}"/bin/pg_upgrade --old-bindir /usr/pgsql-"${old_version}"/bin \
          --new-bindir /usr/pgsql-"${new_version}"/bin --old-datadir /pgdata/pg"${old_version}" \
          --new-datadir /pgdata/pg"${new_version}" --link
          echo -e "\nStep 7: Copying patroni.dynamic.json...\n"
          cp /pgdata/pg"${old_version}"/patroni.dynamic.json /pgdata/pg"${new_version}"
          echo -e "\npg_upgrade Job Complete!"
        - upgrade
        - "19"
        - "25"
        image: img4
        name: database
        resources:
          requests:
            cpu: 3140m
        securityContext:
          privileged: false
        volumeMounts:
        - mountPath: /mnt/some/such
          name: vm1
      restartPolicy: Never
      volumes:
      - hostPath:
          path: ""
        name: vol2
status: {}
	`))
}

func TestGenerateRemoveDataJob(t *testing.T) {
	ctx := context.Background()
	reconciler := &PGUpgradeReconciler{}

	upgrade := &v1beta1.PGUpgrade{}
	upgrade.Namespace = "ns1"
	upgrade.Name = "pgu2"
	upgrade.UID = "uid3"
	upgrade.Spec.Image = pointer.StringPtr("img4")
	upgrade.Spec.PostgresClusterName = "pg5"
	upgrade.Spec.FromPostgresVersion = 19
	upgrade.Spec.ToPostgresVersion = 25
	upgrade.Spec.Resources.Requests = corev1.ResourceList{
		corev1.ResourceCPU: resource.MustParse("3.14"),
	}

	sts := &appsv1.StatefulSet{}
	sts.Name = "sts"
	sts.Spec.Template.Spec = corev1.PodSpec{
		Containers: []corev1.Container{{
			Name:            ContainerDatabase,
			Image:           "img3",
			SecurityContext: &corev1.SecurityContext{Privileged: new(bool)},
			VolumeMounts: []corev1.VolumeMount{
				{Name: "vm1", MountPath: "/mnt/some/such"},
			},
		}},
		Volumes: []corev1.Volume{
			{
				Name: "vol2",
				VolumeSource: corev1.VolumeSource{
					HostPath: new(corev1.HostPathVolumeSource),
				},
			},
		},
	}

	job := reconciler.generateRemoveDataJob(ctx, upgrade, sts)
	assert.Assert(t, marshalMatches(job, `
apiVersion: batch/v1
kind: Job
metadata:
  creationTimestamp: null
  labels:
    postgres-operator.crunchydata.com/cluster: pg5
    postgres-operator.crunchydata.com/pgupgrade: pgu2
    postgres-operator.crunchydata.com/role: removedata
  name: pgu2-sts
  namespace: ns1
  ownerReferences:
  - apiVersion: postgres-operator.crunchydata.com/v1beta1
    blockOwnerDeletion: true
    controller: true
    kind: PGUpgrade
    name: pgu2
    uid: uid3
spec:
  backoffLimit: 0
  template:
    metadata:
      creationTimestamp: null
      labels:
        postgres-operator.crunchydata.com/cluster: pg5
        postgres-operator.crunchydata.com/pgupgrade: pgu2
        postgres-operator.crunchydata.com/role: removedata
    spec:
      containers:
      - command:
        - bash
        - -ceu
        - --
        - |-
          declare -r old_version="$1"
          printf 'Removing PostgreSQL data dir for pg%s...\n\n' "$@"
          echo -e "Checking the directory exists and isn't being used...\n"
          cd /pgdata || exit
          if [ "$(/usr/pgsql-"${old_version}"/bin/pg_controldata /pgdata/pg"${old_version}" | grep -c "shut down in recovery")" -ne 1 ]; then echo -e "Directory in use, cannot remove..."; exit 1; fi
          echo -e "Removing old pgdata directory...\n"
          rm -rf /pgdata/pg"${old_version}" "$(realpath /pgdata/pg${old_version}/pg_wal)"
          echo -e "Remove Data Job Complete!"
        - remove
        - "19"
        image: img4
        name: database
        resources:
          requests:
            cpu: 3140m
        securityContext:
          privileged: false
        volumeMounts:
        - mountPath: /mnt/some/such
          name: vm1
      restartPolicy: Never
      volumes:
      - hostPath:
          path: ""
        name: vol2
status: {}
	`))
}
