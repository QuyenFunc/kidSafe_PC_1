const { app, BrowserWindow, Menu, ipcMain, dialog } = require('electron');
const path = require('path');
const fs = require('fs');
const net = require('net');
const { spawn, exec } = require('child_process');
const axios = require('axios');

// Global variables
let mainWindow;
let loginWindow;
let coreProcess = null;
let isStartingService = false;
let serviceStartTime = null;
let isAuthenticated = false;
let currentUser = null;
const API_BASE_URL = 'http://127.0.0.1:8081/api/v1';

// Utility functions
function isPortAvailable(port) {
    return new Promise((resolve) => {
        const server = net.createServer();
        server.listen(port, () => {
            server.close(() => resolve(true));
        });
        server.on('error', () => resolve(false));
    });
}

function checkAdminRights() {
    return new Promise((resolve) => {
        exec('net session', (error, stdout, stderr) => {
            resolve(!error);
        });
    });
}

async function requestAdminRights() {
    try {
        if (process.platform === 'win32') {
            const isAdmin = await checkAdminRights();
            if (!isAdmin) {
                dialog.showErrorBox(
                    'Administrator Rights Required',
                    'This application requires administrator privileges. Please restart as administrator.'
                );
                app.quit();
                return false;
            }
        }
        return true;
    } catch (error) {
        console.error('Error checking admin rights:', error);
        return false;
    }
}

