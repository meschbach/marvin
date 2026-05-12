# Monitoring and Observability Setup

This comprehensive guide covers setting up monitoring, logging, and observability for Marvin and Slacker deployments to
ensure reliable operation, performance optimization, and effective troubleshooting.

## 🎯 Observability Stack Overview

### Components
- **Metrics Collection**: Prometheus for system and application metrics
- **Visualization**: Grafana for dashboards and alerting
- **Log Aggregation**: Fluent Bit + Elasticsearch + Kibana (ELK) stack
- **Tracing**: Jaeger for distributed tracing (optional)
- **Alerting**: AlertManager for incident notification

### Architecture
```
┌─────────────────────────────────────────────────────────────┐
│                    Marvin/Slacker Pods                       │
│                                                             │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐         │
│  │   Slacker   │  │   Ollama    │  │   Redis     │         │
│  │             │  │             │  │             │         │
│  │ Metrics:8080│  │Metrics:11434│  │Metrics:9121 │         │
│  │ Logs:/var/log│  │ Logs:/var/log│  │ Logs:/var/log│       │
│  └─────────────┘  └─────────────┘  └─────────────┘         │
└─────────────────────┬───────────────────────────────────────┘
                      │
┌─────────────────────▼───────────────────────────────────────┐
│                    Monitoring Layer                          │
│                                                             │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐         │
│  │ Prometheus  │  │ Fluent Bit  │  │ AlertManager│         │
│  │   (Scrape)  │  │  (Collect)  │  │ (Notify)    │         │
│  └─────────────┘  └─────────────┘  └─────────────┘         │
│                                                             │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐         │
│  │  Grafana    │  │Elasticsearch│  │   Jaeger    │         │
│  │ (Dashboard) │  │   (Store)   │  │ (Tracing)   │         │
│  └─────────────┘  └─────────────┘  └─────────────┘         │
└─────────────────────────────────────────────────────────────┘
```

## 📊 Metrics Collection

### Prometheus Configuration

#### prometheus.yml
```yaml
global:
  scrape_interval: 15s
  evaluation_interval: 15s
  external_labels:
    cluster: 'marvin-production'
    replica: 'prometheus-1'

rule_files:
  - "/etc/prometheus/rules/*.yml"

alerting:
  alertmanagers:
    - static_configs:
        - targets:
          - alertmanager:9093

scrape_configs:
  # Slacker metrics
  - job_name: 'slacker'
    kubernetes_sd_configs:
      - role: pod
        namespaces:
          names:
            - marvin-production
    relabel_configs:
      - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_scrape]
        action: keep
        regex: true
      - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_path]
        action: replace
        target_label: __metrics_path__
        regex: (.+)
      - source_labels: [__address__, __meta_kubernetes_pod_annotation_prometheus_io_port]
        action: replace
        regex: ([^:]+)(?::\d+)?;(\d+)
        replacement: $1:$2
        target_label: __address__
      - action: labelmap
        regex: __meta_kubernetes_pod_label_(.+)
      - source_labels: [__meta_kubernetes_namespace]
        action: replace
        target_label: kubernetes_namespace
      - source_labels: [__meta_kubernetes_pod_name]
        action: replace
        target_label: kubernetes_pod_name

  # Ollama metrics
  - job_name: 'ollama'
    static_configs:
      - targets: ['ollama:11434']
    metrics_path: '/metrics'
    scrape_interval: 30s

  # Kubernetes metrics
  - job_name: 'kubernetes-nodes'
    kubernetes_sd_configs:
      - role: node
    relabel_configs:
      - action: labelmap
        regex: __meta_kubernetes_node_label_(.+)

  # Redis metrics
  - job_name: 'redis'
    static_configs:
      - targets: ['redis-exporter:9121']
```

#### ServiceMonitors for Operator
```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: slacker-metrics
  namespace: marvin-production
  labels:
    app.kubernetes.io/name: slacker
    app.kubernetes.io/part-of: marvin
spec:
  selector:
    matchLabels:
      app.kubernetes.io/name: slacker
  endpoints:
  - port: metrics
    interval: 30s
    path: /metrics
    honorLabels: true
---
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: ollama-metrics
  namespace: marvin-production
spec:
  selector:
    matchLabels:
      app.kubernetes.io/name: ollama
  endpoints:
  - port: metrics
    interval: 30s
```

### Custom Metrics in Slacker

