# Docker Deployment Patterns

This guide covers various deployment patterns for Marvin and Slacker using Docker containers, from simple
single-container setups to complex multi-container orchestrations.

## 🎯 Overview

Docker provides a consistent runtime environment for Marvin CLI and Slacker bot, enabling:
- Reproducible deployments across environments
- Isolated execution contexts for security
- Easy scaling and orchestration
- Efficient resource utilization

## 🐳 Basic Docker Setup

### Building the Image
```bash
# Clone and build
git clone https://github.com/meschbach/marvin.git
cd marvin

# Build both CLI and Slacker binaries
go build -o marvin ./cmd/marvin
go build -o slacker ./cmd/slacker

# Build Docker image
docker build -t marvin:latest .
```

### Simple CLI Usage
```bash
# Basic Marvin CLI container
docker run --rm \
  -v $(pwd)/config:/config \
  -v $(pwd)/data:/data \
  marvin:latest \
  marvin query "What is Docker?"

# Interactive mode
docker run -it --rm \
  -v $(pwd)/config:/config \
  -v $(pwd)/data:/data \
  marvin:latest \
  marvin --interactive
```

## 🏗️ Deployment Patterns

### Pattern 1: Single Container Slacker

#### docker-compose.yml
```yaml
version: '3.8'

services:
  slacker:
    image: slacker:latest
    container_name: marvin-slacker
    restart: unless-stopped

    environment:
      - SLACK_BOT_TOKEN=${SLACK_BOT_TOKEN}
      - SLACK_APP_TOKEN=${SLACK_APP_TOKEN}
      - MARVIN_CONFIG=/config/marvin.hcl
      - MARVIN_SESSION_PATH=/data/sessions
      - LOG_LEVEL=info

    volumes:
      - ./config:/config:ro
      - ./data:/data
      - ./logs:/app/logs

    ports:
      - "8080:8080"  # Health checks

    healthcheck:
      test: ["CMD", "wget", "--quiet", "--tries=1", "--spider", "http://localhost:8080/health"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 40s

    networks:
      - marvin-network

    security_opt:
      - no-new-privileges:true

    read_only: true
    tmpfs:
      - /tmp
      - /app/sessions

networks:
  marvin-network:
    driver: bridge
```

#### .env file
```bash
# .env
SLACK_BOT_TOKEN=xoxb-your-bot-token
SLACK_APP_TOKEN=xapp-your-app-token
```

### Pattern 2: Multi-Container with Ollama

#### docker-compose.yml
```yaml
version: '3.8'

services:
  ollama:
    image: ollama/ollama:latest
    container_name: marvin-ollama
    restart: unless-stopped
    ports:
      - "11434:11434"
    volumes:
      - ollama_data:/root/.ollama
    environment:
      - OLLAMA_HOST=0.0.0.0
    networks:
      - marvin-network
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:11434/api/tags"]
      interval: 30s
      timeout: 10s
      retries: 3

  slacker:
    image: slacker:latest
    container_name: marvin-slacker
    restart: unless-stopped
    depends_on:
      ollama:
        condition: service_healthy

    environment:
      - SLACK_BOT_TOKEN=${SLACK_BOT_TOKEN}
      - SLACK_APP_TOKEN=${SLACK_APP_TOKEN}
      - MARVIN_CONFIG=/config/marvin.hcl
      - MARVIN_SESSION_PATH=/data/sessions
      - LOG_LEVEL=info

    volumes:
      - ./config:/config:ro
      - ./data:/data
      - ./logs:/app/logs

    networks:
      - marvin-network

    healthcheck:
      test: ["CMD", "wget", "--quiet", "--tries=1", "--spider", "http://localhost:8080/health"]
      interval: 30s
      timeout: 10s
      retries: 3

    security_opt:
      - no-new-privileges:true

volumes:
  ollama_data:
    driver: local

networks:
  marvin-network:
    driver: bridge
```

### Pattern 3: Production with Reverse Proxy

