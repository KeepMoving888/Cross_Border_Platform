{{/*
Expand the name of the chart.
*/}}
{{- define "cb-platform.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "cb-platform.fullname" -}}
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
{{- define "cb-platform.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "cb-platform.labels" -}}
helm.sh/chart: {{ include "cb-platform.chart" . }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/part-of: cb-platform
{{- end }}

{{/*
Selector labels for a given component (gateway / ai / rag)
Usage: {{ include "cb-platform.selectorLabels" (dict "ctx" . "component" "gateway") }}
*/}}
{{- define "cb-platform.selectorLabels" -}}
app.kubernetes.io/name: {{ include "cb-platform.name" .ctx }}
app.kubernetes.io/instance: {{ .ctx.Release.Name }}
app.kubernetes.io/component: {{ .component }}
{{- end }}

{{/*
Component labels (adds component label on top of common labels)
Usage: {{ include "cb-platform.componentLabels" (dict "ctx" . "component" "gateway") }}
*/}}
{{- define "cb-platform.componentLabels" -}}
{{ include "cb-platform.labels" .ctx }}
app.kubernetes.io/name: {{ include "cb-platform.name" .ctx }}
app.kubernetes.io/instance: {{ .ctx.Release.Name }}
app.kubernetes.io/component: {{ .component }}
{{- end }}

{{/*
Image reference: repository:tag
*/}}
{{- define "cb-platform.image" -}}
{{- $repo := default "ghcr.io/keepmoving888/cross_border_platform" .Values.image.repository -}}
{{- $tag := default .Chart.AppVersion .Values.image.tag -}}
{{- printf "%s:%s" $repo $tag -}}
{{- end }}

{{/*
Service name for a component
*/}}
{{- define "cb-platform.serviceName" -}}
{{- printf "%s-%s" (include "cb-platform.fullname" .ctx) .component -}}
{{- end }}
