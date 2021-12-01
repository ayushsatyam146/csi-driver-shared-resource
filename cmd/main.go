package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	sharev1clientset "github.com/openshift/client-go/sharedresource/clientset/versioned"
	"github.com/openshift/csi-driver-shared-resource/pkg/client"
	"github.com/openshift/csi-driver-shared-resource/pkg/config"
	"github.com/openshift/csi-driver-shared-resource/pkg/controller"
	"github.com/openshift/csi-driver-shared-resource/pkg/hostpath"

	operatorv1 "github.com/openshift/api/operator/v1"
)

var (
	version string // driver version

	cfgFilePath       string // path to configuration file
	endPoint          string // CSI driver API endpoint for Kubernetes kubelet
	driverName        string // name of the CSI driver, registered in the cluster
	nodeID            string // current Kubernetes node identifier
	maxVolumesPerNode int64  // maximum amount of volumes per node, i.e. per driver instance

	shutdownSignals      = []os.Signal{os.Interrupt, syscall.SIGTERM}
	onlyOneSignalHandler = make(chan struct{})
)

var rootCmd = &cobra.Command{
	Use:     "csi-driver-shared-resource",
	Version: "0.0.1",
	Short:   "",
	Long:    ``,
	Run: func(cmd *cobra.Command, args []string) {
		var err error

		cfgManager := config.NewManager(cfgFilePath)
		cfg, err := cfgManager.LoadConfig()
		if err != nil {
			fmt.Printf("Failed to load configuration file '%s': %s", cfgFilePath, err.Error())
			os.Exit(1)
		}

		if !cfg.RefreshResources {
			fmt.Println("Refresh-Resources disabled")

		}
		if kubeClient, err := loadKubernetesClientset(); err != nil {
			fmt.Printf("Failed to load Kubernetes API client: %s", err.Error())
			os.Exit(1)
		} else {
			client.SetClient(kubeClient)
		}
		if shareClient, err := loadSharedresourceClientset(); err != nil {
			fmt.Printf("Failed to load SharedResource API client: %s", err.Error())
			os.Exit(1)
		} else {
			client.SetShareClient(shareClient)
		}

		driver, err := hostpath.NewHostPathDriver(
			hostpath.DataRoot,
			hostpath.VolumeMapRoot,
			driverName,
			nodeID,
			endPoint,
			maxVolumesPerNode,
			version,
		)
		if err != nil {
			fmt.Printf("Failed to initialize driver: %s", err.Error())
			os.Exit(1)
		}

		go runOperator(cfg, driver)
		go watchForConfigChanges(cfgManager)
		driver.Run()
	},
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	klog.InitFlags(nil)
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	cobra.OnInitialize()
	rootCmd.Flags().AddGoFlagSet(flag.CommandLine)

	rootCmd.Flags().StringVar(&cfgFilePath, "config", "/var/run/configmaps/config/config.yaml", "configuration file path")
	rootCmd.Flags().StringVar(&endPoint, "endpoint", "unix:///csi/csi.sock", "CSI endpoint")
	rootCmd.Flags().StringVar(&driverName, "drivername", string(operatorv1.SharedResourcesCSIDriver), "name of the driver")
	rootCmd.Flags().StringVar(&nodeID, "nodeid", "", "node id")
	rootCmd.Flags().Int64Var(&maxVolumesPerNode, "maxvolumespernode", 0, "limit of volumes per node")
}

// loadKubernetesClientset instantiate a clientset using local config.
func loadKubernetesClientset() (kubernetes.Interface, error) {
	kubeRestConfig, err := client.GetConfig()
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(kubeRestConfig)
}

func loadSharedresourceClientset() (sharev1clientset.Interface, error) {
	kubeRestConfig, err := client.GetConfig()
	if err != nil {
		return nil, err
	}
	return sharev1clientset.NewForConfig(kubeRestConfig)
}

// runOperator based on the informed configuration, it will spawn and run the Controller, until
// trapping OS signals.
func runOperator(cfg *config.Config, hp hostpath.HostPathDriver) {
	c, err := controller.NewController(cfg.GetShareRelistInterval(), cfg.RefreshResources, cfg.IgnoredNamespaces, hp)
	if err != nil {
		fmt.Printf("Failed to set up controller: %s", err.Error())
		os.Exit(1)
	}
	stopCh := setupSignalHandler()
	err = c.Run(stopCh)
	if err != nil {
		fmt.Printf("Controller exited: %s", err.Error())
		os.Exit(1)
	}
}

// watchForConfigChanges keeps checking if the informed configuration has changed, and in this case
// makes the operator exit. The new configuration should take place upon new instance started.
func watchForConfigChanges(mgr *config.Manager) {
	for {
		if mgr.ConfigHasChanged() {
			fmt.Println("Configuration has changed on disk, restarting the operator!")
			os.Exit(0)
		}
		time.Sleep(3 * time.Second)
	}
}

// setupSignalHandler registered for SIGTERM and SIGINT. A stop channel is returned
// which is closed on one of these signals. If a second signal is caught, the program
// is terminated with exit code 1.
func setupSignalHandler() (stopCh <-chan struct{}) {
	close(onlyOneSignalHandler) // panics when called twice

	stop := make(chan struct{})
	c := make(chan os.Signal, 2)
	signal.Notify(c, shutdownSignals...)
	go func() {
		<-c
		close(stop)
		<-c
		os.Exit(1) // second signal. Exit directly.
	}()

	return stop
}
