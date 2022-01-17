package common

import (
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
		"Config Suite",
		[]Reporter{printer.NewlineReporter{}})
}

var _ = Describe("GetDIJobDefaultResources", func() {
	defaultResource := `{"resources": {"requests": {"cpu": 1, "memory": "2Gi"}}}`
	err := os.Setenv("DI_JOB_DEFAULT_RESOURCES", defaultResource)
	Expect(err).NotTo(HaveOccurred())
	r, err := GetDIJobDefaultResources()
	Expect(err).NotTo(HaveOccurred())
	Expect(r.Requests.Cpu().Equal(resource.MustParse("1"))).Should(BeTrue())
	Expect(r.Requests.Memory().Equal(resource.MustParse("2Gi"))).Should(BeTrue())
})
