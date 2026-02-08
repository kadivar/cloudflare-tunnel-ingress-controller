package controller

import (
	"context"
	"os"
	"strconv"
	"strings"

	cloudflarecontroller "github.com/STRRL/cloudflare-tunnel-ingress-controller/pkg/cloudflare-controller"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func CreateOrUpdateControlledCloudflared(
	ctx context.Context,
	kubeClient client.Client,
	tunnelClient cloudflarecontroller.TunnelClientInterface,
	namespace string,
	protocol string,
	extraArgs []string,
) error {
	logger := log.FromContext(ctx)
	list := appsv1.DeploymentList{}
	err := kubeClient.List(ctx, &list, &client.ListOptions{
		Namespace: namespace,
		LabelSelector: labels.SelectorFromSet(labels.Set{
			"strrl.dev/cloudflare-tunnel-ingress-controller": "controlled-cloudflared-connector",
		}),
	})
	if err != nil {
		return errors.Wrapf(err, "list controlled-cloudflared-connector in namespace %s", namespace)
	}

	if len(list.Items) > 0 {
		// Check if the existing deployment needs to be updated
		existingDeployment := &list.Items[0]
		desiredReplicas, err := getDesiredReplicas()
		if err != nil {
			return errors.Wrap(err, "get desired replicas")
		}

		// Get token once for all checks
		token, err := tunnelClient.FetchTunnelToken(ctx)
		if err != nil {
			return errors.Wrap(err, "fetch tunnel token")
		}

		desiredDeployment := cloudflaredConnectDeploymentTemplating(protocol, token, namespace, desiredReplicas, extraArgs)
		needsUpdate := connectorSpecNeedsUpdate(existingDeployment, desiredDeployment)

		if needsUpdate {
			existingDeployment.Spec = desiredDeployment.Spec
			err = kubeClient.Update(ctx, existingDeployment)
			if err != nil {
				return errors.Wrap(err, "update controlled-cloudflared-connector deployment")
			}
			logger.Info("Updated controlled-cloudflared-connector deployment", "namespace", namespace)
		}

		return nil
	}

	token, err := tunnelClient.FetchTunnelToken(ctx)
	if err != nil {
		return errors.Wrap(err, "fetch tunnel token")
	}

	replicas, err := getDesiredReplicas()
	if err != nil {
		return errors.Wrap(err, "get desired replicas")
	}

	deployment := cloudflaredConnectDeploymentTemplating(protocol, token, namespace, replicas, extraArgs)
	err = kubeClient.Create(ctx, deployment)
	if err != nil {
		return errors.Wrap(err, "create controlled-cloudflared-connector deployment")
	}
	logger.Info("Created controlled-cloudflared-connector deployment", "namespace", namespace)
	return nil
}

func cloudflaredConnectDeploymentTemplating(protocol string, token string, namespace string, replicas int32, extraArgs []string) *appsv1.Deployment {
	appName := "controlled-cloudflared-connector"

	// Use default values if environment variables are empty
	image := os.Getenv("CLOUDFLARED_IMAGE")
	if image == "" {
		image = "cloudflare/cloudflared:latest"
	}

	pullPolicy := os.Getenv("CLOUDFLARED_IMAGE_PULL_POLICY")
	if pullPolicy == "" {
		pullPolicy = "IfNotPresent"
	}

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appName,
			Namespace: namespace,
			Labels: map[string]string{
				"app": appName,
				"strrl.dev/cloudflare-tunnel-ingress-controller": "controlled-cloudflared-connector",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": appName,
					"strrl.dev/cloudflare-tunnel-ingress-controller": "controlled-cloudflared-connector",
				},
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name: appName,
					Labels: map[string]string{
						"app": appName,
						"strrl.dev/cloudflare-tunnel-ingress-controller": "controlled-cloudflared-connector",
					},
				},
				Spec: buildConnectorPodSpec(image, pullPolicy, protocol, token, extraArgs),
			},
		},
	}
}

// getConnectorPodSpecFromEnv reads optional connector pod spec overrides from env.
// Used for hostNetwork + dnsConfig when cluster DNS and external DNS must both be available (e.g. IPv6-only nodes).
func getConnectorPodSpecFromEnv() (hostNetwork bool, dnsPolicy v1.DNSPolicy, dnsConfig *v1.PodDNSConfig) {
	if os.Getenv("CLOUDFLARED_HOST_NETWORK") == "true" {
		hostNetwork = true
	}
	switch os.Getenv("CLOUDFLARED_DNS_POLICY") {
	case "None":
		dnsPolicy = v1.DNSNone
		nameservers := splitNonEmpty(os.Getenv("CLOUDFLARED_DNS_CONFIG_NAMESERVERS"), ",")
		searches := splitNonEmpty(os.Getenv("CLOUDFLARED_DNS_CONFIG_SEARCHES"), ",")
		if len(nameservers) > 0 {
			dnsConfig = &v1.PodDNSConfig{
				Nameservers: nameservers,
				Searches:    searches,
			}
		}
	case "Default":
		dnsPolicy = v1.DNSDefault
	case "ClusterFirstWithHostNet":
		dnsPolicy = v1.DNSClusterFirstWithHostNet
	default:
		// leave dnsPolicy zero value (ClusterFirst)
	}
	return
}

