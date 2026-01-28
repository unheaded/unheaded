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
            const response = await fetch(`${config.apiBase}/timeline/tasks`);

            if (!response.ok) {
                throw new Error(`HTTP ${response.status}`);
            }

            const data = await response.json();
            window.KanbanBoard.render(data.tasks || []);

        } catch (err) {
            console.error('Failed to fetch tasks:', err);
            window.KanbanBoard.updateConnectionStatus('disconnected', 'Connection error');
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

    // Auto-init after board is ready
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', () => {
            // Wait for board to initialize
            setTimeout(init, 100);
        });
    } else {
        setTimeout(init, 100);
    }

    console.log('timeline-reader.js loaded');
})();
