package cloudproviders

// FIXME: this is a workaround for associating the exact instance / node claim
// after node bootstrapping. This is needed for settings correct provider ID
// in the node object.
const NodeClaimLabelKey = "karpenter.azure.com/node-claim"
