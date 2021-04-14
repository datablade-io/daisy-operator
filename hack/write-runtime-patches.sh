#!/usr/bin/env bash

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
TEST_WEBHOOK_CERTS_DIR=${TEST_WEBHOOK_CERTS_DIR:-/tmp/k8s-webhook-server/serving-certs}
#DOCKER_GATEWAY=$(${SCRIPT_DIR}/docker-gateway.sh)
DOCKER_GATEWAY="host.docker.internal"
CA_CERT_CONTENTS=$(base64 "${TEST_WEBHOOK_CERTS_DIR}"/ca.crt)

cat <<EOF > ${SCRIPT_DIR}/../config/debug/patches/mutating_webhook_configuration.yaml
apiVersion: admissionregistration.k8s.io/v1
kind: MutatingWebhookConfiguration
metadata:
  creationTimestamp: null
  name: mutating-webhook-configuration
webhooks:
- clientConfig:
    caBundle: ${CA_CERT_CONTENTS}
    service: null
    url: https://${DOCKER_GATEWAY}:9443/mutate-daisy-com-v1-daisyinstallation
  name: mdaisyinstallation.kb.io
EOF

cat <<EOF > ${SCRIPT_DIR}/../config/debug/patches/validating_webhook_configuration.yaml
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
metadata:
  name: validating-webhook-configuration
webhooks:
- clientConfig:
    caBundle: ${CA_CERT_CONTENTS}
    service: null
    url: https://${DOCKER_GATEWAY}:9443/validate-daisy-com-v1-daisyinstallation
  name: vdaisyinstallation.kb.io
EOF