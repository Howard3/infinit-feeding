package tests

import (
	"testing"
	"time"

	"geevly/e2e-tests/pageobjects"
	"geevly/e2e-tests/utils"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBasicNavigation(t *testing.T) {
	// Set up test configuration
	config := utils.DefaultTestConfig()
	config.Headless = false // Set to true for CI environments
	config.SlowMo = 200     // Slow down operations for visibility
	config.RecordVideo = true
	config.RecordTraces = true

	// Initialize Playwright
	pw, err := utils.NewPlaywrightTest(config)
	require.NoError(t, err, "Failed to initialize Playwright")
	defer func() {
		err := pw.Close("navigation_test", t.Failed())
		if err != nil {
			t.Logf("Warning: failed to close Playwright resources: %v", err)
		}
	}()

	// Create page objects
	homepage := pageobjects.NewHomepage(pw.Page)

	// Navigate to the homepage
	err = homepage.Navigate()
	require.NoError(t, err, "Failed to navigate to homepage")

	// Get the page title
	title, err := homepage.GetPageTitle()
	require.NoError(t, err, "Failed to get page title")
	assert.Equal(t, "Infinit Feeding", title, "Page title should be 'Infinit Feeding'")

	// Wait for a short time to ensure page is fully loaded
	time.Sleep(500 * time.Millisecond)

	// Test navigation to "How it works" page
	err = homepage.ClickHowItWorks()
	require.NoError(t, err, "Failed to click 'How it works' link")

	// Verify content has changed (this is an HTMX update)
	content, err := homepage.GetContentText()
	require.NoError(t, err, "Failed to get content text")
	assert.Contains(t, content, "How it works", "Content should contain 'How it works' text")

	// Test navigation to "About Us" page
	err = homepage.ClickAboutUs()
	require.NoError(t, err, "Failed to click 'About Us' link")

	// Verify content has changed
	content, err = homepage.GetContentText()
	require.NoError(t, err, "Failed to get content text")
	assert.Contains(t, content, "About Us", "Content should contain 'About Us' text")

	// Navigate back to the homepage by clicking the logo
	err = homepage.ClickLogo()
	require.NoError(t, err, "Failed to click logo to return to homepage")

	// Try to navigate to Feeding page
	err = homepage.ClickFeeding()
	require.NoError(t, err, "Failed to click 'Feeding' link")

	// Check if we can navigate further (if we're signed in)
	isSignedIn, err := homepage.IsSignedIn()
	require.NoError(t, err, "Failed to check if signed in")

	// Log the sign-in status (this test doesn't attempt to sign in)
	t.Logf("User is signed in: %v", isSignedIn)

	// Take a screenshot at the end of the test
	_, err = pw.Page.Screenshot(playwright.PageScreenshotOptions{
		Path:     playwright.String("navigation_test_final.png"),
		FullPage: playwright.Bool(true),
	})
	require.NoError(t, err, "Failed to take screenshot")
}