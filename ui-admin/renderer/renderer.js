const { ipcRenderer } = require('electron');

// Global state
let currentSection = 'dashboard';
let rules = [];
let logs = [];
let profiles = [];
let aiSuggestions = [];
let timeCountdownInterval = null;
let timeData = null;

// Initialize app
document.addEventListener('DOMContentLoaded', async () => {
    console.log('DOM Content Loaded - Initializing app...');
    initializeNavigation();
    initializeEventListeners();
    setupIpcListeners(); // Setup IPC listeners
    displayUserInfo();
    await loadInitialData();
    await loadUserAccountStatus(); // Load user account info
    startAutoRefresh();
    
    // Test modal functionality
    setTimeout(() => {
        console.log('Testing modal elements...');
        const addRuleBtn = document.getElementById('add-rule-btn');
        const addRuleModal = document.getElementById('add-rule-modal');
        const addRuleForm = document.getElementById('add-rule-form');
        
        console.log('Add Rule Button:', addRuleBtn ? 'Found' : 'NOT FOUND');
        console.log('Add Rule Modal:', addRuleModal ? 'Found' : 'NOT FOUND');
        console.log('Add Rule Form:', addRuleForm ? 'Found' : 'NOT FOUND');
        
        if (addRuleBtn && addRuleModal && addRuleForm) {
            console.log('✅ All modal elements found and ready');
        } else {
            console.error('❌ Some modal elements missing');
        }
    }, 1000);
});

function setupIpcListeners() {
    // Listen for login success to refresh UI
    ipcRenderer.on('user-login-success', (event, userData) => {
        console.log('[UI] User login detected:', userData);
        
        // Store user data locally
        window.currentUser = userData;
        
        // Update UI immediately
        if (userData && userData.email) {
            const userStatus = document.getElementById('user-status');
            const userAccount = document.getElementById('user-account');
            const userEmailSpan = document.getElementById('userEmail');
            const userInfoDiv = document.getElementById('userInfo');
            
            if (userStatus) userStatus.textContent = userData.email;
            if (userAccount) userAccount.className = 'user-account logged-in';
            if (userEmailSpan) userEmailSpan.textContent = userData.email;
            if (userInfoDiv) userInfoDiv.style.display = 'block';
        }
        
        // Then refresh full status
        setTimeout(() => {
            loadUserAccountStatus();
            displayUserInfo();
        }, 500);
    });
}

function initializeNavigation() {
    const navItems = document.querySelectorAll('.nav-item');
    const sections = document.querySelectorAll('.content-section');
    
    navItems.forEach(item => {
        item.addEventListener('click', () => {
            const sectionName = item.getAttribute('data-section');
            
            // Update active nav item
            navItems.forEach(nav => nav.classList.remove('active'));
            item.classList.add('active');
            
            // Update active section
            sections.forEach(section => section.classList.remove('active'));
            document.getElementById(sectionName).classList.add('active');
            
            // Update page title
            document.getElementById('page-title').textContent = 
                item.textContent.trim();
            
            currentSection = sectionName;
            loadSectionData(sectionName);
        });
    });
}

function initializeEventListeners() {
    console.log('Initializing event listeners...');
    
    // Refresh button
    const refreshBtn = document.getElementById('refresh-btn');
    if (refreshBtn) {
        refreshBtn.addEventListener('click', () => {
            loadSectionData(currentSection);
            // Force refresh rules to check Firebase sync
            if (currentSection === 'dashboard' || currentSection === 'rules') {
                loadRules();
            }
            // Always refresh user status
            loadUserAccountStatus();
            showNotification('Đã làm mới dữ liệu', 'info');
        });
    }
    
    // System configuration buttons with safe handling
    const configButtons = [
        { id: 'auto-configure-btn', handler: handleAutoConfigureSystem },
        { id: 'restore-system-btn', handler: handleRestoreSystem },
        { id: 'check-status-btn', handler: loadSystemStatus },
        { id: 'fix-dns-btn', handler: handleAutoConfigureSystem },
        { id: 'fix-doh-btn', handler: handleAutoConfigureSystem },
        { id: 'fix-firewall-btn', handler: handleAutoConfigureSystem },
        { id: 'force-firebase-sync-btn', handler: handleForceFirebaseSync },
        { id: 'refresh-firebase-btn', handler: handleForceFirebaseSync }
    ];
    
    configButtons.forEach(({ id, handler }) => {
        const button = document.getElementById(id);
        if (button) {
            button.addEventListener('click', handler);
        }
    });

    // Add rule button with error handling
    const addRuleBtn = document.getElementById('add-rule-btn');
    if (addRuleBtn) {
        addRuleBtn.addEventListener('click', (e) => {
            console.log('Add Rule button clicked');
            e.preventDefault();
            showModal('add-rule-modal');
        });
    } else {
        console.error('Add Rule button not found!');
    }
    
    // Add rule form with error handling
    const addRuleForm = document.getElementById('add-rule-form');
    if (addRuleForm) {
        addRuleForm.addEventListener('submit', handleAddRule);
        console.log('Add rule form event listener attached');
    } else {
        console.error('Add rule form not found!');
    }
    
    // AI suggestion button
    document.getElementById('generate-suggestions').addEventListener('click', handleGenerateSuggestions);
    
    // Add selected AI rules
    document.getElementById('add-selected-rules').addEventListener('click', handleAddSelectedRules);
    
    // Modal close handlers with improved event handling
    document.querySelectorAll('.close, .modal-close').forEach(element => {
        element.addEventListener('click', (e) => {
            e.preventDefault();
            console.log('Close modal clicked');
            closeModals();
        });
    });
    
    // Also close modal when clicking on background
    document.addEventListener('click', (e) => {
        if (e.target.classList.contains('modal')) {
            console.log('Modal background clicked');
            closeModals();
        }
    });
    
    // Close modal with Escape key
    document.addEventListener('keydown', (e) => {
        if (e.key === 'Escape') {
            console.log('Escape key pressed');
            closeModals();
        }
    });
    
    // Settings save
    document.getElementById('save-settings').addEventListener('click', handleSaveSettings);
    
    // Log filter
    document.getElementById('log-filter').addEventListener('change', filterLogs);
}

// System configuration handlers
async function handleAutoConfigureSystem() {
    try {
        const button = document.getElementById('auto-configure-btn');
        button.disabled = true;
        button.innerHTML = '<i class="fas fa-spinner fa-spin"></i> Configuring...';
        
        await apiCall('POST', '/system/configure');
        showNotification('System configured successfully! Please restart browsers.', 'success');
        
        // Reload status after 2 seconds
        setTimeout(loadSystemStatus, 2000);
        
    } catch (error) {
        showNotification('Failed to configure system: ' + error.message, 'error');
    } finally {
        const button = document.getElementById('auto-configure-btn');
        button.disabled = false;
        button.innerHTML = '<i class="fas fa-magic"></i> Auto Configure System';
    }
}

async function handleRestoreSystem() {
    if (!confirm('This will restore original DNS and firewall settings. Continue?')) {
        return;
    }
    
    try {
        await apiCall('POST', '/system/restore');
        showNotification('System settings restored to original state', 'success');
        setTimeout(loadSystemStatus, 2000);
    } catch (error) {
        showNotification('Failed to restore system: ' + error.message, 'error');
    }
}

