# Production Deployment Guidelines

This guide provides comprehensive guidelines for deploying Marvin and Slacker in production environments, focusing on high availability, security, scalability, and operational excellence.

## 🎯 Production Architecture Overview

### Design Principles
- **High Availability**: Redundant components with automatic failover
- **Scalability**: Horizontal scaling capabilities for load handling
- **Security**: Defense-in-depth security posture
- **Observability**: Comprehensive monitoring and alerting
- **Maintainability**: Easy upgrades and maintenance procedures

### Reference Architecture
```
┌─────────────────────────────────────────────────────────────┐
│                        Internet                              │
└─────────────────────┬───────────────────────────────────────┘
                      │
┌─────────────────────▼───────────────────────────────────────┐
│                  Load Balancer (HTTPS)                     │
│                  WAF, Rate Limiting, SSL                    │
└─────────────────────┬───────────────────────────────────────┘
                      │
┌─────────────────────▼───────────────────────────────────────┐
│                  Kubernetes Cluster                          │
│                                                             │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐         │
│  │    Slacker  │  │    Slacker  │  │    Slacker  │         │
│  │  (Primary)  │  │ (Replica 1) │  │ (Replica 2) │         │
│  └─────────────┘  └─────────────┘  └─────────────┘         │
│                                                             │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐         │
│  │    Ollama   │  │    Redis    │  │  Monitoring │         │
│  │   (LLM)     │  │   (Cache)   │  │   (Prom)    │         │
│  └─────────────┘  └─────────────┘  └─────────────┘         │
│                                                             │
│  ┌─────────────────────────────────────────────────────┐   │
│  │            Persistent Storage (SSD)                  │   │
│  │            Sessions, Config, Logs                   │   │
│  └─────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

## 🏗️ Infrastructure Requirements

### Minimum Hardware Specifications
- **Control Plane**: 3 nodes, 4 vCPU, 8GB RAM, 100GB SSD
- **Worker Nodes**: 3+ nodes, 8 vCPU, 16GB RAM, 500GB SSD
- **Storage**: High-performance SSD with >10K IOPS
- **Network**: 10Gbps internal connectivity
- **Load Balancer**: Hardware or cloud-based with SSL termination

### Software Stack
- **Container Runtime**: containerd 1.6+
- **Kubernetes**: v1.25+ with CNI plugin
- **Ingress Controller**: NGINX Ingress Controller or Istio
- **Storage Class**: SSD-backed with volume expansion
- **Monitoring Stack**: Prometheus + Grafana + AlertManager

## 🔒 Security Architecture

### Network Security

#### Network Segmentation
```yaml
# Network Policy for Production
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: marvin-production-netpol
  namespace: marvin-production
spec:
  podSelector:
    matchLabels:
      app.kubernetes.io/part-of: marvin
  policyTypes:
  - Ingress
  - Egress
  ingress:
  - from:
    - namespaceSelector:
        matchLabels:
          name: ingress-nginx
    - podSelector:
        matchLabels:
          app.kubernetes.io/name: slacker
    ports:
    - protocol: TCP
      port: 8080
  egress:
  - to: []
    ports:
    - protocol: TCP
      port: 443  # HTTPS APIs
    - protocol: TCP
      port: 53   # DNS
    - protocol: UDP
      port: 53   # DNS
  - to:
    - podSelector:
        matchLabels:
          app.kubernetes.io/name: ollama
    ports:
    - protocol: TCP
      port: 11434
```

#### TLS Configuration
```yaml
# Certificate Management
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt-prod
spec:
  acme:
    server: https://acme-v02.api.letsencrypt.org/directory
    email: admin@yourcompany.com
    privateKeySecretRef:
      name: letsencrypt-prod
    solvers:
    - http01:
        ingress:
          class: nginx
```

### Secrets Management

#### HashiCorp Vault Integration
```yaml
# Vault Secret Injection
apiVersion: v1
kind: ServiceAccount
metadata:
  name: slacker
  namespace: marvin-production
---
apiVersion: secrets.hashicorp.com/v1beta1
kind: VaultDynamicSecret
metadata:
  name: slacker-slack-tokens
  namespace: marvin-production
spec:
  path: kvv2/marvin/production/slack
  refreshInterval: 1h
  destination:
    name: slacker-secrets
    create: true
```

#### Kubernetes Secrets Rotation
```bash
#!/bin/bash
# secrets-rotation.sh

