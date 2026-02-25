{{/*
Expand the name of the chart.
*/}}
{{- define "unheaded.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "unheaded.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Common labels.
*/}}
{{- define "unheaded.labels" -}}
helm.sh/chart: {{ include "unheaded.chart" . }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: unheaded
{{- end }}

{{/*
Chart label.
*/}}
{{- define "unheaded.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Service-specific labels.
*/}}
{{- define "unheaded.serviceLabels" -}}
{{ include "unheaded.labels" .ctx }}
app.kubernetes.io/name: {{ .service }}
app.kubernetes.io/instance: {{ .ctx.Release.Name }}
app.kubernetes.io/component: {{ .component | default "service" }}
{{- end }}

{{/*
Service selector labels.
*/}}
{{- define "unheaded.selectorLabels" -}}
app.kubernetes.io/name: {{ .service }}
app.kubernetes.io/instance: {{ .ctx.Release.Name }}
{{- end }}

{{/*
Image reference from global + service values.
*/}}
{{- define "unheaded.image" -}}
{{- $registry := .ctx.Values.global.image.registry -}}
{{- $repository := .image.repository -}}
{{- $tag := .image.tag | default .ctx.Values.global.image.tag -}}
{{- printf "%s/%s:%s" $registry $repository $tag }}
{{- end }}

{{/*
Security context (shared across all services).
*/}}
{{- define "unheaded.securityContext" -}}
securityContext:
  readOnlyRootFilesystem: {{ .Values.global.security.readOnlyRootFilesystem }}
  runAsNonRoot: {{ .Values.global.security.runAsNonRoot }}
  runAsUser: {{ .Values.global.security.runAsUser }}
  runAsGroup: {{ .Values.global.security.runAsGroup }}
  allowPrivilegeEscalation: {{ .Values.global.security.allowPrivilegeEscalation }}
  capabilities:
    drop:
      - ALL
  seccompProfile:
    type: {{ .Values.global.security.seccompProfile }}
{{- end }}

{{/*
Standard probes.
*/}}
{{- define "unheaded.probes" -}}
livenessProbe:
  httpGet:
    path: /health
    port: http
  initialDelaySeconds: 5
  periodSeconds: 10
  timeoutSeconds: 3
  failureThreshold: 3
readinessProbe:
  httpGet:
    path: /ready
    port: http
  initialDelaySeconds: 3
  periodSeconds: 5
  timeoutSeconds: 2
  failureThreshold: 3
{{- end }}