async function handleForceFirebaseSync() {
    const button = document.getElementById('force-firebase-sync-btn');
    if (!button) return;
    
    const originalHTML = button.innerHTML;
    button.disabled = true;
    button.innerHTML = '<i class="fas fa-spinner fa-spin"></i> Syncing...';
    
    try {
        const result = await apiCall('POST', '/firebase/force-sync');
        if (result.success) {
            showNotification('Firebase sync completed successfully!', 'success');
            // Refresh rules and stats
            await loadRules();
            await loadStats();
        } else {
            showNotification('Firebase sync failed: ' + (result.error || 'Unknown error'), 'error');
        }
    } catch (error) {
        showNotification('Failed to trigger Firebase sync: ' + error.message, 'error');
    } finally {
        button.disabled = false;
        button.innerHTML = originalHTML;
    }
}

async function loadInitialData() {
    await Promise.all([
        loadStats(),
        loadRules(),
        loadLogs(),
        loadProfiles()
    ]);
}

async function loadSectionData(section) {
    switch (section) {
        case 'dashboard':
            await loadStats();
            break;
        case 'rules':
            await loadRules();
            break;
        case 'time-management':
            await loadTimeManagement();
            break;
        case 'logs':
            await loadLogs();
            break;
        case 'profiles':
            await loadProfiles();
            break;
        case 'system-status':  // Thêm case mới
            await loadSystemStatus();
            break;
    }
}

// API calls
async function apiCall(method, endpoint, data = null) {
    try {
        const result = await ipcRenderer.invoke('api-call', method, endpoint, data);
        if (!result.success) {
            throw new Error(result.error);
        }
        return result.data;
    } catch (error) {
        console.error('API call failed:', error);
        showNotification('API Error: ' + error.message, 'error');
        throw error;
    }
}

async function loadSystemStatus() {
    try {
        const status = await apiCall('GET', '/system/status');
        updateSystemStatusUI(status);
    } catch (error) {
        console.error('Failed to load system status:', error);
    }
}

function updateSystemStatusUI(status) {
    // Update DNS status
    const dnsCard = document.getElementById('dns-status');
    const dnsText = document.getElementById('dns-status-text');
    const fixDnsBtn = document.getElementById('fix-dns-btn');
    
    if (status.dns_configured) {
        dnsCard.classList.add('status-ok');
        dnsCard.classList.remove('status-error');
        dnsText.textContent = 'Configured correctly (127.0.0.1)';
        fixDnsBtn.style.display = 'none';
    } else {
        dnsCard.classList.add('status-error');
        dnsCard.classList.remove('status-ok');
        dnsText.textContent = 'Not configured - DNS may bypass filtering';
        fixDnsBtn.style.display = 'inline-block';
    }
    
    // Update DoH status
    const dohCard = document.getElementById('doh-status');
    const dohText = document.getElementById('doh-status-text');
    const fixDohBtn = document.getElementById('fix-doh-btn');
    
    if (status.doh_disabled) {
        dohCard.classList.add('status-ok');
        dohCard.classList.remove('status-error');
        dohText.textContent = 'Disabled in browsers';
        fixDohBtn.style.display = 'none';
    } else {
        dohCard.classList.add('status-error');
        dohCard.classList.remove('status-ok');
        dohText.textContent = 'May be enabled - can bypass DNS filtering';
        fixDohBtn.style.display = 'inline-block';
    }
    
    // Update Firewall status
    const firewallCard = document.getElementById('firewall-status');
    const firewallText = document.getElementById('firewall-status-text');
    const fixFirewallBtn = document.getElementById('fix-firewall-btn');
    
    if (status.firewall_configured) {
        firewallCard.classList.add('status-ok');
        firewallCard.classList.remove('status-error');
        firewallText.textContent = 'Rules configured correctly';
        fixFirewallBtn.style.display = 'none';
    } else {
        firewallCard.classList.add('status-error');
        firewallCard.classList.remove('status-ok');
        firewallText.textContent = 'Rules not configured';
        fixFirewallBtn.style.display = 'inline-block';
    }
    
    // Show overall status notification
    if (status.overall_status) {
        showNotification('System is properly configured for parental control!', 'success');
    } else {
        showNotification('System configuration needs attention', 'warning');
    }
}

// Data loading functions
async function loadStats() {
    try {
        const stats = await apiCall('GET', '/stats');
        
        document.getElementById('total-rules').textContent = stats.total_rules || 0;
        document.getElementById('blocked-today').textContent = stats.blocked_today || 0;
        
        // Also load system status to get Firebase sync count
        try {
            const systemStatus = await fetch('http://127.0.0.1:8081/api/v1/system/status');
            if (systemStatus.ok) {
                const statusData = await systemStatus.json();
                updateDashboardFirebaseStats(statusData);
            }
        } catch (e) {
            console.log('Could not load system status for Firebase stats');
        }
        
        // Update top blocked domains
        const topBlockedList = document.getElementById('top-blocked-list');
        topBlockedList.innerHTML = '';
        
        if (stats.top_blocked && stats.top_blocked.length > 0) {
            stats.top_blocked.forEach(item => {
                const div = document.createElement('div');
                div.className = 'blocked-item';
                div.innerHTML = `
                    <span class="blocked-domain">${item.domain}</span>
                    <span class="blocked-count">${item.count}</span>
                `;
                topBlockedList.appendChild(div);
            });
        } else {
            topBlockedList.innerHTML = '<p>No blocked domains in the last 7 days</p>';
        }
    } catch (error) {
        console.error('Failed to load stats:', error);
    }
}

async function loadRules() {
    try {
        rules = await apiCall('GET', '/rules');
        renderRulesTable();
    } catch (error) {
        console.error('Failed to load rules:', error);
    }
}

async function loadLogs() {
    try {
        logs = await apiCall('GET', '/logs');
        renderLogsTable();
    } catch (error) {
        console.error('Failed to load logs:', error);
    }
}

async function loadProfiles() {
    try {
        profiles = await apiCall('GET', '/profiles');
        renderProfiles();
    } catch (error) {
        console.error('Failed to load profiles:', error);
    }
}

// Rendering functions
function renderRulesTable() {
    const tbody = document.querySelector('#rules-table tbody');
    tbody.innerHTML = '';
    
    rules.forEach(rule => {
        const row = document.createElement('tr');
        
        // Add special styling for Firebase synced rules
        const isFirebaseRule = rule.category === 'firebase-sync';
        if (isFirebaseRule) {
            row.style.backgroundColor = '#e8f5e8';
            row.style.borderLeft = '4px solid #28a745';
        }
        
        const categoryLabel = isFirebaseRule ? 'firebase-sync 📱' : rule.category;
        const reasonText = isFirebaseRule ? 
            `📱 ${rule.reason || 'Synced from Android app'}` : 
            (rule.reason || 'No reason provided');
        
        row.innerHTML = `
            <td>
                ${isFirebaseRule ? '📱 ' : ''}${rule.domain}
                ${isFirebaseRule ? '<small style="color: #28a745; display: block;">From Android</small>' : ''}
            </td>
            <td><span class="category-badge category-${rule.category}">${categoryLabel}</span></td>
            <td>${reasonText}</td>
            <td>${new Date(rule.created_at).toLocaleDateString()}</td>
            <td>
                <div class="action-buttons">
                    ${isFirebaseRule ?
                        `<button class="btn btn-warning btn-sm" onclick="deleteFirebaseRule(${rule.id}, '${rule.domain}')" title="Delete Firebase rule (will be re-synced from Android)">
                            <i class="fas fa-trash"></i> Delete
                        </button>
                        <span class="btn btn-info btn-sm" style="cursor: default; margin-left: 5px;" title="Synced from Android app">
                            <i class="fas fa-mobile-alt"></i> Android
                        </span>` :
                        `<button class="btn btn-danger btn-sm" onclick="deleteRule(${rule.id})" title="Delete rule permanently">
                            <i class="fas fa-trash"></i> Delete
                        </button>`
                    }
                </div>
            </td>
        `;
        tbody.appendChild(row);
    });
    
    // Show total count with Firebase breakdown
    const firebaseRules = rules.filter(rule => rule.category === 'firebase-sync');
    const localRules = rules.filter(rule => rule.category !== 'firebase-sync');
    
    console.log(`📊 Rules displayed: ${localRules.length} local + ${firebaseRules.length} from Android = ${rules.length} total`);
}

