// KidSafe PC Content Script - Detect blocked pages
console.log('🛡️ KidSafe PC Extension: Content script loaded on:', window.location.href);
console.log('🛡️ KidSafe PC Extension: Document ready state:', document.readyState);

// List of indicators that a page is blocked by hosts file
const BLOCKED_INDICATORS = [
  // Page title indicators
  () => document.title.includes('This site can\'t be reached'),
  () => document.title.includes('Trang web này không thể truy cập'),
  () => document.title.includes('ERR_CONNECTION_REFUSED'),
  () => document.title.includes('ERR_NAME_NOT_RESOLVED'),
  () => document.title.includes('Không bảo mật'),
  () => document.title.includes('NET::ERR_CERT_AUTHORITY_INVALID'),
  
  // URL indicators  
  () => window.location.hostname === '127.0.0.1',
  () => window.location.hostname === 'localhost',
  
  // Page content indicators
  () => document.body && document.body.textContent.includes('ERR_CONNECTION_REFUSED'),
  () => document.body && document.body.textContent.includes('This site can\'t be reached'),
  () => document.body && document.body.textContent.includes('Check if there is a typo'),
  () => document.body && document.body.textContent.includes('DNS_PROBE_FINISHED_NXDOMAIN'),
  () => document.body && document.body.textContent.includes('NET::ERR_CERT_AUTHORITY_INVALID'),
  () => document.body && document.body.textContent.includes('Kết nối của bạn không phải là kết nối riêng tư'),
  () => document.body && document.body.textContent.includes('Những kẻ tấn công có thể đang cố gắng đánh cắp thông tin'),
  
  // Chrome error page specific
  () => document.querySelector('.error-code') && document.querySelector('.error-code').textContent.includes('ERR_'),
  () => document.querySelector('#main-message') && document.querySelector('#main-message').textContent.includes('site can\'t be reached'),
  () => document.querySelector('#main-message') && document.querySelector('#main-message').textContent.includes('riêng tư'),
  
  // Check for specific blocked domains (from your KidSafe PC app)
  () => {
    const blockedDomains = ['jeff.vn', 'www.jeff.vn', 'vnexpress.net', 'www.vnexpress.net'];
    return blockedDomains.some(domain => window.location.hostname.includes(domain));
  }
];

// Check if current page appears to be blocked
function isPageBlocked() {
  // Skip for extension pages, chrome pages, etc.
  if (window.location.protocol === 'chrome-extension:' || 
      window.location.protocol === 'chrome:' ||
      window.location.protocol === 'edge:' ||
      window.location.protocol === 'about:') {
    return false;
  }
  
  return BLOCKED_INDICATORS.some(indicator => {
    try {
      return indicator();
    } catch (e) {
      return false;
    }
  });
}

