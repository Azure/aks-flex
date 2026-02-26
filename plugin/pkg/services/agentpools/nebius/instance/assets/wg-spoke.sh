#!/usr/bin/env bash
#
# wg-spoke.sh — WireGuard spoke peer agent for Kubernetes nodes.
#
# Runs on a k8s node host. Generates a WireGuard key pair, publishes the
# public key to the node's annotations via kubectl, then watches for the
# hub node's public key and endpoint. Configures the local WireGuard
# interface and restarts it whenever the hub configuration changes.
#
# Environment variables:
#   KUBECONFIG        — path to kubeconfig (default: /etc/kubernetes/kubelet.conf)
#   NODE_NAME         — this node's name (default: $(hostname))
#   WG_INTERFACE      — WireGuard interface name (default: wg0)
#   WG_ADDRESS        — local WireGuard address CIDR (default: 100.96.0.2/32)
#   WG_LISTEN_PORT    — local listen port (default: 51820)
#   WG_ALLOWED_IPS    — CIDRs to advertise to the hub (default: WG_ADDRESS)
#   HUB_ALLOWED_IPS   — CIDRs to route through the hub (default: hub's WG address)
#   ANNOTATION_PREFIX — annotation/label prefix (default: wireguard.kube/)
#   POLL_INTERVAL     — seconds between poll iterations (default: 10)
#   WG_DAEMONIZE      — if set to "true", fork the poll loop into the background
#                        so cloud-init or other callers don't block (default: false)
#   WG_LOG_FILE       — log file path when daemonized (default: /var/log/wg-spoke.log)
#

set -euo pipefail

KUBECONFIG="${KUBECONFIG:-/etc/kubernetes/kubelet.conf}"
NODE_NAME="${NODE_NAME:-$(hostname)}"
WG_INTERFACE="${WG_INTERFACE:-wg0}"
WG_ADDRESS="${WG_ADDRESS:-100.96.0.2/32}"
WG_LISTEN_PORT="${WG_LISTEN_PORT:-51820}"
HUB_ALLOWED_IPS="${HUB_ALLOWED_IPS:-}"
ANNOTATION_PREFIX="${ANNOTATION_PREFIX:-wireguard.kube/}"
POLL_INTERVAL="${POLL_INTERVAL:-10}"
WG_DAEMONIZE="${WG_DAEMONIZE:-false}"
WG_LOG_FILE="${WG_LOG_FILE:-/var/log/wg-spoke.log}"

KUBECTL="kubectl --kubeconfig=${KUBECONFIG}"
WG_CONFIG_DIR="/etc/wireguard"
WG_CONFIG="${WG_CONFIG_DIR}/${WG_INTERFACE}.conf"

KEY_ANNOTATION="${ANNOTATION_PREFIX}public-key"
ENDPOINT_ANNOTATION="${ANNOTATION_PREFIX}endpoint"
ALLOWED_IPS_ANNOTATION="${ANNOTATION_PREFIX}allowed-ips"
PEER_LABEL="${ANNOTATION_PREFIX}peer"
HUB_LABEL="${ANNOTATION_PREFIX}hub"

# State tracking for change detection
CURRENT_HUB_KEY=""
CURRENT_HUB_ENDPOINT=""
CURRENT_HUB_ALLOWED_IPS=""

log() { echo "[$(date -u '+%Y-%m-%dT%H:%M:%SZ')] $*"; }

# --- Key generation ---

generate_keys() {
    log "Generating WireGuard key pair..."
    mkdir -p "${WG_CONFIG_DIR}"
    local privkey pubkey
    privkey=$(wg genkey)
    pubkey=$(echo "${privkey}" | wg pubkey)
    echo "${privkey}" > "${WG_CONFIG_DIR}/private.key"
    echo "${pubkey}" > "${WG_CONFIG_DIR}/public.key"
    chmod 600 "${WG_CONFIG_DIR}/private.key"
    log "Public key: ${pubkey}"
}

# --- Node registration ---

register_peer() {
    local pubkey
    pubkey=$(cat "${WG_CONFIG_DIR}/public.key")

    local allowed_ips="${WG_ALLOWED_IPS:-${WG_ADDRESS}}"

    log "Registering as peer on node ${NODE_NAME}..."
    ${KUBECTL} label node "${NODE_NAME}" "${PEER_LABEL}=true" --overwrite
    ${KUBECTL} annotate node "${NODE_NAME}" \
        "${KEY_ANNOTATION}=${pubkey}" \
        "${ALLOWED_IPS_ANNOTATION}=${allowed_ips}" \
        --overwrite
    log "Peer registered"
}

# --- Hub discovery ---

# discover_hub finds the hub node by label and returns its name.
discover_hub() {
    ${KUBECTL} get nodes -l "${HUB_LABEL}=true" \
        -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true
}

