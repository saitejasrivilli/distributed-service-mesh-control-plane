// Command k8s-watcher bridges Kubernetes Endpoints to the control plane's
// service registry: it watches a Service's EndpointSlices and registers,
// heartbeats, and deregisters instances so pod IPs are never hardcoded and
// scaling a Deployment up/down is reflected in the mesh automatically.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func main() {
	namespace := flag.String("namespace", "default", "namespace to watch")
	service := flag.String("service", "backend-a", "Kubernetes Service name whose endpoints to watch")
	controlPlaneAddr := flag.String("control-plane-addr", "http://control-plane:8080", "control-plane management API base URL")
	port := flag.Int("port", 9000, "backend port to register instances with")
	pollInterval := flag.Duration("poll-interval", 2*time.Second, "how often to re-list endpoints")
	kubeconfig := flag.String("kubeconfig", "", "path to kubeconfig; empty uses in-cluster config")
	flag.Parse()

	cfg, err := restConfig(*kubeconfig)
	if err != nil {
		log.Fatalf("build kube config: %v", err)
	}
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		log.Fatalf("build kube client: %v", err)
	}

	w := &watcher{
		clientset:  clientset,
		namespace:  *namespace,
		service:    *service,
		cpAddr:     *controlPlaneAddr,
		port:       *port,
		registered: make(map[string]bool),
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}

	log.Printf("k8s-watcher: watching endpoints for service %s/%s, registering against %s", *namespace, *service, *controlPlaneAddr)
	ctx := context.Background()
	for {
		if err := w.reconcile(ctx); err != nil {
			log.Printf("reconcile error: %v", err)
		}
		time.Sleep(*pollInterval)
	}
}

func restConfig(kubeconfig string) (*rest.Config, error) {
	if kubeconfig != "" {
		return clientcmd.BuildConfigFromFlags("", kubeconfig)
	}
	return rest.InClusterConfig()
}

type watcher struct {
	clientset  kubernetes.Interface
	namespace  string
	service    string
	cpAddr     string
	port       int
	registered map[string]bool
	httpClient *http.Client
}

// reconcile lists the current ready pod IPs behind the watched Service and
// converges the registry to match: registers new instances, heartbeats
// known ones, and deregisters instances no longer present (e.g. after a
// scale-down or pod eviction).
func (w *watcher) reconcile(ctx context.Context) error {
	ep, err := w.clientset.CoreV1().Endpoints(w.namespace).Get(ctx, w.service, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get endpoints: %w", err)
	}

	current := make(map[string]string) // instanceID -> address
	for _, subset := range ep.Subsets {
		for _, addr := range subset.Addresses {
			instanceID := instanceIDFor(addr)
			current[instanceID] = addr.IP
		}
	}

	for instanceID, addr := range current {
		if w.registered[instanceID] {
			if err := w.heartbeat(ctx, instanceID); err != nil {
				log.Printf("heartbeat %s failed, re-registering: %v", instanceID, err)
			} else {
				continue
			}
		}
		if err := w.register(ctx, instanceID, addr); err != nil {
			log.Printf("register %s failed: %v", instanceID, err)
			continue
		}
		w.registered[instanceID] = true
	}

	for instanceID := range w.registered {
		if _, ok := current[instanceID]; !ok {
			if err := w.deregister(ctx, instanceID); err != nil {
				log.Printf("deregister %s failed: %v", instanceID, err)
				continue
			}
			delete(w.registered, instanceID)
		}
	}
	return nil
}

func instanceIDFor(addr corev1.EndpointAddress) string {
	if addr.TargetRef != nil && addr.TargetRef.Name != "" {
		return addr.TargetRef.Name
	}
	return addr.IP
}

func (w *watcher) register(ctx context.Context, instanceID, addr string) error {
	body, _ := json.Marshal(map[string]any{
		"service_name": w.service,
		"instance_id":  instanceID,
		"address":      addr,
		"port":         w.port,
	})
	return w.post(ctx, "/v1/services", body)
}

func (w *watcher) heartbeat(ctx context.Context, instanceID string) error {
	path := fmt.Sprintf("/v1/services/%s/instances/%s/heartbeat", w.service, instanceID)
	return w.post(ctx, path, nil)
}

func (w *watcher) deregister(ctx context.Context, instanceID string) error {
	url := fmt.Sprintf("%s/v1/services/%s/instances/%s", w.cpAddr, w.service, instanceID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return err
	}
	resp, err := w.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return nil
}

func (w *watcher) post(ctx context.Context, path string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.cpAddr+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	resp, err := w.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return nil
}
