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
$ aks-flex-cli network <remote-cloud> deploy
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