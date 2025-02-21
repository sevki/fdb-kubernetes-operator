/*
 * restore_controller_test.go
 *
 * This source file is part of the FoundationDB open source project
 *
 * Copyright 2020 Apple Inc. and the FoundationDB project authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package controllers

import (
	"github.com/FoundationDB/fdb-kubernetes-operator/internal"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"context"

	fdbtypes "github.com/FoundationDB/fdb-kubernetes-operator/api/v1beta1"
	"k8s.io/apimachinery/pkg/types"
)

func reloadRestore(restore *fdbtypes.FoundationDBRestore) error {
	return k8sClient.Get(context.TODO(), types.NamespacedName{Namespace: restore.Namespace, Name: restore.Name}, restore)
}

var _ = Describe("restore_controller", func() {
	var cluster *fdbtypes.FoundationDBCluster
	var restore *fdbtypes.FoundationDBRestore
	var adminClient *mockAdminClient
	var err error

	BeforeEach(func() {
		cluster = internal.CreateDefaultCluster()
		restore = createDefaultRestore(cluster)
		adminClient, err = newMockAdminClientUncast(cluster, k8sClient)
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("Reconciliation", func() {
		BeforeEach(func() {
			err = k8sClient.Create(context.TODO(), cluster)
			Expect(err).NotTo(HaveOccurred())

			result, err := reconcileCluster(cluster)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())

			generation, err := reloadCluster(cluster)
			Expect(err).NotTo(HaveOccurred())
			Expect(generation).NotTo(Equal(0))
			err = k8sClient.Get(context.TODO(), types.NamespacedName{Namespace: cluster.Namespace, Name: cluster.Name}, cluster)
			Expect(err).NotTo(HaveOccurred())

			err = k8sClient.Create(context.TODO(), restore)
			Expect(err).NotTo(HaveOccurred())

			result, err = reconcileRestore(restore)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())

			err = reloadRestore(restore)
			Expect(err).NotTo(HaveOccurred())
			Expect(restore.Status.Running).To(BeTrue())

			err = k8sClient.Get(context.TODO(), types.NamespacedName{Namespace: cluster.Namespace, Name: cluster.Name}, cluster)
			Expect(err).NotTo(HaveOccurred())
		})

		JustBeforeEach(func() {
			result, err := reconcileRestore(restore)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())

			err = reloadRestore(restore)
			Expect(err).NotTo(HaveOccurred())
			err = k8sClient.Get(context.TODO(), types.NamespacedName{Namespace: restore.Namespace, Name: restore.Name}, cluster)
			Expect(err).NotTo(HaveOccurred())
		})

		Context("when reconciling a new restore", func() {
			It("should start a restore", func() {
				status, err := adminClient.GetRestoreStatus()
				Expect(err).NotTo(HaveOccurred())
				Expect(status).To(Equal("blobstore://test@test-service/test-backup?bucket=fdb-backups\n"))
			})
		})

		When("providing custom parameters", func() {
			BeforeEach(func() {
				restore.Spec.CustomParameters = fdbtypes.FoundationDBCustomParameters{
					"knob_http_verbose_level=3",
				}
				err = k8sClient.Update(context.TODO(), restore)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should append the custom parameters to the command", func() {
				Expect(len(adminClient.knobs)).To(BeNumerically("==", 1))
				Expect(adminClient.knobs).To(ContainElements("--knob_http_verbose_level=3"))
			})
		})
	})
})
