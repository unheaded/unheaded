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
{{/*
Container securityContext.

Takes a dict of "ctx" (the root context) and "uid" (the service's own UID from
pkg/uids). Falls back to global.security.runAsUser only if a service has no uid,
which should not happen — the fallback exists so a new service fails visibly on
a shared identity rather than silently inheriting root.

Every service gets its OWN uid. A single shared runAsUser means a compromise of
any one service can read and rewrite every other's on-disk state, and makes file
ownership useless for attributing writes during an audit.
*/}}
{{- define "unheaded.securityContext" -}}
{{- $ctx := .ctx -}}
{{- $uid := .uid | default $ctx.Values.global.security.runAsUser -}}
securityContext:
  readOnlyRootFilesystem: {{ $ctx.Values.global.security.readOnlyRootFilesystem }}
  runAsNonRoot: {{ $ctx.Values.global.security.runAsNonRoot }}
  runAsUser: {{ $uid }}
  runAsGroup: {{ $uid }}
  allowPrivilegeEscalation: {{ $ctx.Values.global.security.allowPrivilegeEscalation }}
  capabilities:
    drop:
      - ALL
  seccompProfile:
    type: {{ $ctx.Values.global.security.seccompProfile }}
{{- end }}

{{/*
Pod-level securityContext.

Distinct from the container one: fsGroup governs ownership of mounted volumes,
and runAsNonRoot here also covers initContainers and any sidecar injected later,
which a container-level context does not. trivy KSV-0118 flags its absence.
*/}}
{{- define "unheaded.podSecurityContext" -}}
{{- $ctx := .ctx -}}
{{- $uid := .uid | default $ctx.Values.global.security.runAsUser -}}
securityContext:
  runAsNonRoot: {{ $ctx.Values.global.security.runAsNonRoot }}
  runAsUser: {{ $uid }}
  runAsGroup: {{ $uid }}
  fsGroup: {{ $uid }}
  seccompProfile:
    type: {{ $ctx.Values.global.security.seccompProfile }}
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
