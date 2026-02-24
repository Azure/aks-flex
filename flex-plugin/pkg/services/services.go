package services

import (
	"context"
	"net"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"k8s.io/client-go/kubernetes"

	"github.com/Azure/aks-flex/flex-plugin/pkg/db"
	"github.com/Azure/aks-flex/flex-plugin/pkg/services/agentpools"
	agentpoolsapi "github.com/Azure/aks-flex/flex-plugin/pkg/services/agentpools/api"
	"github.com/Azure/aks-flex/flex-plugin/pkg/services/networks"
	networksapi "github.com/Azure/aks-flex/flex-plugin/pkg/services/networks/api"
	"github.com/Azure/aks-flex/flex-plugin/pkg/services/peerings"
	peeringsapi "github.com/Azure/aks-flex/flex-plugin/pkg/services/peerings/api"
)

var l = bufconn.Listen(1024 * 1024)
var startErr error
var startOnce sync.Once

func startWithFileDB() error {
	startOnce.Do(func() {
		s := grpc.NewServer()

		agentpoolsDB := db.NewStupidDB("agentpools.db")
		networksDB := db.NewStupidDB("networks.db")
		peeringsDB := db.NewStupidDB("peerings.db")

		agentpoolsapi.RegisterAgentPoolsServer(s, agentpools.NewAgentPoolsServer(agentpoolsDB))
		agentpoolsapi.RegisterInstancesServer(s, agentpools.NewInstancesServer(agentpoolsDB))
		networksapi.RegisterNetworksServer(s, networks.NewNetworksServer(networksDB))
		peeringsapi.RegisterPeeringsServer(s, peerings.NewPeeringsServer(peeringsDB))

		go s.Serve(l)
	})

	return startErr
}

func startWithSecretDB(client kubernetes.Interface, namespace string) error {
	startOnce.Do(func() {
		s := grpc.NewServer()

		agentpoolsDB := db.NewStupidDBWithSecret(client, namespace, "agentpools")
		networksDB := db.NewStupidDBWithSecret(client, namespace, "networks")
		peeringsDB := db.NewStupidDBWithSecret(client, namespace, "peerings")

		agentpoolsapi.RegisterAgentPoolsServer(s, agentpools.NewAgentPoolsServer(agentpoolsDB))
		agentpoolsapi.RegisterInstancesServer(s, agentpools.NewInstancesServer(agentpoolsDB))
		networksapi.RegisterNetworksServer(s, networks.NewNetworksServer(networksDB))
		peeringsapi.RegisterPeeringsServer(s, peerings.NewPeeringsServer(peeringsDB))

		go s.Serve(l)
	})

	return startErr
}

func dial() (*grpc.ClientConn, error) {
	return grpc.NewClient("passthrough:",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return l.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
}

func NewConnection() (*grpc.ClientConn, error) {
	if err := startWithFileDB(); err != nil {
		return nil, err
	}

	return dial()
}

func NewConnectionWithSecretDB(client kubernetes.Interface, namespace string) (*grpc.ClientConn, error) {
	if err := startWithSecretDB(client, namespace); err != nil {
		return nil, err
	}

	return dial()
}
