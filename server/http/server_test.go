package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/controller-runtime/pkg/client"

	div1alpha1 "opendilab.org/di-orchestrator/api/v1alpha1"
	dicommon "opendilab.org/di-orchestrator/common"
	commontypes "opendilab.org/di-orchestrator/common/types"
	diutil "opendilab.org/di-orchestrator/utils"
	testutil "opendilab.org/di-orchestrator/utils/testutils"
)

var _ = Describe("Server Test", func() {
	Context("When send request to server", func() {
		It("Should have correct response when request /v1alpha1/replicas and /v1alpha1/replicas/failed", func() {
			job := testutil.NewDIJob()
			name := diutil.GenerateName(job.Name)
			job.SetName(name)

			By(fmt.Sprintf("Create %dth DIJob", 1))
			var err error
			ctx := context.Background()
			err = creatDIJob(ctx, job)
			Expect(err).NotTo(HaveOccurred())

			By("Send request on POST /v1alpha1/replicas")
			coorname := diutil.ReplicaPodName(job.Name, "coordinator")
			addr := fmt.Sprintf("%s:%d", localServingHost, localServingPort)
			rurl := fmt.Sprintf("http://%s/v1alpha1/replicas", addr)
			var cn, ln int = 2, 3
			req := commontypes.DIJobRequest{
				Namespace:   job.Namespace,
				Coordinator: coorname,
				Collectors: commontypes.ResourceQuantity{
					Replicas: cn,
				},
				Learners: commontypes.ResourceQuantity{
					Replicas: ln,
				},
			}
			rbody, err := json.Marshal(req)
			Expect(err).NotTo(HaveOccurred())

			diresp, err := sendRequest(http.MethodPost, rbody, rurl, http.StatusOK, true)
			Expect(err).NotTo(HaveOccurred())

			Expect(len(diresp.Collectors)).Should(Equal(cn))
			Expect(len(diresp.Learners)).Should(Equal(ln))

			By("Send request on GET /v1alpha1/replicas")
			gurl := fmt.Sprintf("%s?namespace=%s&coordinator=%s", rurl, job.Namespace, coorname)
			resp, err := http.Get(gurl)
			Expect(err).NotTo(HaveOccurred())
			gdiresp, err := parseResponse(resp, http.StatusOK, true)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(gdiresp.Collectors)).Should(Equal(cn))
			Expect(len(gdiresp.Learners)).Should(Equal(ln))

			By("Send request on POST /v1alpha1/replicas/failed")
			furl := fmt.Sprintf("http://%s/v1alpha1/replicas/failed", addr)
			freq := commontypes.DIJobResponse{
				Namespace:   job.Namespace,
				Coordinator: coorname,
				Collectors: []string{
					strings.Split(gdiresp.Collectors[0], ".")[0],
				},
				Learners: []string{
					strings.Split(gdiresp.Learners[0], ".")[0],
				},
			}
			frbody, err := json.Marshal(freq)
			Expect(err).NotTo(HaveOccurred())
			fdiresp, err := sendRequest(http.MethodPost, frbody, furl, http.StatusOK, true)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(fdiresp.Collectors)).Should(Equal(1))
			Expect(len(fdiresp.Learners)).Should(Equal(1))

			By("Waiting for server's cache to be synced")
			time.Sleep(250 * time.Millisecond)

			By("Checking the number of pods has not changed")
			totalPods := cn + ln + 1 // collectors + learners + coordinator
			Eventually(func() int {
				pods, err := diutil.ListPods(ctx, k8sClient, job)
				if err != nil {
					return -1
				}
				return len(pods)
			}, timeout, interval).Should(Equal(totalPods))

			By("Send request on DELETE /v1alpha1/replicas")
			var dcn, dln int = 1, 1
			dreq := commontypes.DIJobRequest{
				Namespace:   job.Namespace,
				Coordinator: coorname,
				Collectors: commontypes.ResourceQuantity{
					Replicas: dcn,
				},
				Learners: commontypes.ResourceQuantity{
					Replicas: dln,
				},
			}
			drbody, err := json.Marshal(dreq)
			Expect(err).NotTo(HaveOccurred())

			ddiresp, err := sendRequest(http.MethodDelete, drbody, rurl, http.StatusOK, true)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(ddiresp.Collectors)).Should(Equal(dcn))
			Expect(len(ddiresp.Learners)).Should(Equal(dln))

			err = testutil.CleanUpJob(ctx, k8sClient, job.DeepCopy())
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should have matched responses when requests are refused", func() {
			job := testutil.NewDIJob()
			name := diutil.GenerateName(job.Name)
			job.SetName(name)

			By(fmt.Sprintf("Create %dth DIJob", 1))
			var err error
			ctx := context.Background()
			err = creatDIJob(ctx, job)
			Expect(err).NotTo(HaveOccurred())

			addr := fmt.Sprintf("%s:%d", localServingHost, localServingPort)

			By("Send request on /healthz")
			hurl := fmt.Sprintf("http://%s/healthz", addr)
			hresp, err := http.Get(hurl)
			Expect(err).NotTo(HaveOccurred())
			Expect(hresp.StatusCode).Should(Equal(http.StatusOK))

			By("Send request on POST /v1alpha1/replicas")
			coorname := diutil.ReplicaPodName(job.Name, "coordinator")
			rurl := fmt.Sprintf("http://%s/v1alpha1/replicas", addr)
			var cn, ln int = 2, 3
			req := commontypes.DIJobRequest{
				Namespace:   job.Namespace,
				Coordinator: coorname,
				Collectors: commontypes.ResourceQuantity{
					Replicas: cn,
				},
				Learners: commontypes.ResourceQuantity{
					Replicas: ln,
				},
			}
			rbody, err := json.Marshal(req)
			Expect(err).NotTo(HaveOccurred())

			diresp, err := sendRequest(http.MethodPost, rbody, rurl, http.StatusOK, true)
			Expect(err).NotTo(HaveOccurred())

			Expect(len(diresp.Collectors)).Should(Equal(cn))
			Expect(len(diresp.Learners)).Should(Equal(ln))

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
			gdiresp, err := parseResponse(resp, http.StatusOK, true)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(gdiresp.Collectors)).Should(Equal(cn))
			Expect(len(gdiresp.Learners)).Should(Equal(ln))

			By("Send request on GET /v1alpha1/replicas with namespace")
			gurl = fmt.Sprintf("%s?namespace=%s", rurl, job.Namespace)
			resp, err = http.Get(gurl)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			var nresp commontypes.Response
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
			freq := commontypes.DIJobResponse{
				Namespace:   job.Namespace,
				Coordinator: coorname,
				Collectors: []string{
					strings.Split(gdiresp.Collectors[0], ".")[0],
				},
				Learners: []string{
					strings.Split(gdiresp.Learners[0], ".")[0],
				},
			}
			frbody, err := json.Marshal(freq)
			Expect(err).NotTo(HaveOccurred())
			fdiresp, err := sendRequest(http.MethodPost, frbody, furl, http.StatusOK, true)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(fdiresp.Collectors)).Should(Equal(1))
			Expect(len(fdiresp.Learners)).Should(Equal(1))

			By("Waiting for server's cache to be synced")
			time.Sleep(250 * time.Millisecond)

			By("Checking the number of pods has not changed")
			totalPods := cn + ln + 1 // collectors + learners + coordinator
			Eventually(func() int {
				pods, err := diutil.ListPods(ctx, k8sClient, job)
				if err != nil {
					return -1
				}
				return len(pods)
			}, timeout, interval).Should(Equal(totalPods))

			By("Send request on POST /v1alpha1/replicas/failed with duplicate replicas")
			fdurl := fmt.Sprintf("http://%s/v1alpha1/replicas/failed", addr)
			fdreq := commontypes.DIJobResponse{
				Namespace:   job.Namespace,
				Coordinator: coorname,
				Collectors: []string{
					strings.Split(gdiresp.Collectors[1], ".")[0],
					strings.Split(gdiresp.Collectors[1], ".")[0],
				},
				Learners: []string{
					strings.Split(gdiresp.Learners[1], ".")[0],
					strings.Split(gdiresp.Learners[1], ".")[0],
				},
			}
			fdrbody, err := json.Marshal(fdreq)
			Expect(err).NotTo(HaveOccurred())
			fddiresp, err := sendRequest(http.MethodPost, fdrbody, fdurl, http.StatusOK, true)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(fddiresp.Collectors)).Should(Equal(1))
			Expect(len(fddiresp.Learners)).Should(Equal(1))

			By("Waiting for server's cache to be synced")
			time.Sleep(250 * time.Millisecond)

			By("Checking the number of pods has not changed")
			dtotalPods := cn + ln + 1 // collectors + learners + coordinator
			Eventually(func() int {
				pods, err := diutil.ListPods(ctx, k8sClient, job)
				if err != nil {
					return -1
				}
				return len(pods)
			}, timeout, interval).Should(Equal(dtotalPods))

			By("Send request on DELETE /v1alpha1/replicas")
			var dcn, dln int = 1, 1
			dreq := commontypes.DIJobRequest{
				Namespace:   job.Namespace,
				Coordinator: coorname,
				Collectors: commontypes.ResourceQuantity{
					Replicas: dcn,
				},
				Learners: commontypes.ResourceQuantity{
					Replicas: dln,
				},
			}
			drbody, err := json.Marshal(dreq)
			Expect(err).NotTo(HaveOccurred())

			ddiresp, err := sendRequest(http.MethodDelete, drbody, rurl, http.StatusOK, true)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(ddiresp.Collectors)).Should(Equal(dcn))
			Expect(len(ddiresp.Learners)).Should(Equal(dln))

			err = testutil.CleanUpJob(ctx, k8sClient, job.DeepCopy())
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should have right replicas created when request gpu for learner", func() {
			addr := fmt.Sprintf("%s:%d", localServingHost, localServingPort)

			type testCase struct {
				ln                int
				gpus              int
				expectedAgg       int
				expectedLearner   int
				expectedDDPL      int
				expectedDDPLPorts int
			}
			testCases := []testCase{
				// learner requests 1 gpu, so no agg or ddpl is created.
				{ln: 1, gpus: 1, expectedAgg: 0, expectedLearner: 1, expectedDDPL: 0, expectedDDPLPorts: 0},
				// learner requests 2 gpus, so we need to create an agg and a ddpl.
				{ln: 1, gpus: 2, expectedAgg: 1, expectedLearner: 0, expectedDDPL: 1, expectedDDPLPorts: 2},
				// learner requests 10 gpus, so we need to create an agg and 2 ddpl, one ddpl with 8 gpus and the other ddpl with 2 gpus.
				{ln: 1, gpus: 10, expectedAgg: 1, expectedLearner: 0, expectedDDPL: 2, expectedDDPLPorts: 10},
				// learner requests 2 gpus, so we need to create an agg and a ddpl.
				{ln: 2, gpus: 2, expectedAgg: 2, expectedLearner: 0, expectedDDPL: 2, expectedDDPLPorts: 4},
			}
			for i := range testCases {
				c := testCases[i]
				job := testutil.NewDIJob()
				name := diutil.GenerateName(job.Name)
				job.SetName(name)

				By(fmt.Sprintf("Create %dth DIJob", i+1))
				var err error
				ctx := context.Background()
				err = creatDIJob(ctx, job)
				Expect(err).NotTo(HaveOccurred())

				By("Send request on POST /v1alpha1/replicas")
				coorname := diutil.ReplicaPodName(job.Name, "coordinator")
				rurl := fmt.Sprintf("http://%s/v1alpha1/replicas", addr)
				var ln int = c.ln
				req := commontypes.DIJobRequest{
					Namespace:   job.Namespace,
					Coordinator: coorname,
					Learners: commontypes.ResourceQuantity{
						Replicas: ln,
						GPU:      resource.MustParse(strconv.Itoa(c.gpus)),
					},
				}
				rbody, err := json.Marshal(req)
				Expect(err).NotTo(HaveOccurred())

				diresp, err := sendRequest(http.MethodPost, rbody, rurl, http.StatusOK, true)
				Expect(err).NotTo(HaveOccurred())

				// # of learners must be as expected
				Expect(len(diresp.Learners)).Should(Equal(ln))

				// # of ddp learners must as expected
				portCount, err := checkDDPLearnersPorts(ctx, c.expectedDDPL)
				Expect(err).NotTo(HaveOccurred())
				Expect(portCount).Should(Equal(c.expectedDDPLPorts))

				// # of aggregators must be as expected
				var aggs corev1.PodList
				err = k8sClient.List(ctx, &aggs, client.MatchingLabels{dicommon.ReplicaTypeLabel: dicommon.AggregatorName})
				Expect(err).NotTo(HaveOccurred())
				Expect(len(aggs.Items)).Should(Equal(c.expectedAgg))

				By("Send request on GET /v1alpha1/replicas with namespace and coordinator")
				gurl := fmt.Sprintf("%s?namespace=%s&coordinator=%s", rurl, job.Namespace, coorname)
				resp, err := http.Get(gurl)
				Expect(err).NotTo(HaveOccurred())
				gdiresp, err := parseResponse(resp, http.StatusOK, true)
				Expect(err).NotTo(HaveOccurred())
				// expect # of learners to be equal to learner+aggregator
				Expect(len(gdiresp.Learners)).Should(Equal(c.expectedLearner + c.expectedAgg))

				if len(aggs.Items) > 0 {
					By("Send request on GET /v1alpha1/replicas with namespace and aggregator")
					agg := aggs.Items[0]
					aurl := fmt.Sprintf("%s?namespace=%s&aggregator=%s", rurl, job.Namespace, agg.Name)
					aresp, err := http.Get(aurl)
					Expect(err).NotTo(HaveOccurred())
					adiresp, err := parseResponse(aresp, http.StatusOK, true)
					Expect(err).NotTo(HaveOccurred())
					// expect # of learners to be equal to ddp learners's gpu ports
					expectedDDPLs := c.expectedDDPLPorts / c.ln
					Expect(len(adiresp.Learners)).Should(Equal(expectedDDPLs))

					By("Send request on POST /v1alpha1/replicas/failed to report failure of aggregator")
					aurl = fmt.Sprintf("http://%s/v1alpha1/replicas/failed", addr)
					areq := commontypes.DIJobResponse{
						Namespace:   job.Namespace,
						Coordinator: coorname,
						Learners:    []string{agg.Name},
					}
					rbody, err := json.Marshal(areq)
					Expect(err).NotTo(HaveOccurred())
					adiresp, err = sendRequest(http.MethodPost, rbody, aurl, http.StatusOK, true)
					Expect(err).NotTo(HaveOccurred())
					Expect(len(adiresp.Learners)).Should(Equal(1))

					portCount, err := checkDDPLearnersPorts(ctx, c.expectedDDPL)
					Expect(err).NotTo(HaveOccurred())
					Expect(portCount).Should(Equal(c.expectedDDPLPorts))
				}

				By("Send request on DELETE /v1alpha1/replicas")
				var dln int = 1
				dreq := commontypes.DIJobRequest{
					Namespace:   job.Namespace,
					Coordinator: coorname,
					Learners: commontypes.ResourceQuantity{
						Replicas: dln,
					},
				}
				drbody, err := json.Marshal(dreq)
				Expect(err).NotTo(HaveOccurred())

				ddiresp, err := sendRequest(http.MethodDelete, drbody, rurl, http.StatusOK, true)
				Expect(err).NotTo(HaveOccurred())
				Expect(len(ddiresp.Learners)).Should(Equal(dln))

				err = testutil.CleanUpJob(ctx, k8sClient, job.DeepCopy())
				Expect(err).NotTo(HaveOccurred())
			}
		})
	})
})

