#!/bin/bash
set -e

echo "=== Installing Playwright with Go bindings ==="

# Check if Go is installed
if ! command -v go &> /dev/null; then
    echo "Error: Go is not installed. Please install Go before continuing."
    exit 1
fi

# Install playwright-go package
echo "Installing playwright-go package..."
go get github.com/playwright-community/playwright-go

# Install Playwright browsers
echo "Installing Playwright browsers..."
go run github.com/playwright-community/playwright-go/cmd/playwright install

# Create test directories if they don't exist
echo "Creating test directories..."
mkdir -p test-results/videos
mkdir -p test-results/traces
mkdir -p test-results/screenshots
mkdir -p test-assets

# Check for success
if [ $? -eq 0 ]; then
    echo ""
    echo "=== Playwright installation completed successfully! ==="
    echo "You can now run tests with: go test ./tests/... -v"
    echo ""
    echo "For camera testing:"
    echo "1. Create a test-assets directory with sample Y4M video files"
    echo "2. Example: ffmpeg -i input.mp4 -pix_fmt yuv420p test-assets/sample-video.y4m"
    echo ""
else
    echo "Installation failed. Please check the error messages above."
    exit 1
fi