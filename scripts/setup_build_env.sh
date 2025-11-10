#!/bin/bash

set -e  # Exit on any error

echo "Starting build environment setup..."

# Install buf
echo "Installing buf..."
if ! GO111MODULE=on GOBIN=/usr/local/bin go install github.com/bufbuild/buf/cmd/buf@v1.30.1; then
    echo "Failed to install buf"
    exit 1
fi

# Install Task
echo "Installing Task..."
if ! sh -c "$(curl --location https://taskfile.dev/install.sh)" -- -d; then
    echo "Failed to install Task"
    exit 1
fi

# Install swag (Swagger documentation generator)
echo "Installing swag..."
if ! GO111MODULE=on GOBIN=/usr/local/bin go install github.com/swaggo/swag/cmd/swag@latest; then
    echo "Failed to install swag"
    exit 1
fi

# Install Node.js and npm
echo "Installing Node.js and npm..."
if ! apt update; then
    echo "Failed to update apt"
    exit 1
fi

# Install Node.js and npm from NodeSource
echo "Setting up NodeSource repository..."
if ! curl -fsSL https://deb.nodesource.com/setup_23.x | bash -; then
    echo "Failed to setup NodeSource repository"
    exit 1
fi

if ! apt-get install -y nodejs; then
    echo "Failed to install Node.js and npm"
    exit 1
fi

# Log Node.js version
echo "Node.js version:"
node --version

# Tailwind CSS will be installed via npm in the main Dockerfile
echo "Tailwind CSS will be available via npm package..."

echo "Build environment setup completed successfully!"