#### Metrics Endpoint Implementation
```go
// metrics.go
package main

import (
    "net/http"
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
    // Request metrics
    requestCount = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "marvin_requests_total",
            Help: "Total number of requests processed",
        },
        []string{"method", "status", "user"},
    )

    responseDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name: "marvin_request_duration_seconds",
            Help: "Request duration in seconds",
            Buckets: prometheus.DefBuckets,
        },
        []string{"method", "status"},
    )

    // LLM metrics
    llmRequestCount = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "marvin_llm_requests_total",
            Help: "Total number of LLM requests",
        },
        []string{"model", "status"},
    )

    llmTokensUsed = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "marvin_llm_tokens_used_total",
            Help: "Total number of tokens used",
        },
        []string{"model", "type"}, // type: prompt, response
    )

    // Tool execution metrics
    toolExecutionCount = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "marvin_tool_executions_total",
            Help: "Total number of tool executions",
        },
        []string{"tool", "status"},
    )

    toolExecutionDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name: "marvin_tool_execution_duration_seconds",
            Help: "Tool execution duration in seconds",
            Buckets: prometheus.DefBuckets,
        },
        []string{"tool", "status"},
    )

    // Slack metrics
    slackConnectionStatus = prometheus.NewGauge(
        prometheus.GaugeOpts{
            Name: "marvin_slack_connection_status",
            Help: "Slack connection status (1 = connected, 0 = disconnected)",
        },
    )

    activeUsers = prometheus.NewGauge(
        prometheus.GaugeOpts{
            Name: "marvin_active_users",
            Help: "Number of currently active users",
        },
    )
)

func init() {
    prometheus.MustRegister(requestCount)
    prometheus.MustRegister(responseDuration)
    prometheus.MustRegister(llmRequestCount)
    prometheus.MustRegister(llmTokensUsed)
    prometheus.MustRegister(toolExecutionCount)
    prometheus.MustRegister(toolExecutionDuration)
    prometheus.MustRegister(slackConnectionStatus)
    prometheus.MustRegister(activeUsers)
}

func MetricsHandler() http.Handler {
    return promhttp.Handler()
}

// Record metrics helpers
func RecordRequest(method, status, user string, duration float64) {
    requestCount.WithLabelValues(method, status, user).Inc()
    responseDuration.WithLabelValues(method, status).Observe(duration)
}

func RecordLLMRequest(model, status string, promptTokens, responseTokens int) {
    llmRequestCount.WithLabelValues(model, status).Inc()
    llmTokensUsed.WithLabelValues(model, "prompt").Add(float64(promptTokens))
    llmTokensUsed.WithLabelValues(model, "response").Add(float64(responseTokens))
}

func RecordToolExecution(tool, status string, duration float64) {
    toolExecutionCount.WithLabelValues(tool, status).Inc()
    toolExecutionDuration.WithLabelValues(tool, status).Observe(duration)
}
```

## 📈 Grafana Dashboards

### Main Dashboard Configuration

#### marvin-overview.json
```json
{
  "dashboard": {
    "id": null,
    "title": "Marvin & Slacker Overview",
    "tags": ["marvin", "slacker"],
    "timezone": "browser",
    "panels": [
      {
        "id": 1,
        "title": "Request Rate",
        "type": "stat",
        "targets": [
          {
            "expr": "sum(rate(marvin_requests_total[5m]))",
            "legendFormat": "Requests/sec"
          }
        ],
        "fieldConfig": {
          "defaults": {
            "unit": "reqps",
            "min": 0
          }
        }
      },
      {
        "id": 2,
        "title": "Response Time",
        "type": "graph",
        "targets": [
          {
            "expr": "histogram_quantile(0.95, sum(rate(marvin_request_duration_seconds_bucket[5m])) by (le))",
            "legendFormat": "95th percentile"
          },
          {
            "expr": "histogram_quantile(0.50, sum(rate(marvin_request_duration_seconds_bucket[5m])) by (le))",
            "legendFormat": "50th percentile"
          }
        ],
        "yAxes": [
          {
            "unit": "s",
            "min": 0
          }
        ]
      },
      {
        "id": 3,
        "title": "Active Users",
        "type": "stat",
        "targets": [
          {
            "expr": "marvin_active_users",
            "legendFormat": "Active Users"
          }
        ]
      },
      {
        "id": 4,
        "title": "Tool Executions",
        "type": "piechart",
        "targets": [
          {
            "expr": "sum(rate(marvin_tool_executions_total[1h])) by (tool)",
            "legendFormat": "{{tool}}"
          }
        ]
      },
      {
        "id": 5,
        "title": "LLM Token Usage",
        "type": "graph",
        "targets": [
          {
            "expr": "sum(rate(marvin_llm_tokens_used_total[5m])) by (type)",
            "legendFormat": "{{type}} tokens/sec"
          }
        ]
      },
      {
        "id": 6,
        "title": "Slack Connection Status",
        "type": "stat",
        "targets": [
          {
            "expr": "marvin_slack_connection_status",
            "legendFormat": "Connected"
          }
        ],
        "fieldConfig": {
          "defaults": {
            "mappings": [
              {
                "options": {
                  "0": {
                    "text": "Disconnected",
                    "color": "red"
                  },
                  "1": {
                    "text": "Connected",
                    "color": "green"
                  }
                },
                "type": "value"
              }
            ]
          }
        }
      }
    ],
    "time": {
      "from": "now-1h",
      "to": "now"
    },
    "refresh": "30s"
  }
}
```

