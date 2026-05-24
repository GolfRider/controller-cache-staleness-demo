# kind + kwok setup

We use kind for a real control plane (real API server, etcd, informers — the
machinery whose staleness we study) and kwok to fake nodes/pods cheaply so we
can generate churn at volume without real kubelets or GPUs.

## 1. kind cluster (real control plane)
    kind create cluster --name staleness

## 2. install kwok (fake node lifecycle)
    KWOK_REPO=kubernetes-sigs/kwok
    KWOK_LATEST=$(curl -s https://api.github.com/repos/${KWOK_REPO}/releases/latest | jq -r .tag_name)
    kubectl apply -f https://github.com/${KWOK_REPO}/releases/download/${KWOK_LATEST}/kwok.yaml
    kubectl apply -f https://github.com/${KWOK_REPO}/releases/download/${KWOK_LATEST}/stage-fast.yaml

## 3. add fake nodes (scale as needed; 1000 for the storm)
    for i in $(seq 1 1000); do
      cat <<NODE | kubectl apply -f -
    apiVersion: v1
    kind: Node
    metadata:
      name: kwok-node-${i}
      labels: {type: kwok}
      annotations: {kwok.x-k8s.io/node: fake}
    spec:
      taints: [{key: kwok.x-k8s.io/node, value: fake, effect: NoSchedule}]
    NODE
    done

Note: pin kwok to your kubectl/k8s minor. jq required for the latest-tag fetch.
