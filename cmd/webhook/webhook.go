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

package webhook

import (
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"

	cmdcommon "opendilab.org/di-orchestrator/cmd/common"
	div1alpha2 "opendilab.org/di-orchestrator/pkg/api/v1alpha2"
)

type CreateOptions struct {
	MetricAddress        string
	ProbeAddress         string
	EnableLeaderElection bool
	Port                 int
}

func NewCreateOptions() *CreateOptions {
	return &CreateOptions{
		MetricAddress:        ":8443",
		ProbeAddress:         ":8080",
		EnableLeaderElection: false,
		Port:                 9443,
	}
}

func (o *CreateOptions) AddFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&o.MetricAddress, "metric-addr", o.MetricAddress, "The address the metric endpoint binds to.")
	cmd.Flags().StringVar(&o.ProbeAddress, "probe-addr", o.ProbeAddress, "The address the probe endpoint binds to.")
	cmd.Flags().BoolVar(&o.EnableLeaderElection, "leader-elect", o.EnableLeaderElection,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	cmd.Flags().IntVarP(&o.Port, "port", "p", o.Port, "The port the webhook endpoint binds to.")
}

func NewCmdWebhook() *cobra.Command {
	o := NewCreateOptions()
	var webhookCmd = &cobra.Command{
		Use:   "webhook",
		Short: "Command to run di-webhook ",
		Long: `Run di-webhook with specified configuration.

Examples:
	# Start di-webhook with port specified.
	di-orchestrator webhook --port 9443
`,
		Run: func(cmd *cobra.Command, args []string) {
			cobra.CheckErr(runCommand(cmd, o))
		},
	}

	o.AddFlags(webhookCmd)
	return webhookCmd
}

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(div1alpha2.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func runCommand(cmd *cobra.Command, options *CreateOptions) error {
	ctrl.SetLogger(cmdcommon.Logger)

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     options.MetricAddress,
		Port:                   options.Port,
		HealthProbeBindAddress: options.ProbeAddress,
		LeaderElection:         options.EnableLeaderElection,
		LeaderElectionID:       "67841a5d.opendilab.org",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		return err
	}

	if err = (&div1alpha2.DIJob{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "DIJob")
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
