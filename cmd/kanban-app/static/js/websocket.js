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
    let eventSource = null; // SSE fallback
    let reconnectAttempts = 0;
    let reconnectTimer = null;
    let heartbeatTimer = null;
    let heartbeatTimeoutTimer = null;
    let isConnecting = false;
    let isManuallyClosed = false;
    let useSSE = false; // Whether to use SSE fallback

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
     * Get SSE URL for fallback
     * @returns {string}
     */
    function getSSEUrl() {
        return `${window.location.origin}/api/v1/stream`;
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
     * Connect via SSE (Server-Sent Events) as fallback
     * @returns {Promise<void>}
     */
    function connectSSE() {
        return new Promise((resolve, reject) => {
            if (eventSource && eventSource.readyState !== EventSource.CLOSED) {
                resolve();
                return;
            }

            const url = getSSEUrl();
            console.log('[SSE] Connecting to:', url);

            try {
                eventSource = new EventSource(url);
            } catch (error) {
                console.error('[SSE] Failed to create EventSource:', error);
                reject(error);
                return;
            }

            eventSource.onopen = () => {
                console.log('[SSE] Connected');
                isConnecting = false;
                reconnectAttempts = 0;
                useSSE = true;
                updateState(ConnectionState.CONNECTED);
                resolve();
            };

            eventSource.onmessage = (event) => {
                try {
                    const data = JSON.parse(event.data);
                    handleMessage({ data: event.data });
                } catch (error) {
                    console.error('[SSE] Parse error:', error);
                }
            };

            eventSource.addEventListener('tasks', (event) => {
                try {
                    const tasks = JSON.parse(event.data);
                    // Dispatch initial tasks load event
                    const handlers = eventHandlers.get('tasks.loaded') || [];
                    handlers.forEach(handler => handler({ tasks }));
                } catch (error) {
                    console.error('[SSE] Tasks parse error:', error);
                }
            });

            // Timeline events - THE META MOMENT
            eventSource.addEventListener('timeline.updated', (event) => {
                try {
                    const data = JSON.parse(event.data);
                    console.log('[SSE] Timeline updated:', data);
                    const handlers = eventHandlers.get('timeline.updated') || [];
                    handlers.forEach(handler => handler(data));
                } catch (error) {
                    console.error('[SSE] Timeline parse error:', error);
                }
            });

            eventSource.addEventListener('timeline.event', (event) => {
                try {
                    const data = JSON.parse(event.data);
                    console.log('[SSE] Timeline event:', data);
                    const handlers = eventHandlers.get('timeline.event') || [];
                    handlers.forEach(handler => handler(data));
                } catch (error) {
                    console.error('[SSE] Timeline event parse error:', error);
                }
            });

            eventSource.onerror = (error) => {
                console.error('[SSE] Error:', error);
                isConnecting = false;
                if (eventSource.readyState === EventSource.CLOSED) {
                    updateState(ConnectionState.DISCONNECTED);
                    if (!isManuallyClosed) {
                        scheduleReconnect();
                    }
                }
            };

            // Connection timeout
            setTimeout(() => {
                if (isConnecting && eventSource.readyState === EventSource.CONNECTING) {
                    eventSource.close();
                    reject(new Error('SSE connection timeout'));
                }
            }, 10000);
        });
    }

    /**
     * Connect to WebSocket server (with SSE fallback)
     * @returns {Promise<void>}
     */
    function connect() {
        return new Promise((resolve, reject) => {
            // If SSE is already working, use it
            if (useSSE && eventSource && eventSource.readyState === EventSource.OPEN) {
                resolve();
                return;
            }

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
                console.warn('[WebSocket] WebSocket not supported, trying SSE fallback');
                isConnecting = false;
                connectSSE().then(resolve).catch(reject);
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
                console.warn('[WebSocket] Error, trying SSE fallback:', error);
                isConnecting = false;
                ws?.close();
                // Try SSE fallback
                connectSSE().then(() => {
                    resolve();
                }).catch(() => {
                    updateState(ConnectionState.ERROR);
                    reject(new Error('Both WebSocket and SSE failed'));
                });
            };

            ws.onclose = (event) => {
                console.log('[WebSocket] Closed:', event.code, event.reason);
                isConnecting = false;
                stopHeartbeat();

                // If WebSocket closes quickly, try SSE
                if (event.code !== 1000 && !useSSE) {
                    console.log('[WebSocket] Trying SSE fallback...');
                    connectSSE().catch(() => {
                        updateState(ConnectionState.DISCONNECTED);
                        if (!isManuallyClosed) {
                            scheduleReconnect();
                        }
                    });
                } else {
                    updateState(ConnectionState.DISCONNECTED);
                    if (!isManuallyClosed) {
                        scheduleReconnect();
                    }
                }
            };

            // Connection timeout - try SSE on timeout
            setTimeout(() => {
                if (isConnecting) {
                    console.warn('[WebSocket] Connection timeout, trying SSE');
                    isConnecting = false;
                    ws?.close();
                    connectSSE().then(resolve).catch(() => {
                        updateState(ConnectionState.ERROR);
                        reject(new Error('Connection timeout'));
                    });
                }
            }, 5000);
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

        if (eventSource) {
            eventSource.close();
            eventSource = null;
        }

        useSSE = false;
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
