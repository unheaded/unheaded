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
        sortBy: 'priority',         // priority, progress, type, owner, updated, created, title
        filters: {
            priority: null,
            assignee: null,
            type: null,
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

        // Initialize toolbar controls
        initToolbar();

        console.log('[Board] Initialized');
    }

    /**
     * Initialize sort/filter toolbar event listeners
     */
    function initToolbar() {
        const sortSelect = document.getElementById('sortSelect');
        const typeFilter = document.getElementById('typeFilter');
        const ownerFilter = document.getElementById('ownerFilter');
        const searchInput = document.getElementById('searchInput');
        const clearBtn = document.getElementById('clearFiltersBtn');

        if (sortSelect) {
            sortSelect.addEventListener('change', (e) => {
                state.sortBy = e.target.value;
                renderAllColumns();
            });
        }

        if (typeFilter) {
            typeFilter.addEventListener('change', (e) => {
                state.filters.type = e.target.value || null;
                renderAllColumns();
                updateTaskCount();
            });
        }

        if (ownerFilter) {
            ownerFilter.addEventListener('change', (e) => {
                state.filters.assignee = e.target.value || null;
                renderAllColumns();
                updateTaskCount();
            });
        }

        if (searchInput) {
            let debounceTimer;
            searchInput.addEventListener('input', (e) => {
                clearTimeout(debounceTimer);
                debounceTimer = setTimeout(() => {
                    state.filters.search = e.target.value;
                    renderAllColumns();
                    updateTaskCount();
                }, 200);
            });
        }

        if (clearBtn) {
            clearBtn.addEventListener('click', clearFilters);
        }
    }

    /**
     * Update the visible task count in the toolbar
     */
    function updateTaskCount() {
        const countEl = document.getElementById('taskCount');
        if (!countEl) return;
        let visible = 0;
        let total = state.tasks.size;
        state.tasks.forEach(task => {
            const passes = applyFilters([task]);
            if (passes.length > 0) visible++;
        });
        countEl.textContent = visible === total
            ? `${total} tasks`
            : `${visible} of ${total} tasks`;
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
            // First try to load timeline cards (THE META MOMENT)
            let tasks = [];
            let source = 'tasks';

            try {
                const timelineResponse = await fetch('/api/v1/timeline/cards');
                if (timelineResponse.ok) {
                    const timelineData = await timelineResponse.json();
                    if (timelineData.tasks && timelineData.tasks.length > 0) {
                        tasks = timelineData.tasks;
                        source = 'timeline';
                        console.log('[Board] Loaded timeline cards:', tasks.length);
                    }
                }
            } catch (timelineError) {
                console.log('[Board] Timeline cards not available, falling back to tasks');
            }

            // Fall back to regular tasks if no timeline data
            if (tasks.length === 0) {
                tasks = await API.tasks.getAll();
                console.log('[Board] Loaded tasks:', tasks.length);
            }

            // Clear existing state
            state.tasks.clear();

            // Populate state - normalize each task
            tasks.forEach(task => {
                const normalized = Cards.normalizeTask(task);
                // Store original task with normalized status
                task._normalizedStatus = normalized.status;
                state.tasks.set(task.id, task);
            });

            // Render all columns
            renderAllColumns();

            state.lastUpdate = new Date();

            if (source === 'timeline') {
                showToast('info', 'Timeline Loaded', 'Kanban is tracking its own timeline');
            }

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
        updateTaskCount();
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

        // Sort tasks by selected sort option
        filteredTasks.sort((a, b) => {
            switch (state.sortBy) {
                case 'priority':
                    if (a.priority !== b.priority) return a.priority - b.priority;
                    return new Date(b.created_at) - new Date(a.created_at);
                case 'progress':
                    return (b.progress || 0) - (a.progress || 0);
                case 'type':
                    return (a.type || '').localeCompare(b.type || '');
                case 'owner':
                    return (a.owner || a.assignee || '').localeCompare(b.owner || b.assignee || '');
                case 'updated':
                    return new Date(b.updated_at || 0) - new Date(a.updated_at || 0);
                case 'created':
                    return new Date(b.created_at || 0) - new Date(a.created_at || 0);
                case 'title':
                    return (a.title || '').localeCompare(b.title || '');
                default:
                    return a.priority - b.priority;
            }
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
     * Normalize a backend status to a frontend column name.
     * Backend uses "todo" / "in-progress"; columns use "backlog" / "in_progress".
     * @param {string} status - Backend or frontend status
     * @returns {string} - Frontend column status
     */
    function normalizeColumnStatus(status) {
        return Cards.STATUS_MAP[status] || status;
    }

    /**
     * Get tasks for a specific status
     * @param {string} status - Column status (frontend format)
     * @returns {Array}
     */
    function getTasksByStatus(status) {
        const tasks = [];
        state.tasks.forEach(task => {
            // Normalize the task status for comparison
            const normalizedStatus = Cards.STATUS_MAP[task.status] || task.status;
            if (normalizedStatus === status) {
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

            // Type filter
            if (state.filters.type && task.type !== state.filters.type) {
                return false;
            }

            // Assignee/owner filter
            if (state.filters.assignee) {
                const owner = task.assignee || task.owner || '';
                if (owner !== state.filters.assignee) {
                    return false;
                }
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
        renderColumn(normalizeColumnStatus(task.status));

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
            // Re-render both columns (normalize backend → frontend column names)
            renderColumn(normalizeColumnStatus(oldTask.status));
            renderColumn(normalizeColumnStatus(task.status));
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
                renderColumn(normalizeColumnStatus(task.status));
            }, 300);
        } else {
            state.tasks.delete(taskId);
        }
    }

    /**
     * Move a task to a different column
     * @param {string} taskId - Task ID
     * @param {string} newStatus - New status (frontend format)
     * @returns {Promise}
     */
    async function moveTask(taskId, newStatus) {
        const task = state.tasks.get(taskId);
        if (!task) return;

        // Get old status in normalized format
        const oldNormalizedStatus = Cards.STATUS_MAP[task.status] || task.status;
        const oldStatus = task.status;

        // Convert new status to API format
        const apiStatus = Cards.STATUS_TO_API[newStatus] || newStatus;

        // Optimistic update - use normalized status for rendering
        task.status = apiStatus;
        renderColumn(oldNormalizedStatus);
        renderColumn(newStatus);

        try {
            // Update via API
            const updatedTask = await API.tasks.move(taskId, apiStatus);
            state.tasks.set(taskId, updatedTask);

            // Check if completed
            const completedStatuses = ['done'];
            const wasNotDone = !completedStatuses.includes(Cards.STATUS_MAP[oldStatus] || oldStatus);
            const isNowDone = newStatus === 'done';

            if (isNowDone && wasNotDone) {
                const card = document.querySelector(`[data-task-id="${taskId}"]`);
                if (card) {
                    card.classList.add('just-completed');
                    setTimeout(() => card.classList.remove('just-completed'), 400);
                }
            }

            showToast('success', 'Task moved', `Moved to ${Cards.STATUS_NAMES[newStatus] || newStatus}`);

        } catch (error) {
            console.error('[Board] Failed to move task:', error);

            // Rollback on error
            task.status = oldStatus;
            renderColumn(oldNormalizedStatus);
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
        state.filters.type = null;
        state.filters.search = '';
        state.sortBy = 'priority';
        // Reset toolbar controls
        const sortEl = document.getElementById('sortSelect');
        const typeEl = document.getElementById('typeFilter');
        const ownerEl = document.getElementById('ownerFilter');
        const searchEl = document.getElementById('searchInput');
        if (sortEl) sortEl.value = 'priority';
        if (typeEl) typeEl.value = '';
        if (ownerEl) ownerEl.value = '';
        if (searchEl) searchEl.value = '';
        renderAllColumns();
        updateTaskCount();
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