#### docker-compose.yml
```yaml
version: '3.8'

services:
  nginx:
    image: nginx:alpine
    container_name: marvin-nginx
    restart: unless-stopped
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./nginx/nginx.conf:/etc/nginx/nginx.conf:ro
      - ./nginx/ssl:/etc/nginx/ssl:ro
    networks:
      - marvin-network
    depends_on:
      - slacker

  slacker:
    image: slacker:latest
    container_name: marvin-slacker
    restart: unless-stopped

    environment:
      - SLACK_BOT_TOKEN=${SLACK_BOT_TOKEN}
      - SLACK_APP_TOKEN=${SLACK_APP_TOKEN}
      - MARVIN_CONFIG=/config/marvin.hcl
      - MARVIN_SESSION_PATH=/data/sessions
      - LOG_LEVEL=warn

    volumes:
      - ./config:/config:ro
      - slacker_data:/data
      - ./logs:/app/logs

    networks:
      - marvin-network

    deploy:
      resources:
        limits:
          cpus: '1.0'
          memory: 1G
        reservations:
          cpus: '0.5'
          memory: 512M

  ollama:
    image: ollama/ollama:latest
    container_name: marvin-ollama
    restart: unless-stopped
    volumes:
      - ollama_data:/root/.ollama
    networks:
      - marvin-network
    deploy:
      resources:
        limits:
          cpus: '2.0'
          memory: 4G
        reservations:
          cpus: '1.0'
          memory: 2G

volumes:
  slacker_data:
    driver: local
  ollama_data:
    driver: local

networks:
  marvin-network:
    driver: bridge
```

#### nginx/nginx.conf
```nginx
events {
    worker_connections 1024;
}

http {
    upstream slacker {
        server slacker:8080;
    }

    server {
        listen 80;
        server_name your-domain.com;
        return 301 https://$server_name$request_uri;
    }

    server {
        listen 443 ssl http2;
        server_name your-domain.com;

        ssl_certificate /etc/nginx/ssl/cert.pem;
        ssl_certificate_key /etc/nginx/ssl/key.pem;
        ssl_protocols TLSv1.2 TLSv1.3;
        ssl_ciphers ECDHE-RSA-AES256-GCM-SHA512:DHE-RSA-AES256-GCM-SHA512;

        location /health {
            proxy_pass http://slacker/health;
            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
        }

        location /ready {
            proxy_pass http://slacker/ready;
            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
        }
    }
}
```

## 🔧 Configuration Management

### Environment-Specific Compose Files

#### Development (docker-compose.dev.yml)
```yaml
version: '3.8'

services:
  slacker:
    environment:
      - LOG_LEVEL=debug
      - MARVIN_CONFIG=/config/marvin.dev.hcl
    volumes:
      - ./config/dev:/config:ro
      - ./data/dev:/data
      - ./logs:/app/logs
    ports:
      - "8080:8080"  # Expose for debugging
```

#### Production (docker-compose.prod.yml)
```yaml
version: '3.8'

services:
  slacker:
    environment:
      - LOG_LEVEL=warn
      - MARVIN_CONFIG=/config/marvin.prod.hcl
    volumes:
      - ./config/prod:/config:ro
      - slacker_data:/data
    ports: []  # No port exposure
    deploy:
      replicas: 2
      update_config:
        parallelism: 1
        delay: 10s
        failure_action: rollback
      restart_policy:
        condition: on-failure
        delay: 5s
        max_attempts: 3
```

### Usage
```bash
# Development
docker-compose -f docker-compose.yml -f docker-compose.dev.yml up -d

# Production
docker-compose -f docker-compose.yml -f docker-compose.prod.yml up -d
```

## 📊 Volume Management

### Data Persistence
```yaml
volumes:
  # Local volume for development
  slacker_data_dev:
    driver: local
    driver_opts:
      type: none
      o: bind
      device: ./data/dev

  # Named volume for production
  slacker_data_prod:
    driver: local
    driver_opts:
      type: none
      o: bind
      device: /opt/marvin/data

  # Backup volume
  slacker_backup:
    driver: local
    driver_opts:
      type: none
      o: bind
      device: /backup/marvin
```

### Backup Strategy
```bash
#!/bin/bash
# backup.sh

DATE=$(date +%Y%m%d_%H%M%S)
BACKUP_DIR="/backup/marvin"
CONTAINER_NAME="marvin-slacker"

# Create backup
docker exec $CONTAINER_NAME tar czf /tmp/marvin_backup_$DATE.tar.gz /data

# Copy to host
docker cp $CONTAINER_NAME:/tmp/marvin_backup_$DATE.tar.gz $BACKUP_DIR/

# Cleanup old backups (keep 7 days)
find $BACKUP_DIR -name "marvin_backup_*.tar.gz" -mtime +7 -delete

echo "Backup completed: $BACKUP_DIR/marvin_backup_$DATE.tar.gz"
```

## 🔄 Lifecycle Management

### Health Monitoring
```yaml
healthcheck:
  test: ["CMD", "wget", "--quiet", "--tries=1", "--spider", "http://localhost:8080/health"]
  interval: 30s
  timeout: 10s
  retries: 3
  start_period: 40s
```

