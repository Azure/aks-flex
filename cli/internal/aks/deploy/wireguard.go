package deploy

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/base64"
	"fmt"
	"log"
	"text/template"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v8"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	utilconfig "github.com/Azure/aks-flex/flex-plugin/pkg/util/config"
	"github.com/Azure/aks-flex/flex-plugin/pkg/util/k8s"
	"github.com/Azure/aks-flex/flex-plugin/pkg/util/wireguard"
)

var (
	//go:embed assets/wireguard-deployment.yaml
	wireguardDeploymentTemplate string
)

const (
	// wgKubeImage is the container image for the wg-kube controller.
	// NOTE: this is a temporary workaround for setting up sites-to-sites via WireGuard.
	// We are working on a more robust CNI based implementation and will replace
	// with that when it's ready.
	wgKubeImage = "ghcr.io/b4fun/wg-kube:sha-11e4656"
)

func deployWireGuard(ctx context.Context, credentials *azidentity.DefaultAzureCredential, cfg *utilconfig.Config) error {
	// Step 1: Get or generate WireGuard keys for the hub
	log.Print("Getting WireGuard keys...")

	keys, err := getOrCreateWireGuardKeys(ctx, credentials, cfg)
	if err != nil {
		return fmt.Errorf("failed to get WireGuard keys: %w", err)
	}

	log.Printf("  Public Key: %s", keys.PublicKey)

	// Step 2: Get the WireGuard gateway node's public IP (retry until the node registers)
	log.Print("Waiting for WireGuard gateway node to register...")
	var gatewayIP, gatewayPrivateIP string
	for {
		var err error
		gatewayIP, gatewayPrivateIP, err = getWireGuardNodeIP(ctx, credentials, cfg)
		if err == nil {
			break
		}
		log.Printf("  %v, retrying in 15s...", err)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(15 * time.Second):
		}
	}

	log.Printf("WireGuard gateway node ready")
	log.Printf("  Public IP: %s", gatewayIP)
	log.Printf("  Private IP: %s", gatewayPrivateIP)

	// Step 3: Update route table with the gateway node's private IP
	log.Print("Updating route table...")
	if err := updateRouteTable(ctx, credentials, cfg, gatewayPrivateIP); err != nil {
		return fmt.Errorf("failed to update route table: %w", err)
	}

	// Step 4: Associate route table with subnets
	log.Print("Associating route table with subnets...")
	if err := associateRouteTableWithSubnets(ctx, credentials, cfg); err != nil {
		return fmt.Errorf("failed to associate route table with subnets: %w", err)
	}

	// Step 5: Deploy WireGuard DaemonSet to Kubernetes
	log.Print("Deploying WireGuard DaemonSet...")
	if err := deployWireGuardToK8s(ctx, credentials, cfg, keys); err != nil {
		return fmt.Errorf("failed to deploy WireGuard to Kubernetes: %w", err)
	}

	return nil
}

// getOrCreateWireGuardKeys checks if the wireguard-keys secret exists and returns those keys,
// otherwise generates new keys.
func getOrCreateWireGuardKeys(ctx context.Context, credentials *azidentity.DefaultAzureCredential, cfg *utilconfig.Config) (*wireguard.KeyPair, error) {
	loader, err := k8s.Loader(ctx, credentials, cfg)
	if err != nil {
		return nil, err
	}

	restconfig, err := loader.ClientConfig()
	if err != nil {
		return nil, err
	}

	cli, err := client.New(restconfig, client.Options{})
	if err != nil {
		return nil, err
	}

	// Try to get existing secret
	secret := &corev1.Secret{}
	err = cli.Get(ctx, types.NamespacedName{
		Namespace: "wireguard",
		Name:      "wireguard-keys",
	}, secret)

	if err == nil {
		// Secret exists, extract keys
		privateKey, ok := secret.Data["private.key"]
		if !ok {
			return nil, fmt.Errorf("wireguard-keys secret missing private.key")
		}
		publicKey, ok := secret.Data["public.key"]
		if !ok {
			return nil, fmt.Errorf("wireguard-keys secret missing public.key")
		}

		log.Print("  Using existing WireGuard keys from secret")
		return &wireguard.KeyPair{
			PrivateKey: string(privateKey),
			PublicKey:  string(publicKey),
		}, nil
	}

	if !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("failed to get wireguard-keys secret: %w", err)
	}

	// Secret doesn't exist, generate new keys
	log.Print("  Generating new WireGuard keys")
	return wireguard.GenerateKeyPair()
}

