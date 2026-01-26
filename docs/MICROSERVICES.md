# Microservices Architecture

## Service Catalog

### timeguru
- Timeline tracking and milestone management
- Maintains timeline.md with JSON/YAML mirrors
- REST API: GET/POST /api/v1/timeline

### captain
- Strategic decisions and vision alignment
- REST API: GET/POST /api/v1/strategy

### micromanager
- Task breakdown and QA oversight
- REST API: GET/POST /api/v1/tasks

### architect
- Infrastructure design and tech decisions
- REST API: GET/POST /api/v1/designs

## Communication Pattern

All services communicate via Busboy (message bus).

TODO: Add detailed service specifications
