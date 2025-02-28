package internal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	runtimeutil "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

func Run(ctx context.Context, group, namespace, labels, container string, allContainers bool, hostPortOffset int) error {

	// Install custom error handler to suppress unhandled error messages from port forwarding
	runtimeutil.ErrorHandlers = []runtimeutil.ErrorHandler{func(ctx context.Context, err error, msg string, keysAndValues ...any) {
		if err == nil {
			return
		}
		// Suppress common port forwarding errors
		if strings.Contains(err.Error(), "error copying from") ||
			strings.Contains(err.Error(), "an error occurred forwarding") {
			return
		}
		// Log other errors
		x := fmt.Sprintf(msg, keysAndValues...)
		y := fmt.Sprintf("%s: %v'", x, err)
		println(y)
	}}

	// connect to the k8s cluster
	kubeConfig := os.Getenv("KUBECONFIG")
	if kubeConfig == "" {
		kubeConfig = clientcmd.RecommendedHomeFile
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeConfig)
	if err != nil {
		return err
	}

	if namespace == "" {
		// Get the namespace associated with the current context
		namespace, _, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeConfig},
			&clientcmd.ConfigOverrides{},
		).Namespace()
		if err != nil {
			return err
		}
	}

	// Create a Kubernetes clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}

	// Create a Discovery client
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return err
	}

	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return err
	}

	// get a list of all the server resources
	resources, err := discoveryClient.ServerPreferredNamespacedResources()
	if err != nil {
		return err
	}

	gvkToResourceType := make(map[schema.GroupVersionKind]string)

	for _, resourceList := range resources {
		gv, _ := schema.ParseGroupVersion(resourceList.GroupVersion)
		for _, resource := range resourceList.APIResources {
			if group != resource.Group {
				continue
			}

			// if the verbs do not include "watch" we cannot watch this resource
			if !slices.Contains(resource.Verbs, "watch") {
				continue
			}

			gvr := gv.WithResource(resource.Name)
			gvk := gv.WithKind(resource.Kind)

			gvkToResourceType[gvk] = resource.Name

			// get the latest resource version
			list, err := dynamicClient.Resource(gvr).Namespace(namespace).List(ctx, metav1.ListOptions{
				LabelSelector: labels,
			})
			if err != nil {
				return err
			}
			resourceVersion := list.GetResourceVersion()

			for _, un := range list.Items {
				phase, reason, message := state(&un)
				fmt.Println(color(resource.Name, join(delim(resource.Name+"/"+un.GetName(), "[", "]"), delim(phase, "(", ")"), delim(reason, "", ":"), message)))
			}

			watch, err := dynamicClient.Resource(gvr).Namespace(namespace).Watch(ctx, metav1.ListOptions{
				LabelSelector:   labels,
				ResourceVersion: resourceVersion,
			})
			if err != nil {
				return err
			}
			defer watch.Stop()
			go func() {
				for event := range watch.ResultChan() {
					// convert to unstructured
					obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(event.Object)
					if err != nil {
						fmt.Printf("error converting to unstructured: %v\n", err)
						continue
					}
					un := &unstructured.Unstructured{Object: obj}
					phase, reason, message := state(un)
					gvk := un.GroupVersionKind()
					resourceType := gvkToResourceType[gvk]

					fmt.Printf(color(resourceType, "[%s/%s] (%s) %s: %s\n"), resourceType, un.GetName(), phase, reason, message)
				}
			}()

			if err != nil {
				return err
			}
		}
	}

	// Create a shared informer factory for only the labelled resource managed-by kit and named after the task
	factory := informers.NewSharedInformerFactoryWithOptions(clientset, 5*time.Second,
		informers.WithNamespace(namespace),
		informers.WithTweakListOptions(func(options *metav1.ListOptions) {
			options.LabelSelector = labels
		}),
	)

	// Create a pod informer
	podInformer := factory.Core().V1().Pods().Informer()

	logging := sync.Map{}        // name/container -> true
	portForwarding := sync.Map{} // port -> sync.Mutex

	// Add event handlers
	processPod := func(obj any) {
		pod := obj.(*corev1.Pod)

		running := make(map[string]bool)

		for _, s := range append(pod.Status.InitContainerStatuses, pod.Status.ContainerStatuses...) {
			running[s.Name] = s.State.Running != nil
		}

		if container == "" {
			container = pod.Annotations["kubectl.kubernetes.io/default-container"]
		}

		for _, ctr := range append(pod.Spec.InitContainers, pod.Spec.Containers...) {
			// skip containers that are not running
			if !running[ctr.Name] {
				continue
			}

			// skip containers that are not the one we want to watch
			if !allContainers && ctr.Name != container {
				continue
			}

			out := colorWriter("pods", fmt.Sprintf("[pods/%s] %s:  ", pod.Name, ctr.Name))
			go func() {
				// start a log tail
				key := pod.Name + "/" + ctr.Name

				// check if the pod is already being logged
				if _, ok := logging.Load(key); ok {
					return
				}

				logging.Store(key, true)
				defer logging.Delete(key)

				pods := clientset.CoreV1().Pods(pod.Namespace)

				// the pod might have been deleted before we get here, if so just exit quietly
				_, err := pods.Get(ctx, pod.Name, metav1.GetOptions{})
				if err != nil {
					return
				}

				defer func() {
					if r := recover(); r != nil {
						fmt.Printf(color("pods", "[pods/%s/%s] error while tailing logs: %v\n"), pod.Name, ctr.Name, r)
					}
				}()

				req := pods.GetLogs(pod.Name, &corev1.PodLogOptions{
					Follow:    true,
					Container: ctr.Name,
					SinceTime: &metav1.Time{Time: time.Now()},
				})
				podLogs, err := req.Stream(ctx)
				if err != nil {
					panic(fmt.Errorf("Error opening stream: %w\n", err))
				}
				defer podLogs.Close()
				_, err = io.Copy(out, podLogs)
				if err != nil && !errors.Is(err, context.Canceled) {
					panic(fmt.Errorf("Error copying stream: %w\n", err))
				}
			}()
			for _, port := range ctr.Ports {
				// only forward host ports
				containerPort := port.ContainerPort
				hostPort := hostPortOffset + int(containerPort)

				// start port-forwarding
				go func(containerPort int32, hostPort int) {
					// check if the pod is already being port-forwarded
					obj, _ := portForwarding.LoadOrStore(hostPort, &sync.Mutex{})
					mu := obj.(*sync.Mutex)

					mu.Lock()
					defer mu.Unlock()

					defer func() {
						if r := recover(); r != nil {
							fmt.Printf(color("pods", "[pods/%s/%s] error while port-forwarding: %d -> %d: %v\n"), pod.Name, ctr.Name, hostPort, containerPort, r)
						}
					}()

					req := clientset.CoreV1().RESTClient().Post().
						Resource("pods").
						Namespace(pod.Namespace).
						Name(pod.Name).
						SubResource("portforward")

					transport, upgrader, err := spdy.RoundTripperFor(config)
					if err != nil {
						if strings.Contains(err.Error(), "not found") {
							return
						}
						panic(err)
					}

					dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, "POST", req.URL())

					stopChan, cancel := context.WithCancel(ctx)
					defer cancel()
					readyChan := make(chan struct{})

					ports := []string{fmt.Sprintf("%d:%d", hostPort, containerPort)}

					fw, err := portforward.New(dialer, ports, stopChan.Done(), readyChan, nil, nil)
					if err != nil {
						panic(err)
					}

					go func() {
						<-readyChan
						fmt.Printf(color("pods", "[pods/%s/%s] forwarding port %d -> %d\n"), pod.Name, ctr.Name, hostPort, containerPort)
						// pod might get deleted, check open and close socket every few seconds
						ticker := time.NewTicker(5 * time.Second)
						defer ticker.Stop()
						for {
							select {
							case <-ticker.C:
								dial, err := net.Dial("tcp", fmt.Sprintf("localhost:%d", hostPort))
								if err != nil {
									cancel()
									return
								}
								_ = dial.Close()
							case <-ctx.Done():
								return
							}
						}
					}()

					if err := fw.ForwardPorts(); err != nil {
						if errors.Is(err, portforward.ErrLostConnectionToPod) {
							return
						}
						panic(err)
					}
				}(containerPort, hostPort)
			}
		}
	}
	_, err = podInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: processPod,
		UpdateFunc: func(_, newObj any) {
			processPod(newObj)
		},
	})
	if err != nil {
		return err
	}

	factory.Start(ctx.Done())

	<-ctx.Done()

	return nil
}
