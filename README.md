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

## License

This project is licensed under the MIT License - see the [LICENSE.md](LICENSE.md) file for details.
