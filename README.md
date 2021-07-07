## Dependencies

- Go 1.15+
- operator-sdk 1.3.0
- kubernetes 1.16+ (support PVC expansion)

## Generate client & crds & other yaml files
```
make generate manifests
```

## Debug in IDE
debug parameter: 
```
#GO Envrionment: GODEBUG=x509ignoreCN=0
--config=config/config.yaml --zap-log-level=2
```

## Build
All below make task should run from parant directory

### Build local binary

```
make all
```

### Build docker image & push to docker registry

```
make docker-build docker-push IMG=registry.foundary.zone:8360/dae/daisy-operator:v0.2
```

## Run
To run daisy operator and run e2e tests, a kubernetes environment should be setup first. 
You can either use an existing kubernetes cluster or setup a kind cluster 
in local dev machine.

### Setup local kind cluster
```
make kind
```

### Deploy to k8s
It use ~/.kube/config file to find the k8s cluster 
define in default context to install daisy operator

```
// install the crd definition
make install

// deploy daisy operator
make deploy IMG=registry.foundary.zone:8360/dae/daisy-operator:v0.2
```

## Tests

unit tests, integration tests and e2e tests have been implemented to 
cover various test scenarios. All can be trigger by make task

### Run unit tests & integration tests
```
make test
```

### Run e2e tests
```
make e2e
```

