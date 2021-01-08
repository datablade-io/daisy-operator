
cat <<EOF | /usr/local/bin/kind create cluster --name kind --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
containerdConfigPatches:
  - |-
    [plugins."io.containerd.grpc.v1.cri".registry.mirrors."registry.foundary.zone:8360"]
      endpoint = ["http://registry.foundary.zone:8360"]
nodes:
  - role: control-plane
  - role: worker
  - role: worker
EOF