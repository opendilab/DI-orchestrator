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
package cmd

import (
	goflag "flag"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"opendilab.org/di-orchestrator/cmd/operator"
	"opendilab.org/di-orchestrator/cmd/server"
)

// rootCmd represents the base command when called without any subcommands
var (
	rootCmd = &cobra.Command{
		Use:   "di-orchestrator",
		Short: "A component responsible for managing DI-engine jobs.",
		Long: `DI Orchestrator is a component responsible for managing DI-engine jobs in Kubernetes cluster.
This application allows you to run di-operator, di-server and di-webhook locally with comandline.`,
		// Uncomment the following line if your bare application
		// has an action associated with it:
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Help()
		},
	}
)

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	cobra.CheckErr(rootCmd.Execute())
}

func init() {
	rootCmd.AddCommand(server.NewCmdServer())
	rootCmd.AddCommand(operator.NewCmdOperator())

	// add all the flags in go flagset into pflagset
	pflag.CommandLine.AddGoFlagSet(goflag.CommandLine)
}