### System Dashboard
```json
{
  "dashboard": {
    "title": "Marvin System Health",
    "panels": [
      {
        "title": "CPU Usage",
        "targets": [
          {
            "expr": "sum(rate(container_cpu_usage_seconds_total{container=\"slacker\"}[5m])) by (pod)",
            "legendFormat": "{{pod}}"
          }
        ]
      },
      {
        "title": "Memory Usage",
        "targets": [
          {
            "expr": "sum(container_memory_usage_bytes{container=\"slacker\"}) by (pod) / 1024 / 1024",
            "legendFormat": "{{pod}} MB"
          }
        ]
      },
      {
        "title": "Network I/O",
        "targets": [
          {
            "expr": "sum(rate(container_network_transmit_bytes_total{container=\"slacker\"}[5m])) by (pod)",
            "legendFormat": "{{pod}} TX"
          },
          {
            "expr": "sum(rate(container_network_receive_bytes_total{container=\"slacker\"}[5m])) by (pod)",
            "legendFormat": "{{pod}} RX"
          }
        ]
      }
    ]
  }
}
```

## 🚨 Alerting Configuration

### Alerting Rules
```yaml
# prometheus-rules.yml
groups:
  - name: marvin.rules
    rules:
      # Service availability alerts
      - alert: MarvinSlackerDown
        expr: up{job="slacker"} == 0
        for: 1m
        labels:
          severity: critical
          team: platform
        annotations:
          summary: "Slacker service is down"
          description: "Slacker has been down for more than 1 minute. Instance: {{ $labels.instance }}"

      - alert: MarvinSlackConnectionLost
        expr: marvin_slack_connection_status == 0
        for: 2m
        labels:
          severity: critical
          team: platform
        annotations:
          summary: "Slack connection lost"
          description: "Slacker has lost connection to Slack for more than 2 minutes"

      # Performance alerts
      - alert: MarvinHighResponseTime
        expr: histogram_quantile(0.95, sum(rate(marvin_request_duration_seconds_bucket[5m])) by (le)) > 5
        for: 5m
        labels:
          severity: warning
          team: platform
        annotations:
          summary: "High response time detected"
          description: "95th percentile response time is {{ $value }}s for more than 5 minutes"

      - alert: MarvinErrorRateHigh
        expr: sum(rate(marvin_requests_total{status=~"5.."}[5m])) / sum(rate(marvin_requests_total[5m])) > 0.05
        for: 3m
        labels:
          severity: warning
          team: platform
        annotations:
          summary: "High error rate detected"
          description: "Error rate is {{ $value | humanizePercentage }} for more than 3 minutes"

      # Resource alerts
      - alert: MarvinHighMemoryUsage
        expr: sum(container_memory_usage_bytes{container="slacker"}) by (pod) / sum(container_spec_memory_limit_bytes{container="slacker"}) by (pod) > 0.9
        for: 5m
        labels:
          severity: warning
          team: platform
        annotations:
          summary: "High memory usage"
          description: "Memory usage is above 90% for pod {{ $labels.pod }}"

      - alert: MarvinHighCPUUsage
        expr: sum(rate(container_cpu_usage_seconds_total{container="slacker"}[5m])) by (pod) > 0.8
        for: 5m
        labels:
          severity: warning
          team: platform
        annotations:
          summary: "High CPU usage"
          description: "CPU usage is above 80% for pod {{ $labels.pod }}"

      # LLM alerts
      - alert: MarvinLLMHighFailureRate
        expr: sum(rate(marvin_llm_requests_total{status="error"}[5m])) / sum(rate(marvin_llm_requests_total[5m])) > 0.1
        for: 2m
        labels:
          severity: critical
          team: platform
        annotations:
          summary: "High LLM failure rate"
          description: "LLM request failure rate is {{ $value | humanizePercentage }} for more than 2 minutes"

      # Tool execution alerts
      - alert: MarvinToolFailureRate
        expr: sum(rate(marvin_tool_executions_total{status="error"}[5m])) / sum(rate(marvin_tool_executions_total[5m])) > 0.2
        for: 3m
        labels:
          severity: warning
          team: platform
        annotations:
          summary: "High tool failure rate"
          description: "Tool execution failure rate is {{ $value | humanizePercentage }} for more than 3 minutes"

  - name: kubernetes.rules
    rules:
      - alert: KubernetesPodCrashLooping
        expr: rate(kube_pod_container_status_restarts_total[15m]) > 0
        for: 1m
        labels:
          severity: warning
          team: platform
        annotations:
          summary: "Pod is crash looping"
          description: "Pod {{ $labels.namespace }}/{{ $labels.pod }} is crash looping"

      - alert: KubernetesNodeNotReady
        expr: kube_node_status_condition{condition="Ready",status="true"} == 0
        for: 10m
        labels:
          severity: critical
          team: platform
        annotations:
          summary: "Kubernetes node not ready"
          description: "Node {{ $labels.node }} has been not ready for more than 10 minutes"
```