// getWireGuardNodeIP retrieves the public and private IP of the WireGuard gateway node from Kubernetes.
func getWireGuardNodeIP(ctx context.Context, credentials *azidentity.DefaultAzureCredential, cfg *utilconfig.Config) (publicIP, privateIP string, err error) {
	loader, err := k8s.Loader(ctx, credentials, cfg)
	if err != nil {
		return "", "", err
	}

	restconfig, err := loader.ClientConfig()
	if err != nil {
		return "", "", err
	}

	cli, err := client.New(restconfig, client.Options{})
	if err != nil {
		return "", "", err
	}

	// List nodes with the wireguard gateway label
	nodeList := &unstructured.UnstructuredList{}
	nodeList.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "NodeList",
	})

	if err := cli.List(ctx, nodeList, client.MatchingLabels{
		"stretch.azure.com/wireguard-gateway": "true",
	}); err != nil {
		return "", "", fmt.Errorf("failed to list wireguard nodes: %w", err)
	}

	if len(nodeList.Items) == 0 {
		return "", "", fmt.Errorf("no wireguard gateway nodes found (waiting for node to register)")
	}

	node := nodeList.Items[0]

	// Extract addresses from the node status
	addresses, found, err := unstructured.NestedSlice(node.Object, "status", "addresses")
	if err != nil || !found {
		return "", "", fmt.Errorf("failed to get node addresses: %w", err)
	}

	for _, addr := range addresses {
		addrMap, ok := addr.(map[string]any)
		if !ok {
			continue
		}
		addrType, _, err := unstructured.NestedString(addrMap, "type")
		if err != nil {
			continue
		}
		addrValue, _, err := unstructured.NestedString(addrMap, "address")
		if err != nil {
			continue
		}

		switch addrType {
		case "ExternalIP":
			publicIP = addrValue
		case "InternalIP":
			privateIP = addrValue
		}
	}

	if publicIP == "" {
		return "", "", fmt.Errorf("wireguard node has no ExternalIP (public IP not assigned yet)")
	}
	if privateIP == "" {
		return "", "", fmt.Errorf("wireguard node has no InternalIP")
	}

	return publicIP, privateIP, nil
}

// updateRouteTable updates the route table with the gateway node's private IP.
func updateRouteTable(ctx context.Context, credentials *azidentity.DefaultAzureCredential, cfg *utilconfig.Config, gatewayPrivateIP string) error {
	routeTablesClient, err := armnetwork.NewRouteTablesClient(cfg.SubscriptionID, credentials, nil)
	if err != nil {
		return err
	}

	routeTable, err := routeTablesClient.Get(ctx, cfg.ResourceGroupName, "nebius-routes", nil)
	if err != nil {
		return fmt.Errorf("failed to get route table: %w", err)
	}

	// Update routes to point to the gateway node
	routeTable.Properties.Routes = []*armnetwork.Route{
		{
			Name: to.Ptr("to-nebius-wg"),
			Properties: &armnetwork.RoutePropertiesFormat{
				AddressPrefix:    to.Ptr("100.96.0.0/12"),
				NextHopType:      to.Ptr(armnetwork.RouteNextHopTypeVirtualAppliance),
				NextHopIPAddress: to.Ptr(gatewayPrivateIP),
			},
		},
	}

	poller, err := routeTablesClient.BeginCreateOrUpdate(ctx, cfg.ResourceGroupName, "nebius-routes", routeTable.RouteTable, nil)
	if err != nil {
		return fmt.Errorf("failed to update route table: %w", err)
	}

	if _, err := poller.PollUntilDone(ctx, nil); err != nil {
		return fmt.Errorf("failed to wait for route table update: %w", err)
	}

	log.Printf("  Route table updated with gateway IP: %s", gatewayPrivateIP)
	return nil
}

