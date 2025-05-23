# Playwright End-to-End Testing with Go

This directory contains end-to-end tests using Playwright with Go bindings for the Infinit Feeding application.

## Prerequisites

- Go 1.19 or later
- Node.js 14+ and npm
- Chrome, Firefox, or WebKit browsers (Playwright will install these for you)

## Installation

### Option 1: Using Taskfile (Recommended)

The easiest way to install and run Playwright tests is using the provided Taskfile:

```bash
# Make the install script executable
chmod +x ./install-playwright.sh

# Run the installation task
task install
```

### Option 2: Manual Installation

```bash
# Install playwright-go package
go get github.com/playwright-community/playwright-go

# Install the browsers
go run github.com/playwright-community/playwright-go/cmd/playwright install
```

If needed, you can also install the browsers manually:

```bash
npx playwright install
```

## Project Structure

```
e2e-tests/
├── pageobjects/    # Page object models
├── tests/          # Test files
├── utils/          # Helper utilities
└── README.md       # This file
```

## Running Tests

### Using Taskfile (Recommended)

The Taskfile provides several convenient commands for running tests:

```bash
# Run all tests
task test

# Run tests with browser visible
task test:headed

# Run tests with video and trace recording
task test:record

# Run a specific test file
task test:specific TEST_FILE=navigation_test.go
```

### Manual Test Execution

You can also run tests directly with Go:

```bash
# Run all tests
go test ./tests/... -v

# Run a specific test
go test ./tests/navigation_test.go -v
```

## Working with HTMX

This project uses HTMX for dynamic updates. Our tests include utilities to wait for HTMX updates to complete before making assertions. The key methods to use are:

- `WaitForHtmxRequest`: Waits for HTMX to send a request
- `WaitForHtmxResponse`: Waits for HTMX to receive a response
- `WaitForHtmxLoad`: Waits for HTMX content to finish loading

## Testing Camera Functionality

For testing camera functionality (QR code scanning, photo capture), we use Chromium flags to mock camera input:

```
--use-fake-device-for-media-stream
--use-file-for-fake-video-capture=path/to/test/video.y4m
```

### Creating Test Videos with Taskfile

You can easily create test videos for camera testing using the Taskfile:

```bash
# Create a default test pattern video
task camera:create-test-video

# Create a video from a static image
task camera:create-test-video IMAGE=path/to/image.jpg

# Convert an existing video to Y4M format
task camera:create-test-video VIDEO=path/to/video.mp4
```

### Using Fake Camera in Tests

Example of starting a browser with fake camera:

```go
browserType.Launch(playwright.BrowserTypeLaunchOptions{
    Args: []string{
        "--use-fake-device-for-media-stream",
        "--use-file-for-fake-video-capture=test-assets/sample-video.y4m",
    },
})
```

See the `tests/camera_test.go` file for a placeholder implementation that will be filled in later.

## VS Code Integration

For VS Code users, install the Playwright Test extension for additional capabilities:

1. Install the "Playwright Test for VSCode" extension
2. Configure the extension to work with Go test files if needed
3. Use the Test Explorer to run and debug tests

## Trace Viewer

Playwright includes a trace viewer for debugging. To enable traces:

```go
context, err := browser.NewContext(playwright.BrowserNewContextOptions{
    RecordVideoDir: playwright.String("videos/"),
})

// Record traces
err = context.Tracing().Start(playwright.TracingStartOptions{
    Screenshots: playwright.Bool(true),
    Snapshots: playwright.Bool(true),
})

// Stop and export traces after test
err = context.Tracing().Stop(playwright.TracingStopOptions{
    Path: playwright.String("trace.zip"),
})
```

### Viewing Traces with Taskfile

Run tests with trace recording and view the results:

```bash
# Run tests with trace recording
task test:record

# View a recorded trace
task trace:view TRACE_FILE=test-results/traces/my-test.zip
```

You can also view traces manually:
```bash
npx playwright show-trace trace.zip
```

## UI Mode (Interactive Testing)

For interactive testing and debugging, Playwright offers a UI mode. However, this is primarily available through the Node.js API. With Go bindings, we can achieve similar functionality through trace recording and playback.

## Using the Taskfile

The project includes a Taskfile.yaml that provides convenient commands for common operations:

```bash
# Show all available tasks
task help

# Run basic operations
task install     # Install Playwright and dependencies
task test        # Run all tests
task clean       # Clean up test artifacts

# More specific operations
task test:headed                # Run tests with browser visible
task test:record                # Run tests with video and trace recording
task test:specific TEST_FILE=navigation_test.go  # Run specific test file

# Camera testing
task camera:create-test-video   # Create a test video
task camera:create-test-video IMAGE=path/to/image.jpg  # Create from image
task camera:create-test-video VIDEO=path/to/video.mp4  # Convert video to Y4M

# Trace viewing
task trace:view TRACE_FILE=test-results/traces/my-test.zip
```

For a complete list of available tasks, run `task help` or inspect the Taskfile.yaml.