function renderLogsTable(filteredLogs = null) {
    const tbody = document.querySelector('#logs-table tbody');
    tbody.innerHTML = '';
    
    const logsToRender = filteredLogs || logs;
    
    logsToRender.forEach(log => {
        const row = document.createElement('tr');
        row.innerHTML = `
            <td>${new Date(log.timestamp).toLocaleString()}</td>
            <td>${log.domain}</td>
            <td>${log.client_ip}</td>
            <td>${log.query_type}</td>
            <td><span class="action-${log.action}">${log.action}</span></td>
        `;
        tbody.appendChild(row);
    });
}

function renderProfiles() {
    const container = document.querySelector('.profiles-grid');
    container.innerHTML = '';
    
    profiles.forEach(profile => {
        const div = document.createElement('div');
        div.className = 'profile-card';
        div.innerHTML = `
            <h3>${profile.name}</h3>
            <p>${profile.description}</p>
            <div class="profile-actions">
                <button class="btn btn-primary btn-sm">Edit</button>
                <button class="btn btn-danger btn-sm">Delete</button>
            </div>
        `;
        container.appendChild(div);
    });
}

// Event handlers
async function handleAddRule(e) {
    e.preventDefault();
    console.log('Add Rule form submitted');
    
    const domainInput = document.getElementById('rule-domain');
    const categoryInput = document.getElementById('rule-category');
    const reasonInput = document.getElementById('rule-reason');
    
    if (!domainInput || !categoryInput || !reasonInput) {
        console.error('Form inputs not found');
        showNotification('Form elements not found', 'error');
        return;
    }
    
    const domain = domainInput.value.trim();
    const category = categoryInput.value;
    const reason = reasonInput.value.trim();
    
    console.log('Form data:', { domain, category, reason });
    
    // Validation
    if (!domain) {
        showNotification('Please enter a domain', 'error');
        return;
    }
    
    if (!reason) {
        showNotification('Please enter a reason', 'error');
        return;
    }
    
    try {
        console.log('Sending API request...');
        await apiCall('POST', '/rules', {
            domain: domain,
            category: category,
            reason: reason,
            profile_id: 1
        });
        
        showNotification('Rule added successfully!', 'success');
        closeModals();
        await loadRules();
        document.getElementById('add-rule-form').reset();
        console.log('Rule added successfully');
    } catch (error) {
        console.error('Failed to add rule:', error);
        showNotification('Failed to add rule: ' + error.message, 'error');
    }
}

async function deleteRule(id) {
    if (!confirm('Are you sure you want to delete this rule?')) {
        return;
    }

    try {
        await apiCall('DELETE', `/rules/${id}`);
        showNotification('Rule deleted successfully!', 'success');
        await loadRules();
    } catch (error) {
        showNotification('Failed to delete rule: ' + error.message, 'error');
    }
}

async function deleteFirebaseRule(id, domain) {
    const confirmMessage = `⚠️ WARNING: You are about to delete a Firebase rule for "${domain}".

This rule was synced from your Android app. If you delete it:
• It will be removed from this PC immediately
• It may be re-added when the Android app syncs again
• To permanently remove it, delete it from the Android app

Are you sure you want to proceed?`;

    if (!confirm(confirmMessage)) {
        return;
    }

    try {
        await apiCall('DELETE', `/rules/${id}`);
        showNotification(`Firebase rule for "${domain}" deleted. Note: It may be re-synced from Android app.`, 'warning');
        await loadRules();
    } catch (error) {
        showNotification('Failed to delete Firebase rule: ' + error.message, 'error');
    }
}

async function handleGenerateSuggestions() {
    const topic = document.getElementById('ai-topic').value;
    const category = document.getElementById('ai-category').value;
    
    if (!topic.trim()) {
        showNotification('Please enter a topic', 'error');
        return;
    }
    
    try {
        const button = document.getElementById('generate-suggestions');
        button.disabled = true;
        button.innerHTML = '<i class="fas fa-spinner fa-spin"></i> Generating...';
        
        const result = await apiCall('POST', '/ai/suggest', {
            topic: topic,
            category: category
        });
        
        aiSuggestions = result.suggestions;
        renderAISuggestions();
        
        document.getElementById('ai-results').style.display = 'block';
        showNotification('AI suggestions generated!', 'success');
        
    } catch (error) {
        showNotification('Failed to generate suggestions: ' + error.message, 'error');
    } finally {
        const button = document.getElementById('generate-suggestions');
        button.disabled = false;
        button.innerHTML = '<i class="fas fa-magic"></i> Generate Suggestions';
    }
}

function renderAISuggestions() {
    const container = document.getElementById('suggestions-list');
    container.innerHTML = '';
    
    aiSuggestions.forEach((suggestion, index) => {
        const div = document.createElement('div');
        div.className = 'suggestion-item';
        div.innerHTML = `
            <div>
                <input type="checkbox" id="suggestion-${index}" data-index="${index}">
                <label for="suggestion-${index}">
                    <strong>${suggestion.domain}</strong> - ${suggestion.reason}
                </label>
            </div>
        `;
        container.appendChild(div);
    });
}

async function handleAddSelectedRules() {
    const checkboxes = document.querySelectorAll('#suggestions-list input[type="checkbox"]:checked');
    
    if (checkboxes.length === 0) {
        showNotification('Please select at least one suggestion', 'error');
        return;
    }
    
    try {
        for (const checkbox of checkboxes) {
            const index = parseInt(checkbox.getAttribute('data-index'));
            const suggestion = aiSuggestions[index];
            
            await apiCall('POST', '/rules', {
                domain: suggestion.domain,
                category: suggestion.category,
                reason: suggestion.reason,
                profile_id: 1
            });
        }
        
        showNotification(`Added ${checkboxes.length} rules successfully!`, 'success');
        await loadRules();
        document.getElementById('ai-results').style.display = 'none';
        document.getElementById('ai-topic').value = '';
        
    } catch (error) {
        showNotification('Failed to add some rules: ' + error.message, 'error');
    }
}

async function handleSaveSettings() {
    // Implementation for saving settings
    showNotification('Settings saved successfully!', 'success');
}

function filterLogs() {
    const filter = document.getElementById('log-filter').value;
    
    if (filter === 'all') {
        renderLogsTable();
    } else {
        const filtered = logs.filter(log => log.action === filter);
        renderLogsTable(filtered);
    }
}

