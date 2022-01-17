/*
Copyright 2021 The OpenDILab authors.

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
package server

import (
	"fmt"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	cmdcommon "opendilab.org/di-orchestrator/cmd/common"
	div2alpha1 "opendilab.org/di-orchestrator/pkg/api/v2alpha1"
	gpualloc "opendilab.org/di-orchestrator/pkg/common/gpuallocator"
	serverdynamic "opendilab.org/di-orchestrator/pkg/server/dynamic"
	serverhttp "opendilab.org/di-orchestrator/pkg/server/http"
)

type CreateOptions struct {
	cmdcommon.GenericFlags

	ServerBindAddress string
	GPUAllocPolicy    string
}

func NewCreateOptions(genFlags cmdcommon.GenericFlags) *CreateOptions {
	return &CreateOptions{
		GenericFlags:      genFlags,
		ServerBindAddress: ":8080",
		GPUAllocPolicy:    gpualloc.SimpleGPUAllocPolicy,
	}
}

func (o *CreateOptions) AddFlags(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&o.ServerBindAddress, "server-bind-address", "b", o.ServerBindAddress,
		"The address for server to bind to.")
	cmd.Flags().StringVarP(&o.GPUAllocPolicy, "gpu-alloc-policy", "p", o.GPUAllocPolicy,
		"The policy for server to allocate gpus to pods.")
}

// serverCmd represents the server command
func NewCmdServer(genFlags cmdcommon.GenericFlags) *cobra.Command {
	o := NewCreateOptions(genFlags)
	var serverCmd = &cobra.Command{
		Use:   "server",
		Short: "Command to run di-server ",
		Long: `Run di-server with specified configuration.

Examples:
	# Start di-server with gpu allocation policy and bind address specified.
	di-orchestrator server -p simple -b :8080
`,
		Run: func(cmd *cobra.Command, args []string) {
			cobra.CheckErr(runCommand(cmd, o))
		},
	}

	o.AddFlags(serverCmd)
	return serverCmd
}

func runCommand(cmd *cobra.Command, options *CreateOptions) error {
	logger := zap.New(zap.UseFlagOptions(&options.GenericFlags.ZapOpts))
	cfg, err := ctrl.GetConfig()
	if err != nil {
		return err
	}

	kubeClient := kubernetes.NewForConfigOrDie(cfg)
	dynamicClient := dynamic.NewForConfigOrDie(cfg)
	dif := dynamicinformer.NewFilteredDynamicSharedInformerFactory(dynamicClient, serverdynamic.ResyncPeriod, corev1.NamespaceAll, nil)
	dyi := serverdynamic.NewDynamicInformer(dif)
	// start dynamic informer
	stopCh := make(chan struct{})
	go dif.Start(stopCh)

	diGVR := schema.GroupVersionResource{
		Group:    div2alpha1.GroupVersion.Group,
		Version:  div2alpha1.GroupVersion.Version,
		Resource: "dijobs",
	}
	diclient := dynamicClient.Resource(diGVR)

	diServer := serverhttp.NewDIServer(kubeClient, diclient, logger, dyi, options.GPUAllocPolicy)
	if err := diServer.Start(options.ServerBindAddress); err != nil {
		return fmt.Errorf("failed to start di-server: %v", err)
	}
	return nil
}