// Kill existing processes function
function killExistingProcesses() {
    return new Promise((resolve) => {
        exec('tasklist /fi "imagename eq parental-control-core.exe" /fo csv', (error, stdout) => {
            if (error || !stdout) {
                resolve();
                return;
            }

            const lines = stdout.split('\n');
            const pids = [];

            for (let i = 1; i < lines.length; i++) {
                const line = lines[i].trim();
                if (line) {
                    const parts = line.split(',');
                    if (parts.length > 1) {
                        const pid = parts[1].replace(/"/g, '');
                        pids.push(pid);
                    }
                }
            }

            if (pids.length === 0) {
                resolve();
                return;
            }

            console.log(`Killing ${pids.length} existing processes:`, pids);
            const killCommands = pids.map(pid => `taskkill /PID ${pid} /F`);

            exec(killCommands.join(' && '), (err) => {
                setTimeout(resolve, 1000); // Wait 1 second after killing
            });
        });
    });
}

// FIXED Core service management
async function startCoreService() {
    // Strict prevent multiple calls
    if (isStartingService) {
        console.log('Service startup already in progress, aborting...');
        return;
    }

    // Check if service started recently (within 10 seconds)
    if (serviceStartTime && (Date.now() - serviceStartTime < 10000)) {
        console.log('Service started recently, skipping...');
        return;
    }

    isStartingService = true;
    serviceStartTime = Date.now();

    try {
        console.log('Killing any existing processes...');
        await killExistingProcesses();

        console.log('Checking port availability...');
        const apiPortAvailable = await isPortAvailable(8081);
        if (!apiPortAvailable) {
            console.log('Port 8081 still busy after killing processes, waiting...');
            await new Promise(resolve => setTimeout(resolve, 2000));
        }

        startActualService();

    } catch (error) {
        console.error('Error in startCoreService:', error);
        isStartingService = false;
    }
}

function startActualService() {
    try {
        console.log('Starting core service...');
        
        const exePath = path.join(__dirname, 'parental-control-core.exe');
        
        if (!fs.existsSync(exePath)) {
            console.error(`Core service executable not found at: ${exePath}`);
            isStartingService = false;
            return;
        }
        
        // Set environment variables for Electron mode
        const env = { ...process.env };
        env.KIDSAFE_ELECTRON_MODE = 'true';
        env.KIDSAFE_USE_REAL_AUTH = 'true'; // Enable Firebase auth for UI login
        
        // Set working directory to core-service for proper config file access
        const coreServiceDir = path.join(__dirname, '..', 'core-service');
        
        coreProcess = spawn(exePath, [], {
            cwd: coreServiceDir, // Run from core-service directory
            stdio: ['ignore', 'pipe', 'pipe'],
            windowsHide: true,
            detached: false,
            env: env
        });

        let apiServerReady = false;
        let dnsServerReady = false;

        coreProcess.stdout.on('data', (data) => {
            const output = data.toString().trim();
            console.log(`Core service: ${output}`);
            
            // Watch for specific ready signals
            if (output.includes('API server confirmed ready')) {
                console.log('✅ API server is ready for connections');
                apiServerReady = true;
            }
            
            if (output.includes('DNS server confirmed ready')) {
                console.log('✅ DNS server is ready for connections');  
                dnsServerReady = true;
            }
            
            // Both servers ready - safe to mark service as started
            if (apiServerReady && dnsServerReady && isStartingService) {
                console.log('🎉 Both servers confirmed ready - service fully operational');
                isStartingService = false;
            }
        });

        coreProcess.stderr.on('data', (data) => {
            console.error(`Core service error: ${data.toString().trim()}`);
        });

        coreProcess.on('error', (error) => {
            console.error('Core process error:', error);
            coreProcess = null;
            isStartingService = false;
            serviceStartTime = null;
        });

        coreProcess.on('exit', (code, signal) => {
            console.log(`Core service exited with code ${code}, signal ${signal}`);
            coreProcess = null;
            isStartingService = false;
            serviceStartTime = null;

            if (signal !== 'SIGTERM' && signal !== 'SIGKILL' && code !== 0) {
                console.log('Service crashed, will restart in 5 seconds...');
                setTimeout(() => {
                    if (!coreProcess && !isStartingService) {
                        startCoreService();
                    }
                }, 5000);
            }
        });

        // Don't mark as started immediately - wait for server ready signals
        console.log('Core service process spawned, waiting for server ready signals...');
        
    } catch (error) {
        console.error('Error starting service:', error);
        coreProcess = null;
        isStartingService = false;
        serviceStartTime = null;
    }
}


// Enhanced cleanup function
function cleanup() {
    if (coreProcess && !coreProcess.killed) {
        console.log('Stopping core service...');
        coreProcess.kill('SIGTERM');

        // Force kill after 3 seconds if not terminated
        setTimeout(() => {
            if (coreProcess && !coreProcess.killed) {
                console.log('Force killing core service...');
                coreProcess.kill('SIGKILL');
            }
        }, 3000);
    }

    // Kill any remaining processes
    exec('taskkill /IM "parental-control-core.exe" /F /T 2>nul', () => {});
}

// Electron app functions
function createLoginWindow() {
    if (loginWindow && !loginWindow.isDestroyed()) {
        loginWindow.show();
        loginWindow.focus();
        return;
    }
    loginWindow = new BrowserWindow({
        width: 500,
        height: 700,
        webPreferences: {
            nodeIntegration: true,
            contextIsolation: false
        },
        icon: path.join(__dirname, 'assets', 'icon.png'),
        title: 'KidSafe PC - Đăng Nhập',
        resizable: false,
        center: true,
        frame: true,
        show: false
    });

    loginWindow.loadFile('login.html');
    
    loginWindow.once('ready-to-show', () => {
        loginWindow.show();
    });

    loginWindow.on('closed', () => {
        loginWindow = null;
        if (!isAuthenticated) {
            app.quit();
        }
    });
}

function createMainWindow() {
    if (mainWindow && !mainWindow.isDestroyed()) {
        mainWindow.show();
        mainWindow.focus();
        return;
    }
    mainWindow = new BrowserWindow({
        width: 1200,
        height: 800,
        webPreferences: {
            nodeIntegration: true,
            contextIsolation: false
        },
        icon: path.join(__dirname, 'assets', 'icon.png'),
        title: 'KidSafe PC - Parental Control Admin Panel',
        show: false
    });

    // Prefer the renderer UI
    mainWindow.loadFile('renderer/index.html');

    // Remove menu bar in production
    Menu.setApplicationMenu(null);

    mainWindow.on('closed', () => {
        mainWindow = null;
        cleanup();
        app.quit();
    });

    mainWindow.once('ready-to-show', () => {
        mainWindow.show();
        // Send current user info to renderer
        if (currentUser) {
            mainWindow.webContents.send('user-login-success', currentUser);
        }
    });
}

function createWindow() {
    if (!isAuthenticated) {
        createLoginWindow();
    } else {
        createMainWindow();
    }
}

// Firebase Authentication Handlers
function setupFirebaseAuthHandlers() {
    // Handle generic API calls
    ipcMain.handle('api-call', async (event, method, endpoint, data = null) => {
        try {
            const url = `${API_BASE_URL}${endpoint}`;
            console.log(`🌐 API Call: ${method} ${url}`, data ? data : '');
            
            const config = {
                method: method,
                url: url,
                headers: {
                    'Content-Type': 'application/json',
                },
                timeout: 10000,
            };

            if (data && (method === 'POST' || method === 'PUT' || method === 'PATCH')) {
                config.data = data;
            }

            const response = await axios(config);
            console.log(`✅ API Response: ${response.status}`, response.data);
            
            return {
                success: true,
                data: response.data
            };
        } catch (error) {
            console.error(`❌ API Error: ${method} ${endpoint}`, error);
            return {
                success: false,
                error: error.response?.data?.message || error.message || 'API call failed'
            };
        }
    });

    // Handle Firebase login
    ipcMain.handle('firebase-login', async (event, { email, password }) => {
        try {
            console.log(`🔐 Attempting Firebase login for: ${email}`);
            
            // Call PC service's Firebase auth API
            const response = await axios.post(`${API_BASE_URL}/auth/login`, {
                email,
                password
            });
            
            if (response.data.success) {
                currentUser = {
                    email,
                    uid: response.data.uid
                };
                isAuthenticated = true;
                
                console.log('✅ Firebase login successful');
                return { success: true, uid: response.data.uid };
            } else {
                return { success: false, error: response.data.error };
            }
        } catch (error) {
            console.error('❌ Firebase login error:', error.message);
            return { 
                success: false, 
                error: error.response?.data?.error || error.message 
            };
        }
    });

    // Save user credentials
    ipcMain.handle('save-user-credentials', async (event, { email, password }) => {
        try {
            const credentialsPath = path.join(__dirname, 'user_credentials.json');
            const credentials = { email, password };
            fs.writeFileSync(credentialsPath, JSON.stringify(credentials, null, 2));
            return { success: true };
        } catch (error) {
            console.error('Failed to save credentials:', error);
            return { success: false, error: error.message };
        }
    });

    // Get saved email
    ipcMain.handle('get-saved-email', async () => {
        try {
            const credentialsPath = path.join(__dirname, 'user_credentials.json');
            if (fs.existsSync(credentialsPath)) {
                const credentials = JSON.parse(fs.readFileSync(credentialsPath, 'utf8'));
                return credentials.email || '';
            }
        } catch (error) {
            console.error('Failed to load saved email:', error);
        }
        return '';
    });

    // Handle login success
    ipcMain.on('login-success', (event, userData) => {
        currentUser = userData;
        isAuthenticated = true;
        
        console.log('[LOGIN] User logged in:', userData.email);
        
        // Close login window and create main window
        if (loginWindow) {
            loginWindow.close();
            loginWindow = null;
        }
        
        createMainWindow();
        
        // Force refresh user status multiple times to ensure it updates
        setTimeout(() => {
            if (mainWindow && !mainWindow.isDestroyed()) {
                console.log('[DEBUG] Sending user-login-success to main window');
                mainWindow.webContents.send('user-login-success', userData);
                // Also send via IPC for get-current-user
                mainWindow.webContents.executeJavaScript(`
                    window.currentUser = ${JSON.stringify(userData)};
                    if (window.displayUserInfo) window.displayUserInfo();
                    if (window.loadUserAccountStatus) window.loadUserAccountStatus();
                `);
            }
        }, 1000);
        
        setTimeout(() => {
            if (mainWindow && !mainWindow.isDestroyed()) {
                console.log('[DEBUG] Sending second refresh signal');
                mainWindow.webContents.send('user-login-success', userData);
            }
        }, 3000);
    });

    // Show help
    ipcMain.on('show-help', () => {
        // You can implement a help dialog here
        dialog.showMessageBox({
            type: 'info',
            title: 'Hướng dẫn setup Firebase',
            message: 'Thiết lập Firebase cho KidSafe PC',
            detail: `
1. Tạo project Firebase tại console.firebase.google.com
2. Bật Authentication với Email/Password
3. Tạo Realtime Database
4. Tải Service Account Key và đặt tên firebase-credentials.json
5. Lấy Web API Key và tạo file firebase-config.json

Cần hỗ trợ thêm? Liên hệ developer.
            `,
            buttons: ['OK']
        });
    });


    
    // Add handler for auth status check
    ipcMain.handle('get-auth-status', async () => {
        try {
            // First check local auth state
            if (currentUser && isAuthenticated) {
                return {
                    authenticated: true,
                    user: currentUser,
                    firebase: {
                        connected: true,
                        sync_count: 0 // Will be updated from API
                    }
                };
            }
            
            // Try to get status from core service
            try {
                const response = await axios.get(`${API_BASE_URL}/system/status`);
                if (response.data) {
                    return {
                        authenticated: response.data.auth_status === 'authenticated',
                        user: response.data.user_email ? {
                            email: response.data.user_email,
                            uid: response.data.user_uid
                        } : null,
                        firebase: {
                            connected: response.data.firebase_connected || false,
                            sync_count: response.data.firebase_sync_count || 0
                        }
                    };
                }
            } catch (apiError) {
                console.log('API status check failed:', apiError.message);
            }
            
            return {
                authenticated: false,
                user: null,
                firebase: {
                    connected: false,
                    sync_count: 0
                }
            };
        } catch (error) {
            console.error('Get auth status error:', error);
            return {
                authenticated: false,
                user: null,
                firebase: {
                    connected: false,
                    sync_count: 0
                }
            };
        }
    });
}

// Initialize app when ready
// Check if app is available before using it
if (typeof app !== 'undefined' && app) {
    // Single instance lock
    const gotTheLock = app.requestSingleInstanceLock();
    if (!gotTheLock) {
        app.quit();
    } else {
        app.on('second-instance', () => {
            if (loginWindow && !loginWindow.isDestroyed()) {
                loginWindow.show();
                loginWindow.focus();
            } else if (mainWindow && !mainWindow.isDestroyed()) {
                mainWindow.show();
                mainWindow.focus();
            }
        });

        // App event handlers
        app.whenReady().then(async () => {
            const hasAdmin = await requestAdminRights();
            if (hasAdmin) {
                // Setup IPC handlers for Firebase auth
                setupFirebaseAuthHandlers();
                
                // Check for saved credentials
                const hasSavedAuth = loadSavedCredentials();
                console.log('🔍 DEBUG: hasSavedAuth =', hasSavedAuth);
                console.log('🔍 DEBUG: isAuthenticated =', isAuthenticated);
                console.log('🔍 DEBUG: currentUser =', currentUser);
                
                // Start core service in background
                console.log('🚀 Starting core service...');
                startCoreService();
                
                // Show UI immediately - don't wait for service
                console.log('🖥️ Showing UI...');
                if (hasSavedAuth) {
                    isAuthenticated = true;
                    createMainWindow();
                } else {
                    // No saved credentials, show login
                    createLoginWindow();
                }
            }
        });
    }
} else {
    // Fallback - try to initialize after a delay
    setTimeout(() => {
        const { app: delayedApp } = require('electron');
        if (delayedApp) {
            console.log('App loaded after delay, restarting...');
            delayedApp.relaunch();
            delayedApp.exit(0);
        }
    }, 100);
}


// App event handlers - only register if app is available
if (typeof app !== 'undefined' && app) {
    app.on('window-all-closed', () => {
        cleanup();
        if (process.platform !== 'darwin') {
            app.quit();
        }
    });

    app.on('activate', () => {
        if (BrowserWindow.getAllWindows().length === 0) {
            createWindow();
        }
    });

    app.on('before-quit', cleanup);
}

// Enhanced API connection with retry logic
async function makeAPICall(url, options = {}) {
    const maxRetries = 15; // Tăng số lần retry
    let retryDelay = 500; // Bắt đầu với delay ngắn hơn
    
    for (let attempt = 1; attempt <= maxRetries; attempt++) {
        try {
            const response = await axios({
                url: `http://127.0.0.1:8081${url}`,
                timeout: 3000,
                ...options
            });
            
            if (attempt > 1) {
                console.log(`✅ API call succeeded on attempt ${attempt}`);
            }
            
            return response.data;
        } catch (error) {
            if (attempt === maxRetries) {
                console.error(`❌ API call failed after ${maxRetries} attempts:`, error.message);
                return { error: `Failed to connect to service after ${maxRetries} attempts` };
            }
            
            // Exponential backoff với cap
            retryDelay = Math.min(retryDelay * 1.2, 2000);
            console.log(`⏳ API call attempt ${attempt} failed, retrying in ${Math.round(retryDelay)}ms...`);
            await new Promise(resolve => setTimeout(resolve, retryDelay));
        }
    }
}

// Updated IPC handlers with retry logic - only register if ipcMain is available
if (typeof ipcMain !== 'undefined' && ipcMain) {
    ipcMain.handle('get-stats', async () => {
        return await makeAPICall('/api/v1/stats');
    });

    ipcMain.handle('get-rules', async () => {
        const result = await makeAPICall('/api/v1/rules');
        return (result && result.error) ? [] : (result || []);
    });

    ipcMain.handle('add-rule', async (event, rule) => {
        return await makeAPICall('/api/v1/rules', {
            method: 'POST',
            data: rule,
            headers: { 'Content-Type': 'application/json' }
        });
    });

    ipcMain.handle('delete-rule', async (event, ruleId) => {
        return await makeAPICall(`/api/v1/rules/${ruleId}`, {
            method: 'DELETE'
        });
    });

    ipcMain.handle('get-logs', async () => {
        const result = await makeAPICall('/api/v1/logs');
        return result.error ? [] : result;
    });

    ipcMain.handle('get-system-status', async () => {
        return await makeAPICall('/api/v1/system/status');
    });

    ipcMain.handle('configure-system', async () => {
        return await makeAPICall('/api/v1/system/configure', {
            method: 'POST'
        });
    });

    ipcMain.handle('restore-system', async () => {
        return await makeAPICall('/api/v1/system/restore', {
            method: 'POST'
        });
    });

    ipcMain.handle('restart-core-service', async () => {
        try {
            cleanup();
            await new Promise(resolve => setTimeout(resolve, 3000)); // Increased delay
            await startCoreService();
            
            // Wait for API to be ready
            await makeAPICall('/api/v1/stats');
            
            return { status: 'success', message: 'Service restarted' };
        } catch (error) {
            console.error('Failed to restart service:', error);
            return { error: 'Failed to restart service' };
        }
    });

    // Authentication IPC handlers
    ipcMain.handle('authenticate-user', async (event, credentials) => {
    try {
        // For now, simulate authentication with local check
        // In production, this would call the Go auth service
        
        if (!credentials.email || !credentials.password) {
            return { success: false, error: 'Email và mật khẩu không được để trống' };
        }

        if (!isValidEmail(credentials.email)) {
            return { success: false, error: 'Email không hợp lệ' };
        }

        if (credentials.password.length < 6) {
            return { success: false, error: 'Mật khẩu phải có ít nhất 6 ký tự' };
        }

        // Generate consistent UID (match Android logic)
        const userUID = generateUserUID(credentials.email);
        
        // Store authentication state
        currentUser = {
            email: credentials.email,
            uid: userUID,
            loginTime: Date.now()
        };
        
        isAuthenticated = true;

        // Save credentials if remember is checked
        if (credentials.remember) {
            // 1) Save to Electron userData (per-user, roaming)
            const userDataPath = app.getPath('userData');
            const credentialsRoaming = path.join(userDataPath, 'user_credentials.json');

            // 2) Save a copy next to the core EXE so the Go service can auto-detect
            const credentialsLocal = path.join(__dirname, 'user_credentials.json');

            const payload = {
                email: credentials.email,
                uid: userUID,
                loginTime: Date.now()
            };

            try {
                fs.writeFileSync(credentialsRoaming, JSON.stringify(payload, null, 2));
            } catch (error) {
                console.error('Failed to save credentials (roaming):', error);
            }

            try {
                fs.writeFileSync(credentialsLocal, JSON.stringify(payload, null, 2));
            } catch (error) {
                console.error('Failed to save credentials (local):', error);
            }
        }

        return { 
            success: true, 
            user: currentUser 
        };

    } catch (error) {
        console.error('Authentication error:', error);
        return { success: false, error: 'Lỗi xác thực. Vui lòng thử lại.' };
    }
    });

    ipcMain.on('close-app', () => {
        if (app) app.quit();
    });

    ipcMain.handle('get-current-user', () => {
        return currentUser;
    });
}

// Helper functions for authentication
function isValidEmail(email) {
    const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
    return emailRegex.test(email);
}

function generateUserUID(email) {
    // Generate consistent UID from email (match Android LocalAuth logic)
    const crypto = require('crypto');
    const hash = crypto.createHash('md5').update(email).digest('hex');
    return `user_${hash.substring(0, 16)}`;
}

function loadSavedCredentials() {
    try {
        // Check multiple possible paths for credentials
        const paths = [
            path.join(app.getPath('userData'), 'user_credentials.json'), // AppData
            path.join(__dirname, 'user_credentials.json'), // ui-admin folder
            path.join(__dirname, '..', 'user_credentials.json'), // project root
        ];
        
        console.log('🔍 Checking credentials in paths:', paths);
        
        for (const credentialsFile of paths) {
            console.log(`🔍 Checking: ${credentialsFile}`);
            if (fs.existsSync(credentialsFile)) {
                console.log(`✅ Found credentials file: ${credentialsFile}`);
                const data = fs.readFileSync(credentialsFile, 'utf8');
                const credentials = JSON.parse(data);
                
                console.log(`🔍 Checking credentials: loginTime=${credentials.loginTime}, sevenDaysAgo=${Date.now() - (7 * 24 * 60 * 60 * 1000)}, now=${Date.now()}`);
                
                // Check if credentials are not too old (7 days)
                const sevenDaysAgo = Date.now() - (7 * 24 * 60 * 60 * 1000);
                if (credentials.loginTime && credentials.loginTime > sevenDaysAgo) {
                    console.log(`✅ Loaded saved credentials for: ${credentials.email}`);
                    currentUser = credentials;
                    isAuthenticated = true;
                    return true;
                } else {
                    console.log(`⚠️ Credentials expired, deleting: ${credentialsFile}`);
                    fs.unlinkSync(credentialsFile); // Delete expired credentials
                }
            }
        }
        
        console.log('❌ No valid credentials found');
    } catch (error) {
        console.error('Failed to load saved credentials:', error);
    }
    return false;
}

// Removed waitForCoreService - simplified startup

// Strict mode relays
// Strict mode handlers removed - simplified hosts-only approach