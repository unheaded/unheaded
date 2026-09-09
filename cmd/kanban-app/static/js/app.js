/**
 * app.js - Main Application Logic
 * Orchestrates all modules and handles user interactions
 */

/**
 * Main Application for The Unheaded Kingdom's Kanban Board
 */
// These pages load several <script> tags sharing one global scope; the names
// below are defined by sibling files, which eslint cannot see file-by-file.
/* global API, Board, Cards, WebSocketClient */

const App = (function() {
    'use strict';

    // DOM Element references
    const elements = {
        // Header
        newTaskBtn: null,
        connectionStatus: null,

        // Task Modal (Create/Edit)
        taskModal: null,
        taskForm: null,
        taskId: null,
        taskTitle: null,
        taskDescription: null,
        taskType: null,
        taskStatus: null,
        taskAssignee: null,
        taskProgress: null,
        modalTitle: null,
        modalSubmit: null,
        modalClose: null,
        modalCancel: null,

        // Task Detail Modal
        taskDetailModal: null,
        detailTitle: null,
        detailPriority: null,
        detailDescription: null,
        detailStatus: null,
        detailType: null,
        detailAssignee: null,
        detailProgress: null,
        detailCreated: null,
        detailUpdated: null,
        detailTags: null,
        detailEdit: null,
        detailDelete: null,
        detailClose: null,

        // Delete Confirmation Modal
        deleteModal: null,
        deleteTaskTitle: null,
        deleteConfirm: null,
        deleteCancel: null,
        deleteClose: null
    };

    // Current state
    let currentTask = null;
    let isEditMode = false;

    /**
     * Initialize the application
     */
    async function init() {
        console.log('[App] Initializing The Unheaded Kingdom Kanban Board...');

        // Cache DOM elements
        cacheElements();

        // Initialize modules
        Board.init();

        // Set up event listeners
        setupEventListeners();

        // Connect WebSocket
        setupWebSocket();

        // Load initial data
        await Board.loadTasks();

        console.log('[App] Initialization complete');
    }

    /**
     * Cache DOM element references
     */
    function cacheElements() {
        // Header
        elements.newTaskBtn = document.getElementById('newTaskBtn');
        elements.connectionStatus = document.getElementById('connectionStatus');

        // Task Modal
        elements.taskModal = document.getElementById('taskModal');
        elements.taskForm = document.getElementById('taskForm');
        elements.taskId = document.getElementById('taskId');
        elements.taskTitle = document.getElementById('taskTitle');
        elements.taskDescription = document.getElementById('taskDescription');
        elements.taskType = document.getElementById('taskType');
        elements.taskStatus = document.getElementById('taskStatus');
        elements.taskAssignee = document.getElementById('taskAssignee');
        elements.taskProgress = document.getElementById('taskProgress');
        elements.modalTitle = document.getElementById('modalTitle');
        elements.modalSubmit = document.getElementById('modalSubmit');
        elements.modalClose = document.getElementById('modalClose');
        elements.modalCancel = document.getElementById('modalCancel');

        // Task Detail Modal
        elements.taskDetailModal = document.getElementById('taskDetailModal');
        elements.detailTitle = document.getElementById('detailTitle');
        elements.detailPriority = document.getElementById('detailPriority');
        elements.detailDescription = document.getElementById('detailDescription');
        elements.detailStatus = document.getElementById('detailStatus');
        elements.detailType = document.getElementById('detailType');
        elements.detailAssignee = document.getElementById('detailAssignee');
        elements.detailProgress = document.getElementById('detailProgress');
        elements.detailCreated = document.getElementById('detailCreated');
        elements.detailUpdated = document.getElementById('detailUpdated');
        elements.detailTags = document.getElementById('detailTags');
        elements.detailEdit = document.getElementById('detailEdit');
        elements.detailDelete = document.getElementById('detailDelete');
        elements.detailClose = document.getElementById('detailClose');

        // Delete Modal
        elements.deleteModal = document.getElementById('deleteModal');
        elements.deleteTaskTitle = document.getElementById('deleteTaskTitle');
        elements.deleteConfirm = document.getElementById('deleteConfirm');
        elements.deleteCancel = document.getElementById('deleteCancel');
        elements.deleteClose = document.getElementById('deleteClose');
    }

    /**
     * Set up event listeners
     */
    function setupEventListeners() {
        // New Task button
        elements.newTaskBtn?.addEventListener('click', openNewTaskModal);

        // Task Modal events
        elements.taskForm?.addEventListener('submit', handleTaskSubmit);
        elements.modalClose?.addEventListener('click', closeTaskModal);
        elements.modalCancel?.addEventListener('click', closeTaskModal);

        // Task Detail Modal events
        elements.detailClose?.addEventListener('click', closeDetailModal);
        elements.detailEdit?.addEventListener('click', handleDetailEdit);
        elements.detailDelete?.addEventListener('click', handleDetailDelete);

        // Delete Modal events
        elements.deleteClose?.addEventListener('click', closeDeleteModal);
        elements.deleteCancel?.addEventListener('click', closeDeleteModal);
        elements.deleteConfirm?.addEventListener('click', confirmDelete);

        // Modal overlay clicks
        elements.taskModal?.addEventListener('click', (e) => {
            if (e.target === elements.taskModal) closeTaskModal();
        });
        elements.taskDetailModal?.addEventListener('click', (e) => {
            if (e.target === elements.taskDetailModal) closeDetailModal();
        });
        elements.deleteModal?.addEventListener('click', (e) => {
            if (e.target === elements.deleteModal) closeDeleteModal();
        });

        // Keyboard shortcuts
        document.addEventListener('keydown', handleKeyboard);

        // Custom events from Cards module
        window.addEventListener('task:view', (e) => openDetailModal(e.detail.task));
        window.addEventListener('task:edit', (e) => openEditTaskModal(e.detail.task));
        window.addEventListener('task:delete', (e) => openDeleteModal(e.detail.task));
        window.addEventListener('task:move', (e) => handleTaskMove(e.detail));
        window.addEventListener('task:update', (e) => handleInlineUpdate(e.detail));
        window.addEventListener('task:review', (e) => handleReviewAction(e.detail));
    }

    /**
     * Set up WebSocket connection
     */
    function setupWebSocket() {
        // Status change handler
        WebSocketClient.onStatusChange((state) => {
            updateConnectionStatus(state);
        });

        // Event handlers for tasks
        WebSocketClient.on('tasks.created', Board.handleTaskCreated);
        WebSocketClient.on('tasks.updated', Board.handleTaskUpdated);
        WebSocketClient.on('tasks.completed', Board.handleTaskCompleted);
        WebSocketClient.on('tasks.deleted', Board.handleTaskDeleted);

        // Timeline event handlers - THE META MOMENT
        WebSocketClient.on('timeline.updates', handleTimelineUpdates);
        WebSocketClient.on('timeline.updated', handleTimelineUpdated);
        WebSocketClient.on('timeline.event', handleTimelineEvent);
        WebSocketClient.on('timeline.task.created', Board.handleTaskCreated);
        WebSocketClient.on('timeline.task.updated', Board.handleTaskUpdated);
        WebSocketClient.on('timeline.task.removed', Board.handleTaskDeleted);

        // Connect
        WebSocketClient.connect().catch(error => {
            console.warn('[App] WebSocket connection failed:', error);
        });
    }

    /**
     * Handle timeline updates (legacy format)
     * @param {Object} data - Timeline update data
     */
    function handleTimelineUpdates(data) {
        console.log('[App] Timeline update received:', data);
        Board.showToast('info', 'Timeline Updated', 'New timeline data from Timeguru');
    }

    /**
     * Handle full timeline updated event
     * @param {Object} data - Timeline data with tasks
     */
    function handleTimelineUpdated(data) {
        console.log('[App] Timeline fully updated:', data);

        // Show notification
        const message = `${data.added || 0} added, ${data.updated || 0} updated, ${data.removed || 0} removed`;
        Board.showToast('success', 'Timeline Synced', message);

        // Reload tasks if we have timeline tasks
        if (data.tasks && data.tasks.length > 0) {
            // Add timeline tasks to board
            data.tasks.forEach(task => {
                if (!Board.getTask(task.id)) {
                    Board.addTask(task);
                } else {
                    Board.updateTask(task);
                }
            });
        }
    }

    /**
     * Handle timeline event notification
     * @param {Object} data - Event data
     */
    function handleTimelineEvent(data) {
        console.log('[App] Timeline event:', data.event);

        // Show a subtle notification
        switch (data.event) {
            case 'timeline_reloaded':
                Board.showToast('info', 'Timeline Reloaded', 'Timeguru detected changes to timeline.md');
                // Trigger a refresh of timeline cards
                fetchTimelineCards();
                break;
            default:
                console.log('[App] Unknown timeline event:', data.event);
        }
    }

    /**
     * Fetch timeline cards from the API
     */
    async function fetchTimelineCards() {
        try {
            const response = await fetch('/api/v1/timeline/cards');
            if (!response.ok) {
                throw new Error(`HTTP ${response.status}`);
            }
            const data = await response.json();

            if (data.tasks && data.tasks.length > 0) {
                console.log('[App] Fetched timeline cards:', data.tasks.length);

                // Update board with timeline tasks
                data.tasks.forEach(task => {
                    if (!Board.getTask(task.id)) {
                        Board.addTask(task);
                    } else {
                        Board.updateTask(task);
                    }
                });

                Board.showToast('success', 'Timeline Loaded', `${data.tasks.length} items from timeline`);
            }
        } catch (error) {
            console.error('[App] Failed to fetch timeline cards:', error);
        }
    }

    // Track previous connection state for notifications
    let previousConnectionState = null;

    /**
     * Update connection status indicator
     * @param {string} state - Connection state
     */
    function updateConnectionStatus(state) {
        if (!elements.connectionStatus) return;

        const statusText = elements.connectionStatus.querySelector('.status-text');

        // Remove all state classes
        elements.connectionStatus.classList.remove('connected', 'disconnected', 'connecting');

        switch (state) {
            case WebSocketClient.ConnectionState.CONNECTED:
                elements.connectionStatus.classList.add('connected');
                statusText.textContent = 'Connected';
                // Show reconnection success if we were previously disconnected/reconnecting
                if (previousConnectionState === WebSocketClient.ConnectionState.RECONNECTING ||
                    previousConnectionState === WebSocketClient.ConnectionState.DISCONNECTED ||
                    previousConnectionState === WebSocketClient.ConnectionState.ERROR) {
                    Board.showToast('success', 'Reconnected', 'Connection restored');
                    // Refresh tasks to sync any missed updates
                    Board.loadTasks();
                }
                break;
            case WebSocketClient.ConnectionState.CONNECTING:
            case WebSocketClient.ConnectionState.RECONNECTING:
                elements.connectionStatus.classList.add('connecting');
                statusText.textContent = state === WebSocketClient.ConnectionState.RECONNECTING ?
                    'Reconnecting...' : 'Connecting...';
                break;
            case WebSocketClient.ConnectionState.ERROR:
                elements.connectionStatus.classList.add('disconnected');
                statusText.textContent = 'Error';
                if (previousConnectionState === WebSocketClient.ConnectionState.CONNECTED) {
                    Board.showToast('error', 'Connection Error', 'Attempting to reconnect...');
                }
                break;
            default:
                elements.connectionStatus.classList.add('disconnected');
                statusText.textContent = 'Disconnected';
                if (previousConnectionState === WebSocketClient.ConnectionState.CONNECTED) {
                    Board.showToast('warning', 'Disconnected', 'Connection lost');
                }
        }

        previousConnectionState = state;
    }

    /**
     * Handle keyboard shortcuts
     * @param {KeyboardEvent} e - Keyboard event
     */
    function handleKeyboard(e) {
        // Escape closes modals
        if (e.key === 'Escape') {
            if (elements.deleteModal?.classList.contains('active')) {
                closeDeleteModal();
            } else if (elements.taskDetailModal?.classList.contains('active')) {
                closeDetailModal();
            } else if (elements.taskModal?.classList.contains('active')) {
                closeTaskModal();
            }
        }

        // Ctrl/Cmd + N for new task
        if ((e.ctrlKey || e.metaKey) && e.key === 'n') {
            e.preventDefault();
            openNewTaskModal();
        }
    }

    /* ============================================
       Task Modal (Create/Edit)
       ============================================ */

    /**
     * Open modal for creating a new task
     */
    function openNewTaskModal() {
        isEditMode = false;
        currentTask = null;

        // Reset form
        elements.taskForm.reset();
        elements.taskId.value = '';

        // Hide GUID (not available for new tasks)
        const guidGroup = document.getElementById('guidGroup');
        if (guidGroup) guidGroup.hidden = true;

        // Set defaults
        elements.taskType.value = 'task';
        elements.taskStatus.value = 'backlog';
        elements.taskProgress.value = '0';

        // Update modal title and button
        elements.modalTitle.textContent = 'New Task';
        elements.modalSubmit.textContent = 'Create Task';

        // Show modal
        elements.taskModal.classList.add('active');
        elements.taskTitle.focus();
    }

    /**
     * Open modal for editing a task
     * @param {Object} task - Task to edit
     */
    function openEditTaskModal(task) {
        isEditMode = true;
        currentTask = task;

        // Close detail modal if open
        closeDetailModal();

        // Normalize status for form
        const normalizedStatus = Cards.STATUS_MAP[task.status] || task.status;

        // Populate form
        elements.taskId.value = task.id;
        elements.taskTitle.value = task.title;
        elements.taskDescription.value = task.description || '';
        elements.taskType.value = task.type || 'task';
        elements.taskStatus.value = normalizedStatus;
        elements.taskAssignee.value = task.owner || task.assignee || '';
        elements.taskProgress.value = task.progress || 0;

        // Show GUID in edit mode (for cross-referencing)
        const guidGroup = document.getElementById('guidGroup');
        const guidInput = document.getElementById('taskGuid');
        if (task.guid && guidGroup && guidInput) {
            guidInput.value = task.guid;
            guidGroup.hidden = false;
        }

        // Update modal title and button
        elements.modalTitle.textContent = 'Edit Task';
        elements.modalSubmit.textContent = 'Save Changes';

        // Show modal
        elements.taskModal.classList.add('active');
        elements.taskTitle.focus();
    }

    /**
     * Close task modal
     */
    function closeTaskModal() {
        elements.taskModal.classList.remove('active');
        elements.taskForm.reset();
        currentTask = null;
        isEditMode = false;
    }

    /**
     * Handle task form submission
     * @param {Event} e - Submit event
     */
    async function handleTaskSubmit(e) {
        e.preventDefault();

        const taskData = {
            title: elements.taskTitle.value.trim(),
            description: elements.taskDescription.value.trim(),
            type: elements.taskType.value,
            status: elements.taskStatus.value,
            assignee: elements.taskAssignee.value.trim(),
            progress: parseInt(elements.taskProgress.value, 10) || 0
        };

        // Validate
        if (!taskData.title) {
            Board.showToast('error', 'Title required', 'Please enter a task title');
            elements.taskTitle.focus();
            return;
        }

        // Disable form during submission and show loading state
        elements.modalSubmit.disabled = true;
        elements.modalCancel.disabled = true;
        const originalText = elements.modalSubmit.textContent;
        elements.modalSubmit.textContent = isEditMode ? 'Saving...' : 'Creating...';
        elements.modalSubmit.classList.add('loading');

        try {
            let task;

            if (isEditMode && currentTask) {
                // Update existing task
                task = await API.tasks.update(currentTask.id, taskData);
                Board.updateTask(task);
                Board.showToast('success', 'Task updated', task.title);
            } else {
                // Create new task
                task = await API.tasks.create(taskData);
                Board.addTask(task);
                Board.showToast('success', 'Task created', task.title);
            }

            closeTaskModal();

        } catch (error) {
            console.error('[App] Task submission failed:', error);
            const errorMsg = error.message || 'An unexpected error occurred';
            Board.showToast('error', 'Operation failed', errorMsg);
            // Keep modal open on error so user can retry
        } finally {
            elements.modalSubmit.disabled = false;
            elements.modalCancel.disabled = false;
            elements.modalSubmit.textContent = originalText;
            elements.modalSubmit.classList.remove('loading');
        }
    }

    /* ============================================
       Task Detail Modal
       ============================================ */

    /**
     * Open task detail modal
     * @param {Object} task - Task to display
     */
    function openDetailModal(task) {
        currentTask = task;

        // Normalize the task
        const normalized = Cards.normalizeTask(task);
        const priority = Cards.PRIORITIES[normalized.priority] || Cards.PRIORITIES[2];

        // Update content
        elements.detailTitle.textContent = task.title;
        elements.detailPriority.textContent = priority.label;
        elements.detailPriority.className = `priority-badge ${priority.class}`;

        // Show GUID in detail view
        const guidRow = document.getElementById('detailGuidRow');
        const guidEl = document.getElementById('detailGuid');
        if (guidRow && guidEl && task.guid) {
            guidEl.value = task.guid;
            guidRow.classList.add('visible');
        } else if (guidRow) {
            guidRow.classList.remove('visible');
        }

        elements.detailDescription.textContent = task.description || 'No description provided.';
        elements.detailStatus.textContent = Cards.STATUS_NAMES[task.status] || task.status;

        if (elements.detailType) {
            elements.detailType.textContent = task.type || 'Task';
        }

        elements.detailAssignee.textContent = task.owner || task.assignee || 'Unassigned';

        if (elements.detailProgress) {
            elements.detailProgress.textContent = (task.progress || 0) + '%';
        }

        const dateOptions = {
            year: 'numeric',
            month: 'short',
            day: 'numeric',
            hour: '2-digit',
            minute: '2-digit'
        };

        elements.detailCreated.textContent = task.created_at ?
            new Date(task.created_at).toLocaleDateString('en-US', dateOptions) : '-';

        if (elements.detailUpdated) {
            elements.detailUpdated.textContent = task.updated_at ?
                new Date(task.updated_at).toLocaleDateString('en-US', dateOptions) : '-';
        }

        // Update tags
        if (task.tags && task.tags.length > 0) {
            elements.detailTags.innerHTML = task.tags
                .map(tag => `<span class="tag">${Cards.escapeHtml(tag)}</span>`)
                .join('');
        } else {
            elements.detailTags.innerHTML = '<span class="text-muted">No tags</span>';
        }

        // Show linked commits (ADR-038)
        var commitsSection = document.getElementById('detailCommitsSection');
        var commitsList = document.getElementById('detailCommits');
        if (commitsSection && commitsList) {
            var commits = [];
            // A malformed commits blob just means no commit list is shown;
            // commits stays [] and the section below is skipped.
            try { commits = JSON.parse(task.commits || '[]'); } catch { commits = []; }
            if (commits.length > 0) {
                commitsSection.hidden = false;
                commitsList.innerHTML = '';
                commits.forEach(function(c) {
                    var entry = document.createElement('div');
                    entry.className = 'commit-entry';
                    var hash = document.createElement('code');
                    hash.className = 'commit-hash';
                    hash.textContent = (c.hash || '').slice(0, 8);
                    hash.title = c.hash + ' (click to copy)';
                    hash.addEventListener('click', function() {
                        navigator.clipboard.writeText(c.hash);
                    });
                    var msg = document.createElement('span');
                    msg.className = 'commit-message';
                    msg.textContent = c.message || '';
                    var date = document.createElement('span');
                    date.className = 'commit-date';
                    date.textContent = c.date ? new Date(c.date).toLocaleDateString() : '';
                    entry.appendChild(hash);
                    entry.appendChild(msg);
                    entry.appendChild(date);
                    commitsList.appendChild(entry);
                });
            } else {
                commitsSection.hidden = true;
            }
        }

        // Show modal
        elements.taskDetailModal.classList.add('active');
    }

    /**
     * Close task detail modal
     */
    function closeDetailModal() {
        elements.taskDetailModal.classList.remove('active');
    }

    /**
     * Handle edit button in detail modal
     */
    function handleDetailEdit() {
        if (currentTask) {
            openEditTaskModal(currentTask);
        }
    }

    /**
     * Handle delete button in detail modal
     */
    function handleDetailDelete() {
        if (currentTask) {
            closeDetailModal();
            openDeleteModal(currentTask);
        }
    }

    /* ============================================
       Delete Confirmation Modal
       ============================================ */

    /**
     * Open delete confirmation modal
     * @param {Object} task - Task to delete
     */
    function openDeleteModal(task) {
        currentTask = task;
        elements.deleteTaskTitle.textContent = task.title;
        elements.deleteModal.classList.add('active');
    }

    /**
     * Close delete modal
     */
    function closeDeleteModal() {
        elements.deleteModal.classList.remove('active');
    }

    /**
     * Confirm and execute delete
     */
    async function confirmDelete() {
        if (!currentTask) return;

        const taskId = currentTask.id;
        const taskTitle = currentTask.title;

        elements.deleteConfirm.disabled = true;
        elements.deleteCancel.disabled = true;
        elements.deleteConfirm.textContent = 'Deleting...';
        elements.deleteConfirm.classList.add('loading');

        try {
            await API.tasks.delete(taskId);
            Board.removeTask(taskId);
            Board.showToast('success', 'Task deleted', taskTitle);
            closeDeleteModal();

        } catch (error) {
            console.error('[App] Delete failed:', error);
            const errorMsg = error.message || 'Could not delete task';
            Board.showToast('error', 'Delete failed', errorMsg);
            // Keep modal open on error
        } finally {
            elements.deleteConfirm.disabled = false;
            elements.deleteCancel.disabled = false;
            elements.deleteConfirm.textContent = 'Delete Task';
            elements.deleteConfirm.classList.remove('loading');
            if (!elements.deleteModal.classList.contains('active')) {
                currentTask = null;
            }
        }
    }

    /* ============================================
       Drag and Drop Handling
       ============================================ */

    /**
     * Handle task move from drag and drop
     * @param {Object} detail - Move details (taskId, newStatus, cardElement)
     */

    /**
     * Handle review actions (Approve, Reject, Request Changes)
     * @param {Object} detail - Review detail (task, action)
     */
    async function handleReviewAction(detail) {
        const { task, action } = detail;

        if (!task) return;

        let newStatus;
        let successMessage;

        switch (action) {
            case 'approve':
                newStatus = 'done';
                successMessage = 'Task approved and moved to Done';
                break;
            case 'reject':
                newStatus = 'backlog';
                successMessage = 'Task rejected and moved to Backlog';
                break;
            case 'changes':
                // Request changes - stay in review but add flag
                try {
                    // Update task with changes-requested flag
                    const updatedTask = await API.tasks.update(task.id, {
                        status: 'review',
                        // Add a marker that changes were requested
                        description: (task.description || '').split('\n\n[Changes requested')[0] + '\n\n[Changes requested - ' + new Date().toLocaleDateString() + ']'
                    });
                    Board.updateTask(updatedTask);
                    Board.showToast('info', 'Changes Requested', 'Author will be notified');
                } catch (error) {
                    console.error('[App] Failed to request changes:', error);
                    Board.showToast('error', 'Failed to request changes', error.message);
                }
                return;
            default:
                return;
        }

        // Move task to new status
        await Board.moveTask(task.id, newStatus);
        Board.showToast('success', 'Review action completed', successMessage);
    }
    async function handleTaskMove(detail) {
        const { taskId, newStatus } = detail;

        const task = Board.getTask(taskId);
        if (!task || task.status === newStatus) return;

        await Board.moveTask(taskId, newStatus);
    }

    /**
     * Handle inline task update
     * @param {Object} detail - Update details (taskId, updates, cardElement)
     */
    async function handleInlineUpdate(detail) {
        const { taskId, updates } = detail;

        try {
            const updatedTask = await API.tasks.update(taskId, updates);
            Board.updateTask(updatedTask);
            Board.showToast('success', 'Task updated');
        } catch (error) {
            console.error('[App] Inline update failed:', error);
            Board.showToast('error', 'Update failed', error.message);
            // Refresh the column to restore original state
            const task = Board.getTask(taskId);
            if (task) {
                Board.renderColumn(task.status);
            }
        }
    }

    /* ============================================
       Public API
       ============================================ */

    return {
        init,
        openNewTaskModal,
        openEditTaskModal,
        openDetailModal,
        openDeleteModal
    };
})();

// Initialize when DOM is ready
document.addEventListener('DOMContentLoaded', () => {
    App.init().catch(error => {
        console.error('[App] Initialization failed:', error);
    });
});

// Make available globally for debugging
window.App = App;