// Utility functions
function showModal(modalId) {
    console.log('Attempting to show modal:', modalId);
    const modal = document.getElementById(modalId);
    if (modal) {
        modal.style.display = 'block';
        modal.style.position = 'fixed';
        modal.style.zIndex = '1000';
        modal.style.left = '0';
        modal.style.top = '0';
        modal.style.width = '100%';
        modal.style.height = '100%';
        modal.style.backgroundColor = 'rgba(0,0,0,0.5)';
        console.log('Modal displayed successfully');
        
        // Clear form if it's the add rule modal
        if (modalId === 'add-rule-modal') {
            const form = document.getElementById('add-rule-form');
            if (form) {
                form.reset();
            }
        }
        
        // Focus on first input
        setTimeout(() => {
            const firstInput = modal.querySelector('input, textarea, select');
            if (firstInput) {
                firstInput.focus();
            }
        }, 100);
    } else {
        console.error('Modal not found:', modalId);
    }
}

function closeModals() {
    document.querySelectorAll('.modal').forEach(modal => {
        modal.style.display = 'none';
    });
}

function showNotification(message, type = 'info') {
    // Create notification element
    const notification = document.createElement('div');
    notification.className = `notification notification-${type}`;
    notification.textContent = message;
    
    // Add styles
    Object.assign(notification.style, {
        position: 'fixed',
        top: '20px',
        right: '20px',
        padding: '12px 20px',
        borderRadius: '8px',
        color: 'white',
        backgroundColor: type === 'success' ? '#34c759' : 
                        type === 'error' ? '#ff3b30' : '#007aff',
        zIndex: '10000',
        boxShadow: '0 4px 12px rgba(0,0,0,0.2)'
    });
    
    document.body.appendChild(notification);
    
    // Auto remove after 3 seconds
    setTimeout(() => {
        if (notification.parentNode) {
            notification.parentNode.removeChild(notification);
        }
    }, 3000);
}

// Display current user info
async function displayUserInfo() {
    try {
        // Get user info from main process
        const user = await ipcRenderer.invoke('get-current-user');
        console.log('[DEBUG] Current user from main:', user);
        
        const userInfoDiv = document.getElementById('userInfo');
        const userEmailSpan = document.getElementById('userEmail');
        
        if (user && user.email) {
            if (userEmailSpan) userEmailSpan.textContent = user.email;
            if (userInfoDiv) userInfoDiv.style.display = 'block';
            
            // Also update the user account status in header
            const userStatus = document.getElementById('user-status');
            if (userStatus) {
                userStatus.textContent = user.email;
                const userAccount = document.getElementById('user-account');
                if (userAccount) {
                    userAccount.className = 'user-account logged-in';
                }
            }
        } else {
            if (userEmailSpan) userEmailSpan.textContent = 'Chưa đăng nhập';
            if (userInfoDiv) userInfoDiv.style.display = 'none';
        }
    } catch (error) {
        console.error('Failed to get user info:', error);
    }
}

// Make functions available globally for main process to call
window.displayUserInfo = displayUserInfo;
window.loadUserAccountStatus = loadUserAccountStatus;

// Real-time updates using Server-Sent Events
let eventSource = null;

function startRealTimeUpdates() {
    // Close existing connection if any
    if (eventSource) {
        eventSource.close();
    }

    // Establish SSE connection for real-time rule updates
    eventSource = new EventSource('/api/v1/events/rules');

    eventSource.onopen = function(event) {
        console.log('📡 Real-time updates connected');
        showNotification('Real-time updates connected', 'success');
    };

    eventSource.onmessage = function(event) {
        try {
            const data = JSON.parse(event.data);
            console.log('📡 Received real-time update:', data);

            if (data.type === 'rules_update') {
                // Update rules in memory
                rules = data.rules;

                // Re-render rules table if we're on the rules page
                if (currentSection === 'rules') {
                    renderRulesTable();
                }

                // Update dashboard stats if we're on dashboard
                if (currentSection === 'dashboard') {
                    loadStats();
                }

                // Show notification for Firebase synced rules
                const firebaseRules = data.rules.filter(rule => rule.category === 'firebase-sync');
                if (firebaseRules.length > 0) {
                    showNotification(`Updated: ${firebaseRules.length} rules synced from Android app`, 'info');
                }
            } else if (data.type === 'connected') {
                console.log('📡 SSE connection established');
            }
        } catch (error) {
            console.error('Error parsing SSE data:', error);
        }
    };

    eventSource.onerror = function(event) {
        console.error('📡 SSE connection error:', event);
        showNotification('Real-time updates disconnected. Retrying...', 'warning');

        // Retry connection after 5 seconds
        setTimeout(() => {
            if (eventSource.readyState === EventSource.CLOSED) {
                startRealTimeUpdates();
            }
        }, 5000);
    };
}

function startAutoRefresh() {
    // Start real-time updates via SSE
    startRealTimeUpdates();

    // Keep minimal polling for non-rules data (stats, logs, user status)
    setInterval(() => {
        if (currentSection === 'dashboard') {
            loadStats();
        } else if (currentSection === 'logs') {
            loadLogs();
        }
        // Always refresh user account status
        loadUserAccountStatus();
    }, 30000); // Reduced frequency since rules are now real-time
}

// Legacy function - now handled by real-time SSE updates
// Keeping for backward compatibility but no longer used
async function checkFirebaseChanges() {
    // This function is deprecated - real-time updates via SSE handle Firebase changes
    console.log('📡 checkFirebaseChanges called but real-time updates are active');
}

// Cleanup function for SSE connection
function cleanupRealTimeUpdates() {
    if (eventSource) {
        eventSource.close();
        eventSource = null;
        console.log('📡 Real-time updates connection closed');
    }
}

// Handle page unload to cleanup SSE connection
window.addEventListener('beforeunload', cleanupRealTimeUpdates);

// Load user account and Firebase status
async function loadUserAccountStatus() {
    console.log('[DEBUG] Loading user account status...');
    try {
        // First try to get auth status from main process
        const authStatus = await ipcRenderer.invoke('get-auth-status');
        console.log('[DEBUG] Auth status from main process:', authStatus);
        
        if (authStatus && authStatus.authenticated) {
            updateUserAccountUI(authStatus);
            
            // Also try to get Firebase sync count from API
            try {
                const response = await fetch('http://127.0.0.1:8081/api/v1/system/status');
                if (response.ok) {
                    const data = await response.json();
                    if (data.firebase_sync_count !== undefined) {
                        // Update sync count
                        const syncStatus = document.getElementById('sync-status');
                        if (syncStatus) {
                            syncStatus.textContent = `Synced: ${data.firebase_sync_count} URLs`;
                        }
                    }
                }
            } catch (e) {
                console.log('[DEBUG] Could not get Firebase sync count');
            }
        } else {
            // Fallback to system status endpoint
            console.log('[DEBUG] Not authenticated locally, trying API...');
            const fallbackResponse = await fetch('http://127.0.0.1:8081/api/v1/system/status');
            if (fallbackResponse.ok) {
                const fallbackData = await fallbackResponse.json();
                console.log('[DEBUG] Fallback data:', fallbackData);
                updateUserAccountUIFallback(fallbackData);
            } else {
                console.log('[DEBUG] Fallback also failed');
                updateUserAccountUIError();
            }
        }
    } catch (error) {
        console.error('[DEBUG] Failed to load user account status:', error);
        updateUserAccountUIError();
    }
}