### AlertManager Configuration
```yaml
# alertmanager.yml
global:
  smtp_smarthost: 'smtp.company.com:587'
  smtp_from: 'alerts@company.com'
  smtp_auth_username: 'alerts@company.com'
  smtp_auth_password: 'password'

route:
  group_by: ['alertname', 'cluster', 'service']
  group_wait: 10s
  group_interval: 10s
  repeat_interval: 12h
  receiver: 'web.hook'
  routes:
  - match:
      severity: critical
    receiver: 'critical-alerts'
    continue: true
  - match:
      severity: warning
    receiver: 'warning-alerts'
    continue: true

receivers:
- name: 'web.hook'
  webhook_configs:
  - url: 'http://alertmanager-webhook:8080/webhook'

- name: 'critical-alerts'
  email_configs:
  - to: 'oncall-critical@company.com'
    subject: '[CRITICAL] Marvin Alert: {{ .GroupLabels.alertname }}'
    body: |
      {{ range .Alerts }}
      Alert: {{ .Annotations.summary }}
      Description: {{ .Annotations.description }}
      Labels: {{ range .Labels.SortedPairs }}{{ .Name }}={{ .Value }} {{ end }}
      {{ end }}
  slack_configs:
  - api_url: 'https://hooks.slack.com/services/YOUR/SLACK/WEBHOOK'
    channel: '#marvin-alerts'
    title: '🚨 Critical Marvin Alert'
    text: '{{ range .Alerts }}{{ .Annotations.summary }}{{ end }}'

- name: 'warning-alerts'
  email_configs:
  - to: 'marvin-team@company.com'
    subject: '[WARNING] Marvin Alert: {{ .GroupLabels.alertname }}'
  slack_configs:
  - api_url: 'https://hooks.slack.com/services/YOUR/SLACK/WEBHOOK'
    channel: '#marvin-alerts'
    title: '⚠️ Warning Marvin Alert'
    color: 'warning'
```

## 📝 Log Management

### Fluent Bit Configuration

