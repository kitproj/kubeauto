package internal

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
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
		if type_ == "Ready" {
			phase = "Ready"
		}
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
		} else {
			phase = "Ready"
		}
	case "apps/v1/ReplicaSet":
		replicaset := &appsv1.ReplicaSet{}
		err := runtime.DefaultUnstructuredConverter.FromUnstructured(un.Object, replicaset)
		if err != nil {
			fmt.Printf("error converting to replicaset: %v\n", err)
			return
		}

		replicas := 1
		if replicaset.Spec.Replicas != nil {
			replicas = int(*replicaset.Spec.Replicas)
		}

		// check the ready replicas
		readyReplicas := replicaset.Status.ReadyReplicas

		if replicas > 0 {
			if readyReplicas == int32(replicas) {
				phase = "Ready"
				message = ""
			} else {
				phase = "Pending"
				message = fmt.Sprintf("ready replicas %d, specified replicas %d", readyReplicas, replicas)
			}
		} else {
			phase = "Inactive"
			message = ""
		}
	case "apps/v1/Deployment":
		deployment := &appsv1.Deployment{}
		err := runtime.DefaultUnstructuredConverter.FromUnstructured(un.Object, deployment)
		if err != nil {
			fmt.Printf("error converting to deployment: %v\n", err)
			return
		}

		replicas := 1
		if deployment.Spec.Replicas != nil {
			replicas = int(*deployment.Spec.Replicas)
		}

		// check the ready replicas
		readyReplicas := deployment.Status.ReadyReplicas

		if replicas > 0 {
			if readyReplicas == int32(replicas) {
				phase = "Ready"
				message = ""
			} else {
				phase = "Pending"
				message = fmt.Sprintf("ready replicas %d, specified replicas %d", readyReplicas, replicas)
			}
		} else {
			phase = "Inactive"
			message = ""
		}
	case "apps/v1/StatefulSet":
		statefulset := &appsv1.StatefulSet{}
		err := runtime.DefaultUnstructuredConverter.FromUnstructured(un.Object, statefulset)
		if err != nil {
			fmt.Printf("error converting to statefulset: %v\n", err)
			return
		}

		replicas := 1
		if statefulset.Spec.Replicas != nil {
			replicas = int(*statefulset.Spec.Replicas)
		}

		// check the ready replicas
		readyReplicas := statefulset.Status.ReadyReplicas

		if replicas > 0 {
			if readyReplicas == int32(replicas) {
				phase = "Ready"
				message = ""
			} else {
				phase = "Pending"
				message = fmt.Sprintf("ready replicas %d, specified replicas %d", readyReplicas, replicas)
			}
		} else {
			phase = "Inactive"
			message = ""
		}

	case "v1/Service":
		service := &corev1.Service{}
		err := runtime.DefaultUnstructuredConverter.FromUnstructured(un.Object, service)
		if err != nil {
			fmt.Printf("error converting to service: %v\n", err)
			return
		}

		message = fmt.Sprintf("%v: %v", service.Spec.Type, service.Spec.ClusterIP)

		// check the load balancer status
		if service.Spec.Type == corev1.ServiceTypeLoadBalancer {
			num := len(service.Status.LoadBalancer.Ingress)
			if num == 0 {
				phase = "Failed"
				message = "no load balancer found"
			} else {
				phase = "Serving"
			}
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

		// Check if pod is ready
		isReady := true
		for _, condition := range pod.Status.Conditions {
			if condition.Type == corev1.PodReady && condition.Status != corev1.ConditionTrue {
				isReady = false
				break
			}
		}

		if isReady && pod.Status.Phase == corev1.PodRunning {
			phase = "Ready"
			message = "Pod is ready and running"
		}
	}

	if un.GetDeletionTimestamp() != nil {
		phase = "Deleting"
		message = ""
	}

	return
}
