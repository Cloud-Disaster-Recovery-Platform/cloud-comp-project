#!/usr/bin/env bash
#
# deploy-infra.sh
# Initializes and applies Terraform infrastructure for Cloud Mirror.
#
# Usage:
#   ./scripts/deploy-infra.sh plan     # preview changes
#   ./scripts/deploy-infra.sh apply    # create/update infrastructure
#   ./scripts/deploy-infra.sh destroy  # tear down infrastructure
#
set -euo pipefail

TERRAFORM_DIR="${TERRAFORM_DIR:-../terraform}"
ACTION="${1:-plan}"

if [ ! -d "$TERRAFORM_DIR" ]; then
    echo "ERROR: Terraform directory not found at ${TERRAFORM_DIR}"
    echo "       Set TERRAFORM_DIR to point to the terraform root module."
    exit 1
fi

cd "$TERRAFORM_DIR"

echo "==> Terraform init"
terraform init -upgrade

case "$ACTION" in
    plan)
        echo "==> Terraform plan"
        terraform plan -out=tfplan
        echo ""
        echo "Review the plan above, then run: ./scripts/deploy-infra.sh apply"
        ;;
    apply)
        echo "==> Terraform apply"
        if [ -f tfplan ]; then
            terraform apply tfplan
            rm -f tfplan
        else
            terraform apply
        fi
        echo ""
        echo "==> Infrastructure deployed. Outputs:"
        terraform output
        ;;
    destroy)
        echo "==> Terraform destroy"
        terraform destroy
        ;;
    *)
        echo "Usage: $0 {plan|apply|destroy}"
        exit 1
        ;;
esac

echo "==> Done."
