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
package common

import (
	goflag "flag"

	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

type GenericFlags struct {
	QPS               float64
	Burst             int
	ServiceDomainName string
	DIServerURL       string
	ZapOpts           *zap.Options
}

func NewGenericFlags() *GenericFlags {
	return &GenericFlags{
		QPS:               5,
		Burst:             10,
		ServiceDomainName: "svc.cluster.local",
		DIServerURL:       "http://di-server.di-system.svc.cluster.local:8081",
		ZapOpts:           &zap.Options{},
	}
}

func (f *GenericFlags) AddFlags(cmd *cobra.Command) {
	goflag.Float64Var(&f.QPS, "qps", f.QPS, "qps for k8s client")
	goflag.IntVar(&f.Burst, "burst", f.Burst, "burst for k8s client")
	goflag.StringVar(&f.ServiceDomainName, "service-domain-name", f.ServiceDomainName, "k8s service domain name")
	goflag.StringVar(&f.DIServerURL, "di-server-url", f.DIServerURL, "url for accessing di server")
	f.ZapOpts.BindFlags(goflag.CommandLine)
}