function updateUserAccountUI(data) {
    console.log('[DEBUG] Updating UI with data:', data);
    
    const userAccount = document.getElementById('user-account');
    const userStatus = document.getElementById('user-status');
    const syncStatus = document.getElementById('sync-status');
    const firebaseSync = document.getElementById('firebase-sync');

    console.log('[DEBUG] UI elements found:', {
        userAccount: !!userAccount,
        userStatus: !!userStatus,
        syncStatus: !!syncStatus,
        firebaseSync: !!firebaseSync
    });

    if (data.authenticated && data.user) {
        console.log('[DEBUG] User is authenticated:', data.user.email);
        userAccount.className = 'user-account logged-in';
        userStatus.textContent = data.user.email;
        
        // Update Firebase sync status
        if (data.firebase && data.firebase.connected) {
            firebaseSync.className = 'firebase-sync connected';
            syncStatus.textContent = `Synced: ${data.firebase.sync_count || 0} URLs`;
        } else {
            firebaseSync.className = 'firebase-sync disconnected';
            syncStatus.textContent = 'Not connected';
        }
    } else {
        console.log('[DEBUG] User is NOT authenticated');
        userAccount.className = 'user-account logged-out';
        userStatus.textContent = 'Not logged in';
        firebaseSync.className = 'firebase-sync disconnected';
        syncStatus.textContent = 'Login required';
    }
}

function updateUserAccountUIFallback(data) {
    const userAccount = document.getElementById('user-account');
    const userStatus = document.getElementById('user-status');
    const syncStatus = document.getElementById('sync-status');
    const firebaseSync = document.getElementById('firebase-sync');

    if (data.auth_status === 'authenticated' && data.user_email) {
        userAccount.className = 'user-account logged-in';
        userStatus.textContent = data.user_email;
        
        // Update Firebase sync status
        if (data.firebase_connected) {
            firebaseSync.className = 'firebase-sync connected';
            syncStatus.textContent = `Synced: ${data.firebase_sync_count || 0} URLs`;
        } else {
            firebaseSync.className = 'firebase-sync disconnected';
            syncStatus.textContent = 'Not connected';
        }
    } else {
        userAccount.className = 'user-account logged-out';
        userStatus.textContent = 'Not logged in';
        firebaseSync.className = 'firebase-sync disconnected';
        syncStatus.textContent = 'Login required';
    }
}

function updateUserAccountUIError() {
    const userAccount = document.getElementById('user-account');
    const userStatus = document.getElementById('user-status');
    const syncStatus = document.getElementById('sync-status');

    userAccount.className = 'user-account logged-out';
    userStatus.textContent = 'Service offline';
    syncStatus.textContent = 'Check connection';
}

// Update dashboard Firebase stat
function updateDashboardFirebaseStats(data) {
    const firebaseStat = document.getElementById('firebase-sync-count');
    if (firebaseStat) {
        if (data.firebase && data.firebase.sync_count !== undefined) {
            firebaseStat.textContent = data.firebase.sync_count;
        } else if (data.firebase_sync_count !== undefined) {
            firebaseStat.textContent = data.firebase_sync_count;
        } else {
            firebaseStat.textContent = '0';
        }
    }
}

// Load system status for the System Status section
async function loadSystemStatus() {
    try {
        const response = await fetch('http://127.0.0.1:8081/api/v1/system/status');
        if (response.ok) {
            const data = await response.json();
            updateSystemStatusUI(data);
        }
    } catch (error) {
        console.error('Failed to load system status:', error);
    }
}

function updateSystemStatusUI(data) {
    // Update hosts file status
    const hostsStatusText = document.getElementById('hosts-status-text');
    const hostsDetails = document.getElementById('hosts-details');
    if (hostsStatusText && hostsDetails) {
        hostsStatusText.textContent = data.hosts_accessible ? 'Working' : 'Error';
        hostsDetails.textContent = `${data.blocked_domains || 0} domains blocked`;
    }

    // Update Firebase status
    const firebaseStatusText = document.getElementById('firebase-status-text');
    const firebaseDetails = document.getElementById('firebase-details');
    if (firebaseStatusText && firebaseDetails) {
        firebaseStatusText.textContent = data.firebase_connected ? 'Connected' : 'Disconnected';
        firebaseDetails.textContent = data.firebase_connected ? 
            `Synced: ${data.firebase_sync_count || 0} URLs` : 
            'Not connected to Firebase';
    }

    // Update auth status
    const authStatusText = document.getElementById('auth-status-text');
    const authDetails = document.getElementById('auth-details');
    if (authStatusText && authDetails) {
        authStatusText.textContent = data.auth_status === 'authenticated' ? 'Logged In' : 'Not Logged In';
        authDetails.textContent = data.auth_status === 'authenticated' ? 
            data.user_email || 'User authenticated' : 
            'Login required for Firebase sync';
    }
}

// Make functions globally available
window.deleteRule = deleteRule;
window.showModal = showModal;
window.closeModals = closeModals;

// Test function for modal (available in console)
window.testModal = function() {
    console.log('Testing Add Rule modal...');
    showModal('add-rule-modal');
};

// ============== TIME MANAGEMENT FUNCTIONS ==============

// Load Time Management data
async function loadTimeManagement() {
    console.log('Loading Time Management data...');
    
    try {
        // Load time status, Android rules, and PC rules in parallel
        const [timeStatus, androidRules, pcRules] = await Promise.all([
            loadTimeStatus(),
            loadAndroidTimeRules(),
            loadPCTimeRules()
        ]);
        
        // Display the data
        displayTimeStatus(timeStatus);
        displayAndroidRules(androidRules);
        displayPCRules(pcRules);
        
        // Start countdown timer
        startTimeCountdown(timeStatus);
        
        // Initialize tab switching
        initializeTimeManagementTabs();
        
        // Initialize time management event listeners
        initializeTimeManagementEvents();
        
    } catch (error) {
        console.error('Failed to load Time Management data:', error);
        showNotification('Failed to load Time Management: ' + error.message, 'error');
    }
}

// Load current time status
async function loadTimeStatus() {
    try {
        const data = await apiCall('GET', '/time/status');
        return data.status;
    } catch (error) {
        console.error('Failed to load time status:', error);
        return null;
    }
}

// Load Android time rules from Firebase
async function loadAndroidTimeRules() {
    try {
        const data = await apiCall('GET', '/time/firebase-rules');
        return data;
    } catch (error) {
        console.error('Failed to load Android time rules:', error);
        return null;
    }
}

// Load PC time rules
async function loadPCTimeRules() {
    try {
        const data = await apiCall('GET', '/time/rules');
        return data.rules;
    } catch (error) {
        console.error('Failed to load PC time rules:', error);
        return null;
    }
}

