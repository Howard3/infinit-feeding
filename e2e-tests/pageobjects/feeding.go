package pageobjects

import (
	"fmt"
	"time"

	"geevly/e2e-tests/utils"

	"github.com/playwright-community/playwright-go"
)

// Feeding represents the page object model for the feeding functionality
type Feeding struct {
	page playwright.Page
}

// NewFeeding creates a new Feeding page object
func NewFeeding(page playwright.Page) *Feeding {
	return &Feeding{
		page: page,
	}
}

// Selectors for feeding page elements
const (
	FeedingLinkSelector    = "a[hx-get='/feeding']"
	QRReaderSelector       = "#reader"
	StartCameraSelector    = "#start_camera"
	MainCameraSelector     = "#main_camera"
	CapturePhotoSelector   = "#capture_photo"
	VideoSelector          = "#video"
	CanvasSelector         = "#canvas"
	PhotoSelector          = "#photo"
	SnapButtonSelector     = "#snap"
	ResetButtonSelector    = "#reset"
	UploadFormSelector     = "#upload_form"
	Base64PhotoSelector    = "#base64_photo"
	SubmitButtonSelector   = "img[hx-post='/feeding/proof']"
	ReceivedMessageSelector = ".text-3xl"
)

// Navigate navigates to the feeding page
func (f *Feeding) Navigate() error {
	response, err := f.page.Goto("/feeding", playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateNetworkidle,
	})
	if err != nil {
		return fmt.Errorf("failed to navigate to feeding page: %w", err)
	}

	if response.Status() >= 400 {
		return fmt.Errorf("feeding page loaded with status code %d", response.Status())
	}

	return nil
}

// ClickFeedingLink clicks the feeding link in the navigation
func (f *Feeding) ClickFeedingLink() error {
	return utils.ClickAndWaitForHtmx(f.page, FeedingLinkSelector)
}

// IsQRReaderVisible checks if the QR reader is visible
func (f *Feeding) IsQRReaderVisible() (bool, error) {
	reader, err := f.page.QuerySelector(QRReaderSelector)
	if err != nil {
		return false, fmt.Errorf("error checking QR reader visibility: %w", err)
	}
	
	return reader != nil, nil
}

// WaitForQRReaderToInitialize waits for the QR reader to be initialized
func (f *Feeding) WaitForQRReaderToInitialize() error {
	// Wait for the QR reader element to be visible
	_, err := f.page.WaitForSelector(QRReaderSelector, playwright.PageWaitForSelectorOptions{
		State: playwright.WaitForSelectorStateVisible,
	})
	if err != nil {
		return fmt.Errorf("timeout waiting for QR reader to be visible: %w", err)
	}
	
	// Wait a bit for the QR scanner to initialize
	time.Sleep(1 * time.Second)
	
	return nil
}

// MockQRCodeScan simulates a QR code scan by calling the success callback function
func (f *Feeding) MockQRCodeScan(qrCode string) error {
	// Execute JavaScript to simulate a successful QR code scan
	_, err := f.page.Evaluate(fmt.Sprintf(`
		if (typeof onScanSuccess === 'function') {
			onScanSuccess("%s", {});
		} else {
			console.error("onScanSuccess function not found");
		}
	`, qrCode))
	
	if err != nil {
		return fmt.Errorf("failed to mock QR code scan: %w", err)
	}
	
	return nil
}

// ClickStartCamera clicks the start camera button
func (f *Feeding) ClickStartCamera() error {
	// Wait for the start camera element to be visible
	_, err := f.page.WaitForSelector(StartCameraSelector, playwright.PageWaitForSelectorOptions{
		State: playwright.WaitForSelectorStateVisible,
	})
	if err != nil {
		return fmt.Errorf("timeout waiting for start camera button: %w", err)
	}
	
	// Click the start camera button
	if err := f.page.Click(StartCameraSelector); err != nil {
		return fmt.Errorf("failed to click start camera button: %w", err)
	}
	
	return nil
}

