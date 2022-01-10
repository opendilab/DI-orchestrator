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
	"context"
	"time"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"

	cmdcommon "opendilab.org/di-orchestrator/cmd/common"
	alloc "opendilab.org/di-orchestrator/pkg/allocator"
	alloctypes "opendilab.org/di-orchestrator/pkg/allocator/types"
	div1alpha2 "opendilab.org/di-orchestrator/pkg/api/v1alpha2"
	dicontext "opendilab.org/di-orchestrator/pkg/context"
	"opendilab.org/di-orchestrator/pkg/controllers"
)

type CreateOptions struct {
	SyncPeriod           *time.Duration
	MetricAddress        string
	ProbeAddress         string
	EnableLeaderElection bool
}

func NewCreateOptions() *CreateOptions {
	DefaultSyncPeriod := 1 * time.Minute
	DefaultMetricAddress := ":8443"
	DefaultProbeAddress := ":8080"
	DefaultEnableLeaderElection := false
	return &CreateOptions{
		SyncPeriod:           &DefaultSyncPeriod,
		MetricAddress:        DefaultMetricAddress,
		ProbeAddress:         DefaultProbeAddress,
		EnableLeaderElection: DefaultEnableLeaderElection,
	}
}

func (o *CreateOptions) AddFlags(cmd *cobra.Command) {
	cmd.Flags().DurationVar(o.SyncPeriod, "sync-period", *o.SyncPeriod, "Resync period for controllers.")
	cmd.Flags().StringVar(&o.MetricAddress, "metric-addr", o.MetricAddress, "The address the metric endpoint binds to.")
	cmd.Flags().StringVar(&o.ProbeAddress, "probe-addr", o.ProbeAddress, "The address the probe endpoint binds to.")
	cmd.Flags().BoolVar(&o.EnableLeaderElection, "leader-elect", o.EnableLeaderElection,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
}

func NewCmdOperator() *cobra.Command {
	o := NewCreateOptions()
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

	utilruntime.Must(div1alpha2.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func runCommand(cmd *cobra.Command, options *CreateOptions) error {
	ctrl.SetLogger(cmdcommon.Logger)

	config := ctrl.GetConfigOrDie()
	mgr, err := ctrl.NewManager(config, ctrl.Options{
		Scheme:                 scheme,
		SyncPeriod:             options.SyncPeriod,
		MetricsBindAddress:     options.MetricAddress,
		HealthProbeBindAddress: options.ProbeAddress,
		LeaderElection:         options.EnableLeaderElection,
		LeaderElectionID:       "12841a5d.opendilab.org",
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
	reconciler := controllers.NewDIJobReconciler(mgr.GetScheme(), ctx)
	if err = reconciler.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DIJob")
		return err
	}

	ctx = dicontext.NewContext(context.Background(),
		config,
		mgr.GetClient(),
		mgr.GetEventRecorderFor("di-allocator"),
		ctrl.Log.WithName("di-allocator"))
	allocator := alloc.NewAllocator(mgr.GetScheme(), ctx, *alloctypes.NewFitPolicy(), *options.SyncPeriod)
	if err = allocator.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create allocator", "allocator", "DIJob")
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
