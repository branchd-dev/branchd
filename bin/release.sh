#!/bin/bash
set -euo pipefail

# Check if version argument provided
if [ $# -eq 0 ]; then
    echo "Usage: bin/release.sh <version>"
    echo "Example: bin/release.sh 0.1.1"
    exit 1
fi

VERSION="$1"

# Add 'v' prefix if not present
if [[ ! $VERSION =~ ^v ]]; then
    VERSION="v$VERSION"
fi

# Validate version format (v1.2.3)
if [[ ! $VERSION =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "ERROR: Invalid version format: $VERSION"
    echo "Version must be in format: 1.2.3 or v1.2.3"
    exit 1
fi

# Get latest tag
LATEST_TAG=$(git tag -l "v*" --sort=-version:refname | head -1 || echo "")

if [ -n "$LATEST_TAG" ]; then
    # Compare versions (remove 'v' prefix for comparison)
    LATEST_VERSION="${LATEST_TAG#v}"
    NEW_VERSION="${VERSION#v}"

    if [ "$(printf '%s\n' "$NEW_VERSION" "$LATEST_VERSION" | sort -V | head -1)" = "$NEW_VERSION" ] && [ "$NEW_VERSION" != "$LATEST_VERSION" ]; then
        echo "ERROR: Version $VERSION is not greater than latest version $LATEST_TAG"
        exit 1
    fi

    echo "Latest version: $LATEST_TAG"
fi

echo "Creating release: $VERSION"

# Create and push tag
git tag -a "$VERSION" -m "Release $VERSION"
git push origin "$VERSION"

echo "✓ Release $VERSION created"
echo "GitHub Actions will build and publish: https://github.com/branchd-dev/branchd/actions"