func splitNonEmpty(s, sep string) []string {
	if s == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(s, ",") {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func buildConnectorPodSpec(image, pullPolicy, protocol, token string, extraArgs []string) v1.PodSpec {
	spec := v1.PodSpec{
		Containers: []v1.Container{
			{
				Name:            "controlled-cloudflared-connector",
				Image:           image,
				ImagePullPolicy: v1.PullPolicy(pullPolicy),
				Command:         buildCloudflaredCommand(protocol, token, extraArgs),
			},
		},
		RestartPolicy: v1.RestartPolicyAlways,
	}
	hostNetwork, dnsPolicy, dnsConfig := getConnectorPodSpecFromEnv()
	spec.HostNetwork = hostNetwork
	if dnsPolicy != "" {
		spec.DNSPolicy = dnsPolicy
	}
	if dnsConfig != nil {
		spec.DNSConfig = dnsConfig
	}
	nodeSelector, affinity := getConnectorSchedulingFromEnv()
	if len(nodeSelector) > 0 {
		spec.NodeSelector = nodeSelector
	}
	if affinity != nil {
		spec.Affinity = affinity
	}
	return spec
}

// getConnectorSchedulingFromEnv reads optional scheduling overrides from env so the controller's desired spec
// includes them and they are not overwritten by reconciliation (e.g. run on workers only, one per node).
func getConnectorSchedulingFromEnv() (nodeSelector map[string]string, affinity *v1.Affinity) {
	if os.Getenv("CLOUDFLARED_NODE_SELECTOR_WORKER") == "true" {
		nodeSelector = map[string]string{"node-role.kubernetes.io/worker": ""}
	}
	if os.Getenv("CLOUDFLARED_SPREAD_ONE_PER_NODE") == "true" {
		affinity = &v1.Affinity{
			PodAntiAffinity: &v1.PodAntiAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
					{
						LabelSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "controlled-cloudflared-connector"},
						},
						TopologyKey: "kubernetes.io/hostname",
					},
				},
			},
		}
	}
	return
}

// connectorSpecNeedsUpdate returns true if existing deployment spec differs from desired (replicas, image, command, scheduling, or pod spec overrides).
func connectorSpecNeedsUpdate(existing, desired *appsv1.Deployment) bool {
	if *existing.Spec.Replicas != *desired.Spec.Replicas {
		return true
	}
	exPod := &existing.Spec.Template.Spec
	desPod := &desired.Spec.Template.Spec
	if exPod.HostNetwork != desPod.HostNetwork || exPod.DNSPolicy != desPod.DNSPolicy {
		return true
	}
	if !dnsConfigEqual(exPod.DNSConfig, desPod.DNSConfig) {
		return true
	}
	if !nodeSelectorEqual(exPod.NodeSelector, desPod.NodeSelector) {
		return true
	}
	if !affinityEqual(exPod.Affinity, desPod.Affinity) {
		return true
	}
	if len(existing.Spec.Template.Spec.Containers) == 0 || len(desired.Spec.Template.Spec.Containers) == 0 {
		return true
	}
	exC := &existing.Spec.Template.Spec.Containers[0]
	desC := &desired.Spec.Template.Spec.Containers[0]
	if exC.Image != desC.Image || exC.ImagePullPolicy != desC.ImagePullPolicy {
		return true
	}
	// Compare command (includes token and extraArgs)
	if !slicesEqual(exC.Command, desC.Command) {
		return true
	}
	return false
}

func nodeSelectorEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func affinityEqual(a, b *v1.Affinity) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	// Compare only PodAntiAffinity required terms we use (one-per-node spread)
	exReq := a.PodAntiAffinity != nil && len(a.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution) > 0
	desReq := b.PodAntiAffinity != nil && len(b.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution) > 0
	return exReq == desReq
}

func dnsConfigEqual(a, b *v1.PodDNSConfig) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return slicesEqual(a.Nameservers, b.Nameservers) && slicesEqual(a.Searches, b.Searches)
}

func getDesiredReplicas() (int32, error) {
	replicaCount := os.Getenv("CLOUDFLARED_REPLICA_COUNT")
	if replicaCount == "" {
		return 1, nil
	}
	replicas, err := strconv.ParseInt(replicaCount, 10, 32)
	if err != nil {
		return 0, errors.Wrap(err, "invalid replica count")
	}
	return int32(replicas), nil
}

func buildCloudflaredCommand(protocol string, token string, extraArgs []string) []string {
	command := []string{
		"cloudflared",
		"--protocol",
		protocol,
		"--no-autoupdate",
		"tunnel",
	}
	
	// Add all extra arguments between "tunnel" and "run"
	if len(extraArgs) > 0 {
		command = append(command, extraArgs...)
	}
	
	// Add metrics, run subcommand and token
	command = append(command, "--metrics", "0.0.0.0:44483", "run", "--token", token)
	
	return command
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i, v := range a {
		if v != b[i] {
			return false
		}
	}
	return true
}
