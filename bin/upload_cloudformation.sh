#!/bin/bash
set -euo pipefail

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
BUCKET_NAME="branchd-cloudformation-templates"
REGION="us-east-1"
TEMPLATE_FILE="cloudformation/branchd.yaml"
S3_KEY="branchd.yaml"

echo -e "${BLUE}=== Branchd CloudFormation Template Upload ===${NC}"
echo ""

# Check if template file exists
if [ ! -f "$TEMPLATE_FILE" ]; then
    echo "ERROR: Template file not found: $TEMPLATE_FILE"
    exit 1
fi

# Validate template syntax
echo "Validating CloudFormation template..."
if aws cloudformation validate-template \
    --template-body "file://$TEMPLATE_FILE" \
    --region "$REGION" > /dev/null 2>&1; then
    echo -e "${GREEN}✓${NC} Template validation passed"
else
    echo "ERROR: Template validation failed!"
    echo "Run this command to see validation errors:"
    echo "  aws cloudformation validate-template --template-body file://$TEMPLATE_FILE --region $REGION"
    exit 1
fi

# Upload to S3
echo ""
echo "Uploading template to S3..."
aws s3 cp "$TEMPLATE_FILE" "s3://$BUCKET_NAME/$S3_KEY" \
    --region "$REGION" \
    --content-type "text/yaml"

echo -e "${GREEN}✓${NC} Upload complete"

# Verify template is accessible
echo ""
echo "Verifying public accessibility..."
S3_URL="https://$BUCKET_NAME.s3.amazonaws.com/$S3_KEY"
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" "$S3_URL")

if [ "$HTTP_CODE" = "200" ]; then
    echo -e "${GREEN}✓${NC} Template is publicly accessible (HTTP $HTTP_CODE)"
else
    echo -e "${YELLOW}⚠${NC} Warning: Template returned HTTP $HTTP_CODE"
fi

# Output URLs
echo ""
echo -e "${BLUE}=== URLs ===${NC}"
echo ""
echo "Template URL:"
echo "  $S3_URL"
echo ""
echo "Launch Stack URL:"
echo "  https://console.aws.amazon.com/cloudformation/home#/stacks/create/review?templateURL=$S3_URL&stackName=branchd"
echo ""
echo -e "${GREEN}✓${NC} CloudFormation template updated successfully!"