// Display time status with countdown
function displayTimeStatus(status) {
    if (!status) {
        document.getElementById('network-status').textContent = 'Error';
        document.getElementById('usage-today').textContent = 'Error';
        document.getElementById('break-status').textContent = 'Error';
        return;
    }
    
    timeData = status;
    
    // Network Status
    const networkStatus = status.is_blocked ? 'BLOCKED' : 'AVAILABLE';
    const networkCard = document.getElementById('network-status-card');
    document.getElementById('network-status').textContent = networkStatus;
    document.getElementById('network-reason').textContent = status.reason || '';
    
    if (status.is_blocked) {
        networkCard.className = 'time-stat-card blocked';
    } else {
        networkCard.className = 'time-stat-card available';
    }
    
    // Usage Today with Countdown
    const usageToday = status.today_usage || 0;
    const dailyLimit = status.daily_limit || 0;
    const remainingMinutes = Math.max(0, dailyLimit - usageToday);
    
    updateCountdownDisplay(remainingMinutes);
    
    // Break Status
    const breakStatus = status.is_break_time ? 'BREAK TIME' : 'ACTIVE';
    document.getElementById('break-status').textContent = breakStatus;
    document.getElementById('break-details').textContent = status.is_break_time ? 'Taking mandatory break' : 'Normal usage';
    
    // Current Session
    const sessionDuration = status.session_duration || 0;
    document.getElementById('current-session').textContent = sessionDuration + ' minutes';
    
    // Statistics
    document.getElementById('last-time-update').textContent = new Date().toLocaleTimeString();
    
    // Calculate time until break/limit
    if (status.has_rules && status.current_rule) {
        const rule = status.current_rule;
        if (rule.breakIntervalMinutes > 0) {
            const timeUntilBreak = Math.max(0, rule.breakIntervalMinutes - sessionDuration);
            document.getElementById('time-until-break').textContent = timeUntilBreak + ' minutes';
        }
        
        document.getElementById('time-until-limit').textContent = remainingMinutes + ' minutes';
    }
}

// Update countdown display (real-time countdown)
function updateCountdownDisplay(totalMinutes) {
    const hours = Math.floor(totalMinutes / 60);
    const minutes = totalMinutes % 60;
    
    // Update main usage display
    const usageTodayElement = document.getElementById('usage-today');
    if (usageTodayElement && timeData) {
        const usageToday = timeData.today_usage || 0;
        const dailyLimit = timeData.daily_limit || 0;
        
        // Show countdown format when time is running
        if (!timeData.is_blocked && dailyLimit > 0) {
            usageTodayElement.innerHTML = '<div class="countdown-display"><span class="countdown-time">' + hours + 'h ' + minutes + 'm</span><small>remaining today</small></div>';
        } else {
            usageTodayElement.textContent = usageToday + '/' + dailyLimit + ' min';
        }
    }
    
    document.getElementById('usage-limit').textContent = totalMinutes + ' minutes remaining';
}

// Start real-time countdown timer
function startTimeCountdown(status) {
    if (timeCountdownInterval) {
        clearInterval(timeCountdownInterval);
    }
    
    if (!status || !status.has_rules || status.is_blocked) {
        return;
    }
    
    const dailyLimit = status.daily_limit || 0;
    if (dailyLimit === 0) {
        return;
    }
    
    let remainingMinutes = Math.max(0, dailyLimit - (status.today_usage || 0));
    
    timeCountdownInterval = setInterval(() => {
        if (remainingMinutes > 0 && !timeData?.is_blocked) {
            remainingMinutes -= 1/60; // Decrease by 1 second
            updateCountdownDisplay(Math.floor(remainingMinutes));
            
            document.getElementById('time-until-limit').textContent = Math.floor(remainingMinutes) + ' minutes';
            
            // Change color when getting close to limit
            const usageCard = document.getElementById('usage-today-card');
            if (remainingMinutes <= 10) {
                usageCard.className = 'time-stat-card warning';
            } else if (remainingMinutes <= 30) {
                usageCard.className = 'time-stat-card caution';
            } else {
                usageCard.className = 'time-stat-card';
            }
            
            // Auto refresh every 5 minutes to sync with server
            if (Math.floor(remainingMinutes) % 5 === 0) {
                loadTimeStatus().then(displayTimeStatus);
            }
        } else {
            clearInterval(timeCountdownInterval);
            updateCountdownDisplay(0);
            
            setTimeout(() => {
                loadTimeStatus().then(displayTimeStatus);
            }, 1000);
        }
    }, 1000); // Update every second
}

// Display Android rules
function displayAndroidRules(androidData) {
    const container = document.getElementById('android-rules-container');
    const countElement = document.getElementById('android-rules-count');
    const statusElement = document.getElementById('android-rules-status');
    
    if (!androidData || !androidData.success) {
        container.innerHTML = '<div class="no-rules-message">No Android rules found or Firebase not connected</div>';
        countElement.textContent = '0';
        statusElement.textContent = 'Not connected';
        return;
    }
    
    const activeRules = androidData.active_rules || [];
    countElement.textContent = activeRules.length.toString();
    statusElement.textContent = activeRules.length + ' active rules';
    
    let html = '';
    
    if (activeRules.length === 0) {
        html = '<div class="no-rules-message">No active time rules from Android app</div>';
    } else {
        html = '<div class="android-rules-list">';
        
        activeRules.forEach(rule => {
            html += '<div class="android-rule-card active"><div class="rule-header"><h4>' + rule.name + '</h4><span class="rule-type">' + rule.rule_type + '</span></div><div class="rule-details"><p class="rule-description">' + rule.description + '</p><div class="rule-limits"><div class="limit-item"><i class="fas fa-clock"></i><span>Daily Limit: ' + rule.daily_limit_minutes + ' minutes</span></div></div><div class="rule-meta"><small>Added by: ' + rule.added_by + '</small></div></div></div>';
        });
        
        html += '</div>';
    }
    
    container.innerHTML = html;
}

// Display PC rules with tabs
function displayPCRules(rules) {
    if (!rules) {
        document.getElementById('weekdays-rules').innerHTML = '<div class="no-rules-message">No PC rules available</div>';
        document.getElementById('weekends-rules').innerHTML = '<div class="no-rules-message">No PC rules available</div>';
        return;
    }
    
    displayDayRules(rules.weekdays, 'weekdays-rules');
    displayDayRules(rules.weekends, 'weekends-rules');
}

// Display individual day rules
function displayDayRules(dayRule, containerId) {
    const container = document.getElementById(containerId);
    
    if (!dayRule || !dayRule.enabled) {
        container.innerHTML = '<div class="no-rules-message">No rules enabled for this period</div>';
        return;
    }
    
    let html = '<div class="day-rule-details"><div class="rule-status enabled"><i class="fas fa-check-circle"></i><span>Rules Enabled</span></div><div class="rule-limits-grid"><div class="limit-card"><div class="limit-icon"><i class="fas fa-clock"></i></div><div class="limit-content"><h4>Daily Limit</h4><p>' + (dayRule.dailyLimitMinutes > 0 ? dayRule.dailyLimitMinutes + ' minutes' : 'No limit') + '</p></div></div></div></div>';
    
    container.innerHTML = html;
}

// Initialize tab switching for time management
function initializeTimeManagementTabs() {
    const tabButtons = document.querySelectorAll('.tab-btn');
    const tabContents = document.querySelectorAll('.tab-content');
    
    tabButtons.forEach(button => {
        button.addEventListener('click', () => {
            const targetTab = button.getAttribute('data-tab');
            
            tabButtons.forEach(btn => btn.classList.remove('active'));
            button.classList.add('active');
            
            tabContents.forEach(content => content.classList.remove('active'));
            document.getElementById(targetTab + '-content').classList.add('active');
        });
    });
}