// Show the KidSafe blocked page
function showBlockedPage() {
  const originalUrl = window.location.href;
  const domain = window.location.hostname || 'this website';
  
  // Create the blocked page HTML
  const blockedPageHTML = `
    <!DOCTYPE html>
    <html lang="en">
    <head>
        <meta charset="UTF-8">
        <meta name="viewport" content="width=device-width, initial-scale=1.0">
        <title>KidSafe PC - Website Protected</title>
        <style>
            * {
                margin: 0;
                padding: 0;
                box-sizing: border-box;
            }
            
            body {
                font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif;
                background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
                min-height: 100vh;
                display: flex;
                align-items: center;
                justify-content: center;
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
                animation: slideIn 0.5s ease-out;
            }
            
            @keyframes slideIn {
                from {
                    opacity: 0;
                    transform: translateY(30px);
                }
                to {
                    opacity: 1;
                    transform: translateY(0);
                }
            }
            
            .shield-icon {
                width: 80px;
                height: 80px;
                margin: 0 auto 30px;
                background: linear-gradient(45deg, #4CAF50, #45a049);
                border-radius: 50%;
                display: flex;
                align-items: center;
                justify-content: center;
                font-size: 40px;
                color: white;
                animation: pulse 2s infinite;
            }
            
            @keyframes pulse {
                0%, 100% { transform: scale(1); }
                50% { transform: scale(1.05); }
            }
            
            h1 {
                color: #333;
                margin-bottom: 20px;
                font-size: 28px;
                font-weight: 600;
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
            
            .info-box {
                background: #e8f5e8;
                border: 1px solid #4CAF50;
                border-radius: 10px;
                padding: 20px;
                margin: 20px 0;
                text-align: left;
            }
            
            .info-box h3 {
                color: #4CAF50;
                margin-bottom: 10px;
                font-size: 18px;
            }
            
            .info-box ul {
                color: #666;
                line-height: 1.8;
                margin-left: 20px;
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
                transition: all 0.3s ease;
                margin-top: 20px;
            }
            
            .back-button:hover {
                transform: translateY(-2px);
                box-shadow: 0 10px 20px rgba(102, 126, 234, 0.3);
            }
            
            .kidsafe-logo {
                margin-top: 30px;
                color: #999;
                font-size: 14px;
            }
            
            .cute-animals {
                font-size: 24px;
                margin: 20px 0;
                animation: bounce 1s infinite alternate;
            }
            
            @keyframes bounce {
                from { transform: translateY(0px); }
                to { transform: translateY(-5px); }
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
            
            <div class="info-box">
                <h3>🌟 Why is this page blocked?</h3>
                <ul>
                    <li>Your parents want to protect you from inappropriate content</li>
                    <li>To give you time for healthy learning and play</li>
                    <li>To help you focus on positive and beneficial things</li>
                </ul>
            </div>
            
            <div class="info-box">
                <h3>🎮 What can you do instead?</h3>
                <ul>
                    <li>Read books or comics</li>
                    <li>Play educational games</li>
                    <li>Watch educational videos on YouTube Kids</li>
                    <li>Draw, color or do crafts</li>
                    <li>Play with friends and family</li>
                </ul>
            </div>
            
            <button class="back-button" onclick="goBack()">
                ← Go back to safe page
            </button>
            
            <div class="kidsafe-logo">
                🏠 Protected by KidSafe PC<br>
                <small>Caring for and protecting our children</small>
            </div>
        </div>
        
        <script>
            function goBack() {
                // Try to go back in history
                if (window.history.length > 1) {
                    window.history.back();
                } else {
                    // If no history, go to a safe page
                    window.location.href = 'about:blank';
                }
            }
            
            // Prevent going forward to the blocked site
            window.addEventListener('popstate', function(event) {
                if (window.location.href.includes('${originalUrl}')) {
                    goBack();
                }
            });
        </script>
    </body>
    </html>
  `;
  
  // Replace the current page content
  document.open();
  document.write(blockedPageHTML);
  document.close();
  
  console.log('KidSafe PC: Blocked page displayed for:', domain);
}

// Main detection logic
function checkAndBlock() {
  console.log('KidSafe PC: Checking page:', window.location.href);
  console.log('KidSafe PC: Page title:', document.title);
  console.log('KidSafe PC: Hostname:', window.location.hostname);
  
  if (isPageBlocked()) {
    console.log('KidSafe PC: Blocked page detected, showing friendly message');
    // Small delay to ensure page is fully loaded
    setTimeout(showBlockedPage, 100);
  } else {
    console.log('KidSafe PC: Page not detected as blocked');
  }
}

// Initial check
if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', checkAndBlock);
} else {
  checkAndBlock();
}

// Also check after a short delay in case page content changes
setTimeout(checkAndBlock, 1000);

// Monitor for dynamic content changes
const observer = new MutationObserver((mutations) => {
  let shouldCheck = false;
  mutations.forEach((mutation) => {
    if (mutation.type === 'childList' && mutation.addedNodes.length > 0) {
      shouldCheck = true;
    }
  });
  
  if (shouldCheck) {
    setTimeout(checkAndBlock, 500);
  }
});

// Start observing
if (document.body) {
  observer.observe(document.body, {
    childList: true,
    subtree: true
  });
} else {
  document.addEventListener('DOMContentLoaded', () => {
    observer.observe(document.body, {
      childList: true,
      subtree: true
    });
  });
}
