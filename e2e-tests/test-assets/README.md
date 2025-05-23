# Test Assets for Playwright E2E Tests

This directory contains assets used for Playwright end-to-end tests, particularly for mocking camera inputs.

## Camera Testing Files

For camera-related tests (QR code scanning, photo capture), Playwright with Chromium can use Y4M format video files to simulate camera input.

### Creating Y4M Files

You can convert existing video files to Y4M format using FFmpeg:

```bash
# Convert an MP4 video to Y4M format
ffmpeg -i input.mp4 -pix_fmt yuv420p output.y4m

# Create a static image video (5 seconds of the same image)
ffmpeg -loop 1 -i input.jpg -c:v libx264 -t 5 -pix_fmt yuv420p temp.mp4
ffmpeg -i temp.mp4 -pix_fmt yuv420p static_image.y4m
rm temp.mp4
```

### Recommended Test Files

Consider creating the following test files:

1. **QR Code Video**: A video showing a QR code that can be scanned
2. **Face/Portrait Video**: For testing photo capture
3. **Static Image Y4M**: A non-moving image for predictable tests

### Using Test Files in Tests

In your test code, reference these files with the browser flag:

```go
browserType.Launch(playwright.BrowserTypeLaunchOptions{
    Args: []string{
        "--use-fake-device-for-media-stream",
        "--use-file-for-fake-video-capture=test-assets/your-file.y4m",
    },
})
```

Or use the test utility function:

```go
config := utils.DefaultTestConfig()
config.UseFakeCamera = true
config.FakeVideoPath = "test-assets/your-file.y4m"
```

## Notes

- Y4M files can be large; consider keeping them small (short duration, lower resolution)
- You might want to use .gitignore to exclude these files from version control
- For QR testing, ensure the QR code in your video is readable and stays on screen long enough