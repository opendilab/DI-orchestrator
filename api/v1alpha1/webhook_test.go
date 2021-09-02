package v1alpha1

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilrand "k8s.io/apimachinery/pkg/util/rand"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Webhook test", func() {
	Context("When creating a DIJob", func() {
		It("Should be validated by webhook before creating", func() {
			type testCase struct {
				cleanPodPolicy       CleanPodPolicy
				expectCleanPodPolicy CleanPodPolicy
			}
			testCases := []testCase{
				{cleanPodPolicy: CleanPodPolicyRunning, expectCleanPodPolicy: CleanPodPolicyRunning},
				{cleanPodPolicy: CleanPodPolicyAll, expectCleanPodPolicy: CleanPodPolicyAll},
				{cleanPodPolicy: CleanPodPolicyNone, expectCleanPodPolicy: CleanPodPolicyNone},
				{cleanPodPolicy: CleanPodPolicy(""), expectCleanPodPolicy: CleanPodPolicyRunning},
				{cleanPodPolicy: CleanPodPolicy("hello"), expectCleanPodPolicy: CleanPodPolicy("will be refused by webhook")},
				{cleanPodPolicy: CleanPodPolicy("sdft"), expectCleanPodPolicy: CleanPodPolicy("will be refused by webhook")},
			}
			for i := range testCases {
				c := testCases[i]
				job := NewDIJob()
				name := GenerateName(job.Name)
				job.SetName(name)

				job.Spec.CleanPodPolicy = c.cleanPodPolicy

				var err error
				ctx := context.Background()
				err = k8sClient.Create(ctx, job, &client.CreateOptions{})
				if err != nil {
					if c.cleanPodPolicy != CleanPodPolicyRunning && c.cleanPodPolicy != CleanPodPolicyNone &&
						c.cleanPodPolicy != CleanPodPolicyAll {
						Expect(err.Error()).To(ContainSubstring("Invalid CleanPodPolicy"))
						continue
					} else {
						Expect(err).NotTo(HaveOccurred())
					}
				}

				cjob := DIJob{}
				jobKey := types.NamespacedName{Namespace: job.Namespace, Name: job.Name}
				Eventually(func() bool {
					err = k8sClient.Get(ctx, jobKey, &cjob)
					if err != nil {
						return false
					}
					return cjob.Spec.CleanPodPolicy == c.expectCleanPodPolicy
				}, timeout, interval).Should(BeTrue())
			}

		})
		It("Should be validated by webhook before updating", func() {
			type testCase struct {
				cleanPodPolicy       CleanPodPolicy
				expectCleanPodPolicy CleanPodPolicy
			}
			testCases := []testCase{
				{cleanPodPolicy: CleanPodPolicyRunning, expectCleanPodPolicy: CleanPodPolicyRunning},
				{cleanPodPolicy: CleanPodPolicyAll, expectCleanPodPolicy: CleanPodPolicyAll},
				{cleanPodPolicy: CleanPodPolicyNone, expectCleanPodPolicy: CleanPodPolicyNone},
				{cleanPodPolicy: CleanPodPolicy(""), expectCleanPodPolicy: CleanPodPolicyRunning},
				{cleanPodPolicy: CleanPodPolicy("hello"), expectCleanPodPolicy: CleanPodPolicy("will be refused by webhook")},
				{cleanPodPolicy: CleanPodPolicy("sdft"), expectCleanPodPolicy: CleanPodPolicy("will be refused by webhook")},
			}
			for i := range testCases {
				c := testCases[i]
				job := NewDIJob()
				name := GenerateName(job.Name)
				job.SetName(name)

				var err error
				ctx := context.Background()
				err = k8sClient.Create(ctx, job, &client.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				job.Spec.CleanPodPolicy = c.cleanPodPolicy
				err = k8sClient.Update(ctx, job, &client.UpdateOptions{})
				if err != nil {
					if c.cleanPodPolicy != CleanPodPolicyRunning && c.cleanPodPolicy != CleanPodPolicyNone &&
						c.cleanPodPolicy != CleanPodPolicyAll {
						Expect(err.Error()).To(ContainSubstring("Invalid CleanPodPolicy"))
						continue
					} else {
						Expect(err).NotTo(HaveOccurred())
					}
				}

				cjob := DIJob{}
				jobKey := types.NamespacedName{Namespace: job.Namespace, Name: job.Name}
				Eventually(func() CleanPodPolicy {
					err = k8sClient.Get(ctx, jobKey, &cjob)
					if err != nil {
						return CleanPodPolicy(err.Error())
					}
					return cjob.Spec.CleanPodPolicy
				}, timeout, interval).Should(Equal(c.expectCleanPodPolicy))
			}

		})
	})
})

const (
	randomLength         = 5
	DIJobName            = "dijob-example"
	DIJobNamespace       = "default"
	DIJobImage           = "alpine:latest"
	DefaultSleepDuration = "5s"
	timeout              = 5 * time.Second
	interval             = 250 * time.Millisecond
)

func NewDIJob() *DIJob {
	return &DIJob{
		TypeMeta: metav1.TypeMeta{
			Kind:       KindDIJob,
			APIVersion: GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      DIJobName,
			Namespace: DIJobNamespace,
		},
		Spec: DIJobSpec{
			Coordinator: CoordinatorSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:    "coordinator",
								Image:   DIJobImage,
								Command: []string{"/bin/sh", "-c", "sleep", DefaultSleepDuration},
							},
						},
					},
				},
			},
			Collector: CollectorSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:    "collector",
								Image:   DIJobImage,
								Command: []string{"/bin/sh", "-c", "sleep", DefaultSleepDuration},
							},
						},
					},
				},
			},
			Learner: LearnerSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:    "learner",
								Image:   DIJobImage,
								Command: []string{"/bin/sh", "-c", "sleep", DefaultSleepDuration},
							},
						},
					},
				},
			},
		},
	}
}

func GenerateName(name string) string {
	return fmt.Sprintf("%s-%s", name, utilrand.String(randomLength))
}
