package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/playwright-community/playwright-go"
)

// TestConfig holds configuration for tests
type TestConfig struct {
	BaseURL         string
	Browser         string
	Headless        bool
	SlowMo          float64
	Timeout         time.Duration
	VideoDir        string
	TracesDir       string
	ScreenshotsDir  string
	RecordVideo     bool
	RecordTraces    bool
	TakeScreenshots bool
	UseFakeCamera   bool
	FakeVideoPath   string
}

// DefaultTestConfig returns the default test configuration
func DefaultTestConfig() *TestConfig {
	return &TestConfig{
		BaseURL:         "http://localhost:3000",
		Browser:         "chromium",
		Headless:        true,
		SlowMo:          0,
		Timeout:         30 * time.Second,
		VideoDir:        "test-results/videos",
		TracesDir:       "test-results/traces",
		ScreenshotsDir:  "test-results/screenshots",
		RecordVideo:     false,
		RecordTraces:    false,
		TakeScreenshots: false,
		UseFakeCamera:   false,
		FakeVideoPath:   "",
	}
}

// PlaywrightTest contains all the necessary components for a Playwright test
type PlaywrightTest struct {
	Pw      *playwright.Playwright
	Browser playwright.Browser
	Context playwright.BrowserContext
	Page    playwright.Page
	Config  *TestConfig
}

// NewPlaywrightTest creates a new Playwright test setup
func NewPlaywrightTest(config *TestConfig) (*PlaywrightTest, error) {
	if config == nil {
		config = DefaultTestConfig()
	}

	// Initialize Playwright
	pw, err := playwright.Run()
	if err != nil {
		return nil, fmt.Errorf("could not start Playwright: %w", err)
	}

	// Choose browser type
	var browserType playwright.BrowserType
	switch config.Browser {
	case "chromium":
		browserType = pw.Chromium
	case "firefox":
		browserType = pw.Firefox
	case "webkit":
		browserType = pw.WebKit
	default:
		return nil, fmt.Errorf("unsupported browser type: %s", config.Browser)
	}

	// Prepare browser launch options
	launchOptions := playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(config.Headless),
		SlowMo:   playwright.Float(config.SlowMo),
	}

	// Add camera mocking if needed
	if config.UseFakeCamera {
		if config.Browser != "chromium" {
			return nil, fmt.Errorf("fake camera is only supported in Chromium")
		}

		args := []string{"--use-fake-device-for-media-stream"}
		if config.FakeVideoPath != "" {
			args = append(args, fmt.Sprintf("--use-file-for-fake-video-capture=%s", config.FakeVideoPath))
		}
		launchOptions.Args = args
	}

	// Launch browser
	browser, err := browserType.Launch(launchOptions)
	if err != nil {
		pw.Stop()
		return nil, fmt.Errorf("could not launch browser: %w", err)
	}

	// Create directories if needed
	if config.RecordVideo {
		if err := os.MkdirAll(config.VideoDir, 0755); err != nil {
			browser.Close()
			pw.Stop()
			return nil, fmt.Errorf("could not create video directory: %w", err)
		}
	}

	if config.TakeScreenshots {
		if err := os.MkdirAll(config.ScreenshotsDir, 0755); err != nil {
			browser.Close()
			pw.Stop()
			return nil, fmt.Errorf("could not create screenshots directory: %w", err)
		}
	}

	// Create browser context
	contextOptions := playwright.BrowserNewContextOptions{
		BaseURL: playwright.String(config.BaseURL),
	}

	if config.RecordVideo {
		contextOptions.RecordVideoDir = playwright.String(config.VideoDir)
	}

	context, err := browser.NewContext(contextOptions)
	if err != nil {
		browser.Close()
		pw.Stop()
		return nil, fmt.Errorf("could not create browser context: %w", err)
	}

	// Start tracing if configured
	if config.RecordTraces {
		if err := os.MkdirAll(config.TracesDir, 0755); err != nil {
			context.Close()
			browser.Close()
			pw.Stop()
			return nil, fmt.Errorf("could not create traces directory: %w", err)
		}

		if err := context.Tracing().Start(playwright.TracingStartOptions{
			Screenshots: playwright.Bool(true),
			Snapshots:   playwright.Bool(true),
		}); err != nil {
			context.Close()
			browser.Close()
			pw.Stop()
			return nil, fmt.Errorf("could not start tracing: %w", err)
		}
	}

	// Set default timeout
	context.SetDefaultTimeout(float64(config.Timeout.Milliseconds()))

	// Create a new page
	page, err := context.NewPage()
	if err != nil {
		context.Close()
		browser.Close()
		pw.Stop()
		return nil, fmt.Errorf("could not create page: %w", err)
	}

	return &PlaywrightTest{
		Pw:      pw,
		Browser: browser,
		Context: context,
		Page:    page,
		Config:  config,
	}, nil
}

// Close cleans up all Playwright resources and saves artifacts if configured
func (t *PlaywrightTest) Close(testName string, failed bool) error {
	// Save trace if enabled
	if t.Config.RecordTraces {
		tracePath := filepath.Join(t.Config.TracesDir, fmt.Sprintf("%s.zip", testName))
		if err := t.Context.Tracing().Stop(playwright.TracingStopOptions{
			Path: playwright.String(tracePath),
		}); err != nil {
			fmt.Printf("Warning: could not save trace to %s: %v\n", tracePath, err)
		}
	}

	// Take screenshot if test failed
	if failed && t.Config.TakeScreenshots {
		screenshotPath := filepath.Join(t.Config.ScreenshotsDir, fmt.Sprintf("%s-failure.png", testName))
		if _, err := t.Page.Screenshot(playwright.PageScreenshotOptions{
			Path: playwright.String(screenshotPath),
			FullPage: playwright.Bool(true),
		}); err != nil {
			fmt.Printf("Warning: could not save screenshot to %s: %v\n", screenshotPath, err)
		}
	}

	// Close all resources
	if err := t.Page.Close(); err != nil {
		fmt.Printf("Warning: could not close page: %v\n", err)
	}
	
	if err := t.Context.Close(); err != nil {
		fmt.Printf("Warning: could not close context: %v\n", err)
	}
	
	if err := t.Browser.Close(); err != nil {
		fmt.Printf("Warning: could not close browser: %v\n", err)
	}
	
	if err := t.Pw.Stop(); err != nil {
		return fmt.Errorf("could not stop Playwright: %w", err)
	}
	
	return nil
}

// GetTestFileName returns the current test file name without extension
func GetTestFileName() string {
	_, file, _, ok := runtime.Caller(1)
	if !ok {
		return "unknown_test"
	}
	
	base := filepath.Base(file)
	return base[:len(base)-3] // Remove .go extension
}

// NavigateAndWaitForLoad navigates to a URL and waits for the page to load
func (t *PlaywrightTest) NavigateAndWaitForLoad(url string) error {
	response, err := t.Page.Goto(url, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateNetworkidle,
	})
	
	if err != nil {
		return fmt.Errorf("navigation failed: %w", err)
	}
	
	if response.Status() >= 400 {
		return fmt.Errorf("page loaded with status code %d", response.Status())
	}
	
	return nil
}

// WaitForHTMXOperation waits for an HTMX operation to complete
func (t *PlaywrightTest) WaitForHTMXOperation() error {
	return WaitForHtmxRequest(t.Page)
}