func creatDIJob(ctx context.Context, job *div1alpha1.DIJob) error {
	var err error
	err = k8sClient.Create(ctx, job, &client.CreateOptions{})
	if err != nil {
		return err
	}

	By("Create coordinator")
	ownRefer := diutil.NewOwnerReference(div1alpha1.GroupVersion.String(), div1alpha1.KindDIJob, job.Name, job.UID, true)

	coorname := diutil.ReplicaPodName(job.Name, "coordinator")
	coorpod := testutil.NewPod(coorname, job.Name, ownRefer)
	lbs := diutil.GenLabels(job.Name)
	coorpod.SetLabels(lbs)

	err = k8sClient.Create(ctx, coorpod, &client.CreateOptions{})
	if err != nil {
		return err
	}

	By("Waiting for server's cache to be synced")
	time.Sleep(250 * time.Millisecond)
	return nil
}

func sendRequest(method string, rbody []byte, url string, expectedCode int, expectedSuccess bool) (*commontypes.DIJobResponse, error) {

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

func parseResponse(resp *http.Response, expectedCode int, expectedSuccess bool) (*commontypes.DIJobResponse, error) {
	defer resp.Body.Close()
	var nresp commontypes.Response
	err := json.NewDecoder(resp.Body).Decode(&nresp)
	if err != nil {
		return nil, err
	}

	Expect(resp.StatusCode).Should(Equal(expectedCode))
	Expect(nresp.Success).Should(Equal(expectedSuccess))

	var diresp commontypes.DIJobResponse
	jsonBytes, err := json.Marshal(nresp.Data)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(jsonBytes, &diresp)
	if err != nil {
		return nil, err
	}
	return &diresp, nil
}

func checkDDPLearnersPorts(ctx context.Context, expectedDDPL int) (int, error) {
	// # of ddp learners must as expected
	var ddpLearners corev1.PodList
	err := k8sClient.List(ctx, &ddpLearners, client.MatchingLabels{dicommon.ReplicaTypeLabel: dicommon.DDPLearnerName})
	if err != nil {
		return -1, err
	}
	portCount := 0
	for _, pod := range ddpLearners.Items {
		portCount += len(pod.Spec.Containers[0].Ports)
		for _, port := range pod.Spec.Containers[0].Ports {
			if port.Name == "master-port" {
				portCount--
			}
		}
	}
	return portCount, nil
}
