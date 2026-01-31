/**
 * board.js - Board Rendering and State Management
 * Handles the Kanban board state, rendering, and column management
 */

/**
 * Board module for The Unheaded Kingdom's Kanban Board
 */
const Board = (function() {
    'use strict';

    // Board state
    const state = {
        tasks: new Map(),           // taskId -> task object
        columns: ['backlog', 'in_progress', 'review', 'done'],
        isLoading: false,
        lastUpdate: null,
        filters: {
            priority: null,
            assignee: null,
            search: ''
        }
    };

    // Column element references
    const columnElements = new Map();

    /**
     * Initialize the board
     */
    function init() {
        // Cache column element references
        state.columns.forEach(status => {
            const column = document.querySelector(`.column-content[data-status="${status}"]`);
            if (column) {
                columnElements.set(status, column);
            }
        });

        // Initialize drag and drop
        Cards.initDragDrop();

        console.log('[Board] Initialized');
    }

    /**
     * Load tasks from the API
     * @returns {Promise}
     */
    async function loadTasks() {
        if (state.isLoading) return;

        state.isLoading = true;
        showLoading(true);

        try {
            const tasks = await API.tasks.getAll();
            console.log('[Board] Loaded tasks:', tasks.length);

            // Clear existing state
            state.tasks.clear();

            // Populate state
            tasks.forEach(task => {
                state.tasks.set(task.id, task);
            });

            // Render all columns
            renderAllColumns();

            state.lastUpdate = new Date();

        } catch (error) {
            console.error('[Board] Failed to load tasks:', error);
            showToast('error', 'Failed to load tasks', error.message);
        } finally {
            state.isLoading = false;
            showLoading(false);
        }
    }

    /**
     * Render all columns
     */
    function renderAllColumns() {
        state.columns.forEach(status => {
            renderColumn(status);
        });
    }

    /**
     * Render a single column
     * @param {string} status - Column status
     */
    function renderColumn(status) {
        const column = columnElements.get(status);
        if (!column) return;

        // Get tasks for this column
        const tasks = getTasksByStatus(status);

        // Apply filters
        const filteredTasks = applyFilters(tasks);

        // Sort tasks by priority (P0 first) then by creation date
        filteredTasks.sort((a, b) => {
            if (a.priority !== b.priority) {
                return a.priority - b.priority;
            }
            return new Date(b.created_at) - new Date(a.created_at);
        });

        // Clear column
        column.innerHTML = '';

        // Render cards
        filteredTasks.forEach(task => {
            const card = Cards.createCard(task);
            column.appendChild(card);
        });

        // Update column count
        updateColumnCount(status, filteredTasks.length);
    }

    /**
     * Get tasks for a specific status
     * @param {string} status - Column status
     * @returns {Array}
     */
    function getTasksByStatus(status) {
        const tasks = [];
        state.tasks.forEach(task => {
            if (task.status === status) {
                tasks.push(task);
            }
        });
        return tasks;
    }

    /**
     * Apply filters to task list
     * @param {Array} tasks - Tasks to filter
     * @returns {Array}
     */
    function applyFilters(tasks) {
        return tasks.filter(task => {
            // Priority filter
            if (state.filters.priority !== null && task.priority !== state.filters.priority) {
                return false;
            }

            // Assignee filter
            if (state.filters.assignee && task.assignee !== state.filters.assignee) {
                return false;
            }

            // Search filter
            if (state.filters.search) {
                const searchLower = state.filters.search.toLowerCase();
                const titleMatch = task.title.toLowerCase().includes(searchLower);
                const descMatch = task.description?.toLowerCase().includes(searchLower);
                const tagMatch = task.tags?.some(tag => tag.toLowerCase().includes(searchLower));

                if (!titleMatch && !descMatch && !tagMatch) {
                    return false;
                }
            }

            return true;
        });
    }

    /**
     * Update column count badge
     * @param {string} status - Column status
     * @param {number} count - Task count
     */
    function updateColumnCount(status, count) {
        const column = document.querySelector(`.column[data-status="${status}"]`);
        if (!column) return;

        const countBadge = column.querySelector('.column-count');
        if (countBadge) {
            countBadge.textContent = count;
            countBadge.dataset.count = count;
        }
    }

    /**
     * Add a task to the board
     * @param {Object} task - Task data
     */
    function addTask(task) {
        state.tasks.set(task.id, task);
        renderColumn(task.status);

        // Animate the new card
        const card = document.querySelector(`[data-task-id="${task.id}"]`);
        if (card) {
            card.classList.add('just-completed');
            setTimeout(() => card.classList.remove('just-completed'), 400);
        }
    }

    /**
     * Update a task on the board
     * @param {Object} task - Updated task data
     */
    function updateTask(task) {
        const oldTask = state.tasks.get(task.id);
        const statusChanged = oldTask && oldTask.status !== task.status;

        state.tasks.set(task.id, task);

        if (statusChanged) {
            // Re-render both columns
            renderColumn(oldTask.status);
            renderColumn(task.status);
        } else {
            // Just update the card
            const card = document.querySelector(`[data-task-id="${task.id}"]`);
            if (card) {
                Cards.updateCard(card, task);
            }
        }
    }

    /**
     * Remove a task from the board
     * @param {string} taskId - Task ID
     */
    function removeTask(taskId) {
        const task = state.tasks.get(taskId);
        if (!task) return;

        // Animate removal
        const card = document.querySelector(`[data-task-id="${taskId}"]`);
        if (card) {
            card.classList.add('deleting');
            setTimeout(() => {
                state.tasks.delete(taskId);
                renderColumn(task.status);
            }, 300);
        } else {
            state.tasks.delete(taskId);
        }
    }

    /**
     * Move a task to a different column
     * @param {string} taskId - Task ID
     * @param {string} newStatus - New status
     * @returns {Promise}
     */
    async function moveTask(taskId, newStatus) {
        const task = state.tasks.get(taskId);
        if (!task) return;

        const oldStatus = task.status;

        // Optimistic update
        task.status = newStatus;
        renderColumn(oldStatus);
        renderColumn(newStatus);

        try {
            // Update via API
            const updatedTask = await API.tasks.move(taskId, newStatus);
            state.tasks.set(taskId, updatedTask);

            // Check if completed
            if (newStatus === 'done' && oldStatus !== 'done') {
                const card = document.querySelector(`[data-task-id="${taskId}"]`);
                if (card) {
                    card.classList.add('just-completed');
                    setTimeout(() => card.classList.remove('just-completed'), 400);
                }
            }

            showToast('success', 'Task moved', `Moved to ${Cards.STATUS_NAMES[newStatus]}`);

        } catch (error) {
            console.error('[Board] Failed to move task:', error);

            // Rollback on error
            task.status = oldStatus;
            renderColumn(oldStatus);
            renderColumn(newStatus);

            showToast('error', 'Failed to move task', error.message);
        }
    }

    /**
     * Get a task by ID
     * @param {string} taskId - Task ID
     * @returns {Object|undefined}
     */
    function getTask(taskId) {
        return state.tasks.get(taskId);
    }

    /**
     * Get all tasks
     * @returns {Array}
     */
    function getAllTasks() {
        return Array.from(state.tasks.values());
    }

    /**
     * Set filter
     * @param {string} filterType - Filter type (priority, assignee, search)
     * @param {*} value - Filter value
     */
    function setFilter(filterType, value) {
        if (filterType in state.filters) {
            state.filters[filterType] = value;
            renderAllColumns();
        }
    }

    /**
     * Clear all filters
     */
    function clearFilters() {
        state.filters.priority = null;
        state.filters.assignee = null;
        state.filters.search = '';
        renderAllColumns();
    }

    /**
     * Show/hide loading state
     * @param {boolean} show - Whether to show loading
     */
    function showLoading(show) {
        const overlay = document.getElementById('loadingOverlay');
        if (overlay) {
            if (show) {
                overlay.classList.remove('hidden');
            } else {
                overlay.classList.add('hidden');
            }
        }
    }

    /**
     * Show toast notification
     * @param {string} type - Toast type (success, error, warning, info)
     * @param {string} title - Toast title
     * @param {string} message - Toast message
     */
    function showToast(type, title, message = '') {
        const container = document.getElementById('toastContainer');
        if (!container) return;

        const icons = {
            success: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="20 6 9 17 4 12"/></svg>',
            error: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><line x1="15" y1="9" x2="9" y2="15"/><line x1="9" y1="9" x2="15" y2="15"/></svg>',
            warning: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M10.29 3.86L1.82 18a2 2 0 001.71 3h16.94a2 2 0 001.71-3L13.71 3.86a2 2 0 00-3.42 0z"/><line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg>',
            info: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><line x1="12" y1="16" x2="12" y2="12"/><line x1="12" y1="8" x2="12.01" y2="8"/></svg>'
        };

        const toast = document.createElement('div');
        toast.className = `toast ${type}`;
        toast.innerHTML = `
            <span class="toast-icon">${icons[type] || icons.info}</span>
            <div class="toast-content">
                <div class="toast-title">${Cards.escapeHtml(title)}</div>
                ${message ? `<div class="toast-message">${Cards.escapeHtml(message)}</div>` : ''}
            </div>
        `;

        container.appendChild(toast);

        // Trigger animation
        requestAnimationFrame(() => {
            toast.classList.add('visible');
        });

        // Auto remove
        setTimeout(() => {
            toast.classList.remove('visible');
            setTimeout(() => toast.remove(), 300);
        }, 4000);
    }

    /**
     * Handle real-time task created event
     * @param {Object} data - Event data
     */
    function handleTaskCreated(data) {
        const task = data.task || data;
        if (!state.tasks.has(task.id)) {
            addTask(task);
            showToast('info', 'New task', task.title);
        }
    }

    /**
     * Handle real-time task updated event
     * @param {Object} data - Event data
     */
    function handleTaskUpdated(data) {
        const task = data.task || data;
        const existingTask = state.tasks.get(task.id);

        if (existingTask) {
            // Check if this is a meaningful update
            const hasChanges = JSON.stringify(existingTask) !== JSON.stringify(task);
            if (hasChanges) {
                updateTask(task);
            }
        }
    }

    /**
     * Handle real-time task completed event
     * @param {Object} data - Event data
     */
    function handleTaskCompleted(data) {
        const task = data.task || data;
        task.status = 'done';
        updateTask(task);
        showToast('success', 'Task completed', task.title);
    }

    /**
     * Handle real-time task deleted event
     * @param {Object} data - Event data
     */
    function handleTaskDeleted(data) {
        const taskId = data.task_id || data.id;
        if (taskId && state.tasks.has(taskId)) {
            removeTask(taskId);
        }
    }

    /**
     * Get board statistics
     * @returns {Object}
     */
    function getStats() {
        const stats = {
            total: state.tasks.size,
            byStatus: {},
            byPriority: {}
        };

        state.columns.forEach(status => {
            stats.byStatus[status] = 0;
        });

        for (let i = 0; i <= 4; i++) {
            stats.byPriority[i] = 0;
        }

        state.tasks.forEach(task => {
            stats.byStatus[task.status] = (stats.byStatus[task.status] || 0) + 1;
            stats.byPriority[task.priority] = (stats.byPriority[task.priority] || 0) + 1;
        });

        return stats;
    }

    // Public API
    return {
        init,
        loadTasks,
        addTask,
        updateTask,
        removeTask,
        moveTask,
        getTask,
        getAllTasks,
        renderColumn,
        renderAllColumns,
        setFilter,
        clearFilters,
        showLoading,
        showToast,
        getStats,

        // Real-time event handlers
        handleTaskCreated,
        handleTaskUpdated,
        handleTaskCompleted,
        handleTaskDeleted,

        // Expose state for debugging
        get state() {
            return {
                taskCount: state.tasks.size,
                isLoading: state.isLoading,
                lastUpdate: state.lastUpdate,
                filters: { ...state.filters }
            };
        }
    };
})();

// Make available globally
window.Board = Board;
