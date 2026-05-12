# Advanced Kubernetes Patterns

## Helm Charts

### Chart Structure
```
charts/marvin-slacker/
├── Chart.yaml
├── values.yaml
├── templates/
│   ├── deployment.yaml
│   ├── service.yaml
│   ├── ingress.yaml
│   └── configmap.yaml
```

### Installation
```bash
helm install marvin charts/marvin-slacker \
  --set image.tag=v1.0.0 \
  --set replicas=3 \
  --set ollama.enabled=true
```

### Values Configuration
```yaml
image:
  repository: marvin-slacker
  tag: latest
replicas: 2

resources:
  limits:
    cpu: 500m
    memory: 512Mi

ingress:
  enabled: true
  host: marvin.company.com
  tls: true
```

## GitOps with ArgoCD

### Application Manifest
```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: marvin-slacker
spec:
  project: default
  source:
    repoURL: https://github.com/company/marvin-config
    targetRevision: HEAD
    path: k8s/overlays/production
  destination:
    server: https://kubernetes.default.svc
    namespace: marvin
```

### Rollout Strategy
```yaml
strategy:
  type: RollingUpdate
  rollingUpdate:
    maxSurge: 1
    maxUnavailable: 0
```

## Kubernetes Operators

### Custom Resource Definition
```yaml
apiVersion: marvin.io/v1
kind: SlackerDeployment
metadata:
  name: production
spec:
  replicas: 3
  model: "ministral-3:3b"
  tools:
    - docker
    - filesystem
  resources:
    requests:
      cpu: 100m
      memory: 128Mi
```

### Operator Features
- **Auto-scaling**: Scale based on conversation volume
- **Health Monitoring**: Automatic restart on failures
- **Config Updates**: Hot-reload configuration changes
- **Backup**: Automated state persistence

## Multi-Cluster Deployment

### Cluster API
```bash
# Create management cluster
clusterctl init --infrastructure aws

# Deploy workload clusters
kubectl apply -f workload-cluster.yaml
```

### Federation
```yaml
apiVersion: core.k8s.io/v1
kind: FederatedDeployment
metadata:
  name: marvin-federated
spec:
  template:
    metadata:
      labels:
        app: marvin-slacker
    spec:
      replicas: 2
  placement:
    clusters:
      - us-east-1
      - us-west-2
```

## Advanced Patterns

### Canary Deployments
```bash
# 10% traffic to new version
kubectl patch service marvin-slacker \
  -p '{"spec":{"selector":{"version":"canary"}}}'

# Monitor and promote
kubectl patch service marvin-slacker \
  -p '{"spec":{"selector":{"version":"stable"}}}'
```

### Blue-Green Deployments
```yaml
apiVersion: argoproj.io/v1alpha1
kind: Rollout
metadata:
  name: marvin-slacker
spec:
  strategy:
    blueGreen:
      activeService: marvin-active
      previewService: marvin-preview
      autoPromotionEnabled: false
```

### Service Mesh Integration
```yaml
apiVersion: networking.istio.io/v1beta1
kind: VirtualService
metadata:
  name: marvin-slacker
spec:
  http:
  - match:
    - headers:
        x-user-type:
          exact: premium
    route:
    - destination:
        host: marvin-slacker
        subset: v2
  - route:
    - destination:
        host: marvin-slacker
        subset: v1
```

## Monitoring in Kubernetes

### Prometheus Operator
```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: marvin-slacker
spec:
  selector:
    matchLabels:
      app: marvin-slacker
  endpoints:
  - port: metrics
    interval: 30s
```

### Custom Metrics
```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: marvin-slacker
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: StatefulSet
    name: marvin-slacker
  minReplicas: 2
  maxReplicas: 10
  metrics:
  - type: External
    external:
      metric:
        name: marvin_conversations_per_second
      target:
        type: AverageValue
        averageValue: "10"
```

## Security Hardening

### Pod Security
```yaml
securityContext:
  runAsNonRoot: true
  runAsUser: 1000
  fsGroup: 2000
  readOnlyRootFilesystem: true
  capabilities:
    drop:
    - ALL
```

### Network Policies
```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: marvin-slacker
spec:
  podSelector:
    matchLabels:
      app: marvin-slacker
  policyTypes:
  - Ingress
  - Egress
  ingress:
  - from:
    - namespaceSelector:
        matchLabels:
          name: ingress-nginx
```

## Backup and Recovery

### Velero Integration
```bash
# Install Velero
velero install --provider aws --bucket marvin-backups

# Backup namespace
velero backup create marvin-backup --include-namespaces marvin

# Restore from backup
velero restore create --from-backup marvin-backup
```

### Stateful Set Recovery
```bash
# Scale down and recover
kubectl scale statefulset marvin-slacker --replicas=0
kubectl delete pvc data-marvin-slacker-0
kubectl apply -f marvin-slacker-statefulset.yaml
```
