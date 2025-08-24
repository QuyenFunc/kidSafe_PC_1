// KidSafe PC Extension Background Script
console.log('KidSafe PC Extension: Background script loaded');

// Extension installation
chrome.runtime.onInstalled.addListener((details) => {
  if (details.reason === 'install') {
    console.log('KidSafe PC Extension installed');
    
    // Set default settings
    chrome.storage.local.set({
      extensionEnabled: true,
      blockedCount: 0,
      installDate: Date.now()
    });
  }
});

// Listen for navigation events to detect blocked pages
chrome.webNavigation.onErrorOccurred.addListener((details) => {
  console.log('🛡️ KidSafe PC: Navigation error detected:', details);
  // Ignore subframes (ads, trackers, widgets). Only act on top-level frame
  if (details.frameId !== 0) {
    console.log('🛡️ KidSafe PC: Ignoring subframe error for url:', details.url);
    return;
  }
  
  // Check if this looks like a hosts file block or security error
  if (details.error && (
    details.error.includes('ERR_CONNECTION_REFUSED') ||
    details.error.includes('ERR_NAME_NOT_RESOLVED') ||
    details.error.includes('ERR_CONNECTION_TIMED_OUT') ||
    details.error.includes('ERR_CERT_AUTHORITY_INVALID')
  )) {
    console.log('🛡️ KidSafe PC: Blocked navigation detected:', details);
    
    // Redirect the tab to our internal blocked page (works even on chrome-error pages)
    chrome.tabs.update(details.tabId, {
      url: chrome.runtime.getURL(`blocked.html?url=${encodeURIComponent(details.url)}`)
    });
    
    // Increment blocked count
    chrome.storage.local.get(['blockedCount'], (result) => {
      const newCount = (result.blockedCount || 0) + 1;
      chrome.storage.local.set({ blockedCount: newCount });
    });
  }
});

// Function to show blocked page (will be injected)
function showKidSafeBlockedPage(originalUrl) {
  try {
    const domain = new URL(originalUrl || window.location.href).hostname;
    
    // Use document.write to bypass TrustedHTML restrictions
    document.open();
    document.write(`
    <!DOCTYPE html>
    <html lang="en">
    <head>
        <meta charset="UTF-8">
        <meta name="viewport" content="width=device-width, initial-scale=1.0">
        <title>KidSafe PC - Website Protected</title>
        <style>
            body {
                font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif;
                background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
                min-height: 100vh;
                display: flex;
                align-items: center;
                justify-content: center;
                margin: 0;
                padding: 20px;
            }
            .container {
                background: white;
                border-radius: 20px;
                padding: 40px;
                max-width: 600px;
                width: 100%;
                text-align: center;
                box-shadow: 0 20px 40px rgba(0, 0, 0, 0.1);
            }
            .shield-icon {
                font-size: 60px;
                margin-bottom: 20px;
            }
            h1 {
                color: #333;
                margin-bottom: 20px;
                font-size: 28px;
            }
            .cute-animals {
                font-size: 24px;
                margin: 20px 0;
            }
            .message {
                color: #666;
                font-size: 18px;
                line-height: 1.6;
                margin-bottom: 30px;
            }
            .domain {
                background: #f8f9fa;
                padding: 15px;
                border-radius: 10px;
                margin: 20px 0;
                font-family: monospace;
                font-size: 16px;
                color: #e74c3c;
                border-left: 4px solid #e74c3c;
            }
            .back-button {
                background: linear-gradient(45deg, #667eea, #764ba2);
                color: white;
                border: none;
                padding: 15px 30px;
                border-radius: 25px;
                font-size: 16px;
                font-weight: 600;
                cursor: pointer;
                margin-top: 20px;
            }
            .back-button:hover {
                transform: translateY(-2px);
                box-shadow: 0 10px 20px rgba(102, 126, 234, 0.3);
            }
        </style>
    </head>
    <body>
        <div class="container">
            <div class="shield-icon">🛡️</div>
            <h1>This Website is Protected</h1>
            <div class="cute-animals">🐰 🐯 🐼 🦄</div>
            <div class="message">
                Hello little friend! This website is not suitable for you and has been protected by your parents to keep you safe.
            </div>
            <div class="domain">${domain}</div>
            <button class="back-button" onclick="history.back()">
                ← Go back to safe page
            </button>
            <div style="margin-top: 30px; color: #999; font-size: 14px;">
                🏠 Protected by KidSafe PC<br>
                <small>Caring for and protecting our children</small>
            </div>
        </div>
    </body>
    </html>
    `);
    document.close();
    
    console.log('🛡️ KidSafe PC: Blocked page displayed for:', domain);
  } catch (error) {
    console.log('🛡️ KidSafe PC: Error showing blocked page:', error);
  }
}

// Listen for tab updates
chrome.tabs.onUpdated.addListener((tabId, changeInfo, tab) => {
  if (changeInfo.status === 'complete' && tab.url) {
    // Check if the URL indicates a potential block (localhost, 127.0.0.1)
    const url = new URL(tab.url);
    if (url.hostname === '127.0.0.1' || url.hostname === 'localhost') {
      console.log('KidSafe PC: Tab loaded blocked content:', tab.url);
      
      // Inject our content script if needed
      chrome.scripting.executeScript({
        target: { tabId: tabId },
        files: ['content.js']
      }).catch((error) => {
        // Ignore errors for chrome:// pages etc.
        console.log('Script injection failed (normal for some pages):', error);
      });
    }
  }
});

// Handle messages from content scripts
chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
  if (message.type === 'pageBlocked') {
    console.log('KidSafe PC: Page blocked notification received:', message);
    
    // Update blocked count
    chrome.storage.local.get(['blockedCount'], (result) => {
      const newCount = (result.blockedCount || 0) + 1;
      chrome.storage.local.set({ 
        blockedCount: newCount,
        lastBlockedSite: message.domain,
        lastBlockedTime: Date.now()
      });
    });
    
    sendResponse({ success: true });
  }
  
  if (message.type === 'getStats') {
    chrome.storage.local.get(['blockedCount', 'lastBlockedSite', 'lastBlockedTime'], (result) => {
      sendResponse(result);
    });
    return true; // Keep message channel open for async response
  }
});

// Update badge text with blocked count
function updateBadge() {
  chrome.storage.local.get(['blockedCount'], (result) => {
    const count = result.blockedCount || 0;
    if (count > 0) {
      chrome.action.setBadgeText({ text: count.toString() });
      chrome.action.setBadgeBackgroundColor({ color: '#4CAF50' });
    }
  });
}

// Update badge periodically
setInterval(updateBadge, 5000);
updateBadge(); // Initial update
