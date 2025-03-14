document.addEventListener('DOMContentLoaded', () => {
    // DOM Elements
    const loginForm = document.getElementById('login-form');
    const loginContainer = document.getElementById('login-container');
    const usernameInput = document.getElementById('username-input');
    const chatMain = document.getElementById('chat-main');
    const messagesContainer = document.getElementById('messages-container');
    const messageForm = document.getElementById('message-form');
    const messageInput = document.getElementById('message-input');
    const connectionStatus = document.getElementById('connection-status');
    const userCountElement = document.getElementById('user-count');
    const countDisplay = document.getElementById('count');

    let username = '';
    let socket = null;
    let activeUserCount = 0;

    // Login Form Submit
    loginForm.addEventListener('submit', (e) => {
        e.preventDefault();
        username = usernameInput.value.trim();
        if (username) {
            connectToChat(username);
        }
    });

    // Message Form Submit
    messageForm.addEventListener('submit', (e) => {
        e.preventDefault();
        sendMessage();
    });

    // Connect to WebSocket
    function connectToChat(username) {
        // Update UI to connecting state
        updateConnectionStatus('connecting', 'Connecting...');

        // Create WebSocket connection
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsUrl = `${protocol}//${window.location.host}/ws?username=${encodeURIComponent(username)}`;
        
        socket = new WebSocket(wsUrl);

        // Connection opened
        socket.addEventListener('open', () => {
            // Update UI for connected state
            loginContainer.classList.add('d-none');
            chatMain.classList.remove('d-none');
            userCountElement.classList.remove('d-none');
            updateConnectionStatus('connected', 'Connected');
            
            // Focus on message input
            messageInput.focus();
        });

        // Listen for messages
        socket.addEventListener('message', (event) => {
            const message = JSON.parse(event.data);
            displayMessage(message);
        });

        // Listen for socket closure
        socket.addEventListener('close', (event) => {
            updateConnectionStatus('disconnected', 'Disconnected');
            
            // Show reconnect option after a delay
            setTimeout(() => {
                if (confirm('Connection lost. Would you like to reconnect?')) {
                    connectToChat(username);
                } else {
                    // Reset UI to login state
                    loginContainer.classList.remove('d-none');
                    chatMain.classList.add('d-none');
                    userCountElement.classList.add('d-none');
                }
            }, 300);
        });

        // Error handling
        socket.addEventListener('error', (error) => {
            console.error('WebSocket error:', error);
            updateConnectionStatus('disconnected', 'Connection Error');
        });
    }

    // Send a message
    function sendMessage() {
        const content = messageInput.value.trim();
        
        if (content && socket && socket.readyState === WebSocket.OPEN) {
            // Create message object
            const message = {
                type: 'message',
                content: content
            };
            
            // Send as JSON
            socket.send(JSON.stringify(message));
            
            // Clear input
            messageInput.value = '';
        }
    }

    // Display a message in the chat
    function displayMessage(message) {
        // Create message element
        const messageElement = document.createElement('div');
        messageElement.classList.add('message', 'mb-3');
        
        // Add appropriate class based on message type
        if (message.type === 'system') {
            // System message
            messageElement.classList.add('message-system', 'text-center', 'text-muted', 'small', 'py-2', 'fst-italic');
            messageElement.textContent = message.content;
        } else {
            // User message
            const isCurrentUser = message.username === username;
            
            if (isCurrentUser) {
                messageElement.classList.add('message-user', 'alert', 'alert-primary', 'mw-75');
            } else {
                messageElement.classList.add('message-other', 'alert', 'alert-light', 'mw-75');
            }
            
            // Create message header (username + timestamp)
            const headerDiv = document.createElement('div');
            headerDiv.classList.add('d-flex', 'mb-1', 'small');
            
            // Create a container for username
            const usernameContainer = document.createElement('div');
            usernameContainer.classList.add('me-auto');
            
            const usernameSpan = document.createElement('span');
            usernameSpan.classList.add('fw-bold');
            usernameSpan.textContent = message.username;
            
            // Create a container for timestamp with extra margin
            const timestampContainer = document.createElement('div');
            timestampContainer.classList.add('ms-3');
            
            const timestampSpan = document.createElement('span');
            timestampSpan.classList.add('text-muted');
            timestampSpan.textContent = formatTime(message.time);
            
            // Add elements to their containers
            usernameContainer.appendChild(usernameSpan);
            timestampContainer.appendChild(timestampSpan);
            
            // Add containers to header
            headerDiv.appendChild(usernameContainer);
            headerDiv.appendChild(timestampContainer);
            
            // Create content element
            const contentDiv = document.createElement('div');
            contentDiv.textContent = message.content;
            
            // Add to message element
            messageElement.appendChild(headerDiv);
            messageElement.appendChild(contentDiv);
        }
        
        // Add to messages container
        messagesContainer.appendChild(messageElement);
        
        // Scroll to bottom
        messagesContainer.scrollTop = messagesContainer.scrollHeight;
        
        // Update user count if it's a join/leave message
        if (message.type === 'system' && 
            (message.content.includes('joined') || message.content.includes('left'))) {
            updateUserCount();
        }
    }

    // Update connection status display
    function updateConnectionStatus(state, text) {
        // Remove existing classes
        connectionStatus.className = 'badge';
        
        // Add appropriate class based on state
        if (state === 'connected') {
            connectionStatus.classList.add('bg-success');
        } else if (state === 'connecting') {
            connectionStatus.classList.add('bg-warning', 'text-dark');
        } else {
            connectionStatus.classList.add('bg-danger');
        }
        
        connectionStatus.textContent = text;
    }
    
    // Format timestamp
    function formatTime(isoString) {
        try {
            const date = new Date(isoString);
            return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
        } catch (e) {
            return '';
        }
    }
    
    // Update the user count
    function updateUserCount() {
        // In a real application, the server would send the user count
        // For this example, we'll simulate it based on join/leave messages
        const systemMessages = messagesContainer.querySelectorAll('.message-system');
        let count = 0;
        
        systemMessages.forEach(msg => {
            if (msg.textContent.includes('joined')) {
                count++;
            } else if (msg.textContent.includes('left')) {
                count = Math.max(0, count - 1);
            }
        });
        
        countDisplay.textContent = count;
        activeUserCount = count;
    }
}); 