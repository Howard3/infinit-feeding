document.addEventListener('DOMContentLoaded', function() {
    // Integrate w/ HTMX to track request performance.
    function getRequestUrl(event) {
        return event.detail.pathInfo?.requestPath || event.detail.requestConfig?.url || 'unknown';
    }

    function getRequestMethod(event) {
        return event.detail.requestConfig?.verb || 'unknown'
    }
    
    document.body.addEventListener('htmx:beforeRequest', function (event) {
        // Store the start time directly on the element
        event.target.dataset.htmxStartTime = performance.now();
    });
    
    document.body.addEventListener('htmx:afterRequest', function (event) {
        // Retrieve the start time from the element
        const startTime = parseFloat(event.target.dataset.htmxStartTime);
    
        if (startTime) {
            const duration = performance.now() - startTime;
    
            // Capture the method and URL
            const method = getRequestMethod(event);
            const url = getRequestUrl(event);
    
            // Track the performance with PostHog
            posthog.capture('htmx_request_cycle', {
                target: event.target.id || 'unknown', 
                url: url,
                method: method,
                duration: duration, // Duration of the request cycle
            });
    
            // Cleanup the data attribute (optional)
            delete event.target.dataset.htmxStartTime;
        }
    });
    
    document.body.addEventListener('htmx:responseError', function (event) {
        // Capture error details
        const method = getRequestMethod(event);
        const url = getRequestUrl(event);
        const status = event.detail.xhr.status || 'unknown';
        const statusText = event.detail.xhr.statusText || 'unknown';
    
        // Track the error with PostHog
        posthog.capture('htmx_request_error', {
            target: event.target.id || 'unknown',
            url: url,
            method: method,
            status: status,
            statusText: statusText,
        });
    });
});

