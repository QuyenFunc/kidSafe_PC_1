// Simple icon creator for KidSafe PC Extension
function createIcon(size) {
    const canvas = document.createElement('canvas');
    canvas.width = size;
    canvas.height = size;
    const ctx = canvas.getContext('2d');
    
    // Background - green shield
    ctx.fillStyle = '#4CAF50';
    ctx.fillRect(0, 0, size, size);
    
    // White shield shape
    ctx.fillStyle = 'white';
    ctx.font = `${size * 0.7}px Arial`;
    ctx.textAlign = 'center';
    ctx.textBaseline = 'middle';
    ctx.fillText('🛡️', size/2, size/2);
    
    return canvas.toDataURL('image/png');
}

// Create all icon sizes
const icon16 = createIcon(16);
const icon32 = createIcon(32);
const icon48 = createIcon(48);
const icon128 = createIcon(128);

console.log('Icons created as base64 data URLs');
console.log('Copy these to create .png files or use in manifest');

// Function to download icon
function downloadIcon(dataUrl, filename) {
    const link = document.createElement('a');
    link.download = filename;
    link.href = dataUrl;
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
}

// Auto-create icons when script runs
if (typeof window !== 'undefined') {
    downloadIcon(icon16, 'icon16.png');
    downloadIcon(icon32, 'icon32.png');
    downloadIcon(icon48, 'icon48.png');
    downloadIcon(icon128, 'icon128.png');
}
