// Debug script to test if extension can run
(function() {
    console.log('🧪 Debug: Manual script injection test');
    console.log('🧪 Debug: URL:', window.location.href);
    console.log('🧪 Debug: Domain:', window.location.hostname);
    console.log('🧪 Debug: Title:', document.title);
    
    // Try to detect error page
    const hasError = document.body && document.body.textContent.includes('NET::ERR_');
    const isErrorPage = window.location.protocol === 'chrome-error:';
    const isSecurityError = document.title.includes('Lỗi bảo mật') || document.title.includes('Security error');
    const isJeffVn = window.location.href.includes('jeff.vn');
    
    console.log('🧪 Debug: Has error:', hasError);
    console.log('🧪 Debug: Is error page:', isErrorPage);
    console.log('🧪 Debug: Is security error:', isSecurityError);
    console.log('🧪 Debug: Is jeff.vn:', isJeffVn);
    
    // Show KidSafe message
    if (hasError || isErrorPage || isSecurityError || isJeffVn) {
        // Use document.write to bypass TrustedHTML restrictions
        document.open();
        document.write(`
            <div style="
                font-family: Arial, sans-serif;
                text-align: center;
                padding: 50px;
                background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
                color: white;
                min-height: 100vh;
            ">
                <h1>🛡️ This Website is Protected</h1>
                <div style="font-size: 24px; margin: 20px 0;">🐰 🐯 🐼 🦄</div>
                <p>Hello! This website has been protected by your parents to keep you safe.</p>
                <p><strong>Domain:</strong> ${window.location.hostname}</p>
                <button onclick="history.back()" style="
                    background: white;
                    color: #667eea;
                    border: none;
                    padding: 15px 30px;
                    border-radius: 25px;
                    font-size: 16px;
                    cursor: pointer;
                    margin-top: 20px;
                ">← Go Back</button>
            </div>
        `);
        document.close();
        console.log('🧪 Debug: KidSafe message displayed');
    }
})();