// WaitForCameraToInitialize waits for the camera to initialize
func (f *Feeding) WaitForCameraToInitialize() error {
	// Wait for the video element to be visible
	_, err := f.page.WaitForSelector(VideoSelector, playwright.PageWaitForSelectorOptions{
		State: playwright.WaitForSelectorStateVisible,
	})
	if err != nil {
		return fmt.Errorf("timeout waiting for camera video to be visible: %w", err)
	}
	
	// Wait for the main camera container to be visible
	_, err = f.page.WaitForSelector(MainCameraSelector, playwright.PageWaitForSelectorOptions{
		State: playwright.WaitForSelectorStateVisible,
	})
	if err != nil {
		return fmt.Errorf("timeout waiting for main camera to be visible: %w", err)
	}
	
	// Wait a bit for the camera to fully initialize
	time.Sleep(1 * time.Second)
	
	return nil
}

// CapturePhoto clicks the capture photo button
func (f *Feeding) CapturePhoto() error {
	// Wait for the capture photo element to be visible
	_, err := f.page.WaitForSelector(CapturePhotoSelector, playwright.PageWaitForSelectorOptions{
		State: playwright.WaitForSelectorStateVisible,
	})
	if err != nil {
		return fmt.Errorf("timeout waiting for capture photo button: %w", err)
	}
	
	// Click the capture photo button
	if err := f.page.Click(CapturePhotoSelector); err != nil {
		return fmt.Errorf("failed to click capture photo button: %w", err)
	}
	
	// Wait for the photo to be captured (upload form becomes visible)
	_, err = f.page.WaitForSelector(UploadFormSelector, playwright.PageWaitForSelectorOptions{
		State: playwright.WaitForSelectorStateVisible,
	})
	if err != nil {
		return fmt.Errorf("timeout waiting for upload form after photo capture: %w", err)
	}
	
	return nil
}

// SubmitPhoto clicks the submit button to upload the captured photo
func (f *Feeding) SubmitPhoto() error {
	// Wait for the submit button to be visible
	_, err := f.page.WaitForSelector(SubmitButtonSelector, playwright.PageWaitForSelectorOptions{
		State: playwright.WaitForSelectorStateVisible,
	})
	if err != nil {
		return fmt.Errorf("timeout waiting for submit button: %w", err)
	}
	
	// Click the submit button and wait for HTMX request to complete
	return utils.ClickAndWaitForHtmx(f.page, SubmitButtonSelector)
}

// ResetCamera clicks the reset button to restart the camera
func (f *Feeding) ResetCamera() error {
	// Wait for the reset button to be visible
	_, err := f.page.WaitForSelector(ResetButtonSelector, playwright.PageWaitForSelectorOptions{
		State: playwright.WaitForSelectorStateVisible,
	})
	if err != nil {
		return fmt.Errorf("timeout waiting for reset button: %w", err)
	}
	
	// Click the reset button
	if err := f.page.Click(ResetButtonSelector); err != nil {
		return fmt.Errorf("failed to click reset button: %w", err)
	}
	
	// Wait for the video element to become visible again
	_, err = f.page.WaitForSelector(VideoSelector, playwright.PageWaitForSelectorOptions{
		State: playwright.WaitForSelectorStateVisible,
	})
	if err != nil {
		return fmt.Errorf("timeout waiting for video to be visible after reset: %w", err)
	}
	
	return nil
}

// IsPhotoDisplayed checks if the captured photo is displayed
func (f *Feeding) IsPhotoDisplayed() (bool, error) {
	photo, err := f.page.QuerySelector(PhotoSelector + ":visible")
	if err != nil {
		return false, fmt.Errorf("error checking if photo is displayed: %w", err)
	}
	
	return photo != nil, nil
}