# get_node_annotation reads an annotation from a given node.
get_node_annotation() {
    local node="$1"
    local annotation="$2"
    # Escape dots and slashes for jsonpath (both are special characters)
    local escaped="${annotation//\./\\.}"
    escaped="${escaped//\//\\/}"
    ${KUBECTL} get node "${node}" \
        -o jsonpath="{.metadata.annotations.${escaped}}" \
        2>/dev/null || true
}

# --- WireGuard config ---

write_config() {
    local hub_key="$1"
    local hub_endpoint="$2"
    local hub_allowed_ips="$3"
    local privkey
    privkey=$(cat "${WG_CONFIG_DIR}/private.key")

    mkdir -p "${WG_CONFIG_DIR}"
    cat > "${WG_CONFIG}" <<EOF
[Interface]
Address = ${WG_ADDRESS}
ListenPort = ${WG_LISTEN_PORT}
PrivateKey = ${privkey}

[Peer]
PublicKey = ${hub_key}
Endpoint = ${hub_endpoint}
AllowedIPs = ${hub_allowed_ips}
PersistentKeepalive = 25
EOF
    chmod 600 "${WG_CONFIG}"
}

restart_wg() {
    log "Restarting WireGuard interface ${WG_INTERFACE}..."
    wg-quick down "${WG_INTERFACE}" 2>/dev/null || true
    wg-quick up "${WG_INTERFACE}"
    log "WireGuard interface ${WG_INTERFACE} is up"
}

# --- Main loop ---

poll_loop() {
    log "Watching for hub node (label ${HUB_LABEL}=true), polling every ${POLL_INTERVAL}s..."

    while true; do
        hub_node=$(discover_hub)
        if [[ -z "${hub_node}" ]]; then
            log "No hub node found (looking for label ${HUB_LABEL}=true), waiting..."
            sleep "${POLL_INTERVAL}"
            continue
        fi

        hub_key=$(get_node_annotation "${hub_node}" "${KEY_ANNOTATION}")
        hub_endpoint=$(get_node_annotation "${hub_node}" "${ENDPOINT_ANNOTATION}")

        if [[ -z "${hub_key}" || -z "${hub_endpoint}" ]]; then
            log "Hub node ${hub_node} not ready yet (key=${hub_key:-<empty>}, endpoint=${hub_endpoint:-<empty>}), waiting..."
            sleep "${POLL_INTERVAL}"
            continue
        fi

        # Resolve what CIDRs to route through the hub:
        # 1. Explicit HUB_ALLOWED_IPS env var (user override)
        # 2. Hub's allowed-ips annotation (hub advertises its own CIDRs)
        # 3. Hub's WireGuard address from the address annotation
        if [[ -n "${HUB_ALLOWED_IPS}" ]]; then
            hub_allowed_ips="${HUB_ALLOWED_IPS}"
        else
            hub_allowed_ips=$(get_node_annotation "${hub_node}" "${ALLOWED_IPS_ANNOTATION}")
            if [[ -z "${hub_allowed_ips}" ]]; then
                hub_allowed_ips="${WG_ADDRESS%.*}.1/32"
                log "Hub has no allowed-ips annotation, defaulting to ${hub_allowed_ips}"
            fi
        fi

        if [[ "${hub_key}" != "${CURRENT_HUB_KEY}" || "${hub_endpoint}" != "${CURRENT_HUB_ENDPOINT}" || "${hub_allowed_ips}" != "${CURRENT_HUB_ALLOWED_IPS}" ]]; then
            log "Hub config changed: node=${hub_node} key=${hub_key} endpoint=${hub_endpoint} allowed-ips=${hub_allowed_ips}"
            write_config "${hub_key}" "${hub_endpoint}" "${hub_allowed_ips}"
            restart_wg
            CURRENT_HUB_KEY="${hub_key}"
            CURRENT_HUB_ENDPOINT="${hub_endpoint}"
            CURRENT_HUB_ALLOWED_IPS="${hub_allowed_ips}"
        fi

        sleep "${POLL_INTERVAL}"
    done
}

main() {
    generate_keys
    register_peer

    if [[ "${WG_DAEMONIZE}" == "true" ]]; then
        log "Daemonizing poll loop (log: ${WG_LOG_FILE})..."
        poll_loop >> "${WG_LOG_FILE}" 2>&1 &
        local pid=$!
        echo "${pid}" > /var/run/wg-spoke.pid
        log "Poll loop running in background (PID ${pid})"
        # Disown so the shell can exit without waiting for the child
        disown "${pid}"
    else
        poll_loop
    fi
}

# Handle shutdown
cleanup() {
    log "Shutting down..."
    wg-quick down "${WG_INTERFACE}" 2>/dev/null || true
    rm -f /var/run/wg-spoke.pid
    exit 0
}
trap cleanup SIGINT SIGTERM

main
