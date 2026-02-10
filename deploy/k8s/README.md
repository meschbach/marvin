# Kubernetes Deployment with Kustomize

This directory contains Kustomize configurations for deploying Slacker on Kubernetes across different environments.

## 🏗️ Directory Structure

```
deploy/k8s/
├── base/                           # Base configuration
│   ├── kustomization.yaml         # Base Kustomize config
│   ├── slacker-statefulset.yaml   # StatefulSet definition
│   ├── slacker-service.yaml       # Service definition
│   ├── slacker-pvc.yaml           # Persistent Volume Claim
│   └── slacker-configmap.yaml     # Configuration
├── overlays/                      # Environment-specific overrides
│   ├── dev/                       # Development environment
│   │   ├── kustomization.yaml
│   │   └── slacker-patch.yaml
│   ├── staging/                   # Staging environment
│   │   ├── kustomization.yaml
│   │   └── slacker-patch.yaml
│   └── production/                # Production environment
│       ├── kustomization.yaml
│       ├── slacker-patch.yaml
│       └── storage-class-patch.yaml
└── README.md                      # This file
```

## 🚀 Quick Start

### Prerequisites
- Kubernetes cluster (v1.20+)
- kubectl configured
- kustomize installed
- Docker registry access for slacker image

### Environment Setup

#### 1. Create Secrets
Create secrets for Slack tokens in each namespace:

```bash
# Create namespaces
kubectl create namespace marvin-dev
kubectl create namespace marvin-staging
kubectl create namespace marvin-production

# Create secrets (replace with actual tokens)
kubectl create secret generic slacker-secrets \
  --from-literal=slack-bot-token="xoxb-your-bot-token" \
  --from-literal=slack-app-token="xapp-your-app-token" \
  --namespace=marvin-dev

kubectl create secret generic slacker-secrets \
  --from-literal=slack-bot-token="xoxb-your-bot-token" \
  --from-literal=slack-app-token="xapp-your-app-token" \
  --namespace=marvin-staging

kubectl create secret generic slacker-secrets \
  --from-literal=slack-bot-token="xoxb-your-bot-token" \
  --from-literal=slack-app-token="xapp-your-app-token" \
  --namespace=marvin-production
```

#### 2. Deploy to Development
```bash
# Deploy to dev environment
kubectl apply -k deploy/k8s/overlays/dev

# Check status
kubectl get pods -n marvin-dev
kubectl logs -n marvin-dev -l app.kubernetes.io/name=slacker
```

#### 3. Deploy to Staging
```bash
# Deploy to staging environment
kubectl apply -k deploy/k8s/overlays/staging

# Check status
kubectl get pods -n marvin-staging
kubectl logs -n marvin-staging -l app.kubernetes.io/name=slacker
```

#### 4. Deploy to Production
```bash
# Deploy to production environment
kubectl apply -k deploy/k8s/overlays/production

# Check status
kubectl get pods -n marvin-production
kubectl logs -n marvin-production -l app.kubernetes.io/name=slacker
```

## 🔧 Configuration

### Environment Differences

| Setting | Development | Staging | Production |
|---------|-------------|---------|-------------|
| Replicas | 1 | 1 | 2 |
| CPU Limits | 200m | 500m | 1000m |
| Memory Limits | 256Mi | 512Mi | 1Gi |
| Storage | 1Gi | 5Gi | 50Gi |
| Storage Class | standard | standard | ssd |
| Log Level | debug | info | warn |

### Customizing Configuration

#### Modify Base Configuration
Edit files in `deploy/k8s/base/` to change:
- Container images
- Basic resource settings
- Service configuration
- Health check settings

#### Override for Environment
Edit patch files in `deploy/k8s/overlays/{environment}/` to change:
- Resource limits and requests
- Environment variables
- Storage settings
- Replica counts

### Custom Configuration Files

Create environment-specific ConfigMaps by editing the ConfigMap references in patch files:

