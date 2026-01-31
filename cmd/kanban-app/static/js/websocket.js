/**
 * websocket.js - WebSocket Client for Real-time Updates
 * Handles WebSocket connection, reconnection, and event handling
 */

/**
 * WebSocket Client for The Unheaded Kingdom
 * Provides real-time updates for tasks and timeline
 */
const WebSocketClient = (function() {
    'use strict';

    // Configuration
    const CONFIG = {
        reconnectInterval: 2000,      // Initial reconnect delay
        maxReconnectInterval: 30000,  // Maximum reconnect delay
        reconnectDecay: 1.5,          // Exponential backoff multiplier
        maxReconnectAttempts: 10,     // Maximum reconnection attempts (0 = infinite)
        heartbeatInterval: 30000,     // Heartbeat ping interval
        heartbeatTimeout: 10000       // Heartbeat response timeout
    };

    // State
    let ws = null;
    let reconnectAttempts = 0;
    let reconnectTimer = null;
    let heartbeatTimer = null;
    let heartbeatTimeoutTimer = null;
    let isConnecting = false;
    let isManuallyClosed = false;

    // Event handlers
    const eventHandlers = new Map();
    const statusCallbacks = new Set();

    // Connection states
    const ConnectionState = {
        CONNECTING: 'connecting',
        CONNECTED: 'connected',
        DISCONNECTED: 'disconnected',
        RECONNECTING: 'reconnecting',
        ERROR: 'error'
    };

    let currentState = ConnectionState.DISCONNECTED;

    /**
     * Get WebSocket URL based on current location
     * @returns {string}
     */
    function getWebSocketUrl() {
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        return `${protocol}//${window.location.host}/ws`;
    }

    /**
     * Update connection state and notify listeners
     * @param {string} newState - New connection state
     */
    function updateState(newState) {
        if (currentState === newState) return;

        const oldState = currentState;
        currentState = newState;

        console.log(`[WebSocket] State changed: ${oldState} -> ${newState}`);

        statusCallbacks.forEach(callback => {
            try {
                callback(newState, oldState);
            } catch (error) {
                console.error('[WebSocket] Status callback error:', error);
            }
        });
    }

    /**
     * Start heartbeat monitoring
     */
    function startHeartbeat() {
        stopHeartbeat();

        heartbeatTimer = setInterval(() => {
            if (ws && ws.readyState === WebSocket.OPEN) {
                // Send ping
                send({ type: 'ping', timestamp: Date.now() });

                // Set timeout for pong response
                heartbeatTimeoutTimer = setTimeout(() => {
                    console.warn('[WebSocket] Heartbeat timeout, reconnecting...');
                    reconnect();
                }, CONFIG.heartbeatTimeout);
            }
        }, CONFIG.heartbeatInterval);
    }

    /**
     * Stop heartbeat monitoring
     */
    function stopHeartbeat() {
        if (heartbeatTimer) {
            clearInterval(heartbeatTimer);
            heartbeatTimer = null;
        }
        if (heartbeatTimeoutTimer) {
            clearTimeout(heartbeatTimeoutTimer);
            heartbeatTimeoutTimer = null;
        }
    }

    /**
     * Handle incoming WebSocket message
     * @param {MessageEvent} event - WebSocket message event
     */
    function handleMessage(event) {
        try {
            const message = JSON.parse(event.data);

            // Handle pong response
            if (message.type === 'pong') {
                if (heartbeatTimeoutTimer) {
                    clearTimeout(heartbeatTimeoutTimer);
                    heartbeatTimeoutTimer = null;
                }
                return;
            }

            // Log message for debugging
            console.log('[WebSocket] Received:', message.type, message);

            // Dispatch to registered handlers
            const handlers = eventHandlers.get(message.type) || [];
            handlers.forEach(handler => {
                try {
                    handler(message.data || message, message);
                } catch (error) {
                    console.error(`[WebSocket] Handler error for ${message.type}:`, error);
                }
            });

            // Also dispatch to wildcard handlers
            const wildcardHandlers = eventHandlers.get('*') || [];
            wildcardHandlers.forEach(handler => {
                try {
                    handler(message.data || message, message);
                } catch (error) {
                    console.error('[WebSocket] Wildcard handler error:', error);
                }
            });

        } catch (error) {
            console.error('[WebSocket] Failed to parse message:', error);
        }
    }

    /**
     * Connect to WebSocket server
     * @returns {Promise<void>}
     */
    function connect() {
        return new Promise((resolve, reject) => {
            if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING)) {
                resolve();
                return;
            }

            if (isConnecting) {
                // Wait for existing connection attempt
                const checkConnection = setInterval(() => {
                    if (currentState === ConnectionState.CONNECTED) {
                        clearInterval(checkConnection);
                        resolve();
                    } else if (currentState === ConnectionState.ERROR || currentState === ConnectionState.DISCONNECTED) {
                        clearInterval(checkConnection);
                        reject(new Error('Connection failed'));
                    }
                }, 100);
                return;
            }

            isConnecting = true;
            isManuallyClosed = false;
            updateState(ConnectionState.CONNECTING);

            const url = getWebSocketUrl();
            console.log('[WebSocket] Connecting to:', url);

            try {
                ws = new WebSocket(url);
            } catch (error) {
                isConnecting = false;
                updateState(ConnectionState.ERROR);
                reject(error);
                return;
            }

            ws.onopen = () => {
                console.log('[WebSocket] Connected');
                isConnecting = false;
                reconnectAttempts = 0;
                updateState(ConnectionState.CONNECTED);
                startHeartbeat();
                resolve();
            };

            ws.onmessage = handleMessage;

            ws.onerror = (error) => {
                console.error('[WebSocket] Error:', error);
                isConnecting = false;
                updateState(ConnectionState.ERROR);
            };

            ws.onclose = (event) => {
                console.log('[WebSocket] Closed:', event.code, event.reason);
                isConnecting = false;
                stopHeartbeat();
                updateState(ConnectionState.DISCONNECTED);

                // Attempt reconnection if not manually closed
                if (!isManuallyClosed) {
                    scheduleReconnect();
                }
            };

            // Connection timeout
            setTimeout(() => {
                if (isConnecting) {
                    isConnecting = false;
                    ws?.close();
                    updateState(ConnectionState.ERROR);
                    reject(new Error('Connection timeout'));
                }
            }, 10000);
        });
    }

    /**
     * Schedule reconnection attempt
     */
    function scheduleReconnect() {
        if (isManuallyClosed) return;

        if (CONFIG.maxReconnectAttempts > 0 && reconnectAttempts >= CONFIG.maxReconnectAttempts) {
            console.error('[WebSocket] Max reconnection attempts reached');
            updateState(ConnectionState.ERROR);
            return;
        }

        updateState(ConnectionState.RECONNECTING);

        const delay = Math.min(
            CONFIG.reconnectInterval * Math.pow(CONFIG.reconnectDecay, reconnectAttempts),
            CONFIG.maxReconnectInterval
        );

        console.log(`[WebSocket] Reconnecting in ${delay}ms (attempt ${reconnectAttempts + 1})`);

        reconnectTimer = setTimeout(() => {
            reconnectAttempts++;
            connect().catch(() => {
                // Connection failed, scheduleReconnect will be called again via onclose
            });
        }, delay);
    }

    /**
     * Force reconnection
     */
    function reconnect() {
        stopHeartbeat();
        clearTimeout(reconnectTimer);

        if (ws) {
            ws.onclose = null; // Prevent automatic reconnect from onclose
            ws.close();
            ws = null;
        }

        isConnecting = false;
        connect().catch(error => {
            console.error('[WebSocket] Reconnect failed:', error);
            scheduleReconnect();
        });
    }

    /**
     * Disconnect from WebSocket server
     */
    function disconnect() {
        isManuallyClosed = true;
        isConnecting = false;
        stopHeartbeat();
        clearTimeout(reconnectTimer);

        if (ws) {
            ws.close(1000, 'Client disconnect');
            ws = null;
        }

        updateState(ConnectionState.DISCONNECTED);
    }

    /**
     * Send a message through WebSocket
     * @param {Object} message - Message to send
     * @returns {boolean} - Whether the message was sent
     */
    function send(message) {
        if (!ws || ws.readyState !== WebSocket.OPEN) {
            console.warn('[WebSocket] Cannot send, not connected');
            return false;
        }

        try {
            ws.send(JSON.stringify(message));
            return true;
        } catch (error) {
            console.error('[WebSocket] Send error:', error);
            return false;
        }
    }

    /**
     * Register an event handler
     * @param {string} eventType - Event type to listen for
     * @param {Function} handler - Handler function
     * @returns {Function} - Unsubscribe function
     */
    function on(eventType, handler) {
        if (!eventHandlers.has(eventType)) {
            eventHandlers.set(eventType, []);
        }
        eventHandlers.get(eventType).push(handler);

        // Return unsubscribe function
        return () => off(eventType, handler);
    }

    /**
     * Unregister an event handler
     * @param {string} eventType - Event type
     * @param {Function} handler - Handler function to remove
     */
    function off(eventType, handler) {
        const handlers = eventHandlers.get(eventType);
        if (handlers) {
            const index = handlers.indexOf(handler);
            if (index > -1) {
                handlers.splice(index, 1);
            }
        }
    }

    /**
     * Register a one-time event handler
     * @param {string} eventType - Event type
     * @param {Function} handler - Handler function
     */
    function once(eventType, handler) {
        const wrapper = (...args) => {
            off(eventType, wrapper);
            handler(...args);
        };
        on(eventType, wrapper);
    }

    /**
     * Register a status change callback
     * @param {Function} callback - Callback function(newState, oldState)
     * @returns {Function} - Unsubscribe function
     */
    function onStatusChange(callback) {
        statusCallbacks.add(callback);
        // Immediately call with current state
        callback(currentState, null);
        return () => statusCallbacks.delete(callback);
    }

    /**
     * Check if connected
     * @returns {boolean}
     */
    function isConnected() {
        return ws && ws.readyState === WebSocket.OPEN;
    }

    /**
     * Get current connection state
     * @returns {string}
     */
    function getState() {
        return currentState;
    }

    // Public API
    return {
        connect,
        disconnect,
        reconnect,
        send,
        on,
        off,
        once,
        onStatusChange,
        isConnected,
        getState,
        ConnectionState
    };
})();

// Make available globally
window.WebSocketClient = WebSocketClient;
