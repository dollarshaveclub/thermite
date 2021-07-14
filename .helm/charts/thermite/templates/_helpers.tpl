{{/*
Expand the name of the chart.
*/}}
{{- define "thermite.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "thermite.fullname" -}}
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
{{- define "thermite.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "thermite.labels" -}}
helm.sh/chart: {{ include "thermite.chart" . }}
{{ include "thermite.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "thermite.selectorLabels" -}}
app.kubernetes.io/name: {{ include "thermite.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "thermite.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "thermite.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{- define "thermite.jobSpec" -}}
spec:
  template:
    metadata:
      annotations:
      {{- with .Values.podAnnotations }}
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with .Values.vaultInjector }}
      {{- if .enabled }}
        vault.hashicorp.com/agent-inject: "true"
        vault.hashicorp.com/agent-pre-populate: "true"
        vault.hashicorp.com/agent-pre-populate-only: "true"
        vault.hashicorp.com/agent-inject-secret-{{ .name }}: {{ .path | quote }}
        vault.hashicorp.com/agent-inject-template-{{ .name }}: |
          {{ "{{" }}- with secret "{{ .path }}" {{ "}}" }}
          [default]
          aws_access_key_id={{ "{{" }} .Data.{{ .keys.awsAccessKeyID }} {{ "}}" }}
          aws_secret_access_key={{ "{{" }} .Data.{{ .keys.awsSecretAccessKey }} {{ "}}" }}
          {{ "{{" }}- end {{ "}}" }}
        vault.hashicorp.com/role: {{ .role | quote }}
        vault.hashicorp.com/auth-path: {{ .authPath | quote }}
      {{- end }}
      {{- end }}
      labels:
        {{- include "thermite.selectorLabels" . | nindent 8 }}
      {{- if or .Values.datadog.apm.enabled .Values.datadog.statsd.enabled }}
        tags.datadoghq.com/env: {{ .Values.datadog.env | quote }}
        tags.datadoghq.com/service: {{ .Values.datadog.service | quote }}
        tags.datadoghq.com/version: {{ .Values.datadog.version | default .Chart.AppVersion | quote }}
      {{- end }}
    spec:
      {{- with .Values.imagePullSecrets }}
      imagePullSecrets:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      serviceAccountName: {{ include "thermite.serviceAccountName" . | quote }}
      {{- with .Values.podSecurityContext }}
      securityContext:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      restartPolicy: Never
      containers:
        - name: {{ .Chart.Name | quote }}
          image: {{ printf "%s:%s" .Values.image.repository (.Values.image.tag | default .Chart.AppVersion) | quote }}
          imagePullPolicy: {{ .Values.image.pullPolicy }}
          args:
          {{- with .Values.removeImages }}
            - "--remove-images"
            - {{ . | quote }}
          {{- end }}
          {{- with .Values.periodTagKey }}
            - "--period-tag-key"
            - {{ . | quote }}
          {{- end }}
          {{- with .Values.datadog.statsd }}
          {{- if .enabled }}
            - "--statsd-namespace"
            - {{ .namespace | quote }}
          {{- range $t := .tags }}
            - "--statsd-tag"
            - {{ . | quote }}
          {{- end }}
          {{- end }}
          {{- end }}
          env:
          {{- with .Values.awsRegion }}
          - name: AWS_DEFAULT_REGION
            value: {{ . | quote }}
          {{- end }}
          {{- with .Values.secret }}
          {{- if .enabled }}
          - name: AWS_SHARED_CREDENTIALS_FILE
            value: {{ printf "%s/%s" .mountPath .name | quote }}
          {{- end }}
          {{- end }}
          {{- with .Values.vaultInjector }}
          {{- if .enabled }}
          - name: AWS_SHARED_CREDENTIALS_FILE
            value: {{ printf "/vault/secrets/%s" .name | quote }}
          {{- end }}
          {{- end }}
          - name: DD_AGENT_HOST
            value: {{ .Values.datadog.host | quote }}
          - name: DD_ENV
            value: {{ .Values.datadog.env  | quote }}
          - name: DD_SERVICE
            value: {{ .Values.datadog.service | quote }}
          - name: DD_VERSION
            value: {{ .Values.datadog.version | default .Chart.AppVersion | quote }}
          {{- with .Values.datadog.apm }}
          {{- if .enabled }}
          - name: DD_TRACE_AGENT_PORT
            value: {{ .port | quote }}
          {{- end }}
          {{- end }}
          {{- with .Values.datadog.statsd }}
          {{- if .enabled }}
          - name: DD_DOGSTATSD_PORT
            value: {{ .port | quote }}
          {{- end }}
          {{- end }}
          {{- with .Values.secret }}
          {{- if .enabled }}
          envFrom:
          - secretRef:
              name: {{ .name | quote }}
          {{- end }}
          {{- end }}
          {{- with .Values.resources }}
          resources:
            {{- toYaml . | nindent 8 }}
          {{- end }}
          {{- with .Values.securityContext }}
          securityContext:
            {{- toYaml .| nindent 16 }}
          {{- end }}
      {{- with .Values.nodeSelector }}
      nodeSelector:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with .Values.affinity }}
      affinity:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with .Values.tolerations }}
      tolerations:
        {{- toYaml . | nindent 8 }}
      {{- end }}
{{- end }}