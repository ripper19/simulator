# Simulator Helm chart

Deploys the simulator API and worker cluster on Kubernetes. PostgreSQL, Redis,
and RabbitMQ are expected as external/managed services (configure via
`values.yaml`); only the stateless API and workers run in-cluster.

## Install

```sh
helm install simulator ./deployments/helm/simulator \
  --set config.databaseUrl='postgres://...' \
  --set config.redisAddr='...' \
  --set config.amqpUrl='amqp://...' \
  --set jwt.secret='...'
```

## Scaling workers

Workers are stateless and horizontally scalable: each worker consumes jobs from
the same RabbitMQ queue (QoS=1), so adding replicas increases throughput
linearly up to the broker/database limits.

```sh
# 1 -> 4 -> 16 -> 100 workers
helm upgrade simulator ./deployments/helm/simulator --set worker.replicaCount=4
helm upgrade simulator ./deployments/helm/simulator --set worker.replicaCount=16
helm upgrade simulator ./deployments/helm/simulator --set worker.replicaCount=100
```

Scaling model:

- **API** is nearly stateless (in-process simulation control + Postgres) and
  scales behind a load balancer; sticky sessions are not required.
- **Workers** scale out freely; each holds one job at a time (RabbitMQ prefetch
  = 1), so `replicaCount` directly bounds concurrent jobs.
- The **broker** (RabbitMQ) and **Postgres/Redis** are the shared bottlenecks;
  scale them (clustered RabbitMQ, connection pooling, read replicas) before
  scaling workers past their capacity.
