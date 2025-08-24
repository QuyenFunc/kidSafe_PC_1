// KidSafe PC Extension Popup Script
document.addEventListener('DOMContentLoaded', function() {
    const loading = document.getElementById('loading');
    const content = document.getElementById('content');
    const blockedCountEl = document.getElementById('blockedCount');
    const lastBlockedEl = document.getElementById('lastBlocked');
    
    // Load stats from storage
    chrome.storage.local.get(['blockedCount', 'lastBlockedSite', 'lastBlockedTime'], function(result) {
        const blockedCount = result.blockedCount || 0;
        const lastBlockedSite = result.lastBlockedSite;
        const lastBlockedTime = result.lastBlockedTime;
        
        // Update UI
        blockedCountEl.textContent = blockedCount;
        
        if (lastBlockedTime) {
            const timeAgo = getTimeAgo(lastBlockedTime);
            lastBlockedEl.textContent = timeAgo;
            lastBlockedEl.title = lastBlockedSite || 'Unknown site';
        } else {
            lastBlockedEl.textContent = 'None';
        }
        
        // Hide loading, show content
        loading.style.display = 'none';
        content.style.display = 'block';
    });
    
    // Button event handlers
    document.getElementById('testBtn').addEventListener('click', function() {
        // Open a test page that should be blocked
        chrome.tabs.create({ url: 'http://127.0.0.1/kidsafe-test' });
        window.close();
    });
    
    document.getElementById('statsBtn').addEventListener('click', function() {
        // Show detailed stats
        showStats();
    });
    
    document.getElementById('helpBtn').addEventListener('click', function() {
        // Show help information
        showHelp();
    });
});

function getTimeAgo(timestamp) {
    const now = Date.now();
    const diff = now - timestamp;
    const minutes = Math.floor(diff / (1000 * 60));
    const hours = Math.floor(diff / (1000 * 60 * 60));
    const days = Math.floor(diff / (1000 * 60 * 60 * 24));
    
    if (minutes < 1) {
        return 'Just now';
    } else if (minutes < 60) {
        return `${minutes} min ago`;
    } else if (hours < 24) {
        return `${hours} hours ago`;
    } else {
        return `${days} days ago`;
    }
}

function showStats() {
    chrome.storage.local.get(null, function(result) {
        const stats = {
            'Total pages blocked': result.blockedCount || 0,
            'Install date': result.installDate ? new Date(result.installDate).toLocaleDateString('en-US') : 'Unknown',
            'Last blocked site': result.lastBlockedSite || 'None',
            'Last blocked time': result.lastBlockedTime ? new Date(result.lastBlockedTime).toLocaleString('en-US') : 'None'
        };
        
        let message = 'KidSafe PC Extension Statistics:\\n\\n';
        for (const [key, value] of Object.entries(stats)) {
            message += `${key}: ${value}\\n`;
        }
        
        alert(message);
    });
}

function showHelp() {
    const helpText = `
KidSafe PC Extension - Help

🛡️ Functions:
- Automatically detects websites blocked by KidSafe PC
- Shows friendly messages for children
- Statistics on number of pages blocked

🔧 How it works:
- Extension works together with KidSafe PC application
- When hosts file blocks a website, extension shows beautiful notification page
- No additional configuration needed

❓ Support:
- Make sure KidSafe PC application is running
- Extension needs access to all websites to work
- Restart browser if you encounter issues

💡 Tips:
- Check statistics to see protection effectiveness
- Extension works on all tabs
- Notification page helps children understand why website is blocked
    `;
    
    alert(helpText);
}
