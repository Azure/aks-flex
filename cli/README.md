# aks-flex-cli

## Preparing Configuration

```
$ aks-flex-cli config env > .env
```

## Initializing Azure Network

```
$ aks-flex-cli network deploy
```

## Initializing AKS Cluster

```
$ aks-flex-cli aks deploy --cilium --wireguard
```

## Initializing Remote Cloud Network

```
$ aks-flex-cli config network <remote-cloud> > network.json
```

```
$ aks-flex-cli plugin apply -f network.json
```

## Creating First Agent Pool

```
$ aks-flex-cli config agentpools <remote-cloud> > agentpool.json
```

```
$ aks-flex-cli plugin apply -f agentpool.json
```

```
$ aks-flex-cli plugin get agentpools <name>
```

## Deleting Agent Pool

```
$ aks-flex-cli plugin delete agentpools <name>
```

## Prepare an (existing) cluster

```
$ aks-flex-cli config k8s-bootstrap > cluster-settings.yaml
```

## Bootstrap a Node (manually)

```
$ aks-flex-cli config node-bootstrap <remote-cloud> > user-config.yaml
```
