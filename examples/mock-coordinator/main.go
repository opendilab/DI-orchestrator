package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"
)

var (
	DefaultNervexServer = "http://nervex-server.nervex-system:8080"
	NamespaceEnv        = "KUBERNETES_POD_NAMESPACE"
	NameEnv             = "KUBERNETES_POD_NAME"
)

func main() {
	var nervexServer string
	flag.StringVar(&nervexServer, "nervex-server", DefaultNervexServer, " URL to nervex-server.")
	flag.Parse()

	namespace := os.Getenv(NamespaceEnv)
	if namespace == "" {
		namespace = "default"
	}
	name := os.Getenv(NameEnv)
	if name == "" {
		name = "default"
	}

	req := map[string]interface{}{
		"namespace":   namespace,
		"coordinator": name,
		"actors": map[string]interface{}{
			"cpu":      "0.1",
			"memory":   "50Mi",
			"replicas": 1,
		},
		"learners": map[string]interface{}{
			"cpu":      "0.1",
			"memory":   "50Mi",
			"gpu":      "0",
			"replicas": 1,
		},
	}
	actors, learners, err := add(nervexServer, req)
	if err != nil {
		panic(err)
	}

	fmt.Printf("created actors: %v\n", actors)
	fmt.Printf("created learners: %v\n", learners)
	waitForStart(actors, 1*time.Minute)
	waitForStart(learners, 1*time.Minute)
}

func add(server string, req map[string]interface{}) (actors []interface{}, learners []interface{}, err error) {
	reqJson, err := json.Marshal(req)
	if err != nil {
		fmt.Printf("failed to marshal\n")
		return
	}

	url := fmt.Sprintf("%s%s", server, "/api/v1alpha1/add")
	fmt.Printf("%s\n", url)
	resp, err := http.Post(url, "application/json", bytes.NewReader(reqJson))
	if err != nil {
		fmt.Printf("failed to post\n")
		return
	}

	result, err := ioutil.ReadAll(resp.Body)
	fmt.Printf("%s\n", string(result))

	respJson := make(map[string]interface{}, 0)
	err = json.Unmarshal(result, &respJson)
	if err != nil {
		fmt.Printf("failed to unmarshal\n")
		return
	}

	return respJson["actors"].([]interface{}), respJson["learners"].([]interface{}), nil
}

func delete(server string, req map[string]interface{}) {

}

func waitForStart(pods []interface{}, timeout time.Duration) {
	fmt.Printf("wait for pod to start...\n")
	start := time.Now()
	for {
		startCount := 0
		for _, pod := range pods {
			url := fmt.Sprintf("%s%s", "http://", pod.(string))
			resp, err := http.Get(url)
			if err != nil {
				fmt.Printf("%v\n", err)
				continue
			}
			if resp.StatusCode != http.StatusOK {

			}
			startCount++
			fmt.Printf("pod %s is ready\n", pod.(string))
		}

		time.Sleep(2 * time.Second)
		if startCount == len(pods) {
			break
		}

		if time.Now().Sub(start) > timeout {
			fmt.Printf("timeout waiting for pods to start!\n")
			break
		}
	}

}
