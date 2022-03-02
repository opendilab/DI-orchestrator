package common

import (
	"fmt"
	"os"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"
)

func TestConfig(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecsWithDefaultAndCustomReporters(t,
		"CommonConfig Suite",
		[]Reporter{printer.NewlineReporter{}})
}

var _ = Describe("Test common config", func() {
	Context("Get DIJob default resources", func() {
		It("returns the default resources", func() {
			type testCase struct {
				resource  string
				expectCPU string
				expectMem string
			}
			testCases := []testCase{
				{resource: `{"resources": {"requests": {"cpu": 1, "memory": "2Gi"}}}`, expectCPU: "1", expectMem: "2Gi"},
				{resource: `{"resources": {"requests": {"cpu": 2, "memory": "3Gi"}}}`, expectCPU: "2", expectMem: "3Gi"},
				{resource: "", expectCPU: "1", expectMem: "2Gi"},
			}
			for i := range testCases {
				c := testCases[i]
				By(fmt.Sprintf("Create the %dth DIJob", i+1))
				err := os.Setenv("DI_JOB_DEFAULT_RESOURCES", c.resource)
				Expect(err).NotTo(HaveOccurred())
				r, err := GetDIJobDefaultResources()
				Expect(err).NotTo(HaveOccurred())
				Expect(r.Requests.Cpu().Equal(resource.MustParse(c.expectCPU))).Should(BeTrue())
				Expect(r.Requests.Memory().Equal(resource.MustParse(c.expectMem))).Should(BeTrue())
			}
		})
	})
})