# Rotate Slack tokens quarterly
NAMESPACE="marvin-production"
SECRET_NAME="slacker-secrets"

# Generate new tokens (automate via Slack API)
NEW_BOT_TOKEN=$(get_new_bot_token)
NEW_APP_TOKEN=$(get_new_app_token)

# Update secret
kubectl create secret generic $SECRET_NAME \
  --from-literal=slack-bot-token="$NEW_BOT_TOKEN" \
  --from-literal=slack-app-token="$NEW_APP_TOKEN" \
  --namespace=$NAMESPACE \
  --dry-run=client -o yaml | kubectl apply -f -

# Restart deployment to pick up new tokens
kubectl rollout restart statefulset/slacker -n $NAMESPACE
```

## 📈 High Availability

### Multi-AZ Deployment

#### Kubernetes Topology
```yaml
# Anti-affinity rules for high availability
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: slacker
spec:
  template:
    spec:
      affinity:
        podAntiAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            podAffinityTerm:
              labelSelector:
                matchExpressions:
                - key: app.kubernetes.io/name
                  operator: In
                  values:
                  - slacker
              topologyKey: kubernetes.io/hostname
          - weight: 50
            podAffinityTerm:
              labelSelector:
                matchExpressions:
                - key: app.kubernetes.io/name
                  operator: In
                  values:
                  - slacker
              topologyKey: topology.kubernetes.io/zone
```

#### Database Redundancy
```yaml
# Redis Cluster Configuration
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: redis-cluster
spec:
  replicas: 6
  template:
    spec:
      containers:
      - name: redis
        image: redis:7-alpine
        command:
        - redis-server
        - --cluster-enabled
        - "yes"
        - --cluster-config-file
        - /data/nodes.conf
        - --cluster-node-timeout
        - "5000"
        - --appendonly
        - "yes"
```

### Disaster Recovery

#### Backup Strategy
```bash
#!/bin/bash
# production-backup.sh

NAMESPACE="marvin-production"
BACKUP_DIR="/backup/marvin"
DATE=$(date +%Y%m%d_%H%M%S)
S3_BUCKET="marvin-backups"

# Backup persistent volumes
kubectl exec -n $NAMESPACE deployment/slacker -- tar czf /tmp/data_backup_$DATE.tar.gz /data
kubectl cp $NAMESPACE/deployment/slacker:/tmp/data_backup_$DATE.tar.gz $BACKUP_DIR/

# Backup Kubernetes resources
kubectl get all -n $NAMESPACE -o yaml > $BACKUP_DIR/k8s_resources_$DATE.yaml
kubectl get configmaps -n $NAMESPACE -o yaml >> $BACKUP_DIR/k8s_resources_$DATE.yaml
kubectl get secrets -n $NAMESPACE -o yaml >> $BACKUP_DIR/k8s_resources_$DATE.yaml

# Upload to S3
aws s3 cp $BACKUP_DIR/data_backup_$DATE.tar.gz s3://$S3_BUCKET/
aws s3 cp $BACKUP_DIR/k8s_resources_$DATE.yaml s3://$S3_BUCKET/

# Cleanup local files (keep 7 days)
find $BACKUP_DIR -name "*.tar.gz" -mtime +7 -delete
find $BACKUP_DIR -name "*.yaml" -mtime +7 -delete

echo "Backup completed: $DATE"
```

#### Restore Procedure
```bash
#!/bin/bash
# production-restore.sh

BACKUP_DATE=$1
NAMESPACE="marvin-production"
BACKUP_DIR="/backup/marvin"
S3_BUCKET="marvin-backups"

if [ -z "$BACKUP_DATE" ]; then
    echo "Usage: $0 <backup_date>"
    exit 1
fi

# Download from S3
aws s3 cp s3://$S3_BUCKET/data_backup_$BACKUP_DATE.tar.gz $BACKUP_DIR/
aws s3 cp s3://$S3_BUCKET/k8s_resources_$BACKUP_DATE.yaml $BACKUP_DIR/

# Stop services
kubectl scale deployment slacker --replicas=0 -n $NAMESPACE

# Restore data
kubectl cp $BACKUP_DIR/data_backup_$BACKUP_DATE.tar.gz $NAMESPACE/$(kubectl get pod -n $NAMESPACE -l app.kubernetes.io/name=slacker -o jsonpath='{.items[0].metadata.name}'):/tmp/
kubectl exec -n $NAMESPACE deployment/slacker -- tar xzf /tmp/data_backup_$BACKUP_DATE.tar.gz -C /

