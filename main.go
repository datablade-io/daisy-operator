/*
Copyright 2020.

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

package main

import (
	"flag"
	"os"

	"github.com/spf13/pflag"

	"github.com/daisy/daisy-operator/controllers/daisymanager"

	v1 "github.com/daisy/daisy-operator/api/v1"
	"github.com/daisy/daisy-operator/controllers"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	daisycomv1 "github.com/daisy/daisy-operator/api/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	zapcr "sigs.k8s.io/controller-runtime/pkg/log/zap"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(v1.AddToScheme(scheme))
	utilruntime.Must(daisycomv1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var configPath string
	var certDir string

	pflag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	pflag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	pflag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	pflag.StringVar(&configPath, "config", "./config.yaml", "The path of daisy operator config file")
	pflag.StringVar(&certDir, "cert-dir", "", "The path of certificate used by webhook")

	opts := zapcr.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)

	// Add flags registered by imported packages (e.g. glog and
	// controller-runtime)
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)

	// Add the zap logger flag set to the CLI. The flag set must
	// be added before calling pflag.Parse().
	//pflag.CommandLine.AddFlagSet(zapcr())
	pflag.Parse()

	//encoderConfig := zap.NewDevelopmentEncoderConfig()
	//encoderConfig.EncodeCaller = zapcore.ShortCallerEncoder
	//encoderConfig.CallerKey = "caller"
	//consoleEncoder := zapcore.NewConsoleEncoder(encoderConfig)

	//logger := zapcr.New(zapcr.WriteTo(os.Stdout), zapcr.Encoder(consoleEncoder), zapcr.RawZapOpts(zap.AddCaller()))
	logger := zapcr.New(zapcr.UseFlagOptions(&opts))

	ctrl.SetLogger(logger)
	//ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "8a30d6a4.daisy.com",
		CertDir:                certDir,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	deps := &daisymanager.Dependencies{
		Client:   mgr.GetClient(),
		Log:      ctrl.Log.WithName("controllers").WithName("DaisyInstallation"),
		Recorder: mgr.GetEventRecorderFor("DaisyController"),
	}
	var dmm daisymanager.Manager
	var cfgMgr *daisymanager.ConfigManager
	if cfgMgr, err = daisymanager.NewConfigManager(deps.Client, configPath); err != nil {
		setupLog.Error(err, "unable to create config manager", "configPath", configPath)
		os.Exit(1)
	}

	if dmm, err = daisymanager.NewDaisyMemberManager(deps, cfgMgr); err != nil {
		setupLog.Error(err, "unable to create daisy member manager", "configPath", configPath)
		os.Exit(1)
	}

	if err = (&controllers.DaisyInstallationReconciler{
		Client:   deps.Client,
		Log:      deps.Log,
		Recorder: deps.Recorder,
		Scheme:   mgr.GetScheme(),
		DMM:      dmm,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DaisyInstallation")
		os.Exit(1)
	}

	if err = (&v1.DaisyInstallation{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "DaisyInstallation")
		os.Exit(1)
	}

	if err = (&controllers.DaisyTemplateReconciler{
		Client: mgr.GetClient(),
		CfgMgr: cfgMgr,
		Log:    ctrl.Log.WithName("controllers").WithName("DaisyTemplate"),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DaisyTemplate")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("health", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("check", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