// Initialize time management event listeners
function initializeTimeManagementEvents() {
    const refreshTimeBtn = document.getElementById('refresh-time-btn');
    if (refreshTimeBtn) {
        refreshTimeBtn.addEventListener('click', () => {
            loadTimeManagement();
            showNotification('Time management data refreshed', 'info');
        });
    }
    
    const manualBlockBtn = document.getElementById('manual-block-btn');
    if (manualBlockBtn) {
        manualBlockBtn.addEventListener('click', async () => {
            if (!confirm('Block internet access immediately?')) return;
            
            try {
                await apiCall('POST', '/time/toggle', { action: 'block', reason: 'Manual block by admin' });
                showNotification('Internet access blocked', 'warning');
                setTimeout(() => loadTimeStatus().then(displayTimeStatus), 1000);
            } catch (error) {
                showNotification('Failed to block internet: ' + error.message, 'error');
            }
        });
    }
    
    const manualUnblockBtn = document.getElementById('manual-unblock-btn');
    if (manualUnblockBtn) {
        manualUnblockBtn.addEventListener('click', async () => {
            if (!confirm('Unblock internet access immediately?')) return;
            
            try {
                await apiCall('POST', '/time/toggle', { action: 'unblock', reason: 'Manual unblock by admin' });
                showNotification('Internet access unblocked', 'success');
                setTimeout(() => loadTimeStatus().then(displayTimeStatus), 1000);
            } catch (error) {
                showNotification('Failed to unblock internet: ' + error.message, 'error');
            }
        });
    }
}

function updateUserAccountUIError() {
    const userAccount = document.getElementById('user-account');
    const userStatus = document.getElementById('user-status');
    const syncStatus = document.getElementById('sync-status');

    userAccount.className = 'user-account logged-out';
    userStatus.textContent = 'Service offline';
    syncStatus.textContent = 'Check connection';
}

// Update dashboard Firebase stat
function updateDashboardFirebaseStats(data) {
    const firebaseStat = document.getElementById('firebase-sync-count');
    if (firebaseStat) {
        if (data.firebase && data.firebase.sync_count !== undefined) {
            firebaseStat.textContent = data.firebase.sync_count;
        } else if (data.firebase_sync_count !== undefined) {
            firebaseStat.textContent = data.firebase_sync_count;
        } else {
            firebaseStat.textContent = '0';
        }
    }
}

// Load system status for the System Status section
async function loadSystemStatus() {
    try {
        const response = await fetch('http://127.0.0.1:8081/api/v1/system/status');
        if (response.ok) {
            const data = await response.json();
            updateSystemStatusUI(data);
        }
    } catch (error) {
        console.error('Failed to load system status:', error);
    }
}

function updateSystemStatusUI(data) {
    // Update hosts file status
    const hostsStatusText = document.getElementById('hosts-status-text');
    const hostsDetails = document.getElementById('hosts-details');
    if (hostsStatusText && hostsDetails) {
        hostsStatusText.textContent = data.hosts_accessible ? 'Working' : 'Error';
        hostsDetails.textContent = `${data.blocked_domains || 0} domains blocked`;
    }

    // Update Firebase status
    const firebaseStatusText = document.getElementById('firebase-status-text');
    const firebaseDetails = document.getElementById('firebase-details');
    if (firebaseStatusText && firebaseDetails) {
        firebaseStatusText.textContent = data.firebase_connected ? 'Connected' : 'Disconnected';
        firebaseDetails.textContent = data.firebase_connected ? 
            `Synced: ${data.firebase_sync_count || 0} URLs` : 
            'Not connected to Firebase';
    }

    // Update auth status
    const authStatusText = document.getElementById('auth-status-text');
    const authDetails = document.getElementById('auth-details');
    if (authStatusText && authDetails) {
        authStatusText.textContent = data.auth_status === 'authenticated' ? 'Logged In' : 'Not Logged In';
        authDetails.textContent = data.auth_status === 'authenticated' ? 
            data.user_email || 'User authenticated' : 
            'Login required for Firebase sync';
    }
}

// Make functions globally available
window.deleteRule = deleteRule;
window.showModal = showModal;
window.closeModals = closeModals;

// Test function for modal (available in console)
window.testModal = function() {
    console.log('Testing Add Rule modal...');
    showModal('add-rule-modal');
};

// ============== TIME MANAGEMENT FUNCTIONS ==============

// Load Time Management data
async function loadTimeManagement() {
    console.log('Loading Time Management data...');
    
    try {
        // Load time status, Android rules, and PC rules in parallel
        const [timeStatus, androidRules, pcRules] = await Promise.all([
            loadTimeStatus(),
            loadAndroidTimeRules(),
            loadPCTimeRules()
        ]);
        
        // Display the data
        displayTimeStatus(timeStatus);
        displayAndroidRules(androidRules);
        displayPCRules(pcRules);
        
        // Start countdown timer
        startTimeCountdown(timeStatus);
        
        // Initialize tab switching
        initializeTimeManagementTabs();
        
        // Initialize time management event listeners
        initializeTimeManagementEvents();
        
    } catch (error) {
        console.error('Failed to load Time Management data:', error);
        showNotification('Failed to load Time Management: ' + error.message, 'error');
    }
}

// Load current time status
async function loadTimeStatus() {
    try {
        const data = await apiCall('GET', '/time/status');
        return data.status;
    } catch (error) {
        console.error('Failed to load time status:', error);
        return null;
    }
}

// Load Android time rules from Firebase
async function loadAndroidTimeRules() {
    try {
        const data = await apiCall('GET', '/time/firebase-rules');
        return data;
    } catch (error) {
        console.error('Failed to load Android time rules:', error);
        return null;
    }
}

// Load PC time rules
async function loadPCTimeRules() {
    try {
        const data = await apiCall('GET', '/time/rules');
        return data.rules;
    } catch (error) {
        console.error('Failed to load PC time rules:', error);
        return null;
    }
}

// Display time status with countdown
function displayTimeStatus(status) {
    if (!status) {
        document.getElementById('network-status').textContent = 'Error';
        document.getElementById('usage-today').textContent = 'Error';
        document.getElementById('break-status').textContent = 'Error';
        return;
    }
    
    timeData = status;
    
    // Network Status
    const networkStatus = status.is_blocked ? 'BLOCKED' : 'AVAILABLE';
    const networkCard = document.getElementById('network-status-card');
    document.getElementById('network-status').textContent = networkStatus;
    document.getElementById('network-reason').textContent = status.reason || '';
    
    if (status.is_blocked) {
        networkCard.className = 'time-stat-card blocked';
    } else {
        networkCard.className = 'time-stat-card available';
    }
    
    // Usage Today with Countdown
    const usageToday = status.today_usage || 0;
    const dailyLimit = status.daily_limit || 0;
    const remainingMinutes = Math.max(0, dailyLimit - usageToday);
    
    updateCountdownDisplay(remainingMinutes);
    
    // Break Status
    const breakStatus = status.is_break_time ? 'BREAK TIME' : 'ACTIVE';
    document.getElementById('break-status').textContent = breakStatus;
    document.getElementById('break-details').textContent = status.is_break_time ? 'Taking mandatory break' : 'Normal usage';
    
    // Current Session
    const sessionDuration = status.session_duration || 0;
    document.getElementById('current-session').textContent = sessionDuration + ' minutes';
    
    // Statistics
    document.getElementById('last-time-update').textContent = new Date().toLocaleTimeString();
    
    // Calculate time until break/limit
    if (status.has_rules && status.current_rule) {
        const rule = status.current_rule;
        if (rule.breakIntervalMinutes > 0) {
            const timeUntilBreak = Math.max(0, rule.breakIntervalMinutes - sessionDuration);
            document.getElementById('time-until-break').textContent = timeUntilBreak + ' minutes';
        }
        
        document.getElementById('time-until-limit').textContent = remainingMinutes + ' minutes';
    }
}

