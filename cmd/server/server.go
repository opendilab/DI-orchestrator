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
	"context"
	"flag"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	cmdcommon "opendilab.org/di-orchestrator/cmd/common"
	div2alpha1 "opendilab.org/di-orchestrator/pkg/api/v2alpha1"
	dicontext "opendilab.org/di-orchestrator/pkg/context"
	"opendilab.org/di-orchestrator/pkg/server"
)

type CreateOptions struct {
	cmdcommon.GenericFlags

	ServerBindAddress string
	ProbeAddress      string
	MetricAddress     string
}

func NewCreateOptions(genFlags cmdcommon.GenericFlags) *CreateOptions {
	return &CreateOptions{
		GenericFlags:      genFlags,
		ServerBindAddress: ":8081",
		ProbeAddress:      ":8080",
		MetricAddress:     ":8089",
	}
}

func (o *CreateOptions) AddFlags(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&o.ServerBindAddress, "server-bind-address", "s", o.ServerBindAddress,
		"The address for server to bind to.")
	cmd.Flags().StringVarP(&o.ProbeAddress, "probe-address", "p", o.ProbeAddress,
		"The address for probe to connect to.")
	cmd.Flags().StringVar(&o.MetricAddress, "metric-addr", o.MetricAddress, "The address the metric endpoint binds to.")
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

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(div2alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func runCommand(cmd *cobra.Command, options *CreateOptions) error {
	flag.Parse()
	logger := zap.New(zap.UseFlagOptions(options.GenericFlags.ZapOpts))
	ctrl.SetLogger(logger)

	config := ctrl.GetConfigOrDie()
	mgr, err := ctrl.NewManager(config, ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     options.MetricAddress,
		HealthProbeBindAddress: options.ProbeAddress,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		return err
	}

	ctx := dicontext.NewContext(context.Background(),
		config,
		mgr.GetClient(),
		mgr.GetEventRecorderFor("di-operator"),
		ctrl.Log.WithName("di-operator"))
	diServer := server.NewDIServer(ctx, options.ServerBindAddress)
	mgr.Add(diServer)

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		return err
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		return err
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		return err
	}
	return nil
}
