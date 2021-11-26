package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	dicommon "opendilab.org/di-orchestrator/pkg/common"
	gpualloc "opendilab.org/di-orchestrator/pkg/common/gpuallocator"
	serverdynamic "opendilab.org/di-orchestrator/pkg/server/dynamic"
	serverhttp "opendilab.org/di-orchestrator/pkg/server/http"
)

var (
	DefaultLeaseLockNamespace = "di-system"
	DefaultLeaseLockName      = "di-server"

	DefaultAGConfigNamespace = "di-system"
	DefaultAGConfigName      = "aggregator-config"
)

func main() {
	var kubeconfig, serverBindAddress, leaseLockName, leaseLockNamespace, agconfigNamespace, agconfigName string
	var gpuAllocPolicy, serverAddr string
	var enableLeaderElection bool
	if flag.Lookup("kubeconfig") == nil {
		flag.StringVar(&kubeconfig, "kubeconfig", "", "The kubeconfig file to access kubernetes cluster. ")
	}

	flag.StringVar(&serverBindAddress, "server-bind-address", ":8080", "The address for server to bind to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&leaseLockNamespace, "lease-lock-namespace", DefaultLeaseLockNamespace, "The lease lock resource namespace")
	flag.StringVar(&leaseLockName, "lease-lock-name", DefaultLeaseLockName, "The lease lock resource name")
	flag.StringVar(&agconfigNamespace, "agconfig-namespace", DefaultAGConfigNamespace, "The AggregatorConfig namespace to manage actors and learners.")
	flag.StringVar(&agconfigName, "agconfig-name", DefaultAGConfigName, "The AggregatorConfig name to manage actors and learners.")
	flag.StringVar(&gpuAllocPolicy, "gpu-alloc-policy", gpualloc.SimpleGPUAllocPolicy, "The policy for server to allocate gpus to pods.")
	flag.StringVar(&serverAddr, "server-address", dicommon.DefaultServerURL, "The address to connect to  server.")
	flag.Parse()

	kubeconfig = flag.Lookup("kubeconfig").Value.String()

	cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		log.Fatalf("Failed to get kubeconfig: %v", err)
	}

	kubeClient := kubernetes.NewForConfigOrDie(cfg)
	dynamicClient := dynamic.NewForConfigOrDie(cfg)

	dif := dynamicinformer.NewFilteredDynamicSharedInformerFactory(dynamicClient, serverdynamic.ResyncPeriod, corev1.NamespaceAll, nil)

	dyi := serverdynamic.NewDynamicInformer(dif)

	// start dynamic informer
	stopCh := make(chan struct{})
	go dif.Start(stopCh)

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	logger := zap.New(zap.UseFlagOptions(&opts))

	agconfig := fmt.Sprintf("%s/%s", agconfigNamespace, agconfigName)
	diServer := serverhttp.NewDIServer(kubeClient, dynamicClient, logger, agconfig, dyi, gpuAllocPolicy)
	dicommon.DefaultServerURL = serverAddr

	if !enableLeaderElection {
		if err := diServer.Start(serverBindAddress); err != nil {
			log.Fatalf("Failed to start DIServer: %v", err)
		}
		return
	}

	run := func(ctx context.Context) {
		if err := diServer.Start(serverBindAddress); err != nil {
			log.Fatalf("Failed to start DIServer: %v", err)
		}
	}

	// use a Go context so we can tell the leaderelection code when we
	// want to step down
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// listen for interrupts or the Linux SIGTERM signal and cancel
	// our context, which the leader election code will observe and
	// step down
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-ch
		log.Println("Received termination, signaling shutdown")
		cancel()
	}()

	// we use the Lease lock type since edits to Leases are less common
	// and fewer objects in the cluster watch "all Leases".
	id := fmt.Sprintf("%s.opendilab.org", uuid.New().String())
	lock := &resourcelock.LeaseLock{
		LeaseMeta: metav1.ObjectMeta{
			Name:      leaseLockName,
			Namespace: leaseLockNamespace,
		},
		Client: kubeClient.CoordinationV1(),
		LockConfig: resourcelock.ResourceLockConfig{
			Identity: id,
		},
	}

	// start the leader election code loop
	leaderelection.RunOrDie(ctx, leaderelection.LeaderElectionConfig{
		Lock: lock,
		// IMPORTANT: you MUST ensure that any code you have that
		// is protected by the lease must terminate **before**
		// you call cancel. Otherwise, you could have a background
		// loop still running and another process could
		// get elected before your background loop finished, violating
		// the stated goal of the lease.
		ReleaseOnCancel: true,
		LeaseDuration:   60 * time.Second,
		RenewDeadline:   15 * time.Second,
		RetryPeriod:     5 * time.Second,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(ctx context.Context) {
				// we're notified when we start - this is where you would
				// usually put your code
				log.Printf("leader elected: %s\n", id)
				run(ctx)
			},
			OnStoppedLeading: func() {
				// we can do cleanup here
				log.Printf("leader lost: %s\n", id)
				os.Exit(0)
			},
			OnNewLeader: func(identity string) {
				// we're notified when new leader elected
				if identity == id {
					// I just got the lock
					return
				}
				log.Printf("new leader elected: %s\n", identity)
			},
		},
	})
}
