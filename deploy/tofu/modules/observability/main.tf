# Unheaded Observability Module
# Manages: Prometheus stack, Grafana dashboards, OTel collector, Alertmanager config

variable "environment" { type = string }
variable "retention_days" { type = number; default = 7 }
variable "grafana_admin_password" { type = string; sensitive = true }

resource "helm_release" "prometheus" {
  name       = "prometheus"
  repository = "https://prometheus-community.github.io/helm-charts"
  chart      = "kube-prometheus-stack"
  namespace  = "unheaded-system"
  version    = "56.0.0"

  set { name = "prometheus.prometheusSpec.retention"; value = "${var.retention_days}d" }
  set { name = "grafana.adminPassword"; value = var.grafana_admin_password }
}

resource "helm_release" "otel" {
  name       = "otel-collector"
  repository = "https://open-telemetry.github.io/opentelemetry-helm-charts"
  chart      = "opentelemetry-collector"
  namespace  = "unheaded-system"

  set { name = "mode"; value = "deployment" }
}