// Update countdown display (real-time countdown)
function updateCountdownDisplay(totalMinutes) {
    const hours = Math.floor(totalMinutes / 60);
    const minutes = totalMinutes % 60;
    
    // Update main usage display
    const usageTodayElement = document.getElementById('usage-today');
    if (usageTodayElement && timeData) {
        const usageToday = timeData.today_usage || 0;
        const dailyLimit = timeData.daily_limit || 0;
        
        // Show countdown format when time is running
        if (!timeData.is_blocked && dailyLimit > 0) {
            usageTodayElement.innerHTML = '<div class="countdown-display"><span class="countdown-time">' + hours + 'h ' + minutes + 'm</span><small>remaining today</small></div>';
        } else {
            usageTodayElement.textContent = usageToday + '/' + dailyLimit + ' min';
        }
    }
    
    document.getElementById('usage-limit').textContent = totalMinutes + ' minutes remaining';
}

// Start real-time countdown timer
function startTimeCountdown(status) {
    if (timeCountdownInterval) {
        clearInterval(timeCountdownInterval);
    }
    
    if (!status || !status.has_rules || status.is_blocked) {
        return;
    }
    
    const dailyLimit = status.daily_limit || 0;
    if (dailyLimit === 0) {
        return;
    }
    
    let remainingMinutes = Math.max(0, dailyLimit - (status.today_usage || 0));
    
    timeCountdownInterval = setInterval(() => {
        if (remainingMinutes > 0 && !timeData?.is_blocked) {
            remainingMinutes -= 1/60; // Decrease by 1 second
            updateCountdownDisplay(Math.floor(remainingMinutes));
            
            document.getElementById('time-until-limit').textContent = Math.floor(remainingMinutes) + ' minutes';
            
            // Change color when getting close to limit
            const usageCard = document.getElementById('usage-today-card');
            if (remainingMinutes <= 10) {
                usageCard.className = 'time-stat-card warning';
            } else if (remainingMinutes <= 30) {
                usageCard.className = 'time-stat-card caution';
            } else {
                usageCard.className = 'time-stat-card';
            }
            
            // Auto refresh every 5 minutes to sync with server
            if (Math.floor(remainingMinutes) % 5 === 0) {
                loadTimeStatus().then(displayTimeStatus);
            }
        } else {
            clearInterval(timeCountdownInterval);
            updateCountdownDisplay(0);
            
            setTimeout(() => {
                loadTimeStatus().then(displayTimeStatus);
            }, 1000);
        }
    }, 1000); // Update every second
}

// Display Android rules
function displayAndroidRules(androidData) {
    const container = document.getElementById('android-rules-container');
    const countElement = document.getElementById('android-rules-count');
    const statusElement = document.getElementById('android-rules-status');
    
    if (!androidData || !androidData.success) {
        container.innerHTML = '<div class="no-rules-message">No Android rules found or Firebase not connected</div>';
        countElement.textContent = '0';
        statusElement.textContent = 'Not connected';
        return;
    }
    
    const activeRules = androidData.active_rules || [];
    countElement.textContent = activeRules.length.toString();
    statusElement.textContent = activeRules.length + ' active rules';
    
    let html = '';
    
    if (activeRules.length === 0) {
        html = '<div class="no-rules-message">No active time rules from Android app</div>';
    } else {
        html = '<div class="android-rules-list">';
        
        activeRules.forEach(rule => {
            html += '<div class="android-rule-card active"><div class="rule-header"><h4>' + rule.name + '</h4><span class="rule-type">' + rule.rule_type + '</span></div><div class="rule-details"><p class="rule-description">' + rule.description + '</p><div class="rule-limits"><div class="limit-item"><i class="fas fa-clock"></i><span>Daily Limit: ' + rule.daily_limit_minutes + ' minutes</span></div></div><div class="rule-meta"><small>Added by: ' + rule.added_by + '</small></div></div></div>';
        });
        
        html += '</div>';
    }
    
    container.innerHTML = html;
}

// Display PC rules with tabs
function displayPCRules(rules) {
    if (!rules) {
        document.getElementById('weekdays-rules').innerHTML = '<div class="no-rules-message">No PC rules available</div>';
        document.getElementById('weekends-rules').innerHTML = '<div class="no-rules-message">No PC rules available</div>';
        return;
    }
    
    displayDayRules(rules.weekdays, 'weekdays-rules');
    displayDayRules(rules.weekends, 'weekends-rules');
}

// Display individual day rules
function displayDayRules(dayRule, containerId) {
    const container = document.getElementById(containerId);
    
    if (!dayRule || !dayRule.enabled) {
        container.innerHTML = '<div class="no-rules-message">No rules enabled for this period</div>';
        return;
    }
    
    let html = '<div class="day-rule-details"><div class="rule-status enabled"><i class="fas fa-check-circle"></i><span>Rules Enabled</span></div><div class="rule-limits-grid"><div class="limit-card"><div class="limit-icon"><i class="fas fa-clock"></i></div><div class="limit-content"><h4>Daily Limit</h4><p>' + (dayRule.dailyLimitMinutes > 0 ? dayRule.dailyLimitMinutes + ' minutes' : 'No limit') + '</p></div></div></div></div>';
    
    container.innerHTML = html;
}

// Initialize tab switching for time management
function initializeTimeManagementTabs() {
    const tabButtons = document.querySelectorAll('.tab-btn');
    const tabContents = document.querySelectorAll('.tab-content');
    
    tabButtons.forEach(button => {
        button.addEventListener('click', () => {
            const targetTab = button.getAttribute('data-tab');
            
            tabButtons.forEach(btn => btn.classList.remove('active'));
            button.classList.add('active');
            
            tabContents.forEach(content => content.classList.remove('active'));
            document.getElementById(targetTab + '-content').classList.add('active');
        });
    });
}

// Initialize time management event listeners
function initializeTimeManagementEvents() {
    const refreshTimeBtn = document.getElementById('refresh-time-btn');
    if (refreshTimeBtn) {
        refreshTimeBtn.addEventListener('click', () => {
            loadTimeManagement();
            showNotification('Time management data refreshed', 'info');
        });
    }
    
    const manualBlockBtn = document.getElementById('manual-block-btn');
    if (manualBlockBtn) {
        manualBlockBtn.addEventListener('click', async () => {
            if (!confirm('Block internet access immediately?')) return;
            
            try {
                await apiCall('POST', '/time/toggle', { action: 'block', reason: 'Manual block by admin' });
                showNotification('Internet access blocked', 'warning');
                setTimeout(() => loadTimeStatus().then(displayTimeStatus), 1000);
            } catch (error) {
                showNotification('Failed to block internet: ' + error.message, 'error');
            }
        });
    }
    
    const manualUnblockBtn = document.getElementById('manual-unblock-btn');
    if (manualUnblockBtn) {
        manualUnblockBtn.addEventListener('click', async () => {
            if (!confirm('Unblock internet access immediately?')) return;
            
            try {
                await apiCall('POST', '/time/toggle', { action: 'unblock', reason: 'Manual unblock by admin' });
                showNotification('Internet access unblocked', 'success');
                setTimeout(() => loadTimeStatus().then(displayTimeStatus), 1000);
            } catch (error) {
                showNotification('Failed to unblock internet: ' + error.message, 'error');
            }
        });
    }
}
