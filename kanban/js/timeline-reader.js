// Timeline reader - connects to API and streams updates
// Uses Busboy message bus for real-time updates

(function() {
    'use strict';

    // Config
    const config = {
        apiBase: '/api/v1',
        pollInterval: 5000,
        reconnectDelay: 3000,
        maxReconnectAttempts: 10
    };

    // State
    let eventSource = null;
    let reconnectAttempts = 0;
    let pollTimer = null;

    // Initialize connection
    async function init() {
        console.log('Initializing timeline reader...');

        // Try SSE first, fall back to polling
        try {
            await connectSSE();
        } catch (err) {
            console.warn('SSE not available, falling back to polling:', err);
            startPolling();
        }
    }

    // Connect via Server-Sent Events
    async function connectSSE() {
        return new Promise((resolve, reject) => {
            if (typeof EventSource === 'undefined') {
                reject(new Error('EventSource not supported'));
                return;
            }

            window.KanbanBoard.updateConnectionStatus('connecting', 'Connecting...');

            eventSource = new EventSource(`${config.apiBase}/timeline/stream`);

            eventSource.onopen = () => {
                console.log('SSE connected');
                reconnectAttempts = 0;
                window.KanbanBoard.updateConnectionStatus('connected', 'Connected (live)');
                resolve();
            };

            eventSource.onmessage = (event) => {
                try {
                    const data = JSON.parse(event.data);
                    handleMessage(data);
                } catch (err) {
                    console.error('Failed to parse SSE message:', err);
                }
            };

            eventSource.addEventListener('tasks', (event) => {
                try {
                    const tasks = JSON.parse(event.data);
                    window.KanbanBoard.render(tasks);
                } catch (err) {
                    console.error('Failed to parse tasks event:', err);
                }
            });

            eventSource.addEventListener('task-update', (event) => {
                try {
                    const task = JSON.parse(event.data);
                    window.KanbanBoard.updateCard(task);
                } catch (err) {
                    console.error('Failed to parse task-update event:', err);
                }
            });

            eventSource.addEventListener('task-delete', (event) => {
                try {
                    const { id } = JSON.parse(event.data);
                    window.KanbanBoard.removeCard(id);
                } catch (err) {
                    console.error('Failed to parse task-delete event:', err);
                }
            });

            eventSource.onerror = (err) => {
                console.error('SSE error:', err);
                eventSource.close();
                eventSource = null;

                if (reconnectAttempts < config.maxReconnectAttempts) {
                    reconnectAttempts++;
                    window.KanbanBoard.updateConnectionStatus('connecting',
                        `Reconnecting (${reconnectAttempts}/${config.maxReconnectAttempts})...`);
                    setTimeout(() => connectSSE().catch(() => startPolling()), config.reconnectDelay);
                } else {
                    window.KanbanBoard.updateConnectionStatus('disconnected', 'Disconnected');
                    startPolling();
                }

                reject(err);
            };

            // Timeout for initial connection
            setTimeout(() => {
                if (eventSource && eventSource.readyState === EventSource.CONNECTING) {
                    eventSource.close();
                    reject(new Error('SSE connection timeout'));
                }
            }, 5000);
        });
    }

    // Handle incoming message
    function handleMessage(data) {
        switch (data.type) {
            case 'tasks':
                window.KanbanBoard.render(data.tasks);
                break;
            case 'task-update':
                window.KanbanBoard.updateCard(data.task);
                break;
            case 'task-delete':
                window.KanbanBoard.removeCard(data.id);
                break;
            case 'ping':
                // Keep-alive, ignore
                break;
            default:
                console.log('Unknown message type:', data.type);
        }
    }

    // Start polling fallback
    function startPolling() {
        if (pollTimer) return;

        console.log('Starting polling mode');
        window.KanbanBoard.updateConnectionStatus('connected', 'Connected (polling)');

        // Initial fetch
        fetchTasks();

        // Poll periodically
        pollTimer = setInterval(fetchTasks, config.pollInterval);
    }

    // Stop polling
    function stopPolling() {
        if (pollTimer) {
            clearInterval(pollTimer);
            pollTimer = null;
        }
    }

    // Fetch tasks via REST
    async function fetchTasks() {
        try {
            var response = await fetch(config.apiBase + '/timeline/tasks');

            if (!response.ok) {
                throw new Error('HTTP ' + response.status);
            }

            var data = await response.json();
            window.KanbanBoard.render(data.tasks || []);

        } catch (err) {
            console.error('Failed to fetch tasks:', err);
            var friendlyMsg = getFriendlyErrorMessage(err);
            window.KanbanBoard.updateConnectionStatus('disconnected', friendlyMsg);

            // Show error state in columns if no tasks loaded yet
            var tasks = window.KanbanBoard.getTasks ? window.KanbanBoard.getTasks() : [];
            if (tasks.length === 0) {
                showFetchError(friendlyMsg);
            }
        }
    }

    // Convert raw errors to user-friendly messages
    function getFriendlyErrorMessage(error) {
        var msg = error.message || String(error);
        if (msg.includes('Failed to fetch') || msg.includes('NetworkError')) {
            return 'Cannot reach the server. Check your connection.';
        }
        if (msg.includes('HTTP 500') || msg.includes('HTTP 502') || msg.includes('HTTP 503')) {
            return 'Server is temporarily unavailable.';
        }
        if (msg.includes('HTTP 401') || msg.includes('HTTP 403')) {
            return 'Authentication error.';
        }
        if (msg.includes('HTTP 404')) {
            return 'Task endpoint not found.';
        }
        if (msg.includes('timeout') || msg.includes('Timeout')) {
            return 'Server response timed out.';
        }
        return 'Connection error';
    }

    // Show error state in the board columns
    function showFetchError(message) {
        var columns = document.querySelectorAll('.column .cards');
        if (columns.length > 0) {
            // Only show in first column to avoid repetition
            columns[0].innerHTML = '<div class="error-state">' +
                '<div class="error-state-icon" aria-hidden="true">&#x26A0;</div>' +
                '<div class="error-state-title">Unable to load tasks</div>' +
                '<div class="error-state-message">' + message + '</div>' +
                '<button class="error-state-action" onclick="window.TimelineReader.fetchTasks()" aria-label="Retry loading tasks">Retry</button>' +
            '</div>';
        }
    }

    // Disconnect
    function disconnect() {
        if (eventSource) {
            eventSource.close();
            eventSource = null;
        }
        stopPolling();
        window.KanbanBoard.updateConnectionStatus('disconnected', 'Disconnected');
    }

    // Reconnect
    function reconnect() {
        disconnect();
        reconnectAttempts = 0;
        init();
    }

    // Export
    window.TimelineReader = {
        init,
        disconnect,
        reconnect,
        fetchTasks
    };

    // Wire up reconnect banner retry button
    function wireReconnectBanner() {
        var retryBtn = document.getElementById('reconnect-retry-btn');
        if (retryBtn && !retryBtn._kanbanBound) {
            retryBtn._kanbanBound = true;
            retryBtn.addEventListener('click', function() {
                reconnect();
            });
        }
    }

    // Auto-init after board is ready
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', function() {
            // Wait for board to initialize
            setTimeout(function() {
                wireReconnectBanner();
                init();
            }, 100);
        });
    } else {
        setTimeout(function() {
            wireReconnectBanner();
            init();
        }, 100);
    }

    console.log('timeline-reader.js loaded');
})();
