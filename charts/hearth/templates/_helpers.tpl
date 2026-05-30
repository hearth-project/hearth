{{- define "hearth.name" -}}
{{- default .Chart.Name .Values.nameOverride -}}
{{- end -}}

{{- define "hearth.fullname" -}}
{{- printf "%s-%s" .Release.Name (include "hearth.name" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "hearth.tag" -}}
{{- .Values.image.tag | default .Chart.AppVersion -}}
{{- end -}}

{{- define "hearth.operatorImage" -}}
{{- printf "%s/%s:%s" .Values.image.registry .Values.image.operator (include "hearth.tag" .) -}}
{{- end -}}

{{- define "hearth.gatewayImage" -}}
{{- printf "%s/%s:%s" .Values.image.registry .Values.image.gateway (include "hearth.tag" .) -}}
{{- end -}}

{{- define "hearth.labels" -}}
app.kubernetes.io/name: {{ include "hearth.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version }}
{{- end -}}

{{- define "hearth.selectorLabels" -}}
app.kubernetes.io/name: {{ include "hearth.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: controller-manager
{{- end -}}

{{- define "hearth.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (printf "%s-controller-manager" (include "hearth.fullname" .)) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}
