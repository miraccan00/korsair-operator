{{/*
Expand the chart name.
*/}}
{{- define "korsair.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a fully-qualified app name.
*/}}
{{- define "korsair.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- include "korsair.name" . }}
{{- end }}
{{- end }}

{{/*
Service account name for the operator.
*/}}
{{- define "korsair.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "korsair.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Service account name for the API server.
*/}}
{{- define "korsair.apiServiceAccountName" -}}
{{- printf "%s-api" (include "korsair.fullname" .) }}
{{- end }}

{{/*
Common labels applied to all resources.
*/}}
{{- define "korsair.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | quote }}
app.kubernetes.io/name: {{ include "korsair.name" . }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}

{{/*
Selector labels for pod selection (must remain stable across upgrades).
*/}}
{{- define "korsair.selectorLabels" -}}
app.kubernetes.io/name: {{ include "korsair.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}
