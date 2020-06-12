package kube_port_forward

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

func NewRequest(kubeconfig, context, namespace, podName string) (*PortForwardAPodRequest, error) {
	config, err := buildConfigFromFlags(context, kubeconfig)
	if err != nil {
		return nil, err
	}
	req := &PortForwardAPodRequest{
		RestConfig: config,
		Pod: v1.Pod{
			ObjectMeta: metav1.ObjectMeta {
				Name: podName,
				Namespace: namespace,
			},
		},
		Streams: genericclioptions.IOStreams{
			In: os.Stdin,
			Out: os.Stdout,
			ErrOut: os.Stderr,
		},
		StopCh: make(chan struct{}, 1),
		ReadyCh: make(chan struct{}),
	}
	

	return req, nil
}

func buildConfigFromFlags(context, kubeconfigPath string) (*rest.Config, error) {
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath},
		&clientcmd.ConfigOverrides{
			CurrentContext: context,
		}).ClientConfig()
}

type PortForwardAPodRequest struct {
	// RestConfig is the kubernetes config
	RestConfig *rest.Config
	// Pod is the selected pod for this port forwarding
	Pod v1.Pod
	// LocalPort is the local port that will be selected to expose the PodPort
	LocalPort int
	// PodPort is the target port for the pod
	PodPort int
	// Steams configures where to write or read input from
	Streams genericclioptions.IOStreams
	// StopCh is the channel used to manage the port forward lifecycle
	StopCh <-chan struct{}
	// ReadyCh communicates when the tunnel is ready to receive traffic
	ReadyCh chan struct{}
}

func WaitForForwarding(req PortForwardAPodRequest) error {
	readyCh := req.ReadyCh
	failureCh := make(chan error)
	go func() {
		err := PortForwardAPod(req)
		if err != nil {
			failureCh <- err
			panic(err.Error())
		}
	}()

	select {
	case err := <-failureCh:
		return err
	case <-readyCh:
		return nil
	}
}

func PortForwardAPod(req PortForwardAPodRequest) error {
	path := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/portforward",
		req.Pod.Namespace, req.Pod.Name)
	hostIP := strings.TrimLeft(req.RestConfig.Host, "htps:/")

	transport, upgrader, err := spdy.RoundTripperFor(req.RestConfig)
	if err != nil {
		return err
	}

	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, &url.URL{Scheme: "https", Path: path, Host: hostIP})
	fw, err := portforward.New(dialer, []string{fmt.Sprintf("%d:%d", req.LocalPort, req.PodPort)}, req.StopCh, req.ReadyCh, req.Streams.Out, req.Streams.ErrOut)
	if err != nil {
		return err
	}
	return fw.ForwardPorts()
}

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // windows
}
