# Kubernetes Deployment Guide

This guide covers deploying Slacker (Marvin's Slack bot) on Kubernetes using Kustomize for environment management.

## 🎯 Overview

Slacker is deployed as a Kubernetes StatefulSet to provide:
- Stable network identity for consistent Slack connections
- Persistent storage for sessions and credentials
- Configurable resources per environment
- Rolling updates and scaling capabilities
- Health monitoring and automatic recovery

## 🏗️ Architecture

### Components
- **StatefulSet**: Manages Slacker pod lifecycle
- **Service**: Exposes Slacker for monitoring and health checks
- **PersistentVolumeClaim**: Provides persistent storage for data
- **ConfigMap**: Contains configuration files
- **Secret**: Stores Slack tokens and sensitive data

### Deployment Pattern
```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│   Development   │    │     Staging      │    │   Production    │
│   (1 replica)   │    │   (1 replica)    │    │   (2 replicas)  │
│   1Gi storage   │    │   5Gi storage    │    │   50Gi storage  │
│   Debug logging │    │   Info logging   │    │   Warn logging  │
└─────────────────┘    └──────────────────┘    └─────────────────┘
```

## 🚀 Quick Start

### Prerequisites
- Kubernetes cluster v1.20+
- kubectl configured and accessible
- kustomize v3.0+
- Docker registry with slacker image
- Slack Bot and App tokens

### Step 1: Prepare Environment

#### Install kustomize
```bash
# Using kubectl built-in kustomize
kubectl apply -k deploy/k8s/overlays/dev

# Or standalone kustomize
kustomize build deploy/k8s/overlays/dev | kubectl apply -f -
```

#### Create Namespaces
```bash
kubectl create namespace marvin-dev
kubectl create namespace marvin-staging  
kubectl create namespace marvin-production
```

### Step 2: Configure Secrets

Create secrets for each environment with your actual Slack tokens:

```bash
# Development
kubectl create secret generic slacker-secrets \
  --from-literal=slack-bot-token="xoxb-your-dev-bot-token" \
  --from-literal=slack-app-token="xapp-your-dev-app-token" \
  --namespace=marvin-dev

# Staging
kubectl create secret generic slacker-secrets \
  --from-literal=slack-bot-token="xoxb-your-staging-bot-token" \
  --from-literal=slack-app-token="xapp-your-staging-app-token" \
  --namespace=marvin-staging

# Production
kubectl create secret generic slacker-secrets \
  --from-literal=slack-bot-token="xoxb-your-production-bot-token" \
  --from-literal=slack-app-token="xapp-your-production-app-token" \
  --namespace=marvin-production
```

### Step 3: Deploy to Development

```bash
# Deploy to development
kubectl apply -k deploy/k8s/overlays/dev

# Verify deployment
kubectl get pods -n marvin-dev -w

# Check logs
kubectl logs -n marvin-dev -l app.kubernetes.io/name=slacker -f
```

### Step 4: Deploy to Staging

```bash
# Deploy to staging
kubectl apply -k deploy/k8s/overlays/staging

# Verify deployment
kubectl get pods -n marvin-staging

# Check service status
kubectl get svc -n marvin-staging
```

### Step 5: Deploy to Production

```bash
# Deploy to production (more cautious approach)
kubectl apply -k deploy/k8s/overlays/production

# Monitor rollout
kubectl rollout status statefulset/slacker -n marvin-production

# Verify health
kubectl get pods -n marvin-production
kubectl describe pod -n marvin-production -l app.kubernetes.io/name=slacker
```

## 🔧 Configuration Management

### Base Configuration
The base configuration in `deploy/k8s/base/` defines:
- StatefulSet with standard resource limits
- Service configuration
- Basic health checks
- Default storage settings

### Environment Overrides
Each environment in `overlays/` can override:
- Resource limits and requests
- Storage class and size
- Environment variables
- Replica counts
- Logging levels

### Customizing for Your Environment

#### Modify Resource Limits
Edit the patch files to match your cluster resources:

```yaml
# overlays/production/slacker-patch.yaml
spec:
  template:
    spec:
      containers:
      - name: slacker
        resources:
          limits:
            cpu: 2000m  # Adjust based on your needs
            memory: 2Gi
          requests:
            cpu: 1000m
            memory: 1Gi
```

#### Update Storage Configuration
```yaml
# overlays/production/storage-class-patch.yaml
spec:
  storageClassName: fast-ssd  # Use your high-performance storage class
  resources:
    requests:
      storage: 100Gi  # Adjust based on expected usage
```

#### Custom Configuration Files
Create environment-specific ConfigMaps:

```bash
# Create custom config for production
kubectl create configmap slacker-production-config \
  --from-file=marvin.hcl=./marvin.production.hcl \
  --namespace=marvin-production
```

## 📊 Monitoring and Observability

### Health Checks
Slacker includes built-in health endpoints:
- `/health` - Liveness probe (30s delay, 10s interval)
- `/ready` - Readiness probe (5s delay, 5s interval)

### Monitoring Setup

#### Metrics Collection
Add Prometheus monitoring:

```yaml
# monitoring.yaml
apiVersion: v1
kind: Service
metadata:
  name: slacker-metrics
  labels:
    app.kubernetes.io/name: slacker
  annotations:
    prometheus.io/scrape: "true"
    prometheus.io/port: "9090"
    prometheus.io/path: "/metrics"
spec:
  ports:
  - port: 9090
    targetPort: 9090
    name: metrics
  selector:
    app.kubernetes.io/name: slacker
```

#### Log Aggregation
Configure structured logging with Fluent Bit:

```yaml
# fluentbit-config.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: fluent-bit-config
data:
  fluent-bit.conf: |
    [SERVICE]
        Flush         1
        Log_Level     info
        Daemon        off
        Parsers_File  parsers.conf

    [INPUT]
        Name              tail
        Path              /var/log/containers/*slacker*.log
        Parser            docker
        Tag               kube.*
        Refresh_Interval  5

    [OUTPUT]
        Name  forward
        Match *
        Host  fluentd.logging.svc.cluster.local
        Port  24224
```

## 🔄 Lifecycle Management

### Rolling Updates

#### Update Image Version
```bash
# Update image tag
cd deploy/k8s/overlays/production
kustomize edit set image slacker=your-registry/slacker:v1.2.3

# Apply update
kubectl apply -k .

# Monitor rollout
kubectl rollout status statefulset/slacker -n marvin-production
```

#### Configuration Updates
```bash
# Update ConfigMap
kubectl create configmap slacker-production-config \
  --from-file=marvin.hcl=./marvin.production.hcl \
  --namespace=marvin-production \
  --dry-run=client -o yaml | kubectl apply -f -

# Restart to pick up changes
kubectl rollout restart statefulset/slacker -n marvin-production
```

### Scaling Operations

#### Manual Scaling
```bash
# Scale production deployment
kubectl scale statefulset slacker --replicas=3 -n marvin-production

# Scale back down
kubectl scale statefulset slacker --replicas=2 -n marvin-production
```

#### Auto-scaling
Configure Horizontal Pod Autoscaler:

```yaml
# hpa.yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: slacker-hpa
  namespace: marvin-production
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: StatefulSet
    name: slacker
  minReplicas: 2
  maxReplicas: 5
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
```

## 🔒 Security Hardening

### Network Policies
Restrict pod communication:

```yaml
# network-policy.yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: slacker-netpol
  namespace: marvin-production
spec:
  podSelector:
    matchLabels:
      app.kubernetes.io/name: slacker
  policyTypes:
  - Egress
  egress:
  - to: []
    ports:
    - protocol: TCP
      port: 443  # Slack API
    - protocol: TCP
      port: 80   # HTTP endpoints
    - protocol: TCP
      port: 53   # DNS
  - to:
    - namespaceSelector:
        matchLabels:
          name: monitoring
    ports:
    - protocol: TCP
      port: 9090  # Metrics
```

### Pod Security Context
```yaml
# Enhanced security in StatefulSet
securityContext:
  runAsNonRoot: true
  runAsUser: 1000
  runAsGroup: 1000
  fsGroup: 1000
  seccompProfile:
    type: RuntimeDefault
  capabilities:
    drop:
    - ALL
```

### RBAC Configuration
```yaml
# rbac.yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  namespace: marvin-production
  name: slacker-operator
rules:
- apiGroups: [""]
  resources: ["configmaps", "secrets"]
  verbs: ["get", "list", "watch"]
- apiGroups: [""]
  resources: ["pods"]
  verbs: ["get", "list"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: slacker-binding
  namespace: marvin-production
subjects:
- kind: ServiceAccount
  name: slacker
  namespace: marvin-production
roleRef:
  kind: Role
  name: slacker-operator
  apiGroup: rbac.authorization.k8s.io
```

## 🚨 Troubleshooting

### Common Issues

#### Pod Fails to Start
```bash
# Check pod events
kubectl describe pod -n marvin-production -l app.kubernetes.io/name=slacker

# Check logs
kubectl logs -n marvin-production -l app.kubernetes.io/name=slacker --previous

# Verify secrets
kubectl get secret slacker-secrets -n marvin-production -o yaml
```

#### Slack Connection Issues
```bash
# Test Slack API connectivity
kubectl exec -n marvin-production -it $(kubectl get pod -n marvin-production -l app.kubernetes.io/name=slacker -o jsonpath='{.items[0].metadata.name}') -- curl -I https://api.slack.com

# Verify tokens
kubectl get secret slacker-secrets -n marvin-production --template='{{.data.slack-bot-token | base64decode}}'
```

#### Storage Issues
```bash
# Check PVC status
kubectl get pvc -n marvin-production

# Check storage class
kubectl get storageclass

# Mount verification
kubectl exec -n marvin-production -it $(kubectl get pod -n marvin-production -l app.kubernetes.io/name=slacker -o jsonpath='{.items[0].metadata.name}') -- df -h
```

### Performance Debugging

#### Resource Usage
```bash
# Check resource usage
kubectl top pods -n marvin-production

# Get detailed metrics
kubectl describe pod -n marvin-production -l app.kubernetes.io/name=slacker
```

#### Network Debugging
```bash
# Test connectivity to LLM service
kubectl exec -n marvin-production -it $(kubectl get pod -n marvin-production -l app.kubernetes.io/name=slacker -o jsonpath='{.items[0].metadata.name}') -- curl -v http://ollama:11434/api/tags
```

## 📋 Best Practices

### Deployment Strategy
1. **Deploy to dev first** - Validate configuration
2. **Test in staging** - Verify integration
3. **Production rollout** - Use progressive deployment
4. **Monitor closely** - Watch for issues
5. **Rollback if needed** - Have rollback plan ready

### Configuration Management
- Use GitOps for configuration management
- Store secrets securely (Vault, AWS Secrets Manager)
- Implement proper backup procedures
- Document all customizations

### Operational Excellence
- Set up proper monitoring and alerting
- Implement log aggregation
- Regular security audits
- Performance tuning and optimization

## 🆘 Support

For additional support:
1. Check the [troubleshooting guide](../configuration/troubleshooting.md)
2. Review [monitoring setup](../deployment/monitoring.md)
3. Consult the [complete configuration reference](../configuration/complete-reference.md)
4. Check existing GitHub issues

Remember to never commit sensitive data like Slack tokens to version control.