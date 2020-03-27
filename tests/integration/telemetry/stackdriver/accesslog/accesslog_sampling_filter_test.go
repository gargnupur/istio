// Copyright 2019 Istio Authors
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

package tcp

import (
	"testing"

	"fmt"
	"time"

	"istio.io/istio/pkg/test/framework"
	"istio.io/istio/pkg/test/framework/components/bookinfo"
	"istio.io/istio/pkg/test/framework/components/galley"
	"istio.io/istio/pkg/test/framework/components/ingress"
	"istio.io/istio/pkg/test/framework/components/istio"
	"istio.io/istio/pkg/test/framework/components/namespace"
	"istio.io/istio/pkg/test/framework/label"
	"istio.io/istio/pkg/test/framework/resource"
	"istio.io/istio/pkg/test/framework/resource/environment"
	util "istio.io/istio/tests/integration/mixer"
)

var (
	ist        istio.Instance
	bookinfoNs namespace.Instance
	g          galley.Instance
	ing        ingress.Instance
)

func TestAccessLog(t *testing.T) { // nolint:interfacer
	framework.
		NewTest(t).
		RequiresEnvironment(environment.Kube).
		Run(func(ctx framework.TestContext) {
			addr := ing.HTTPAddress()
			url := fmt.Sprintf("http://%s/productpage", addr.String())
			t.Logf("url: %v", url)
			t.Logf("url: %v", url)
			t.Logf("url: %v", url)

			util.AllowRuleSync(t)

			util.AllowRuleSync(t)
			/*systemNM := namespace.ClaimSystemNamespaceOrFail(ctx, ctx)
			acc, err := file.AsString(accesslogConfig)
			if err != nil {
				t.Errorf("unable to load config %s, err:%v", accesslogConfig, err)
			}

			g.ApplyConfigOrFail(
				t,
				systemNM,
				acc,
			)
			defer g.DeleteConfig(
				systemNM,
				acc,
			)

			util.AllowRuleSync(t)
			st, err := file.AsString(stackdriverConfig)
			if err != nil {
				t.Errorf("unable to load config %s, err:%v", stackdriverConfig, err)
			}
			g.ApplyConfigOrFail(
				t,
				systemNM,
				st,
			)
			defer g.DeleteConfig(
				systemNM,
				st,
			)
			retry.UntilSuccessOrFail(t, func() error {
				util.SendTraffic(ing, t, "Sending traffic", url, "", 200)
				return nil
			}, retry.Delay(3*time.Second), retry.Timeout(80*time.Second))*/

			time.Sleep(time.Minute * 240)
		})
}

func TestMain(m *testing.M) {
	framework.
		NewSuite("accesslog_filter", m).
		RequireEnvironment(environment.Kube).
		Label(label.CustomSetup).
		SetupOnEnv(environment.Kube, istio.Setup(&ist, setupConfig)).
		Setup(testsetup).
		Run()
}

func setupConfig(cfg *istio.Config) {
	if cfg == nil {
		return
	}
	// disable mixer telemetry and enable telemetry v2
	// This turns on telemetry v2 for both HTTP and TCP.
	// disable mixer telemetry and enable stackdriver filter
	cfg.Values["telemetry.enabled"] = "true"
	cfg.Values["telemetry.v1.enabled"] = "false"
	cfg.Values["telemetry.v2.enabled"] = "true"
	cfg.Values["telemetry.v2.stackdriver.enabled"] = "true"
	cfg.Values["telemetry.v2.stackdriver.logging"] = "true"
	cfg.Values["telemetry.v2.accessLog.enabled"] = "true"
	cfg.Values["telemetry.v2.accessLog.logWindowDuration"] = "43200s"
	//cfg.Values["telemetry.v2.stackdriver.enabled"] = "true"
	//cfg.Values["telemetry.v2.stackdriver.logging"] = "true"
	//cfg.Values["telemetry.v2.stackdriver.configOverride"] = "false"
	//cfg.Values["prometheus.enabled"] = "true"
	cfg.Values["global.proxy.logLevel"] = "debug"
	cfg.Values["global.proxy.componentLogLevel"] = "misc:info"
}

func testsetup(ctx resource.Context) (err error) {
	bookinfoNs, err = namespace.New(ctx, namespace.Config{
		Prefix: "istio-bookinfo",
		Inject: true,
	})
	if err != nil {
		return
	}
	if _, err := bookinfo.Deploy(ctx, bookinfo.Config{Namespace: bookinfoNs, Cfg: bookinfo.BookInfo}); err != nil {
		return err
	}
	if _, err = bookinfo.Deploy(ctx, bookinfo.Config{Namespace: bookinfoNs, Cfg: bookinfo.BookinfoRatingsv2}); err != nil {
		return err
	}
	if _, err = bookinfo.Deploy(ctx, bookinfo.Config{Namespace: bookinfoNs, Cfg: bookinfo.BookinfoDB}); err != nil {
		return err
	}
	g, err = galley.New(ctx, galley.Config{})
	if err != nil {
		return err
	}
	ing, err = ingress.New(ctx, ingress.Config{Istio: ist})
	if err != nil {
		return err
	}

	yamlText, err := bookinfo.NetworkingBookinfoGateway.LoadGatewayFileWithNamespace(bookinfoNs.Name())
	if err != nil {
		return err
	}
	err = g.ApplyConfig(bookinfoNs, yamlText)
	if err != nil {
		return err
	}

	return nil
}

func buildQuery() (destinationQuery string) {
	destinationQuery = `istio_tcp_connections_opened_total{reporter="destination",`
	labels := map[string]string{
		"request_protocol":               "tcp",
		"response_code":                  "0",
		"destination_app":                "mongodb",
		"destination_version":            "v1",
		"destination_workload_namespace": bookinfoNs.Name(),
		"destination_service_namespace":  bookinfoNs.Name(),
		"source_app":                     "ratings",
		"source_version":                 "v2",
		"source_workload":                "ratings-v2",
		"source_workload_namespace":      bookinfoNs.Name(),
	}
	for k, v := range labels {
		destinationQuery += fmt.Sprintf(`%s=%q,`, k, v)
	}
	destinationQuery += "}"
	return
}
