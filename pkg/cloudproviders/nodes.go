package cloudproviders

import (
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
)

// FIXME: this is a workaround for associating the exact instance / node claim
// after node bootstrapping. This is needed for settings correct provider ID
// in the node object.
const NodeClaimLabelKey = "karpenter.azure.com/node-claim"

func NodeClaimLabelKeyValue(nodeClaim *karpv1.NodeClaim) string {
	return NodeClaimLabelKey + "=" + nodeClaim.Name
}
