/**
 * api.js - API Client for Timeguru
 * Handles all HTTP communication with the backend API
 */

/**
 * API Client for The Unheaded Kingdom's Timeguru service
 */
const API = (function() {
    'use strict';

    // Configuration
    const CONFIG = {
        baseUrl: '/api/v1',
        timeout: 30000, // 30 seconds
        retryAttempts: 3,
        retryDelay: 1000
    };

    // Request queue for offline support (future enhancement)
    const requestQueue = [];

    /**
     * Custom error class for API errors
     */
    class APIError extends Error {
        constructor(message, status, data = null) {
            super(message);
            this.name = 'APIError';
            this.status = status;
            this.data = data;
        }
    }

    /**
     * Sleep utility for retry delays
     * @param {number} ms - Milliseconds to sleep
     * @returns {Promise}
     */
    function sleep(ms) {
        return new Promise(resolve => setTimeout(resolve, ms));
    }

    /**
     * Build URL with query parameters
     * @param {string} endpoint - API endpoint
     * @param {Object} params - Query parameters
     * @returns {string}
     */
    function buildUrl(endpoint, params = {}) {
        const url = new URL(CONFIG.baseUrl + endpoint, window.location.origin);
        Object.keys(params).forEach(key => {
            if (params[key] !== undefined && params[key] !== null) {
                url.searchParams.append(key, params[key]);
            }
        });
        return url.toString();
    }

    /**
     * Make an HTTP request with retry logic
     * @param {string} method - HTTP method
     * @param {string} endpoint - API endpoint
     * @param {Object} options - Request options
     * @returns {Promise<Object>}
     */
    async function request(method, endpoint, options = {}) {
        const { body, params, headers = {}, retry = true } = options;

        const url = buildUrl(endpoint, params);
        const fetchOptions = {
            method,
            headers: {
                'Content-Type': 'application/json',
                'Accept': 'application/json',
                ...headers
            }
        };

        if (body && method !== 'GET') {
            fetchOptions.body = JSON.stringify(body);
        }

        let lastError;
        const attempts = retry ? CONFIG.retryAttempts : 1;

        for (let attempt = 1; attempt <= attempts; attempt++) {
            try {
                const controller = new AbortController();
                const timeoutId = setTimeout(() => controller.abort(), CONFIG.timeout);
                fetchOptions.signal = controller.signal;

                const response = await fetch(url, fetchOptions);
                clearTimeout(timeoutId);

                // Parse response
                let data = null;
                const contentType = response.headers.get('Content-Type');
                if (contentType && contentType.includes('application/json')) {
                    data = await response.json();
                }

                // Handle error responses
                if (!response.ok) {
                    const errorMessage = data?.error || data?.message || `Request failed with status ${response.status}`;
                    throw new APIError(errorMessage, response.status, data);
                }

                return data;

            } catch (error) {
                lastError = error;

                // Don't retry on certain errors
                if (error instanceof APIError) {
                    if (error.status >= 400 && error.status < 500) {
                        throw error; // Client errors shouldn't be retried
                    }
                }

                if (error.name === 'AbortError') {
                    throw new APIError('Request timed out', 408);
                }

                // Log retry attempt
                if (attempt < attempts) {
                    console.warn(`[API] Request failed, retrying (${attempt}/${attempts})...`, error);
                    await sleep(CONFIG.retryDelay * attempt);
                }
            }
        }

        // All retries exhausted
        if (lastError instanceof APIError) {
            throw lastError;
        }
        throw new APIError(lastError.message || 'Network error', 0);
    }

    /**
     * Tasks API
     */
    const tasks = {
        /**
         * Get all tasks
         * @param {Object} filters - Optional filters (status, priority, assignee)
         * @returns {Promise<Array>}
         */
        async getAll(filters = {}) {
            const data = await request('GET', '/tasks', { params: filters });
            return data?.tasks || data || [];
        },

        /**
         * Get a single task by ID
         * @param {string} id - Task ID
         * @returns {Promise<Object>}
         */
        async getById(id) {
            if (!id) throw new APIError('Task ID is required', 400);
            return request('GET', `/tasks/${id}`);
        },

        /**
         * Create a new task
         * @param {Object} taskData - Task data
         * @returns {Promise<Object>}
         */
        async create(taskData) {
            if (!taskData.title) {
                throw new APIError('Task title is required', 400);
            }

            const payload = {
                title: taskData.title.trim(),
                description: taskData.description?.trim() || '',
                priority: parseInt(taskData.priority, 10) || 2,
                status: taskData.status || 'backlog',
                assignee: taskData.assignee?.trim() || '',
                due_date: taskData.due_date || null,
                tags: Array.isArray(taskData.tags) ? taskData.tags :
                      taskData.tags ? taskData.tags.split(',').map(t => t.trim()).filter(Boolean) : []
            };

            return request('POST', '/tasks', { body: payload });
        },

        /**
         * Update an existing task
         * @param {string} id - Task ID
         * @param {Object} updates - Fields to update
         * @returns {Promise<Object>}
         */
        async update(id, updates) {
            if (!id) throw new APIError('Task ID is required', 400);

            const payload = {};

            // Only include fields that are being updated
            if (updates.title !== undefined) payload.title = updates.title.trim();
            if (updates.description !== undefined) payload.description = updates.description.trim();
            if (updates.priority !== undefined) payload.priority = parseInt(updates.priority, 10);
            if (updates.status !== undefined) payload.status = updates.status;
            if (updates.assignee !== undefined) payload.assignee = updates.assignee.trim();
            if (updates.due_date !== undefined) payload.due_date = updates.due_date;
            if (updates.tags !== undefined) {
                payload.tags = Array.isArray(updates.tags) ? updates.tags :
                              updates.tags ? updates.tags.split(',').map(t => t.trim()).filter(Boolean) : [];
            }

            return request('PUT', `/tasks/${id}`, { body: payload });
        },

        /**
         * Delete a task
         * @param {string} id - Task ID
         * @returns {Promise<void>}
         */
        async delete(id) {
            if (!id) throw new APIError('Task ID is required', 400);
            await request('DELETE', `/tasks/${id}`);
        },

        /**
         * Move a task to a different status/column
         * @param {string} id - Task ID
         * @param {string} newStatus - New status
         * @returns {Promise<Object>}
         */
        async move(id, newStatus) {
            return this.update(id, { status: newStatus });
        },

        /**
         * Batch update tasks
         * @param {Array} updates - Array of {id, updates} objects
         * @returns {Promise<Array>}
         */
        async batchUpdate(updates) {
            const results = await Promise.allSettled(
                updates.map(({ id, updates: taskUpdates }) =>
                    this.update(id, taskUpdates)
                )
            );

            return results.map((result, index) => ({
                id: updates[index].id,
                success: result.status === 'fulfilled',
                data: result.status === 'fulfilled' ? result.value : null,
                error: result.status === 'rejected' ? result.reason.message : null
            }));
        }
    };

    /**
     * Timeline API
     */
    const timeline = {
        /**
         * Get timeline data
         * @param {Object} options - Query options (start, end, granularity)
         * @returns {Promise<Object>}
         */
        async get(options = {}) {
            return request('GET', '/timeline', { params: options });
        },

        /**
         * Get task history/audit log
         * @param {string} taskId - Task ID (optional, get all if not provided)
         * @returns {Promise<Array>}
         */
        async getHistory(taskId = null) {
            const endpoint = taskId ? `/timeline/tasks/${taskId}` : '/timeline/history';
            return request('GET', endpoint);
        }
    };

    /**
     * Health check
     * @returns {Promise<Object>}
     */
    async function healthCheck() {
        try {
            return await request('GET', '/health', { retry: false });
        } catch (error) {
            return { status: 'unhealthy', error: error.message };
        }
    }

    /**
     * Set configuration options
     * @param {Object} options - Configuration options
     */
    function configure(options) {
        Object.assign(CONFIG, options);
    }

    // Public API
    return {
        tasks,
        timeline,
        healthCheck,
        configure,
        APIError,

        // Expose config for debugging
        get config() {
            return { ...CONFIG };
        }
    };
})();

// Make available globally
window.API = API;