#### fluent-bit.conf
```ini
[SERVICE]
    Flush         5
    Log_Level     info
    Daemon        off
    HTTP_Server   On
    HTTP_Listen   0.0.0.0
    HTTP_Port     2020
    Parsers_File  parsers.conf

[INPUT]
    Name              tail
    Path              /var/log/containers/*slacker*.log
    Parser            docker
    Tag               kube.slacker
    Refresh_Interval  5
    Mem_Buf_Limit     50MB
    Skip_Long_Lines   On
    Buffer_Chunk_Size 1MB
    Buffer_Max_Size   5MB

[INPUT]
    Name              tail
    Path              /var/log/containers/*ollama*.log
    Parser            docker
    Tag               kube.ollama
    Refresh_Interval  5
    Mem_Buf_Limit     50MB

[FILTER]
    Name                kubernetes
    Match               kube.*
    Kube_URL            https://kubernetes.default.svc:443
    Kube_CA_File        /var/run/secrets/kubernetes.io/serviceaccount/ca.crt
    Kube_Token_File     /var/run/secrets/kubernetes.io/serviceaccount/token
    Merge_Log           On
    K8S-Logging.Parser  On
    K8S-Logging.Exclude On
    Labels              On
    Annotations         On

[FILTER]
    Name                modify
    Match               kube.*
    Add                 cluster marvin-production
    Add                 environment production

[OUTPUT]
    Name  es
    Match kube.*
    Host  elasticsearch.logging.svc.cluster.local
    Port  9200
    Index marvin-logs
    Type  _doc
    Logstash_Format On
    Logstash_Prefix marvin
    Logstash_DateFormat %Y.%m.%d
    Include_Tag_Key On
    Tag_Key @log_name
    Retry_Limit False
```

#### parsers.conf
```ini
[PARSER]
    Name   docker
    Format json
    Time_Key time
    Time_Format %Y-%m-%dT%H:%M:%S.%L
    Time_Keep   On
    Decode_Field_as_json log

[PARSER]
    Name   marvin
    Format regex
    Regex  ^(?<time>[^ ]*) (?<level>[^ ]*) (?<message>.*)$
    Time_Key time
    Time_Format %Y-%m-%dT%H:%M:%S.%L
```

### Elasticsearch Index Templates
```json
{
  "index_patterns": ["marvin-*"],
  "template": {
    "settings": {
      "number_of_shards": 3,
      "number_of_replicas": 1,
      "index.lifecycle.name": "marvin-ilm-policy",
      "index.lifecycle.rollover_alias": "marvin-logs"
    },
    "mappings": {
      "properties": {
        "@timestamp": {
          "type": "date"
        },
        "level": {
          "type": "keyword"
        },
        "kubernetes": {
          "properties": {
            "pod_name": {
              "type": "keyword"
            },
            "namespace": {
              "type": "keyword"
            },
            "labels": {
              "type": "object"
            }
          }
        },
        "log": {
          "type": "text"
        },
        "cluster": {
          "type": "keyword"
        },
        "environment": {
          "type": "keyword"
        }
      }
    }
  }
}
```

### Kibana Dashboards

#### Log Analysis Dashboard
```json
{
  "dashboard": {
    "title": "Marvin Log Analysis",
    "panels": [
      {
        "title": "Log Levels Over Time",
        "type": "histogram",
        "queries": [
          {
            "query": "kubernetes.labels.app.kubernetes.io/name:slacker",
            "aggregation": "date_histogram",
            "field": "@timestamp"
          }
        ]
      },
      {
        "title": "Error Messages",
        "type": "table",
        "queries": [
          {
            "query": "level:ERROR AND kubernetes.labels.app.kubernetes.io/name:slacker",
            "fields": ["@timestamp", "message", "kubernetes.pod_name"]
          }
        ]
      }
    ]
  }
}
```

## 🔍 Distributed Tracing (Optional)

### Jaeger Integration
```yaml
apiVersion: jaegertracing.io/v1
kind: Jaeger
metadata:
  name: marvin-jaeger
spec:
  strategy: production
  storage:
    type: elasticsearch
    elasticsearch:
      nodeCount: 3
      storage:
        size: 100Gi
```

### OpenTelemetry in Slacker
```go
// tracing.go
package main

import (
    "context"
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/exporters/jaeger"
    "go.opentelemetry.io/otel/sdk/resource"
    "go.opentelemetry.io/otel/sdk/trace"
    semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
)

func initTracing() {
    exporter, err := jaeger.New(jaeger.WithCollectorEndpoint(jaeger.WithEndpoint("http://jaeger-collector:14268/api/traces")))
    if err != nil {
        log.Fatalf("Failed to create Jaeger exporter: %v", err)
    }

    tp := trace.NewTracerProvider(
        trace.WithBatcher(exporter),
        trace.WithResource(resource.NewWithAttributes(
            semconv.SchemaURL,
            semconv.ServiceNameKey.String("marvin-slacker"),
        )),
    )

    otel.SetTracerProvider(tp)
}

func traceLLMRequest(ctx context.Context, model, prompt string) (context.Context, trace.Span) {
    tracer := otel.Tracer("marvin-llm")
    return tracer.Start(ctx, "llm_request",
        trace.WithAttributes(
            attribute.String("llm.model", model),
            attribute.String("llm.prompt_length", strconv.Itoa(len(prompt))),
        ),
    )
}

func traceToolExecution(ctx context.Context, tool string) (context.Context, trace.Span) {
    tracer := otel.Tracer("marvin-tools")
    return tracer.Start(ctx, "tool_execution",
        trace.WithAttributes(
            attribute.String("tool.name", tool),
        ),
    )
}
```

