package tests

import (
	"testing"
	"time"

	"geevly/e2e-tests/pageobjects"
	"geevly/e2e-tests/utils"

	"github.com/playwright-community/playwright-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHTMXInteractions demonstrates testing HTMX-driven UI updates
func TestHTMXInteractions(t *testing.T) {
	// Set up test configuration
	config := utils.DefaultTestConfig()
	config.Headless = false // Set to true for CI environments
	config.SlowMo = 300     // Slow down operations for visibility
	config.RecordTraces = true

	// Initialize Playwright
	pw, err := utils.NewPlaywrightTest(config)
	require.NoError(t, err, "Failed to initialize Playwright")
	defer func() {
		err := pw.Close("htmx_test", t.Failed())
		if err != nil {
			t.Logf("Warning: failed to close Playwright resources: %v", err)
		}
	}()

	// Create page objects
	homepage := pageobjects.NewHomepage(pw.Page)

	// Navigate to the homepage
	err = homepage.Navigate()
	require.NoError(t, err, "Failed to navigate to homepage")

	// Wait for the content to load
	err = homepage.WaitForContentLoad()
	require.NoError(t, err, "Failed to wait for content to load")

	// Store the initial content for comparison
	initialContent, err := homepage.GetContentText()
	require.NoError(t, err, "Failed to get initial content text")

	// Test HTMX navigation to "How it works" page
	t.Log("Clicking 'How it works' link...")
	err = homepage.ClickHowItWorks()
	require.NoError(t, err, "Failed to click 'How it works' link")

	// Wait for HTMX to update the content
	t.Log("Waiting for HTMX content update...")
	time.Sleep(500 * time.Millisecond) // Give a little extra time for visual confirmation

	// Verify content has changed via HTMX swap
	updatedContent, err := homepage.GetContentText()
	require.NoError(t, err, "Failed to get updated content text")
	assert.NotEqual(t, initialContent, updatedContent, "Content should have changed after HTMX swap")
	assert.Contains(t, updatedContent, "How it works", "Content should contain 'How it works' text")

	// Test HTMX navigation to "About Us" page
	t.Log("Clicking 'About Us' link...")
	err = homepage.ClickAboutUs()
	require.NoError(t, err, "Failed to click 'About Us' link")

	// Explicitly wait for HTMX operations to complete
	err = utils.WaitForHtmxSwap(pw.Page)
	require.NoError(t, err, "Failed to wait for HTMX swap")

	// Verify content has changed again
	aboutContent, err := homepage.GetContentText()
	require.NoError(t, err, "Failed to get About Us content")
	assert.NotEqual(t, updatedContent, aboutContent, "Content should have changed after HTMX swap")
	assert.Contains(t, aboutContent, "About Us", "Content should contain 'About Us' text")

	// Test custom HTMX JavaScript interaction
	t.Log("Testing custom HTMX JavaScript interaction...")
	jsCode := `
		const contentDiv = document.querySelector('#content');
		htmx.ajax('GET', '/how-it-works', {target: '#content', swap: 'innerHTML'});
	`
	err = utils.ExecuteHtmxCall(pw.Page, jsCode)
	require.NoError(t, err, "Failed to execute custom HTMX JavaScript")

	// Verify we're back to the "How it works" page
	finalContent, err := homepage.GetContentText()
	require.NoError(t, err, "Failed to get final content")
	assert.Contains(t, finalContent, "How it works", "Content should contain 'How it works' text again")

	// Take a screenshot at the end of the test
	_, err = pw.Page.Screenshot(playwright.PageScreenshotOptions{
		Path:     playwright.String("htmx_test_final.png"),
		FullPage: playwright.Bool(true),
	})
	require.NoError(t, err, "Failed to take screenshot")
}

// TestHTMXResponseHandling demonstrates waiting for HTMX responses
func TestHTMXResponseHandling(t *testing.T) {
	// Set up test configuration
	config := utils.DefaultTestConfig()
	config.Headless = false // Set to true for CI environments

	// Initialize Playwright
	pw, err := utils.NewPlaywrightTest(config)
	require.NoError(t, err, "Failed to initialize Playwright")
	defer func() {
		err := pw.Close("htmx_response_test", t.Failed())
		if err != nil {
			t.Logf("Warning: failed to close Playwright resources: %v", err)
		}
	}()

	// Add event listeners to log HTMX events for debugging
	_, err = pw.Page.AddInitScript(`
		document.addEventListener('htmx:beforeRequest', function(evt) {
			console.log('HTMX Request Started:', evt.detail.path);
		});
		document.addEventListener('htmx:afterRequest', function(evt) {
			console.log('HTMX Request Completed:', evt.detail.path);
		});
		document.addEventListener('htmx:beforeSwap', function(evt) {
			console.log('HTMX Before Swap');
		});
		document.addEventListener('htmx:afterSwap', function(evt) {
			console.log('HTMX After Swap');
		});
	`)
	require.NoError(t, err, "Failed to add HTMX event listeners")

	// Create page objects
	homepage := pageobjects.NewHomepage(pw.Page)

	// Navigate to the homepage
	err = homepage.Navigate()
	require.NoError(t, err, "Failed to navigate to homepage")

	// Click each navigation link and demonstrate different ways to wait for HTMX responses
	links := []struct {
		name     string
		clickFn  func() error
		waitType string
		waitFn   func() error
	}{
		{
			name:     "How it works",
			clickFn:  homepage.ClickHowItWorks,
			waitType: "WaitForHtmxRequest",
			waitFn:   func() error { return utils.WaitForHtmxRequest(pw.Page) },
		},
		{
			name:     "About Us",
			clickFn:  homepage.ClickAboutUs,
			waitType: "WaitForHtmxSwap",
			waitFn:   func() error { return utils.WaitForHtmxSwap(pw.Page) },
		},
		{
			name:     "Logo/Home",
			clickFn:  homepage.ClickLogo,
			waitType: "WaitForHtmxSelector",
			waitFn: func() error {
				_, err := utils.WaitForHtmxSelector(pw.Page, "#content")
				return err
			},
		},
	}

	for _, link := range links {
		t.Logf("Testing HTMX navigation to '%s' using %s", link.name, link.waitType)
		
		// Click the link
		err = link.clickFn()
		require.NoError(t, err, "Failed to click '%s' link", link.name)
		
		// Wait using the specified wait function
		err = link.waitFn()
		require.NoError(t, err, "Failed to wait for HTMX update using %s", link.waitType)
		
		// Verify content has changed
		content, err := homepage.GetContentText()
		require.NoError(t, err, "Failed to get content text")
		
		// For home page, check URL instead of content
		if link.name == "Logo/Home" {
			assert.Equal(t, "/", homepage.GetCurrentURL(), "URL should be homepage")
		} else {
			assert.Contains(t, content, link.name, "Content should contain '%s' text", link.name)
		}
		
		// Brief pause for visual confirmation
		time.Sleep(300 * time.Millisecond)
	}
}