// associateRouteTableWithSubnets associates the nebius-routes route table with the aks and nodes subnets.
func associateRouteTableWithSubnets(ctx context.Context, credentials *azidentity.DefaultAzureCredential, cfg *utilconfig.Config) error {
	subnetsClient, err := armnetwork.NewSubnetsClient(cfg.SubscriptionID, credentials, nil)
	if err != nil {
		return err
	}

	routeTableID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/routeTables/nebius-routes",
		cfg.SubscriptionID, cfg.ResourceGroupName)

	// Subnets to update
	subnets := []string{"aks", "nodes"}

	for _, subnetName := range subnets {
		// Get current subnet configuration
		subnet, err := subnetsClient.Get(ctx, cfg.ResourceGroupName, "vnet", subnetName, nil)
		if err != nil {
			return fmt.Errorf("failed to get subnet %s: %w", subnetName, err)
		}

		// Skip if route table is already associated
		if subnet.Properties.RouteTable != nil && subnet.Properties.RouteTable.ID != nil {
			if *subnet.Properties.RouteTable.ID == routeTableID {
				log.Printf("  Route table already associated with subnet %s", subnetName)
				continue
			}
		}

		// Associate route table
		subnet.Properties.RouteTable = &armnetwork.RouteTable{
			ID: to.Ptr(routeTableID),
		}

		log.Printf("  Associating route table with subnet %s...", subnetName)
		poller, err := subnetsClient.BeginCreateOrUpdate(ctx, cfg.ResourceGroupName, "vnet", subnetName, subnet.Subnet, nil)
		if err != nil {
			return fmt.Errorf("failed to update subnet %s: %w", subnetName, err)
		}

		if _, err := poller.PollUntilDone(ctx, nil); err != nil {
			return fmt.Errorf("failed to wait for subnet %s update: %w", subnetName, err)
		}

		log.Printf("  Route table associated with subnet %s", subnetName)
	}

	return nil
}

// deployWireGuardToK8s deploys the WireGuard DaemonSet to the AKS cluster.
func deployWireGuardToK8s(
	ctx context.Context,
	credentials *azidentity.DefaultAzureCredential,
	cfg *utilconfig.Config,
	keys *wireguard.KeyPair,
) error {
	loader, err := k8s.Loader(ctx, credentials, cfg)
	if err != nil {
		return err
	}

	restconfig, err := loader.ClientConfig()
	if err != nil {
		return err
	}

	cli, err := client.New(restconfig, client.Options{})
	if err != nil {
		return err
	}

	// Template the YAML with keys and wg-kube image
	tmpl, err := template.New("wireguard").Parse(wireguardDeploymentTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse WireGuard deployment template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, map[string]string{
		"privateKeyBase64": base64.StdEncoding.EncodeToString([]byte(keys.PrivateKey)),
		"publicKeyBase64":  base64.StdEncoding.EncodeToString([]byte(keys.PublicKey)),
		"wgKubeImage":      wgKubeImage,
	}); err != nil {
		return fmt.Errorf("failed to execute WireGuard deployment template: %w", err)
	}

	if err := k8s.ApplyYAMLSpec(ctx, cli, &buf, "stretch"); err != nil {
		return fmt.Errorf("failed to apply WireGuard deployment YAML: %w", err)
	}

	log.Printf("  WireGuard DaemonSet deployed successfully")
	return nil
}
