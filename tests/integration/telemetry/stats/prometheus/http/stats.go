// +build integ
// Copyright Istio Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package http

import (
	"fmt"
	"testing"
	"time"

	"istio.io/istio/pkg/config/protocol"
	"istio.io/istio/pkg/test/framework"
	"istio.io/istio/pkg/test/framework/components/echo"
	"istio.io/istio/pkg/test/framework/components/echo/echoboot"
	"istio.io/istio/pkg/test/framework/components/istio"
	"istio.io/istio/pkg/test/framework/components/namespace"
	"istio.io/istio/pkg/test/framework/components/prometheus"
	"istio.io/istio/pkg/test/framework/features"
	"istio.io/istio/pkg/test/framework/resource"
	"istio.io/istio/pkg/test/util/retry"
	util "istio.io/istio/tests/integration/telemetry"
	promUtil "istio.io/istio/tests/integration/telemetry/stats/prometheus"
)

var (
	client, server echo.Instances
	ist            istio.Instance
	appNsInst      namespace.Instance
	promInst       prometheus.Instance
)

// GetIstioInstance gets Istio instance.
func GetIstioInstance() *istio.Instance {
	return &ist
}

// GetAppNamespace gets bookinfo instance.
func GetAppNamespace() namespace.Instance {
	return appNsInst
}

// GetPromInstance gets prometheus instance.
func GetPromInstance() prometheus.Instance {
	return promInst
}

// TestStatsFilter includes common test logic for stats and mx exchange filters running
// with nullvm and wasm runtime.
func TestStatsFilter(t *testing.T, feature features.Feature) {
	framework.NewTest(t).
		Features(feature).
		Run(func(ctx framework.TestContext) {
			sourceQuery, destinationQuery, appQuery := buildQuery()
			retry.UntilSuccessOrFail(t, func() error {
				if err := SendTraffic(); err != nil {
					return err
				}
				for _, c := range ctx.Clusters() {
					// Query client side metrics
					if err := promUtil.QueryPrometheus(t, c, sourceQuery, GetPromInstance()); err != nil {
						t.Logf("prometheus values for istio_requests_total: \n%s", util.PromDump(c, promInst, "istio_requests_total"))
						return err
					}
					if err := promUtil.QueryPrometheus(t, c, destinationQuery, GetPromInstance()); err != nil {
						t.Logf("prometheus values for istio_requests_total: \n%s", util.PromDump(c, promInst, "istio_requests_total"))
						return err
					}
					// This query will continue to increase due to readiness probe; don't wait for it to converge
					if err := promUtil.QueryFirstPrometheus(t, c, appQuery, GetPromInstance()); err != nil {
						t.Logf("prometheus values for istio_echo_http_requests_total: \n%s", util.PromDump(c, promInst, "istio_echo_http_requests_total"))
						return err
					}
				}
				return nil
			}, retry.Delay(3*time.Second), retry.Timeout(80*time.Second))
		})
}

// TestSetup set up bookinfo app for stats testing.
func TestSetup(ctx resource.Context) (err error) {
	appNsInst, err = namespace.New(ctx, namespace.Config{
		Prefix: "echo",
		Inject: true,
	})
	if err != nil {
		return
	}

	builder := echoboot.NewBuilder(ctx)
	for _, c := range ctx.Clusters() {
		builder.
			With(nil, echo.Config{
				Service:   "client",
				Namespace: appNsInst,
				Cluster:   c,
				Ports:     nil,
				Subsets:   []echo.SubsetConfig{{}},
			}).
			With(nil, echo.Config{
				Service:   "server",
				Namespace: appNsInst,
				Cluster:   c,
				Subsets:   []echo.SubsetConfig{{}},
				Ports: []echo.Port{
					{
						Name:         "http",
						Protocol:     protocol.HTTP,
						InstancePort: 8090,
					},
				},
			}).
			Build()
	}
	echos, err := builder.Build()
	if err != nil {
		return err
	}
	client = echos.Match(echo.Service("client"))
	server = echos.Match(echo.Service("server"))
	promInst, err = prometheus.New(ctx, prometheus.Config{})
	if err != nil {
		return
	}
	return nil
}

// SendTraffic makes a client call to the "server" service on the http port.
func SendTraffic() error {
	for _, cltInstance := range client {
		_, err := cltInstance.Call(echo.CallOptions{
			Target:   server[0],
			PortName: "http",
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// BuildQueryCommon is the shared function to construct prom query for istio_request_total metric.
func BuildQueryCommon(labels map[string]string, ns string) (sourceQuery, destinationQuery, appQuery string) {
	sourceQuery = `istio_requests_total{reporter="source",`
	destinationQuery = `istio_requests_total{reporter="destination",`

	for k, v := range labels {
		sourceQuery += fmt.Sprintf(`%s=%q,`, k, v)
		destinationQuery += fmt.Sprintf(`%s=%q,`, k, v)
	}
	sourceQuery += "}"
	destinationQuery += "}"
	appQuery += `istio_echo_http_requests_total{kubernetes_namespace="` + ns + `"}`
	return
}

func buildQuery() (sourceQuery, destinationQuery, appQuery string) {
	ns := GetAppNamespace()
	labels := map[string]string{
		"request_protocol":               "http",
		"response_code":                  "200",
		"destination_app":                "server",
		"destination_version":            "v1",
		"destination_service":            "server." + ns.Name() + ".svc.cluster.local",
		"destination_service_name":       "server",
		"destination_workload_namespace": ns.Name(),
		"destination_service_namespace":  ns.Name(),
		"source_app":                     "client",
		"source_version":                 "v1",
		"source_workload":                "client-v1",
		"source_workload_namespace":      ns.Name(),
	}

	return BuildQueryCommon(labels, ns.Name())
}