```yaml
# Example: Add custom configuration
data:
  marvin-production.hcl: |
    llm {
      model = "llama3.1:8b"
      host = "http://ollama.production.svc.cluster.local:11434"
      temperature = 0.1
      max_tokens = 4096
    }
    
    admin_users = ["U0PRODUCTION", "U0ADMIN"]
    
    assistant {
      name = "Production Assistant"
      personality = "professional and precise"
      system_prompt = "You are a professional AI assistant for production use. Be accurate, concise, and helpful."
    }
    
    # Production tools
    http_tool "api-gateway" {
      url = "https://api.company.com"
      headers = {
        "Authorization" = "Bearer ${API_GATEWAY_TOKEN}"
      }
      timeout = "30s"
    }
```

## 🔍 Monitoring and Troubleshooting

### Health Checks
- **Liveness Probe**: `/health` endpoint, 30s initial delay, 10s interval
- **Readiness Probe**: `/ready` endpoint, 5s initial delay, 5s interval

### Logs
```bash
# View logs for all environments
kubectl logs -n marvin-dev -l app.kubernetes.io/name=slacker -f
kubectl logs -n marvin-staging -l app.kubernetes.io/name=slacker -f
kubectl logs -n marvin-production -l app.kubernetes.io/name=slacker -f
```

### Debugging
```bash
# Get detailed pod information
kubectl describe pod -n marvin-dev -l app.kubernetes.io/name=slacker

# Access pod shell (for debugging)
kubectl exec -n marvin-dev -it $(kubectl get pod -n marvin-dev -l app.kubernetes.io/name=slacker -o jsonpath='{.items[0].metadata.name}') -- sh

# Check events
kubectl get events -n marvin-dev --sort-by='.lastTimestamp'
```

## 🔄 Updates and Maintenance

### Rolling Updates
```bash
# Update image version
kustomize edit set image slacker=slacker:v1.2.3

# Apply update
kubectl apply -k deploy/k8s/overlays/production

# Watch rollout status
kubectl rollout status statefulset/slacker -n marvin-production
```

### Scaling
```bash
# Scale production deployment
kubectl scale statefulset slacker --replicas=3 -n marvin-production
```

### Backup and Recovery
```bash
# Backup persistent data
kubectl exec -n marvin-production -it $(kubectl get pod -n marvin-production -l app.kubernetes.io/name=slacker -o jsonpath='{.items[0].metadata.name}') -- tar czf /tmp/backup.tar.gz /data

# Copy backup locally
kubectl cp marvin-production/$(kubectl get pod -n marvin-production -l app.kubernetes.io/name=slacker -o jsonpath='{.items[0].metadata.name}'):/tmp/backup.tar.gz ./backup.tar.gz
```

## 🔒 Security Considerations

### Network Policies
Consider adding network policies to restrict traffic:
```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: slacker-netpol
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
      port: 443  # HTTPS to Slack API
    - protocol: TCP
      port: 80   # HTTP to LLM endpoints
```

### RBAC
Configure appropriate RBAC permissions:
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  namespace: marvin-production
  name: slacker-operator
rules:
- apiGroups: [""]
  resources: ["configmaps", "secrets"]
  verbs: ["get", "list", "watch"]
```

## 📊 Performance Tuning

### Resource Optimization
- Monitor resource usage with `kubectl top pods`
- Adjust limits based on actual usage patterns
- Consider vertical pod autoscaling for production

### Storage Performance
- Use SSD storage class for production workloads
- Monitor I/O metrics and adjust storage quotas
- Implement backup strategies for persistent data

## 🆘 Support

For issues with Kubernetes deployment:
1. Check pod logs for error messages
2. Verify secret creation and token validity
3. Ensure network connectivity to Slack API
4. Validate LLM service accessibility
5. Review resource limits and requests

Refer to the main documentation for additional support resources.