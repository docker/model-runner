{{/*
Expand the name of the chart.
*/}}
{{- define "docker-model-runner.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "docker-model-runner.fullname" -}}
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
Create chart name and version as used by the chart label.
*/}}
{{- define "docker-model-runner.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "docker-model-runner.labels" -}}
helm.sh/chart: {{ include "docker-model-runner.chart" . }}
{{ include "docker-model-runner.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "docker-model-runner.selectorLabels" -}}
app.kubernetes.io/name: {{ include "docker-model-runner.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
GPU resource limits
*/}}
{{- define "docker-model-runner.gpuResources" -}}
{{- if .Values.gpu.enabled }}
{{- if eq .Values.gpu.vendor "nvidia" }}
nvidia.com/gpu: {{ .Values.gpu.count }}
{{- else if eq .Values.gpu.vendor "amd" }}
amd.com/gpu: {{ .Values.gpu.count }}
{{- end }}
{{- end }}
{{- end }}