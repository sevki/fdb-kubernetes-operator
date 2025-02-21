/*
 * replace_failed_process_groups_test.go
 *
 * This source file is part of the FoundationDB open source project
 *
 * Copyright 2021 Apple Inc. and the FoundationDB project authors
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
	"context"
	"time"

	"github.com/FoundationDB/fdb-kubernetes-operator/internal/replacements"

	"k8s.io/utils/pointer"

	"github.com/FoundationDB/fdb-kubernetes-operator/internal"

	fdbtypes "github.com/FoundationDB/fdb-kubernetes-operator/api/v1beta1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("replace_failed_process_groups", func() {
	var cluster *fdbtypes.FoundationDBCluster
	var err error
	var result bool

	BeforeEach(func() {
		cluster = internal.CreateDefaultCluster()
		err = k8sClient.Create(context.TODO(), cluster)
		Expect(err).NotTo(HaveOccurred())

		result, err := reconcileCluster(cluster)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Requeue).To(BeFalse())

		generation, err := reloadCluster(cluster)
		Expect(err).NotTo(HaveOccurred())
		Expect(generation).To(Equal(int64(1)))
	})

	JustBeforeEach(func() {
		adminClient, err := newMockAdminClientUncast(cluster, k8sClient)
		Expect(err).NotTo(HaveOccurred())
		Expect(adminClient).NotTo(BeNil())
		result = replacements.ReplaceFailedProcessGroups(log, cluster, adminClient)
	})

	Context("with no missing processes", func() {
		It("should return false", func() {
			Expect(result).To(BeFalse())
		})

		It("should not mark anything for removal", func() {
			Expect(getRemovedProcessGroupIDs(cluster)).To(Equal([]string{}))
		})
	})

	Context("with a process that has been missing for a long time", func() {
		BeforeEach(func() {
			processGroup := fdbtypes.FindProcessGroupByID(cluster.Status.ProcessGroups, "storage-2")
			processGroup.ProcessGroupConditions = append(processGroup.ProcessGroupConditions, &fdbtypes.ProcessGroupCondition{
				ProcessGroupConditionType: fdbtypes.MissingProcesses,
				Timestamp:                 time.Now().Add(-1 * time.Hour).Unix(),
			})
		})

		Context("with no other removals", func() {
			It("should return true", func() {
				Expect(result).To(BeTrue())
			})

			It("should mark the process group for removal", func() {
				Expect(getRemovedProcessGroupIDs(cluster)).To(Equal([]string{"storage-2"}))
			})

			It("should not be marked to skip exclusion", func() {
				for _, pg := range cluster.Status.ProcessGroups {
					if pg.ProcessGroupID != "storage-2" {
						continue
					}

					Expect(pg.ExclusionSkipped).To(BeFalse())
				}
			})
		})

		Context("with multiple failed processes", func() {
			BeforeEach(func() {
				processGroup := fdbtypes.FindProcessGroupByID(cluster.Status.ProcessGroups, "storage-3")
				processGroup.ProcessGroupConditions = append(processGroup.ProcessGroupConditions, &fdbtypes.ProcessGroupCondition{
					ProcessGroupConditionType: fdbtypes.MissingProcesses,
					Timestamp:                 time.Now().Add(-1 * time.Hour).Unix(),
				})
			})

			It("should return true", func() {
				Expect(result).To(BeTrue())
			})

			It("should mark the first process group for removal", func() {
				Expect(getRemovedProcessGroupIDs(cluster)).To(Equal([]string{"storage-2"}))
			})

			It("should not be marked to skip exclusion", func() {
				for _, pg := range cluster.Status.ProcessGroups {
					if pg.ProcessGroupID != "storage-2" {
						continue
					}

					Expect(pg.ExclusionSkipped).To(BeFalse())
				}
			})
		})

		Context("with another in-flight exclusion", func() {
			BeforeEach(func() {
				processGroup := fdbtypes.FindProcessGroupByID(cluster.Status.ProcessGroups, "storage-3")
				processGroup.MarkForRemoval()
			})

			It("should return false", func() {
				Expect(result).To(BeFalse())
			})

			It("should not mark the process group for removal", func() {
				Expect(getRemovedProcessGroupIDs(cluster)).To(Equal([]string{"storage-3"}))
			})

			When("max concurrent replacements is set to two", func() {
				BeforeEach(func() {
					replacements := 2
					cluster.Spec.AutomationOptions.Replacements.MaxConcurrentReplacements = &replacements
				})

				It("should return true", func() {
					Expect(result).To(BeTrue())
				})

				It("should mark the process group for removal", func() {
					Expect(getRemovedProcessGroupIDs(cluster)).To(Equal([]string{"storage-2", "storage-3"}))
				})
			})

			When("max concurrent replacements is set to zero", func() {
				BeforeEach(func() {
					replacements := 0
					cluster.Spec.AutomationOptions.Replacements.MaxConcurrentReplacements = &replacements
				})

				It("should return false", func() {
					Expect(result).To(BeFalse())
				})

				It("should not mark the process group for removal", func() {
					Expect(getRemovedProcessGroupIDs(cluster)).To(Equal([]string{"storage-3"}))
				})
			})
		})

		Context("with another complete exclusion", func() {
			BeforeEach(func() {
				processGroup := fdbtypes.FindProcessGroupByID(cluster.Status.ProcessGroups, "storage-3")
				processGroup.MarkForRemoval()
				processGroup.SetExclude()
			})

			It("should return true", func() {
				Expect(result).To(BeTrue())
			})

			It("should mark the process group for removal", func() {
				Expect(getRemovedProcessGroupIDs(cluster)).To(Equal([]string{"storage-2", "storage-3"}))
			})
		})

		Context("with no addresses", func() {
			BeforeEach(func() {
				processGroup := fdbtypes.FindProcessGroupByID(cluster.Status.ProcessGroups, "storage-2")
				processGroup.Addresses = nil
			})

			It("should return true", func() {
				Expect(result).To(BeTrue())
			})

			It("should mark the process group for removal", func() {
				Expect(getRemovedProcessGroupIDs(cluster)).To(Equal([]string{"storage-2"}))
			})

			It("should marked to skip exclusion", func() {
				for _, pg := range cluster.Status.ProcessGroups {
					if pg.ProcessGroupID != "storage-2" {
						continue
					}

					Expect(pg.ExclusionSkipped).To(BeTrue())
				}
			})

			When("the cluster is not available", func() {
				BeforeEach(func() {
					processGroup := fdbtypes.FindProcessGroupByID(cluster.Status.ProcessGroups, "storage-2")
					processGroup.Addresses = nil

					adminClient, err := newMockAdminClientUncast(cluster, k8sClient)
					Expect(err).NotTo(HaveOccurred())
					adminClient.frozenStatus = &fdbtypes.FoundationDBStatus{
						Client: fdbtypes.FoundationDBStatusLocalClientInfo{
							DatabaseStatus: fdbtypes.FoundationDBStatusClientDBStatus{
								Available: false,
							},
						},
					}
				})

				It("should return false", func() {
					Expect(result).To(BeFalse())
				})

				It("should not mark the process group for removal", func() {
					Expect(getRemovedProcessGroupIDs(cluster)).To(Equal([]string{}))
				})
			})

			When("the cluster doesn't have full fault tolerance", func() {
				BeforeEach(func() {
					processGroup := fdbtypes.FindProcessGroupByID(cluster.Status.ProcessGroups, "storage-2")
					processGroup.Addresses = nil

					adminClient, err := newMockAdminClientUncast(cluster, k8sClient)
					Expect(err).NotTo(HaveOccurred())
					adminClient.maxZoneFailuresWithoutLosingData = pointer.Int(0)
				})

				It("should return false", func() {
					Expect(result).To(BeFalse())
				})

				It("should not mark the process group for removal", func() {
					Expect(getRemovedProcessGroupIDs(cluster)).To(Equal([]string{}))
				})
			})
		})
	})

	Context("with a process that has been missing for a brief time", func() {
		BeforeEach(func() {
			processGroup := fdbtypes.FindProcessGroupByID(cluster.Status.ProcessGroups, "storage-2")
			processGroup.ProcessGroupConditions = append(processGroup.ProcessGroupConditions, &fdbtypes.ProcessGroupCondition{
				ProcessGroupConditionType: fdbtypes.MissingProcesses,
				Timestamp:                 time.Now().Unix(),
			})
		})

		It("should return false", func() {
			Expect(result).To(BeFalse())
		})

		It("should not mark the process group for removal", func() {
			Expect(getRemovedProcessGroupIDs(cluster)).To(Equal([]string{}))
		})
	})

	Context("with a process that has had an incorrect pod spec for a long time", func() {
		BeforeEach(func() {
			processGroup := fdbtypes.FindProcessGroupByID(cluster.Status.ProcessGroups, "storage-2")
			processGroup.ProcessGroupConditions = append(processGroup.ProcessGroupConditions, &fdbtypes.ProcessGroupCondition{
				ProcessGroupConditionType: fdbtypes.IncorrectPodSpec,
				Timestamp:                 time.Now().Add(-1 * time.Hour).Unix(),
			})
		})

		It("should return false", func() {
			Expect(result).To(BeFalse())
		})

		It("should not mark the process group for removal", func() {
			Expect(getRemovedProcessGroupIDs(cluster)).To(Equal([]string{}))
		})
	})
})

// getRemovedProcessGroupIDs returns a list of ids for the process groups that
// are marked for removal.
func getRemovedProcessGroupIDs(cluster *fdbtypes.FoundationDBCluster) []string {
	results := make([]string, 0)
	for _, processGroupStatus := range cluster.Status.ProcessGroups {
		if processGroupStatus.IsMarkedForRemoval() {
			results = append(results, processGroupStatus.ProcessGroupID)
		}
	}
	return results
}
