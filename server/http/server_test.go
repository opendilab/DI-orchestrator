package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	nervexv1alpha1 "go-sensephoenix.sensetime.com/nervex-operator/api/v1alpha1"
	servertypes "go-sensephoenix.sensetime.com/nervex-operator/server/types"
	nervexutil "go-sensephoenix.sensetime.com/nervex-operator/utils"
	testutil "go-sensephoenix.sensetime.com/nervex-operator/utils/testutils"
)

var _ = Describe("Server Test", func() {
	Context("When send request to server", func() {
		It("Should create the right number of replicas", func() {
			job := testutil.NewNerveXJob()
			name := nervexutil.GenerateName(job.Name)
			job.SetName(name)

			By(fmt.Sprintf("Create %dth NerveXJob", 1))
			var err error
			ctx := context.Background()
			err = creatNerveXJob(ctx, job)
			Expect(err).NotTo(HaveOccurred())

			By("Send request on POST /v1alpha1/replicas")
			coorname := nervexutil.ReplicaPodName(job.Name, "coordinator")
			addr := fmt.Sprintf("%s:%d", localServingHost, localServingPort)
			rurl := fmt.Sprintf("http://%s/v1alpha1/replicas", addr)
			var cn, ln int = 2, 3
			req := servertypes.NerveXJobRequest{
				Namespace:   job.Namespace,
				Coordinator: coorname,
				Collectors: servertypes.ResourceQuantity{
					Replicas: cn,
				},
				Learners: servertypes.ResourceQuantity{
					Replicas: ln,
				},
			}

			njresp, err := sendRequest(http.MethodPost, req, rurl)
			Expect(err).NotTo(HaveOccurred())

			Expect(len(njresp.Collectors)).Should(Equal(cn))
			Expect(len(njresp.Learners)).Should(Equal(ln))

			By("Send request on GET /v1alpha1/replicas")
			gurl := fmt.Sprintf("%s?namespace=%s&coordinator=%s", rurl, job.Namespace, coorname)
			resp, err := http.Get(gurl)
			Expect(err).NotTo(HaveOccurred())
			gnjresp, err := parseResponse(resp)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(gnjresp.Collectors)).Should(Equal(cn))
			Expect(len(gnjresp.Learners)).Should(Equal(ln))

			By("Send request on DELETE /v1alpha1/replicas")
			var dcn, dln int = 1, 1
			dreq := servertypes.NerveXJobRequest{
				Namespace:   job.Namespace,
				Coordinator: coorname,
				Collectors: servertypes.ResourceQuantity{
					Replicas: dcn,
				},
				Learners: servertypes.ResourceQuantity{
					Replicas: dln,
				},
			}
			dnjresp, err := sendRequest(http.MethodDelete, dreq, rurl)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(dnjresp.Collectors)).Should(Equal(dcn))
			Expect(len(dnjresp.Learners)).Should(Equal(dln))
		})
	})
})

func creatNerveXJob(ctx context.Context, job *nervexv1alpha1.NerveXJob) error {
	var err error
	err = k8sClient.Create(ctx, job, &client.CreateOptions{})
	if err != nil {
		return err
	}

	By("Create coordinator")
	ownRefer := metav1.OwnerReference{
		APIVersion: nervexv1alpha1.GroupVersion.String(),
		Kind:       nervexv1alpha1.KindNerveXJob,
		Name:       job.Name,
		UID:        job.GetUID(),
		Controller: func(c bool) *bool { return &c }(true),
	}
	coorname := nervexutil.ReplicaPodName(job.Name, "coordinator")
	coorpod := testutil.NewPod(coorname, job.Name, ownRefer)
	err = k8sClient.Create(ctx, coorpod, &client.CreateOptions{})
	if err != nil {
		return err
	}

	By("Waiting for server's cache to be synced")
	time.Sleep(250 * time.Millisecond)
	return nil
}

func sendRequest(method string, req servertypes.NerveXJobRequest, url string) (*servertypes.NerveXJobResponse, error) {
	rbody, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	// Create client
	reqs, err := http.NewRequest(method, url, bytes.NewReader(rbody))
	if err != nil {
		return nil, err
	}
	reqs.Header.Add("Content-Type", "application/json")

	// Fetch Request
	resp, err := http.DefaultClient.Do(reqs)
	if err != nil {
		return nil, err
	}

	return parseResponse(resp)
}

func parseResponse(resp *http.Response) (*servertypes.NerveXJobResponse, error) {
	defer resp.Body.Close()
	var nresp servertypes.Response
	err := json.NewDecoder(resp.Body).Decode(&nresp)
	if err != nil {
		return nil, err
	}

	Expect(nresp.Success).Should(BeTrue())

	var njresp servertypes.NerveXJobResponse
	jsonBytes, err := json.Marshal(nresp.Data)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(jsonBytes, &njresp)
	if err != nil {
		return nil, err
	}
	return &njresp, nil
}