# Restart services
kubectl scale deployment slacker --replicas=3 -n $NAMESPACE

echo "Restore completed: $BACKUP_DATE"
```

## 📊 Monitoring and Observability

### Prometheus Configuration

#### ServiceMonitors
```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: slacker-metrics
  namespace: marvin-production
spec:
  selector:
    matchLabels:
      app.kubernetes.io/name: slacker
  endpoints:
  - port: metrics
    interval: 30s
    path: /metrics
---
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: slacker-alerts
  namespace: marvin-production
spec:
  groups:
  - name: marvin.rules
    rules:
    - alert: SlackerDown
      expr: up{job="slacker"} == 0
      for: 1m
      labels:
        severity: critical
      annotations:
        summary: "Slacker instance is down"
        description: "Slacker has been down for more than 1 minute."
    - alert: HighMemoryUsage
      expr: container_memory_usage_bytes{name="slacker"} / container_spec_memory_limit_bytes{name="slacker"} > 0.9
      for: 5m
      labels:
        severity: warning
      annotations:
        summary: "High memory usage detected"
        description: "Slacker memory usage is above 90%."
```

### Logging Architecture

#### Fluent Bit Configuration
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: fluent-bit-config
data:
  fluent-bit.conf: |
    [SERVICE]
        Flush         5
        Log_Level     info
        Daemon        off
        HTTP_Server   On
        HTTP_Listen   0.0.0.0
        HTTP_Port     2020

    [INPUT]
        Name              tail
        Path              /var/log/containers/*slacker*.log
        Parser            docker
        Tag               kube.*
        Refresh_Interval  5
        Mem_Buf_Limit     50MB
        Skip_Long_Lines   On

    [FILTER]
        Name                kubernetes
        Match               kube.*
        Kube_URL            https://kubernetes.default.svc:443
        Kube_CA_File        /var/run/secrets/kubernetes.io/serviceaccount/ca.crt
        Kube_Token_File     /var/run/secrets/kubernetes.io/serviceaccount/token
        Merge_Log           On
        K8S-Logging.Parser  On
        K8S-Logging.Exclude On

    [OUTPUT]
        Name  es
        Match *
        Host  elasticsearch.logging.svc.cluster.local
        Port  9200
        Index marvin-logs
        Type  _doc
```

## 🔄 Deployment Strategies

### Blue-Green Deployment
```bash
#!/bin/bash
# blue-green-deploy.sh

NAMESPACE="marvin-production"
NEW_VERSION=$1

if [ -z "$NEW_VERSION" ]; then
    echo "Usage: $0 <new_version>"
    exit 1
fi

# Deploy to green environment
kubectl apply -k deploy/k8s/overlays/production-green

# Wait for green to be ready
kubectl wait --for=condition=available deployment/slacker-green -n $NAMESPACE --timeout=300s

# Run smoke tests
./scripts/smoke-tests.sh green

# Switch traffic
kubectl patch service slacker -n $NAMESPACE -p '{"spec":{"selector":{"version":"green"}}}'

# Wait and verify
sleep 30
./scripts/smoke-tests.sh production

# Clean up blue environment
kubectl delete -k deploy/k8s/overlays/production-blue

echo "Blue-green deployment completed: $NEW_VERSION"
```

### Canary Deployment
```yaml
# Istio VirtualService for Canary
apiVersion: networking.istio.io/v1beta1
kind: VirtualService
metadata:
  name: slacker
spec:
  http:
  - match:
    - headers:
        canary:
          exact: "true"
    route:
    - destination:
        host: slacker
        subset: canary
      weight: 100
  - route:
    - destination:
        host: slacker
        subset: stable
      weight: 90
    - destination:
        host: slacker
        subset: canary
      weight: 10
```

## 🔧 Performance Optimization

### Resource Tuning

#### JVM Optimization (if using Java-based components)
```yaml
env:
- name: JAVA_OPTS
  value: >-
    -Xms512m -Xmx2g
    -XX:+UseG1GC
    -XX:MaxGCPauseMillis=200
    -XX:+UnlockExperimentalVMOptions
    -XX:+UseCGroupMemoryLimitForHeap
    -Djava.security.egd=file:/dev/./urandom
```