## 📋 Health Checks and Probes

### Health Endpoints Implementation
```go
// health.go
package main

import (
    "encoding/json"
    "net/http"
    "time"
)

type HealthStatus struct {
    Status    string                 `json:"status"`
    Timestamp time.Time              `json:"timestamp"`
    Checks    map[string]CheckResult `json:"checks"`
}

type CheckResult struct {
    Status  string        `json:"status"`
    Message string        `json:"message,omitempty"`
    Latency time.Duration `json:"latency,omitempty"`
}

func HealthHandler(w http.ResponseWriter, r *http.Request) {
    status := HealthStatus{
        Status:    "healthy",
        Timestamp: time.Now(),
        Checks:    make(map[string]CheckResult),
    }

    // Check Slack connection
    if slackConnected {
        status.Checks["slack"] = CheckResult{
            Status: "healthy",
            Message: "Connected to Slack",
        }
    } else {
        status.Status = "unhealthy"
        status.Checks["slack"] = CheckResult{
            Status:  "unhealthy",
            Message: "Not connected to Slack",
        }
    }

    // Check LLM connectivity
    start := time.Now()
    if err := checkLLMHealth(); err != nil {
        status.Status = "unhealthy"
        status.Checks["llm"] = CheckResult{
            Status:  "unhealthy",
            Message: err.Error(),
            Latency: time.Since(start),
        }
    } else {
        status.Checks["llm"] = CheckResult{
            Status:  "healthy",
            Message: "LLM responding",
            Latency: time.Since(start),
        }
    }

    // Check Redis cache
    start = time.Now()
    if err := checkRedisHealth(); err != nil {
        status.Checks["cache"] = CheckResult{
            Status:  "degraded",
            Message: err.Error(),
            Latency: time.Since(start),
        }
    } else {
        status.Checks["cache"] = CheckResult{
            Status:  "healthy",
            Message: "Cache responding",
            Latency: time.Since(start),
        }
    }

    w.Header().Set("Content-Type", "application/json")
    statusCode := http.StatusOK
    if status.Status != "healthy" {
        statusCode = http.StatusServiceUnavailable
    }
    w.WriteHeader(statusCode)
    json.NewEncoder(w).Encode(status)
}

func ReadyHandler(w http.ResponseWriter, r *http.Request) {
    // Readiness check - are we ready to serve traffic?
    if !slackConnected || !llmReady {
        http.Error(w, "Service not ready", http.StatusServiceUnavailable)
        return
    }
    w.WriteHeader(http.StatusOK)
    fmt.Fprintf(w, "ready")
}
```

## 🎯 Best Practices

### Monitoring Best Practices
1. **Set meaningful alerts** based on SLOs, not just thresholds
2. **Use structured logging** for better searchability and analysis
3. **Monitor the right metrics** that directly impact user experience
4. **Implement progressive alerting** to reduce alert fatigue
5. **Regularly review and tune** dashboards and alerts

### Logging Best Practices
1. **Use consistent log formats** across all components
2. **Include correlation IDs** for tracing requests across services
3. **Log at appropriate levels** (DEBUG, INFO, WARN, ERROR)
4. **Avoid logging sensitive information** (tokens, passwords)
5. **Implement log rotation** and retention policies

### Observability Strategy
1. **Start with key metrics** and expand as needed
2. **Use Service Level Objectives** to measure reliability
3. **Implement distributed tracing** for complex workflows
4. **Create role-specific dashboards** for different teams
5. **Automate incident response** where possible

This monitoring and observability setup provides comprehensive visibility into Marvin and Slacker operations, enabling
proactive issue detection and efficient troubleshooting.
