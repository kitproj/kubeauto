package internal

import (
	"fmt"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

func state(un *unstructured.Unstructured) (phase string, reason string, message string) {
	status, _, _ := unstructured.NestedMap(un.Object, "status")

	if status != nil {
		phase, _, _ = unstructured.NestedString(un.Object, "status", "phase")
		reason, _, _ = unstructured.NestedString(un.Object, "status", "reason")
		message, _, _ = unstructured.NestedString(un.Object, "status", "message")
	} else {
		// check the root of the resource, e.g. events
		phase, _, _ = unstructured.NestedString(un.Object, "phase")
		reason, _, _ = unstructured.NestedString(un.Object, "reason")
		message, _, _ = unstructured.NestedString(un.Object, "message")
	}

	failureConditionTypes := map[string]bool{
		"ReplicaFailure": true,
		"PodFailed":      true,
		"Failed":         true,
	}

	// conditions
	conditions, _, _ := unstructured.NestedSlice(un.Object, "status", "conditions")
	for _, condition := range conditions {
		condition := condition.(map[string]interface{})
		type_, _ := condition["type"].(string)
		if failureConditionTypes[type_] {
			phase = "Failed"
			reason, _ = condition["reason"].(string)
			message, _ = condition["message"].(string)
		}
	}

	// observed generation, used by deployments, statefulsets, etc.
	generation, _, _ := unstructured.NestedInt64(un.Object, "metadata", "generation")
	observedGeneration, _, _ := unstructured.NestedInt64(un.Object, "status", "observedGeneration")

	if generation > 0 {
		if generation != observedGeneration {
			phase = "Pending"
			message = fmt.Sprintf("observed generation %d does not match generation %d", observedGeneration, generation)
		} else {
			phase = "Running"
		}
	}

	// ready replicas, used by deployments, statefulsets, etc.
	readyReplicas, _, _ := unstructured.NestedInt64(un.Object, "status", "readyReplicas")
	replicas, _, _ := unstructured.NestedInt64(un.Object, "spec", "replicas")
	if readyReplicas != replicas {
		phase = "Pending"
		message = fmt.Sprintf("ready replicas %d does not match replicas %d", readyReplicas, replicas)
	}

	if un.GetDeletionTimestamp() != nil {
		phase = "Deleting"
		message = "resource is being deleted"
	}

	switch un.GetAPIVersion() + "/" + un.GetKind() {
	case "networking.k8s.io/v1/Ingress":
		ingress := &networkingv1.Ingress{}
		err := runtime.DefaultUnstructuredConverter.FromUnstructured(un.Object, ingress)
		if err != nil {
			fmt.Printf("error converting to ingress: %v\n", err)
			return
		}
		num := len(ingress.Status.LoadBalancer.Ingress)
		if num == 0 {
			phase = "Failed"
			message = "no load balancer found"
		}
	case "v1/Pod":
		pod := &corev1.Pod{}
		err := runtime.DefaultUnstructuredConverter.FromUnstructured(un.Object, pod)
		if err != nil {
			fmt.Printf("error converting to pod: %v\n", err)
			return
		}

		// check the container status
		for _, ctr := range append(pod.Status.InitContainerStatuses, pod.Status.ContainerStatuses...) {
			if waiting := ctr.State.Waiting; waiting != nil {
				phase = "Waiting"
				reason = waiting.Reason
				message = fmt.Sprintf("container %q is waiting: %s", ctr.Name, waiting.Message)
			}
			if terminated := ctr.State.Terminated; terminated != nil && terminated.ExitCode != 0 {
				phase = "Failed"
				reason = terminated.Reason
				message = fmt.Sprintf("container %q exited with code %d: %s", ctr.Name, terminated.ExitCode, terminated.Message)
			}
		}
	}
	return
}
