package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
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
		It("Should have correct response when request /v1alpha1/replicas and /v1alpha1/replicas/failed", func() {
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
			rbody, err := json.Marshal(req)
			Expect(err).NotTo(HaveOccurred())

			njresp, err := sendRequest(http.MethodPost, rbody, rurl, http.StatusOK, true)
			Expect(err).NotTo(HaveOccurred())

			Expect(len(njresp.Collectors)).Should(Equal(cn))
			Expect(len(njresp.Learners)).Should(Equal(ln))

			By("Send request on GET /v1alpha1/replicas")
			gurl := fmt.Sprintf("%s?namespace=%s&coordinator=%s", rurl, job.Namespace, coorname)
			resp, err := http.Get(gurl)
			Expect(err).NotTo(HaveOccurred())
			gnjresp, err := parseResponse(resp, http.StatusOK, true)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(gnjresp.Collectors)).Should(Equal(cn))
			Expect(len(gnjresp.Learners)).Should(Equal(ln))

			By("Send request on POST /v1alpha1/replicas/failed")
			furl := fmt.Sprintf("http://%s/v1alpha1/replicas/failed", addr)
			freq := servertypes.NerveXJobResponse{
				Namespace:   job.Namespace,
				Coordinator: coorname,
				Collectors: []string{
					strings.Split(gnjresp.Collectors[0], ".")[0],
				},
				Learners: []string{
					strings.Split(gnjresp.Learners[0], ".")[0],
				},
			}
			frbody, err := json.Marshal(freq)
			Expect(err).NotTo(HaveOccurred())
			fnjresp, err := sendRequest(http.MethodPost, frbody, furl, http.StatusOK, true)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(fnjresp.Collectors)).Should(Equal(1))
			Expect(len(fnjresp.Learners)).Should(Equal(1))

			By("Waiting for server's cache to be synced")
			time.Sleep(250 * time.Millisecond)

			By("Checking the number of pods has not changed")
			totalPods := cn + ln + 1 // collectors + learners + coordinator
			Eventually(func() int {
				pods, err := nervexutil.ListPods(ctx, k8sClient, job)
				if err != nil {
					return -1
				}
				return len(pods)
			}, timeout, interval).Should(Equal(totalPods))

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
			drbody, err := json.Marshal(dreq)
			Expect(err).NotTo(HaveOccurred())

			dnjresp, err := sendRequest(http.MethodDelete, drbody, rurl, http.StatusOK, true)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(dnjresp.Collectors)).Should(Equal(dcn))
			Expect(len(dnjresp.Learners)).Should(Equal(dln))

			err = testutil.CleanUpJob(ctx, k8sClient, job.DeepCopy(), timeout, interval)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should have matched responses when requests are refused", func() {
			job := testutil.NewNerveXJob()
			name := nervexutil.GenerateName(job.Name)
			job.SetName(name)

			By(fmt.Sprintf("Create %dth NerveXJob", 1))
			var err error
			ctx := context.Background()
			err = creatNerveXJob(ctx, job)
			Expect(err).NotTo(HaveOccurred())

			addr := fmt.Sprintf("%s:%d", localServingHost, localServingPort)

			By("Send request on /healthz")
			hurl := fmt.Sprintf("http://%s/healthz", addr)
			hresp, err := http.Get(hurl)
			Expect(err).NotTo(HaveOccurred())
			Expect(hresp.StatusCode).Should(Equal(http.StatusOK))

			By("Send request on POST /v1alpha1/replicas")
			coorname := nervexutil.ReplicaPodName(job.Name, "coordinator")
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
			rbody, err := json.Marshal(req)
			Expect(err).NotTo(HaveOccurred())

			njresp, err := sendRequest(http.MethodPost, rbody, rurl, http.StatusOK, true)
			Expect(err).NotTo(HaveOccurred())

			Expect(len(njresp.Collectors)).Should(Equal(cn))
			Expect(len(njresp.Learners)).Should(Equal(ln))

			By("Send not found resource on POST /v1alpha1/replicas")
			req.Coordinator = "not-exists"
			rbody, err = json.Marshal(req)
			Expect(err).NotTo(HaveOccurred())

			By("Send bad request on POST /v1alpha1/replicas")
			_, err = sendRequest(http.MethodPost, rbody, rurl, http.StatusNotFound, false)
			Expect(err).NotTo(HaveOccurred())

			By("Send not implemented method on /v1alpha1/replicas")
			_, err = sendRequest(http.MethodPatch, rbody, rurl, http.StatusNotImplemented, false)
			Expect(err).NotTo(HaveOccurred())

			rbody = []byte{}
			_, err = sendRequest(http.MethodPost, rbody, rurl, http.StatusBadRequest, false)
			Expect(err).NotTo(HaveOccurred())

			By("Send request on GET /v1alpha1/replicas with namespace and coordinator")
			gurl := fmt.Sprintf("%s?namespace=%s&coordinator=%s", rurl, job.Namespace, coorname)
			resp, err := http.Get(gurl)
			Expect(err).NotTo(HaveOccurred())
			gnjresp, err := parseResponse(resp, http.StatusOK, true)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(gnjresp.Collectors)).Should(Equal(cn))
			Expect(len(gnjresp.Learners)).Should(Equal(ln))

			By("Send request on GET /v1alpha1/replicas with namespace")
			gurl = fmt.Sprintf("%s?namespace=%s", rurl, job.Namespace)
			resp, err = http.Get(gurl)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			var nresp servertypes.Response
			err = json.NewDecoder(resp.Body).Decode(&nresp)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).Should(Equal(http.StatusOK))
			Expect(nresp.Success).Should(BeTrue())

			By("Send request on GET /v1alpha1/replicas")
			gurl = rurl
			resp, err = http.Get(gurl)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			err = json.NewDecoder(resp.Body).Decode(&nresp)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).Should(Equal(http.StatusOK))
			Expect(nresp.Success).Should(BeTrue())

			By("Send request on POST /v1alpha1/replicas/failed")
			furl := fmt.Sprintf("http://%s/v1alpha1/replicas/failed", addr)
			freq := servertypes.NerveXJobResponse{
				Namespace:   job.Namespace,
				Coordinator: coorname,
				Collectors: []string{
					strings.Split(gnjresp.Collectors[0], ".")[0],
				},
				Learners: []string{
					strings.Split(gnjresp.Learners[0], ".")[0],
				},
			}
			frbody, err := json.Marshal(freq)
			Expect(err).NotTo(HaveOccurred())
			fnjresp, err := sendRequest(http.MethodPost, frbody, furl, http.StatusOK, true)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(fnjresp.Collectors)).Should(Equal(1))
			Expect(len(fnjresp.Learners)).Should(Equal(1))

			By("Waiting for server's cache to be synced")
			time.Sleep(250 * time.Millisecond)

			By("Checking the number of pods has not changed")
			totalPods := cn + ln + 1 // collectors + learners + coordinator
			Eventually(func() int {
				pods, err := nervexutil.ListPods(ctx, k8sClient, job)
				if err != nil {
					return -1
				}
				return len(pods)
			}, timeout, interval).Should(Equal(totalPods))

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
			drbody, err := json.Marshal(dreq)
			Expect(err).NotTo(HaveOccurred())

			dnjresp, err := sendRequest(http.MethodDelete, drbody, rurl, http.StatusOK, true)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(dnjresp.Collectors)).Should(Equal(dcn))
			Expect(len(dnjresp.Learners)).Should(Equal(dln))

			err = testutil.CleanUpJob(ctx, k8sClient, job.DeepCopy(), timeout, interval)
			Expect(err).NotTo(HaveOccurred())
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
	lbs := nervexutil.GenLabels(job.Name)
	coorpod.SetLabels(lbs)

	err = k8sClient.Create(ctx, coorpod, &client.CreateOptions{})
	if err != nil {
		return err
	}

	By("Waiting for server's cache to be synced")
	time.Sleep(250 * time.Millisecond)
	return nil
}

func sendRequest(method string, rbody []byte, url string, expectedCode int, expectedSuccess bool) (*servertypes.NerveXJobResponse, error) {

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

	return parseResponse(resp, expectedCode, expectedSuccess)
}

func parseResponse(resp *http.Response, expectedCode int, expectedSuccess bool) (*servertypes.NerveXJobResponse, error) {
	defer resp.Body.Close()
	var nresp servertypes.Response
	err := json.NewDecoder(resp.Body).Decode(&nresp)
	if err != nil {
		return nil, err
	}

	Expect(resp.StatusCode).Should(Equal(expectedCode))
	Expect(nresp.Success).Should(Equal(expectedSuccess))

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
