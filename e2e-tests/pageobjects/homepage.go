package pageobjects

import (
	"fmt"

	"geevly/e2e-tests/utils"

	"github.com/playwright-community/playwright-go"
)

// Homepage represents the page object model for the home page
type Homepage struct {
	page playwright.Page
}

// NewHomepage creates a new Homepage page object
func NewHomepage(page playwright.Page) *Homepage {
	return &Homepage{
		page: page,
	}
}

// Selectors for homepage elements
const (
	HeaderLogoSelector     = "a[hx-get='/']"
	NavigationSelector     = "nav"
	SignInButtonSelector   = "a[hx-get='/sign-in']"
	AdminLinkSelector      = "a[hx-get='/admin']"
	StaffLinkSelector      = "a[hx-get='/staff']"
	FeedingLinkSelector    = "a[hx-get='/feeding']"
	HowItWorksLinkSelector = "a[hx-get='/how-it-works']"
	AboutUsLinkSelector    = "a[hx-get='/about']"
	ContentDivSelector     = "#content"
)

// Navigate navigates to the homepage
func (h *Homepage) Navigate() error {
	response, err := h.page.Goto("/", playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateNetworkidle,
	})
	if err != nil {
		return fmt.Errorf("failed to navigate to homepage: %w", err)
	}

	if response.Status() >= 400 {
		return fmt.Errorf("homepage loaded with status code %d", response.Status())
	}

	return nil
}

// ClickSignIn clicks the sign-in link and waits for the HTMX update
func (h *Homepage) ClickSignIn() error {
	return utils.ClickAndWaitForHtmx(h.page, SignInButtonSelector)
}

// ClickLogo clicks the logo to navigate to the homepage
func (h *Homepage) ClickLogo() error {
	return utils.ClickAndWaitForHtmx(h.page, HeaderLogoSelector)
}

// ClickAdmin clicks the admin link
func (h *Homepage) ClickAdmin() error {
	return utils.ClickAndWaitForHtmx(h.page, AdminLinkSelector)
}

// ClickStaff clicks the staff link
func (h *Homepage) ClickStaff() error {
	return utils.ClickAndWaitForHtmx(h.page, StaffLinkSelector)
}

// ClickFeeding clicks the feeding link
func (h *Homepage) ClickFeeding() error {
	return utils.ClickAndWaitForHtmx(h.page, FeedingLinkSelector)
}

// ClickHowItWorks clicks the "How it works" link
func (h *Homepage) ClickHowItWorks() error {
	return utils.ClickAndWaitForHtmx(h.page, HowItWorksLinkSelector)
}

// ClickAboutUs clicks the "About Us" link
func (h *Homepage) ClickAboutUs() error {
	return utils.ClickAndWaitForHtmx(h.page, AboutUsLinkSelector)
}

// IsSignedIn checks if the user is signed in
func (h *Homepage) IsSignedIn() (bool, error) {
	signInButton, err := h.page.QuerySelector(SignInButtonSelector)
	if err != nil {
		return false, fmt.Errorf("error checking sign-in status: %w", err)
	}
	
	// If sign-in button is not present, user is signed in
	return signInButton == nil, nil
}

// IsAdminLinkVisible checks if the admin link is visible
func (h *Homepage) IsAdminLinkVisible() (bool, error) {
	adminLink, err := h.page.QuerySelector(AdminLinkSelector)
	if err != nil {
		return false, fmt.Errorf("error checking admin link visibility: %w", err)
	}
	
	return adminLink != nil, nil
}

// IsStaffLinkVisible checks if the staff link is visible
func (h *Homepage) IsStaffLinkVisible() (bool, error) {
	staffLink, err := h.page.QuerySelector(StaffLinkSelector)
	if err != nil {
		return false, fmt.Errorf("error checking staff link visibility: %w", err)
	}
	
	return staffLink != nil, nil
}

// GetPageTitle returns the title of the page
func (h *Homepage) GetPageTitle() (string, error) {
	return h.page.Title()
}

// GetCurrentURL returns the current URL
func (h *Homepage) GetCurrentURL() string {
	return h.page.URL()
}

// WaitForContentLoad waits for the content div to be loaded
func (h *Homepage) WaitForContentLoad() error {
	_, err := utils.WaitForHtmxSelector(h.page, ContentDivSelector)
	return err
}

// GetContentText gets the text content of the main content div
func (h *Homepage) GetContentText() (string, error) {
	content, err := h.page.QuerySelector(ContentDivSelector)
	if err != nil {
		return "", fmt.Errorf("failed to get content element: %w", err)
	}
	
	text, err := content.TextContent()
	if err != nil {
		return "", fmt.Errorf("failed to get content text: %w", err)
	}
	
	return text, nil
}