package agent

import (
	"net/http"
	"time"

	"github.com/mikhailv/keenetic-dns/agent/rpc/v1/agentv1connect"
)

type NetworkServiceClient = agentv1connect.NetworkServiceClient

func NewNetworkServiceClient(baseURL string, timeout time.Duration) NetworkServiceClient {
	return agentv1connect.NewNetworkServiceClient(&http.Client{Timeout: timeout}, baseURL)
}