### Log Management
```yaml
services:
  slacker:
    logging:
      driver: "json-file"
      options:
        max-size: "10m"
        max-file: "3"
        labels: "service=slacker,environment=production"
```

### Auto-restart Policies
```yaml
restart_policy:
  condition: on-failure
  delay: 5s
  max_attempts: 3
  window: 120s
```

## 🔒 Security Hardening

### Container Security
```yaml
services:
  slacker:
    security_opt:
      - no-new-privileges:true
      - apparmor:docker-default
      - seccomp:default
    read_only: true
    tmpfs:
      - /tmp:noexec,nosuid,size=100m
      - /app/sessions:noexec,nosuid,size=500m
    user: "1000:1000"
    cap_drop:
      - ALL
    cap_add:
      - CHOWN
      - SETGID
      - SETUID
```

### Network Isolation
```yaml
networks:
  marvin-internal:
    driver: bridge
    internal: true  # No internet access

  marvin-external:
    driver: bridge
    internal: false  # Internet access for Slack API

services:
  slacker:
    networks:
      - marvin-internal
      - marvin-external

  ollama:
    networks:
      - marvin-internal
```

## 📈 Performance Optimization

### Resource Constraints
```yaml
services:
  slacker:
    deploy:
      resources:
        limits:
          cpus: '1.0'
          memory: 1G
          pids: 100
        reservations:
          cpus: '0.5'
          memory: 512M
```

### Caching Layer
```yaml
services:
  redis:
    image: redis:alpine
    container_name: marvin-redis
    restart: unless-stopped
    volumes:
      - redis_data:/data
    networks:
      - marvin-network

  slacker:
    environment:
      - REDIS_URL=redis://redis:6379
      - CACHE_TTL=300
    depends_on:
      - redis

volumes:
  redis_data:
    driver: local
```

## 🚀 Production Deployment

### Docker Swarm
```yaml
version: '3.8'

services:
  slacker:
    image: slacker:latest
    deploy:
      mode: replicated
      replicas: 3
      update_config:
        parallelism: 1
        delay: 10s
        failure_action: rollback
        order: start-first
      restart_policy:
        condition: any
        delay: 5s
        max_attempts: 3
        window: 120s
      placement:
        constraints:
          - node.role == worker
          - node.labels.marvin == true
```

### Monitoring Integration
```yaml
services:
  slacker:
    environment:
      - PROMETHEUS_ENABLED=true
      - PROMETHEUS_PORT=9090
    labels:
      - "prometheus.job=marvin"
      - "prometheus.port=9090"
      - "prometheus.path=/metrics"
```

## 🆘 Troubleshooting

### Common Issues

#### Container Won't Start
```bash
# Check logs
docker logs marvin-slacker

# Inspect container
docker inspect marvin-slacker

# Check environment
docker exec marvin-slacker env | grep SLACK
```

#### Volume Mount Issues
```bash
# Check volume mounts
docker exec marvin-slacker df -h

# Verify file permissions
docker exec marvin-slacker ls -la /config

# Test file access
docker exec marvin-slacker cat /config/marvin.hcl
```

#### Network Connectivity
```bash
# Test DNS resolution
docker exec marvin-slacker nslookup api.slack.com

# Test connectivity
docker exec marvin-slacker curl -I https://api.slack.com

# Check network configuration
docker network ls
docker network inspect marvin_marvin-network
```

### Debug Commands
```bash
# Enter container shell
docker exec -it marvin-slacker sh

# Run health check manually
docker exec marvin-slacker wget --quiet --tries=1 --spider http://localhost:8080/health

# Monitor resource usage
docker stats marvin-slacker
```

## 📋 Best Practices

1. **Use specific image tags** instead of `latest`
2. **Implement health checks** for all services
3. **Use read-only filesystems** with tmpfs for writable data
4. **Configure proper resource limits** to prevent resource exhaustion
5. **Implement backup strategies** for persistent data
6. **Use secrets management** for sensitive data
7. **Implement log rotation** to prevent disk space issues
8. **Monitor container health** and set up alerts
9. **Regularly update base images** for security patches
10. **Test disaster recovery procedures** regularly

## 🆚 Docker vs Kubernetes

| Feature | Docker Compose | Kubernetes |
|---------|----------------|------------|
| Ease of Use | Simple | Complex |
| Scaling | Basic | Advanced |
| Self-healing | Limited | Built-in |
| Service Discovery | Basic | Advanced |
| Load Balancing | External | Built-in |
| Storage Management | Simple | Advanced |
| Multi-Host | Limited | Native |
| Ecosystem | Docker | Cloud Native |

Choose Docker Compose for simple deployments and development, Kubernetes for production-scale deployments with high
availability requirements.
