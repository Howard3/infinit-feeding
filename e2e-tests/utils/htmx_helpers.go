package utils

import (
	"fmt"
	"time"

	"github.com/playwright-community/playwright-go"
)

// HTMXEvents contains constants for HTMX event names
const (
	HTMXRequestStart  = "htmx:beforeRequest"
	HTMXRequestEnd    = "htmx:afterRequest"
	HTMXBeforeSwap    = "htmx:beforeSwap"
	HTMXAfterSwap     = "htmx:afterSwap"
	HTMXResponseError = "htmx:responseError"
	HTMXLoadEvent     = "htmx:load"
)

// HTMXTimeout is the default timeout for HTMX operations
const HTMXTimeout = 30 * time.Second

// WaitForHtmxRequest waits for an HTMX request to complete
// It first waits for a request to start, then for it to complete
func WaitForHtmxRequest(page playwright.Page) error {
	// Wait for the request to start
	if _, err := page.WaitForEvent(HTMXRequestStart, playwright.PageWaitForEventOptions{
		Timeout: playwright.Float(HTMXTimeout.Seconds() * 1000),
	}); err != nil {
		return fmt.Errorf("timeout waiting for HTMX request to start: %w", err)
	}

	// Wait for the request to complete
	if _, err := page.WaitForEvent(HTMXRequestEnd, playwright.PageWaitForEventOptions{
		Timeout: playwright.Float(HTMXTimeout.Seconds() * 1000),
	}); err != nil {
		return fmt.Errorf("timeout waiting for HTMX request to complete: %w", err)
	}

	return nil
}

// WaitForHtmxSwap waits for HTMX content swap to complete
func WaitForHtmxSwap(page playwright.Page) error {
	// Wait for the swap to start
	if _, err := page.WaitForEvent(HTMXBeforeSwap, playwright.PageWaitForEventOptions{
		Timeout: playwright.Float(HTMXTimeout.Seconds() * 1000),
	}); err != nil {
		return fmt.Errorf("timeout waiting for HTMX swap to start: %w", err)
	}

	// Wait for the swap to complete
	if _, err := page.WaitForEvent(HTMXAfterSwap, playwright.PageWaitForEventOptions{
		Timeout: playwright.Float(HTMXTimeout.Seconds() * 1000),
	}); err != nil {
		return fmt.Errorf("timeout waiting for HTMX swap to complete: %w", err)
	}

	return nil
}

// WaitForHtmxLoad waits for an HTMX load event
func WaitForHtmxLoad(page playwright.Page) error {
	if _, err := page.WaitForEvent(HTMXLoadEvent, playwright.PageWaitForEventOptions{
		Timeout: playwright.Float(HTMXTimeout.Seconds() * 1000),
	}); err != nil {
		return fmt.Errorf("timeout waiting for HTMX load event: %w", err)
	}
	return nil
}

// WaitForHtmxSelector waits for an element to appear after an HTMX operation
func WaitForHtmxSelector(page playwright.Page, selector string) (playwright.ElementHandle, error) {
	element, err := page.WaitForSelector(selector, playwright.PageWaitForSelectorOptions{
		State:   playwright.WaitForSelectorStateAttached,
		Timeout: playwright.Float(HTMXTimeout.Seconds() * 1000),
	})
	if err != nil {
		return nil, fmt.Errorf("timeout waiting for element %s after HTMX operation: %w", selector, err)
	}
	return element, nil
}

// ClickAndWaitForHtmx clicks an element and waits for the HTMX request and swap to complete
func ClickAndWaitForHtmx(page playwright.Page, selector string) error {
	// First ensure element is visible
	element, err := page.WaitForSelector(selector, playwright.PageWaitForSelectorOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(HTMXTimeout.Seconds() * 1000),
	})
	if err != nil {
		return fmt.Errorf("timeout waiting for element %s to be visible: %w", selector, err)
	}

	// Set up an event listener for HTMX request start
	requestStarted := make(chan bool, 1)
	if _, err := page.AddListener(HTMXRequestStart, func() {
		requestStarted <- true
	}); err != nil {
		return fmt.Errorf("failed to add listener for HTMX request start: %w", err)
	}

	// Click the element
	if err := element.Click(); err != nil {
		return fmt.Errorf("failed to click element %s: %w", selector, err)
	}

	// Wait for the request to start or timeout
	select {
	case <-requestStarted:
		// Request started, now wait for it to complete
		return WaitForHtmxRequest(page)
	case <-time.After(HTMXTimeout):
		return fmt.Errorf("timeout waiting for HTMX request to start after clicking %s", selector)
	}
}

// WaitForHtmxResponse waits for an HTMX response and checks if it's an error
func WaitForHtmxResponse(page playwright.Page) error {
	// Set up an event listener for errors
	errorOccurred := make(chan error, 1)
	if _, err := page.AddListener(HTMXResponseError, func(event map[string]interface{}) {
		if event != nil {
			if errMsg, ok := event["error"].(string); ok {
				errorOccurred <- fmt.Errorf("HTMX response error: %s", errMsg)
			} else {
				errorOccurred <- fmt.Errorf("HTMX response error occurred")
			}
		}
	}); err != nil {
		return fmt.Errorf("failed to add listener for HTMX response error: %w", err)
	}

	// Wait for the request to complete
	requestCompleted := make(chan bool, 1)
	if _, err := page.AddListener(HTMXRequestEnd, func() {
		requestCompleted <- true
	}); err != nil {
		return fmt.Errorf("failed to add listener for HTMX request end: %w", err)
	}

	// Wait for either the request to complete or an error to occur
	select {
	case err := <-errorOccurred:
		return err
	case <-requestCompleted:
		return nil
	case <-time.After(HTMXTimeout):
		return fmt.Errorf("timeout waiting for HTMX response")
	}
}

// ExecuteHtmxCall executes custom JavaScript to trigger an HTMX call and waits for completion
func ExecuteHtmxCall(page playwright.Page, jsCode string) error {
	// Execute the JavaScript
	if _, err := page.Evaluate(jsCode); err != nil {
		return fmt.Errorf("failed to execute JavaScript for HTMX call: %w", err)
	}

	// Wait for the HTMX operation to complete
	return WaitForHtmxRequest(page)
}