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

package e2e

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/openshift/kueue-operator/test/e2e/testutils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kueuev1beta2 "sigs.k8s.io/kueue/apis/kueue/v1beta2"
)

var _ = Describe("Cohort", Label("operator", "cohort"), Ordered, func() {

	It("should create an explicit Cohort CR with resourceGroups", Label("D2"), func(ctx context.Context) {
		By("Creating a ResourceFlavor")
		rf, cleanupRF, err := testutils.NewResourceFlavor().WithGenerateName().CreateWithObject(ctx, clients.UpstreamKueueClient)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(cleanupRF)

		By("Creating an explicit Cohort with resourceGroups")
		cohort, cleanupCohort, err := testutils.NewCohort().WithGenerateName().
			WithResourceGroups([]kueuev1beta2.ResourceGroup{{
				CoveredResources: []corev1.ResourceName{"cpu", "memory"},
				Flavors: []kueuev1beta2.FlavorQuotas{{
					Name: kueuev1beta2.ResourceFlavorReference(rf.Name),
					Resources: []kueuev1beta2.ResourceQuota{
						{Name: "cpu", NominalQuota: resource.MustParse("1")},
						{Name: "memory", NominalQuota: resource.MustParse("1Gi")},
					},
				}},
			}}).
			CreateWithObject(ctx, clients.UpstreamKueueClient)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(cleanupCohort)

		By("Verifying the Cohort CR exists and has the correct spec")
		fetched, err := clients.UpstreamKueueClient.KueueV1beta2().Cohorts().Get(ctx, cohort.Name, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(fetched.Spec.ResourceGroups).To(HaveLen(1))
		Expect(fetched.Spec.ResourceGroups[0].CoveredResources).To(ConsistOf(
			corev1.ResourceName("cpu"),
			corev1.ResourceName("memory"),
		))
		Expect(fetched.Spec.ResourceGroups[0].Flavors).To(HaveLen(1))
		Expect(fetched.Spec.ResourceGroups[0].Flavors[0].Resources).To(HaveLen(2))
	})

	It("should allow a child Cohort ClusterQueue to borrow from parent Cohort", Label("D4"), func(ctx context.Context) {
		By("Creating a ResourceFlavor")
		rf, cleanupRF, err := testutils.NewResourceFlavor().WithGenerateName().CreateWithObject(ctx, clients.UpstreamKueueClient)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(cleanupRF)

		By("Creating a parent Cohort with a shared resource pool")
		parentCohort, cleanupParent, err := testutils.NewCohort().WithGenerateName().
			WithResourceGroups([]kueuev1beta2.ResourceGroup{{
				CoveredResources: []corev1.ResourceName{"cpu", "memory"},
				Flavors: []kueuev1beta2.FlavorQuotas{{
					Name: kueuev1beta2.ResourceFlavorReference(rf.Name),
					Resources: []kueuev1beta2.ResourceQuota{
						{Name: "cpu", NominalQuota: resource.MustParse("2")},
						{Name: "memory", NominalQuota: resource.MustParse("2Gi")},
					},
				}},
			}}).
			CreateWithObject(ctx, clients.UpstreamKueueClient)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(cleanupParent)

		By("Creating a child Cohort that references the parent")
		childCohort, cleanupChild, err := testutils.NewCohort().WithGenerateName().
			WithParentName(parentCohort.Name).
			CreateWithObject(ctx, clients.UpstreamKueueClient)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(cleanupChild)

		By("Creating a ClusterQueue with nominalQuota 0 in the child Cohort")
		cq, cleanupCQ, err := testutils.NewClusterQueue().WithGenerateName().
			WithFlavorName(rf.Name).
			WithCPU("0").
			WithMemory("0").
			WithCohort(childCohort.Name).
			CreateWithObject(ctx, clients.UpstreamKueueClient)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(cleanupCQ)

		By("Creating a Namespace and LocalQueue")
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "cohort-hier-",
				Labels:       map[string]string{testutils.OpenShiftManagedLabel: "true"},
			},
		}
		cleanupNs, err := testutils.CreateNamespace(kubeClient, ns)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(cleanupNs)

		lq, cleanupLQ, err := testutils.NewLocalQueue(ns.Name, "hier-lq").
			WithClusterQueue(cq.Name).
			CreateWithObject(ctx, clients.UpstreamKueueClient)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(cleanupLQ)

		By("Creating a Job that borrows from the parent Cohort through the hierarchy")
		builder := testutils.NewTestResourceBuilder(ns.Name, lq.Name)
		job := builder.NewJob()
		job.Labels[testutils.QueueLabel] = lq.Name
		job.Spec.Template.Spec.Containers[0].Resources.Requests = corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("500m"),
			corev1.ResourceMemory: resource.MustParse("256Mi"),
		}
		job.Spec.Template.Spec.Containers[0].Command = []string{"sh", "-c", "echo hierarchical borrow; sleep 60"}
		createdJob, err := kubeClient.BatchV1().Jobs(ns.Name).Create(ctx, job, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			testutils.CleanUpJob(ctx, kubeClient, ns.Name, createdJob.Name)
		})

		By("Verifying the workload is admitted (resources borrowed from parent Cohort)")
		checkWorkloadCondition(ctx, ns.Name, string(createdJob.UID), kueuev1beta2.WorkloadAdmitted, "hierarchical-borrow")

		By("Verifying the ClusterQueue shows borrowed resources")
		Eventually(func() error {
			fetchedCQ, err := clients.UpstreamKueueClient.KueueV1beta2().ClusterQueues().Get(ctx, cq.Name, metav1.GetOptions{})
			if err != nil {
				return err
			}
			for _, flavorUsage := range fetchedCQ.Status.FlavorsUsage {
				for _, resourceUsage := range flavorUsage.Resources {
					if resourceUsage.Name == corev1.ResourceCPU {
						if resourceUsage.Borrowed.Cmp(resource.MustParse("500m")) >= 0 {
							return nil
						}
						return fmt.Errorf("expected borrowed CPU >= 500m, got %s", resourceUsage.Borrowed.String())
					}
				}
			}
			return fmt.Errorf("CPU resource not found in ClusterQueue status")
		}, testutils.OperatorReadyTime, testutils.OperatorPoll).Should(Succeed(), "ClusterQueue should show borrowed CPU from parent Cohort")
	})

	It("should enforce BorrowingLimit and LendingLimit on Cohort resourceGroups", Label("D5"), func(ctx context.Context) {
		By("Creating a ResourceFlavor")
		rf, cleanupRF, err := testutils.NewResourceFlavor().WithGenerateName().CreateWithObject(ctx, clients.UpstreamKueueClient)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(cleanupRF)

		By("Creating a parent Cohort with a large resource pool")
		parentCohort, cleanupParent, err := testutils.NewCohort().WithGenerateName().
			WithResourceGroups([]kueuev1beta2.ResourceGroup{{
				CoveredResources: []corev1.ResourceName{"cpu", "memory"},
				Flavors: []kueuev1beta2.FlavorQuotas{{
					Name: kueuev1beta2.ResourceFlavorReference(rf.Name),
					Resources: []kueuev1beta2.ResourceQuota{
						{Name: "cpu", NominalQuota: resource.MustParse("4")},
						{Name: "memory", NominalQuota: resource.MustParse("4Gi")},
					},
				}},
			}}).
			CreateWithObject(ctx, clients.UpstreamKueueClient)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(cleanupParent)

		borrowLimit := resource.MustParse("500m")
		By("Creating a child Cohort with a BorrowingLimit of 500m CPU from the parent")
		childCohort, cleanupChild, err := testutils.NewCohort().WithGenerateName().
			WithParentName(parentCohort.Name).
			WithResourceGroups([]kueuev1beta2.ResourceGroup{{
				CoveredResources: []corev1.ResourceName{"cpu", "memory"},
				Flavors: []kueuev1beta2.FlavorQuotas{{
					Name: kueuev1beta2.ResourceFlavorReference(rf.Name),
					Resources: []kueuev1beta2.ResourceQuota{
						{Name: "cpu", NominalQuota: resource.MustParse("0"), BorrowingLimit: &borrowLimit},
						{Name: "memory", NominalQuota: resource.MustParse("0")},
					},
				}},
			}}).
			CreateWithObject(ctx, clients.UpstreamKueueClient)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(cleanupChild)

		By("Creating a ClusterQueue with nominalQuota 0 in the child Cohort")
		cq, cleanupCQ, err := testutils.NewClusterQueue().WithGenerateName().
			WithFlavorName(rf.Name).
			WithCPU("0").
			WithMemory("0").
			WithCohort(childCohort.Name).
			CreateWithObject(ctx, clients.UpstreamKueueClient)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(cleanupCQ)

		By("Creating a Namespace and LocalQueue")
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "cohort-limit-",
				Labels:       map[string]string{testutils.OpenShiftManagedLabel: "true"},
			},
		}
		cleanupNs, err := testutils.CreateNamespace(kubeClient, ns)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(cleanupNs)

		lq, cleanupLQ, err := testutils.NewLocalQueue(ns.Name, "limit-lq").
			WithClusterQueue(cq.Name).
			CreateWithObject(ctx, clients.UpstreamKueueClient)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(cleanupLQ)

		By("Creating a Job within the borrowing limit (250m CPU)")
		builder := testutils.NewTestResourceBuilder(ns.Name, lq.Name)
		withinLimitJob := builder.NewJob()
		withinLimitJob.Labels[testutils.QueueLabel] = lq.Name
		withinLimitJob.Spec.Template.Spec.Containers[0].Resources.Requests = corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("250m"),
			corev1.ResourceMemory: resource.MustParse("128Mi"),
		}
		withinLimitJob.Spec.Template.Spec.Containers[0].Command = []string{"sh", "-c", "sleep 300"}
		createdWithinJob, err := kubeClient.BatchV1().Jobs(ns.Name).Create(ctx, withinLimitJob, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			testutils.CleanUpJob(ctx, kubeClient, ns.Name, createdWithinJob.Name)
		})

		By("Verifying the first job is admitted")
		checkWorkloadCondition(ctx, ns.Name, string(createdWithinJob.UID), kueuev1beta2.WorkloadAdmitted, "within-limit")

		By("Creating a second Job that would exceed the borrowing limit (500m more CPU)")
		exceedJob := builder.NewJob()
		exceedJob.Labels[testutils.QueueLabel] = lq.Name
		exceedJob.Spec.Template.Spec.Containers[0].Resources.Requests = corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("500m"),
			corev1.ResourceMemory: resource.MustParse("128Mi"),
		}
		exceedJob.Spec.Template.Spec.Containers[0].Command = []string{"sh", "-c", "sleep 300"}
		createdExceedJob, err := kubeClient.BatchV1().Jobs(ns.Name).Create(ctx, exceedJob, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			testutils.CleanUpJob(ctx, kubeClient, ns.Name, createdExceedJob.Name)
		})

		By("Verifying the second job is NOT admitted (stays pending due to borrowing limit)")
		Consistently(func() bool {
			workloads, err := clients.UpstreamKueueClient.KueueV1beta2().Workloads(ns.Name).List(ctx, metav1.ListOptions{
				LabelSelector: fmt.Sprintf("kueue.x-k8s.io/job-uid=%s", string(createdExceedJob.UID)),
			})
			if err != nil || len(workloads.Items) == 0 {
				return true
			}
			for _, condition := range workloads.Items[0].Status.Conditions {
				if condition.Type == kueuev1beta2.WorkloadAdmitted && condition.Status == metav1.ConditionTrue {
					return false
				}
			}
			return true
		}, testutils.ConsistentlyLongTimeout, testutils.ConsistentlyLongPoll).Should(BeTrue(), "Second job should not be admitted due to borrowing limit")
	})

	It("should allow a ClusterQueue to borrow from Cohort shared pool", Label("D3"), func(ctx context.Context) {
		By("Creating a ResourceFlavor")
		rf, cleanupRF, err := testutils.NewResourceFlavor().WithGenerateName().CreateWithObject(ctx, clients.UpstreamKueueClient)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(cleanupRF)

		By("Creating an explicit Cohort with a shared CPU and memory pool")
		cohort, cleanupCohort, err := testutils.NewCohort().WithGenerateName().
			WithResourceGroups([]kueuev1beta2.ResourceGroup{{
				CoveredResources: []corev1.ResourceName{"cpu", "memory"},
				Flavors: []kueuev1beta2.FlavorQuotas{{
					Name: kueuev1beta2.ResourceFlavorReference(rf.Name),
					Resources: []kueuev1beta2.ResourceQuota{
						{Name: "cpu", NominalQuota: resource.MustParse("1")},
						{Name: "memory", NominalQuota: resource.MustParse("1Gi")},
					},
				}},
			}}).
			CreateWithObject(ctx, clients.UpstreamKueueClient)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(cleanupCohort)

		By("Creating a ClusterQueue with nominalQuota 0 that joins the Cohort")
		cq, cleanupCQ, err := testutils.NewClusterQueue().WithGenerateName().
			WithFlavorName(rf.Name).
			WithCPU("0").
			WithMemory("0").
			WithCohort(cohort.Name).
			CreateWithObject(ctx, clients.UpstreamKueueClient)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(cleanupCQ)

		By("Creating a Namespace and LocalQueue")
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "cohort-borrow-",
				Labels: map[string]string{
					testutils.OpenShiftManagedLabel: "true",
				},
			},
		}
		cleanupNs, err := testutils.CreateNamespace(kubeClient, ns)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(cleanupNs)

		lq, cleanupLQ, err := testutils.NewLocalQueue(ns.Name, "cohort-lq").
			WithClusterQueue(cq.Name).
			CreateWithObject(ctx, clients.UpstreamKueueClient)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(cleanupLQ)

		By("Creating a Job that requires resources from the Cohort shared pool")
		builder := testutils.NewTestResourceBuilder(ns.Name, lq.Name)
		job := builder.NewJob()
		job.Labels[testutils.QueueLabel] = lq.Name
		job.Spec.Template.Spec.Containers[0].Resources.Requests = corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("250m"),
			corev1.ResourceMemory: resource.MustParse("256Mi"),
		}
		job.Spec.Template.Spec.Containers[0].Command = []string{"sh", "-c", "echo borrowing from cohort; sleep 60"}
		createdJob, err := kubeClient.BatchV1().Jobs(ns.Name).Create(ctx, job, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			testutils.CleanUpJob(ctx, kubeClient, ns.Name, createdJob.Name)
		})

		By("Verifying the workload is admitted (resources borrowed from Cohort)")
		checkWorkloadCondition(ctx, ns.Name, string(createdJob.UID), kueuev1beta2.WorkloadAdmitted, "cohort-borrow")

		By("Verifying the ClusterQueue shows borrowed resources")
		Eventually(func() error {
			fetchedCQ, err := clients.UpstreamKueueClient.KueueV1beta2().ClusterQueues().Get(ctx, cq.Name, metav1.GetOptions{})
			if err != nil {
				return err
			}
			for _, flavorUsage := range fetchedCQ.Status.FlavorsUsage {
				for _, resourceUsage := range flavorUsage.Resources {
					if resourceUsage.Name == corev1.ResourceCPU {
						if resourceUsage.Borrowed.Cmp(resource.MustParse("250m")) >= 0 {
							return nil
						}
						return fmt.Errorf("expected borrowed CPU >= 250m, got %s", resourceUsage.Borrowed.String())
					}
				}
			}
			return fmt.Errorf("CPU resource not found in ClusterQueue status")
		}, testutils.OperatorReadyTime, testutils.OperatorPoll).Should(Succeed(), "ClusterQueue should show borrowed CPU from Cohort")
	})
})
