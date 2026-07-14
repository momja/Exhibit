---
id: av-i6y1
status: closed
deps: []
links: []
created: 2026-07-14T04:58:51Z
type: task
priority: 2
assignee: Max Omdal
---
# Add details for deploying via docker to README.md


## Notes

**2026-07-14T05:23:12Z**

Closing as already covered. The README "Building" section documents deploying via Docker: a docker build plus a docker run with both ports mapped (8080/8081), AUTH_TOKEN, APP_ORIGIN and RENDER_ORIGIN set, and a persistent -v /data volume. The Compose-based deploy (app service plus optional Litestream/MinIO replication profiles) is documented in docs/technical_stack.md section 12 and docker-compose.yml. Reopen if the Compose path should also be surfaced directly in the README.