#### Database Optimization
```yaml
# PostgreSQL Configuration (if using)
apiVersion: v1
kind: ConfigMap
metadata:
  name: postgres-config
data:
  postgresql.conf: |
    # Memory settings
    shared_buffers = 256MB
    effective_cache_size = 1GB
    work_mem = 4MB
    
    # Connection settings
    max_connections = 200
    max_prepared_transactions = 200
    
    # Performance settings
    random_page_cost = 1.1
    effective_io_concurrency = 200
```

### Caching Strategy

#### Multi-Level Cache
```yaml
# Redis Configuration for Caching
apiVersion: apps/v1
kind: Deployment
metadata:
  name: redis-cache
spec:
  template:
    spec:
      containers:
      - name: redis
        image: redis:7-alpine
        command:
        - redis-server
        - --maxmemory
        - 512mb
        - --maxmemory-policy
        - allkeys-lru
        - --save
        - "900 1"
        - --save
        - "300 10"
        resources:
          requests:
            memory: 256Mi
            cpu: 100m
          limits:
            memory: 512Mi
            cpu: 500m
```

## 🧪 Testing in Production

### Feature Flags
```yaml
# Feature Flag Configuration
apiVersion: v1
kind: ConfigMap
metadata:
  name: feature-flags
data:
  enable_new_llm_model: "false"
  enable_advanced_tools: "true"
  enable_analytics: "false"
```

### Chaos Engineering

#### Chaos Mesh Experiments
```yaml
apiVersion: chaos-mesh.org/v1alpha1
kind: PodChaos
metadata:
  name: slacker-pod-failure
spec:
  selector:
    namespaces:
      - marvin-production
    labelSelectors:
      app.kubernetes.io/name: slacker
  mode: one
  action: pod-kill
  duration: "30s"
```

## 📋 Operational Procedures

### Routine Maintenance

#### Weekly Checklist
- [ ] Review system logs for anomalies
- [ ] Check resource utilization trends
- [ ] Verify backup integrity
- [ ] Update security patches
- [ ] Review and rotate secrets
- [ ] Performance baseline verification

#### Monthly Checklist
- [ ] Disaster recovery drill
- [ ] Security audit
- [ ] Capacity planning review
- [ ] Cost optimization analysis
- [ ] Documentation updates

### Incident Response

#### Severity Levels
- **P0**: Service完全不可用，影响所有用户
- **P1**: 核心功能不可用，影响大部分用户
- **P2**: 部分功能不可用，影响少数用户
- **P3**: 性能下降或非关键功能问题

#### Response Team
```yaml
# On-call Schedule
apiVersion: v1
kind: ConfigMap
metadata:
  name: oncall-schedule
data:
  primary: "oncall-primary@yourcompany.com"
  secondary: "oncall-secondary@yourcompany.com"
  escalation: "sre-team@yourcompany.com"
  slack_channel: "#marvin-alerts"
```

## 🎯 Success Metrics

### SLOs and SLIs

#### Service Level Objectives
- **Availability**: 99.9% (monthly)
- **Latency**: P95 < 2 seconds
- **Error Rate**: < 0.1%
- **Recovery Time**: < 5 minutes for P0 incidents

#### Service Level Indicators
```yaml
# Prometheus SLI Rules
- record: marvin:availability:5m
  expr: sum(rate(http_requests_total{status!~"5.."}[5m])) / sum(rate(http_requests_total[5m]))

- record: marvin:latency:p95:5m
  expr: histogram_quantile(0.95, sum(rate(http_request_duration_seconds_bucket[5m])) by (le))

- record: marvin:error_rate:5m
  expr: sum(rate(http_requests_total{status=~"5.."}[5m])) / sum(rate(http_requests_total[5m]))
```

### Business Metrics
- **User Engagement**: Daily active users
- **Tool Usage**: Tool execution success rate
- **Response Quality**: User satisfaction scores
- **System Health**: Uptime and performance trends

## 🚨 Emergency Procedures

### Immediate Response
1. **Assess Impact**: Determine severity and affected users
2. **Communicate**: Notify stakeholders and users
3. **Stabilize**: Apply temporary fixes to stop bleeding
4. **Investigate**: Root cause analysis
5. **Resolve**: Implement permanent fix
6. **Review**: Post-mortem and prevention

### Escalation Matrix
```bash
# Alert Escalation
if [ $SEVERITY == "P0" ]; then
    notify primary-oncall
    sleep 5m
    if [ ! $RESOLVED ]; then
        notify secondary-oncall
        notify engineering-manager
    fi
fi
```

This production deployment guide provides a comprehensive framework for running Marvin and Slacker in enterprise environments with proper security, reliability, and operational excellence.