// GetReceivedMessage gets the confirmation message after successful feeding
func (f *Feeding) GetReceivedMessage() (string, error) {
	msgElement, err := f.page.QuerySelector(ReceivedMessageSelector)
	if err != nil {
		return "", fmt.Errorf("failed to get received message element: %w", err)
	}
	
	if msgElement == nil {
		return "", fmt.Errorf("received message element not found")
	}
	
	msg, err := msgElement.TextContent()
	if err != nil {
		return "", fmt.Errorf("failed to get received message text: %w", err)
	}
	
	return msg, nil
}

// WaitForSuccessfulSubmission waits for the successful submission message
func (f *Feeding) WaitForSuccessfulSubmission() error {
	_, err := f.page.WaitForSelector(ReceivedMessageSelector, playwright.PageWaitForSelectorOptions{
		State: playwright.WaitForSelectorStateVisible,
	})
	return err
}

// MockCameraWithJavaScript injects JavaScript to mock the camera functionality
func (f *Feeding) MockCameraWithJavaScript() error {
	// Inject JavaScript to mock getUserMedia and canvas operations
	_, err := f.page.AddInitScript(`
		// Mock navigator.mediaDevices.getUserMedia
		if (navigator.mediaDevices === undefined) {
			navigator.mediaDevices = {};
		}
		
		navigator.mediaDevices.getUserMedia = function() {
			return Promise.resolve(new MediaStream([{
				// Mock video track
				kind: 'video',
				stop: function() {}
			}]));
		};
		
		// Mock canvas operations when they occur
		const originalGetContext = HTMLCanvasElement.prototype.getContext;
		HTMLCanvasElement.prototype.getContext = function() {
			const context = originalGetContext.apply(this, arguments);
			if (context && context.drawImage) {
				const originalDrawImage = context.drawImage;
				context.drawImage = function() {
					// Mock the drawImage operation
					originalDrawImage.apply(this, arguments);
				};
			}
			return context;
		};
		
		// Mock toDataURL to return a simple data URL
		HTMLCanvasElement.prototype.toDataURL = function() {
			return 'data:image/jpeg;base64,/9j/4AAQSkZJRgABAQEAYABgAAD/2wBDAAIBAQIBAQICAgICAgICAwUDAwMDAwYEBAMFBwYHBwcGBwcICQsJCAgKCAcHCg0KCgsMDAwMBwkODw0MDgsMDAz/2wBDAQICAgMDAwYDAwYMCAcIDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAz/wAARCAABAAEDASIAAhEBAxEB/8QAHwAAAQUBAQEBAQEAAAAAAAAAAAECAwQFBgcICQoL/8QAtRAAAgEDAwIEAwUFBAQAAAF9AQIDAAQRBRIhMUEGE1FhByJxFDKBkaEII0KxwRVS0fAkM2JyggkKFhcYGRolJicoKSo0NTY3ODk6Q0RFRkdISUpTVFVWV1hZWmNkZWZnaGlqc3R1dnd4eXqDhIWGh4iJipKTlJWWl5iZmqKjpKWmp6ipqrKztLW2t7i5usLDxMXGx8jJytLT1NXW19jZ2uHi4+Tl5ufo6erx8vP09fb3+Pn6/8QAHwEAAwEBAQEBAQEBAQAAAAAAAAECAwQFBgcICQoL/8QAtREAAgECBAQDBAcFBAQAAQJ3AAECAxEEBSExBhJBUQdhcRMiMoEIFEKRobHBCSMzUvAVYnLRChYkNOEl8RcYGRomJygpKjU2Nzg5OkNERUZHSElKU1RVVldYWVpjZGVmZ2hpanN0dXZ3eHl6goOEhYaHiImKkpOUlZaXmJmaoqOkpaanqKmqsrO0tba3uLm6wsPExcbHyMnK0tPU1dbX2Nna4uPk5ebn6Onq8vP09fb3+Pn6/9oADAMBAAIRAxEAPwD8nKKKK/7+P/gB/9k=';
		};
	`)
	
	return err
}