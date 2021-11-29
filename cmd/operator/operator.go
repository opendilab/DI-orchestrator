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
package operator

import (
	"os"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"

	cmdcommon "opendilab.org/di-orchestrator/cmd/common"
	div1alpha1 "opendilab.org/di-orchestrator/pkg/api/v1alpha1"
	dicommon "opendilab.org/di-orchestrator/pkg/common"
	"opendilab.org/di-orchestrator/pkg/controllers"
)

type OperatorOptions struct {
	MetricAddress        string
	ProbeAddress         string
	EnableLeaderElection bool
}

func NewOperatorOptions() *OperatorOptions {
	return &OperatorOptions{
		MetricAddress:        ":8443",
		ProbeAddress:         ":8080",
		EnableLeaderElection: false,
	}
}

func (o *OperatorOptions) AddFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&o.MetricAddress, "metric-addr", o.MetricAddress, "The address the metric endpoint binds to.")
	cmd.Flags().StringVar(&o.ProbeAddress, "probe-addr", o.ProbeAddress, "The address the probe endpoint binds to.")
	cmd.Flags().BoolVar(&o.EnableLeaderElection, "leader-elect", o.EnableLeaderElection,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
}

func NewCmdOperator() *cobra.Command {
	o := NewOperatorOptions()
	var operatorCmd = &cobra.Command{
		Use:   "operator",
		Short: "Command to run di-operator ",
		Long: `Run di-operator with specified configuration.

Examples:
	# Start di-operator with metric address and probe address specified.
	di-orchestrator operator --metric-addr :8443 --probe-addr :8080
`,
		Run: func(cmd *cobra.Command, args []string) {
			cobra.CheckErr(runCommand(cmd, o))
		},
	}

	o.AddFlags(operatorCmd)
	return operatorCmd
}

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(div1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func runCommand(cmd *cobra.Command, options *OperatorOptions) error {
	ctrl.SetLogger(cmdcommon.Logger)

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     options.MetricAddress,
		HealthProbeBindAddress: options.ProbeAddress,
		LeaderElection:         options.EnableLeaderElection,
		LeaderElectionID:       "12841a5d.opendilab.org",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		return err
	}

	// set DefaultDIServerURL
	serverAddr := os.Getenv(dicommon.ServerURLEnv)
	if serverAddr != "" {
		dicommon.DefaultServerURL = serverAddr
	}

	reconciler := &controllers.DIJobReconciler{
		Client:   mgr.GetClient(),
		Log:      ctrl.Log.WithName("controllers").WithName("DIJob"),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("di-orchestrator"),
	}
	if err = reconciler.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DIJob")
		return err
	}

	//+kubebuilder:scaffold:builder

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
