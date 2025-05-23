# Infinit Feeding 

## Introduction

This document provides setup and installation instructions for the Infinit Feeding. The project requires generating Protobuf files, generating CSS using Tailwind CLI, and generating templates using Templ CLI. This guide assumes you have basic knowledge of Go, CSS, and protocol buffers.

## Prerequisites

Before you begin, ensure you have the following installed:
- [Go](https://golang.org/dl/): Programming language used for this project.
- [Node.js and npm](https://nodejs.org/en/download/): Required for TailwindCSS.
- [Buf](https://docs.buf.build/installation): Tool for working with Protocol Buffers.
- [Tailwind CLI](https://tailwindcss.com/docs/installation): Used for generating CSS.
- [Templ CLI](https://templ.dev/docs/getting-started/installation): Used for template generation.
- [Taskfile](https://taskfile.dev/#/installation): A task runner / simpler Make alternative written in Go.
- [Playwright](https://playwright.dev/) (optional): For running end-to-end tests. Installation script provided in the e2e-tests directory.

## Setup

Follow these steps to set up the project environment:

1. **Install Taskfile CLI**  
   Taskfile simplifies and documents common tasks in the project. Install Taskfile CLI by following the instructions [here](https://taskfile.dev/#/installation).

2. **Install Buf**  
   Buf is used to generate Go code from protocol buffer files. Install Buf by following the instructions [here](https://docs.buf.build/installation).

3. **Install Tailwind CLI**  
   Tailwind CSS is used for styling the project. Install Tailwind CLI globally via npm:
   ```bash
   npm install -g tailwindcss
   ```

4. **Install Templ CLI**  
   Templ is used for template generation. Install Templ CLI by following the instructions provided [here](https://templ.dev/docs/getting-started/installation).

## Running the Project

To run the project, use the Taskfile commands defined for various tasks. Here's how to use them:

### Development

- **Start Development Server with Live Reload**  
  Use the `dev` task to start the development server with live reloading for Tailwind CSS and Go server:
  ```bash
  task dev
  ```

### API Documentation
The project uses Swagger/OpenAPI for API documentation. To generate or update the API documentation:

1. **Install Swagger Tools**
   ```bash
   go install github.com/swaggo/swag/cmd/swag@latest
   ```

2. **Generate Documentation**
   ```bash
   ./scripts/swagger.sh
   ```

3. **View Documentation**  
   First ensure GO_ENV is set to "development":
   ```bash
   export GO_ENV=development
   ```
   
   Once the server is running, visit:
   ```
   http://localhost:3000/swagger/index.html
   ```

Note: The Swagger UI is only available when GO_ENV is set to "development". The Swagger UI provides an interactive interface to explore and test the API endpoints.

### End-to-End Testing with Playwright

The project includes end-to-end testing using Playwright with Go bindings to test the application's functionality, including HTMX interactions.

1. **Install Playwright with Go Bindings**
   
   Using the installation script:
   ```bash
   cd e2e-tests
   ./install-playwright.sh
   ```
   
   Or using the Taskfile:
   ```bash
   cd e2e-tests
   task install
   ```
   
   This will:
   - Install the `playwright-go` package
   - Install required browsers
   - Set up test directories

2. **Run the Tests**

   Using the Taskfile (recommended):
   ```bash
   cd e2e-tests
   task test                   # Run all tests
   task test:headed            # Run with browser visible
   task test:record            # Run with video and trace recording
   task test:specific TEST_FILE=navigation_test.go  # Run specific test
   ```

   Or using Go directly:
   ```bash
   cd e2e-tests
   go test ./tests/... -v
   ```

3. **Camera Testing Utilities**

   The Taskfile includes utilities for camera testing:
   ```bash
   cd e2e-tests
   task camera:create-test-video         # Create a test pattern video
   task camera:create-test-video VIDEO=path/to/video.mp4  # Convert video to Y4M
   ```

The test suite includes special utilities for handling HTMX-driven UI updates and camera interactions. For detailed information about the testing framework, test organization, and camera testing setup, refer to the documentation in the `e2e-tests` directory.

### Build Dependencies

- **Build All Dependencies**  
  This includes generating templates, Tailwind CSS, and Protobuf files:
  ```bash
  task build:dependencies
  ```

- **Generate Templates**  
  Generate template files with Templ CLI:
  ```bash
  task build:templates
  ```

- **Generate Tailwind CSS**  
  Generate the project's CSS using Tailwind CLI:
  ```bash
  task build:tailwind
  ```

- **Watch Tailwind CSS for Changes**  
  Automatically regenerate CSS when changes are detected:
  ```bash
  task watch:tailwind
  ```

- **Generate Protobuf Files**  
  Generate Go code from Protobuf files using Buf in the `events` directory:
  ```bash
  task build:buf
  ```

## Additional Information

For more detailed information about each tool used in this project, please refer to their official documentation.

- Buf: [https://docs.buf.build/](https://docs.buf.build/)
- Tailwind CSS: [https://tailwindcss.com/docs](https://tailwindcss.com/docs)
- Templ: [https://templ.dev/docs](https://templ.dev/docs)
- Taskfile: [https://taskfile.dev/#/](https://taskfile.dev/#/)
- Playwright: [https://playwright.dev/](https://playwright.dev/)
- Playwright Go: [https://github.com/playwright-community/playwright-go](https://github.com/playwright-community/playwright-go)

## Contributing

Please read [CONTRIBUTING.md](/CONTRIBUTING.md) for details on our code of conduct, and the process for submitting pull requests to us.

## License

This project is licensed under the MIT License - see the [LICENSE.md](LICENSE.md) file for details.
